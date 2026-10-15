package buildeventrecorder

import (
	"context"
	"reflect"

	bes "github.com/bazelbuild/bazel/src/main/java/com/google/devtools/build/lib/buildeventstream/proto"
	"github.com/bazelbuild/bazel/src/main/protobuf"
	"github.com/buildbarn/bb-portal/ent/gen/ent"
	"github.com/buildbarn/bb-portal/ent/gen/ent/action"
	"github.com/buildbarn/bb-portal/ent/gen/ent/bazelinvocation"
	"github.com/buildbarn/bb-portal/ent/gen/ent/configuration"
	"github.com/buildbarn/bb-portal/internal/database"
	"github.com/buildbarn/bb-portal/pkg/invocation/files"
	"github.com/buildbarn/bb-storage/pkg/util"
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

func (r *buildEventRecorder) saveActionExecuted(ctx context.Context, tx database.Handle, actionExecuted *bes.ActionExecuted, actionCompletedID *bes.BuildEventId_ActionCompletedId) error {
	if actionExecuted == nil || actionCompletedID == nil {
		return nil
	}
	if actionCompletedID.Label == "" {
		return nil
	}
	// Failed actions are always stored. Successful actions are sampled, so
	// that storage per invocation stays bounded (see ActionSampleStrategy).
	if actionExecuted.Success && r.actionSampling.cap == 0 {
		return nil
	}

	create := tx.Ent().Action.Create().
		SetBazelInvocationID(r.InvocationDbID).
		SetLabel(actionCompletedID.Label).
		SetSuccess(actionExecuted.Success).
		SetExitCode(actionExecuted.ExitCode).
		SetCommandLine(actionExecuted.CommandLine)

	if actionExecuted.Success {
		create.
			SetSampled(true).
			SetSampleKey(r.actionSampling.strategy.SampleKey(actionCompletedID, actionExecuted))
	}
	if primaryOutput := actionCompletedID.PrimaryOutput; primaryOutput != "" {
		create.SetPrimaryOutput(primaryOutput)
	}

	if configID := actionCompletedID.Configuration.GetId(); configID != "" {
		// This results in a database query per stored ActionExecuted event,
		// which is bounded by the number of failed actions plus the sample
		// cap.
		configDbID, err := r.findOrCreateActionConfiguration(ctx, tx, configID)
		if err != nil {
			return err
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
	err := create.Exec(ctx)
	if err != nil {
		return util.StatusWrap(err, "failed to save Action")
	}
	if actionExecuted.Success {
		if err := r.trimSampledActions(ctx, tx); err != nil {
			return err
		}
	}
	return nil
}

// trimSampledActions deletes the sampled action with the highest sample key
// (the most recent one on ties) once the invocation holds more sampled
// actions than the cap. Called after every sampled insert, it keeps exactly
// the cap actions with the lowest keys.
func (r *buildEventRecorder) trimSampledActions(ctx context.Context, tx database.Handle) error {
	evict, err := tx.Ent().Action.Query().
		Where(
			action.BazelInvocationID(r.InvocationDbID),
			action.Sampled(true),
		).
		Order(ent.Asc(action.FieldSampleKey), ent.Asc(action.FieldID)).
		Offset(r.actionSampling.cap).
		IDs(ctx)
	if err != nil {
		return util.StatusWrap(err, "failed to query sampled Actions")
	}
	if len(evict) == 0 {
		return nil
	}
	if _, err := tx.Ent().Action.Delete().Where(action.IDIn(evict...)).Exec(ctx); err != nil {
		return util.StatusWrap(err, "failed to delete Actions evicted from the sample")
	}
	return nil
}

// findOrCreateActionConfiguration returns the database ID of the invocation's
// Configuration with the given ID. Bazel references some configurations from
// ActionExecuted events (notably "system", used by the workspace status
// action) without ever announcing them in a Configuration event. Rather than
// failing the whole event batch, a bare Configuration row is created for them.
func (r *buildEventRecorder) findOrCreateActionConfiguration(ctx context.Context, tx database.Handle, configID string) (int64, error) {
	id, err := tx.Ent().Configuration.Query().
		Where(
			configuration.ConfigurationID(configID),
			configuration.HasBazelInvocationWith(bazelinvocation.ID(r.InvocationDbID)),
		).
		OnlyID(ctx)
	if err == nil {
		return id, nil
	}
	if !ent.IsNotFound(err) {
		return 0, util.StatusWrapf(err, "failed to query Configuration with ID %#v for ActionExecuted", configID)
	}
	created, err := tx.Ent().Configuration.Create().
		SetConfigurationID(configID).
		SetBazelInvocationID(r.InvocationDbID).
		Save(ctx)
	if err != nil {
		return 0, util.StatusWrapf(err, "failed to create Configuration with ID %#v for ActionExecuted", configID)
	}
	return created.ID, nil
}
