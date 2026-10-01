package buildeventrecorder

import (
	"context"
	"math/rand"
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

// successfulActionSampleCap is the maximum number of successful actions
// persisted per invocation. Keeps storage bounded while providing enough
// sample data for inter-invocation action comparisons.
const successfulActionSampleCap = 100

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
// conventionally ends in the message name. We look for known suffixes and fall
// back to the raw type URL tail so new strategy types surface without code
// changes.
func cacheStatusFromStrategyDetails(details []*anypb.Any) string {
	for _, d := range details {
		if d == nil {
			continue
		}
		url := d.GetTypeUrl()
		switch {
		case strings.HasSuffix(url, "RemoteSpawnMetrics"):
			return "remote cache hit"
		case strings.HasSuffix(url, "SpawnMetrics"):
			// SpawnMetrics appears for both local and remote-exec actions;
			// keep the raw suffix for now — callers can refine later.
			return "remote"
		}
		// For unrecognized types, surface the last path component so the
		// data is at least queryable.
		if idx := strings.LastIndexAny(url, "/."); idx >= 0 {
			return url[idx+1:]
		}
		return url
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

	// Algorithm R reservoir sampling for successful actions.
	// Every successful action has an equal 1-in-N probability of being in
	// the final sample of size successfulActionSampleCap, regardless of
	// arrival order.
	var reservoirSlot int = -1 // -1 means: do not persist this action
	if actionExecuted.Success {
		r.successfulActionsSeen++
		n := r.successfulActionsSeen
		if n <= successfulActionSampleCap {
			// Reservoir not yet full — always include.
			reservoirSlot = n - 1
		} else {
			// Reservoir full — include with probability k/n.
			if j := rand.Intn(n); j < successfulActionSampleCap {
				reservoirSlot = j
			}
		}
		if reservoirSlot < 0 {
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
		create.SetStartTime(actionExecuted.StartTime.AsTime()) // was wrongly using EndTime
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

	// Update reservoir tracking for sampled successful actions.
	if reservoirSlot >= 0 {
		if reservoirSlot < len(r.reservoirIDs) {
			// Displace: evict the previously sampled action at this slot.
			if err := tx.Ent().Action.UpdateOneID(r.reservoirIDs[reservoirSlot]).SetSampled(false).Exec(ctx); err != nil {
				return util.StatusWrap(err, "failed to evict displaced Action from reservoir")
			}
			r.reservoirIDs[reservoirSlot] = action.ID
		} else {
			r.reservoirIDs = append(r.reservoirIDs, action.ID)
		}
	}

	return nil
}
