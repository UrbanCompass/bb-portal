package buildeventrecorder

import (
	"context"
	"reflect"
	"strings"

	bes "github.com/bazelbuild/bazel/src/main/java/com/google/devtools/build/lib/buildeventstream/proto"
	"github.com/bazelbuild/bazel/src/main/protobuf"
	"github.com/buildbarn/bb-portal/ent/gen/ent/bazelinvocation"
	"github.com/buildbarn/bb-portal/ent/gen/ent/configuration"
	"github.com/buildbarn/bb-portal/internal/database"
	"github.com/buildbarn/bb-portal/pkg/invocation/files"
	"github.com/buildbarn/bb-storage/pkg/util"
	"google.golang.org/protobuf/types/known/anypb"
)

func getErrorCodeFromFailureDetail(failureDetail *protobuf.FailureDetail) string {
	if failureDetail == nil || failureDetail.Category == nil {
		return ""
	}
	detailValue := reflect.ValueOf(failureDetail.Category)
	if detailValue.Kind() == reflect.Ptr {
		detailValue = detailValue.Elem()
	}
	if detailValue.Kind() != reflect.Struct {
		return ""
	}

	for i := 0; i < detailValue.NumField(); i++ {
		fieldValue := detailValue.Field(i)
		if fieldValue.Kind() != reflect.Ptr {
			continue
		}

		method := fieldValue.MethodByName("GetCode")
		if !method.IsValid() {
			continue
		}

		result := method.Call(nil)
		if len(result) == 0 {
			continue
		}

		stringer := result[0].MethodByName("String")
		if !stringer.IsValid() {
			continue
		}
		return stringer.Call(nil)[0].String()
	}
	return ""
}

// cacheStatusFromStrategyDetails extracts a human-readable cache status string
// from an ActionExecuted's StrategyDetails Any list. Returns an empty string
// when no recognized message is found.
//
// Bazel encodes execution strategy info as proto Any messages whose type URL
// conventionally ends in the message name. We match known message names and
// fall back to the raw name so new strategy types surface without code
// changes.
func cacheStatusFromStrategyDetails(details []*anypb.Any) string {
	for _, d := range details {
		if d == nil {
			continue
		}
		url := d.GetTypeUrl()
		// The message name is the last path or package component of the
		// type URL. Match it exactly: a suffix match would classify
		// LocalSpawnMetrics as SpawnMetrics.
		name := url
		if idx := strings.LastIndexAny(url, "/."); idx >= 0 {
			name = url[idx+1:]
		}
		switch name {
		case "RemoteSpawnMetrics":
			return "remote cache hit"
		case "SpawnMetrics":
			// SpawnMetrics appears for both local and remote-exec actions;
			// keep it coarse for now and refine later.
			return "remote"
		}
		// For unrecognized types, surface the name so the data is at least
		// queryable.
		return name
	}
	return ""
}

func (r *buildEventRecorder) saveActionExecuted(ctx context.Context, tx database.Handle, actionExecuted *bes.ActionExecuted, actionCompletedID *bes.BuildEventId_ActionCompletedId) error {
	if actionExecuted == nil || actionCompletedID == nil {
		return nil
	}
	if actionCompletedID.Label == "" {
		return nil
	}

	// Failures are always persisted. Successes are sampled per invocation
	// (see actionSampler); a slot of -1 means this one is not persisted.
	sampleSlot := -1
	if actionExecuted.Success {
		sampleSlot = r.actionSampler.admit()
		if sampleSlot < 0 {
			return nil
		}
	}

	create := tx.Ent().Action.Create().
		SetBazelInvocationID(r.InvocationDbID).
		SetLabel(actionCompletedID.Label).
		SetSuccess(actionExecuted.Success).
		SetExitCode(actionExecuted.ExitCode).
		SetCommandLine(actionExecuted.CommandLine)

	// Store the primary output path as the stable join key for inter-invocation
	// action comparisons (label + type + primaryOutput).
	if po := actionCompletedID.PrimaryOutput; po != "" {
		create.SetPrimaryOutput(po)
	}

	// Best-effort cache status from StrategyDetails.
	if status := cacheStatusFromStrategyDetails(actionExecuted.StrategyDetails); status != "" {
		create.SetCacheStatus(status)
	}

	if actionExecuted.Success {
		create.SetSampled(true)
	}

	if configID := actionCompletedID.Configuration.GetId(); configID != "" {
		configDbID, err := tx.Ent().Configuration.Query().
			Where(
				configuration.ConfigurationID(configID),
				configuration.HasBazelInvocationWith(bazelinvocation.ID(r.InvocationDbID)),
			).
			OnlyID(ctx)
		if err != nil {
			return util.StatusWrapf(err, "failed to query Configuration with ID %#v for ActionExecuted", configID)
		}
		create.SetConfigurationID(configDbID)
	}

	if actionExecuted.Type != "" {
		create.SetType(actionExecuted.Type)
	}
	if failureMessage := actionExecuted.GetFailureDetail().GetMessage(); failureMessage != "" {
		create.SetFailureMessage(failureMessage)
	}
	if failureCode := getErrorCodeFromFailureDetail(actionExecuted.GetFailureDetail()); failureCode != "" {
		create.SetFailureCode(failureCode)
	}
	if actionExecuted.StartTime != nil {
		create.SetStartTime(actionExecuted.StartTime.AsTime())
	}
	if actionExecuted.EndTime != nil {
		create.SetEndTime(actionExecuted.EndTime.AsTime())
	}
	if file := actionExecuted.GetStdout(); file != nil {
		if parsedFile := files.ParseBepFile(file); parsedFile != nil {
			fileDbID, err := SaveSingleFile(ctx, tx, r.InstanceNameDbID, *parsedFile)
			if err != nil {
				return util.StatusWrap(err, "Failed to save stdout to database")
			}
			create.SetStdoutID(fileDbID)
		}
	}
	if file := actionExecuted.GetStderr(); file != nil {
		if parsedFile := files.ParseBepFile(file); parsedFile != nil {
			fileDbID, err := SaveSingleFile(ctx, tx, r.InstanceNameDbID, *parsedFile)
			if err != nil {
				return util.StatusWrap(err, "Failed to save stderr to database")
			}
			create.SetStderrID(fileDbID)
		}
	}
	action, err := create.Save(ctx)
	if err != nil {
		return util.StatusWrap(err, "failed to save Action")
	}

	// Track sampled successes, deleting the row a new action displaces.
	if sampleSlot >= 0 {
		if sampleSlot < len(r.sampledActionIDs) {
			if err := tx.Ent().Action.DeleteOneID(r.sampledActionIDs[sampleSlot]).Exec(ctx); err != nil {
				return util.StatusWrap(err, "failed to evict displaced sampled Action")
			}
			r.sampledActionIDs[sampleSlot] = action.ID
		} else {
			r.sampledActionIDs = append(r.sampledActionIDs, action.ID)
		}
	}

	return nil
}
