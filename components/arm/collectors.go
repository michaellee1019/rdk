package arm

import (
	"context"
	"errors"
	"time"

	v1 "go.viam.com/api/common/v1"
	pb "go.viam.com/api/component/arm/v1"
	"google.golang.org/protobuf/types/known/anypb"

	"go.viam.com/rdk/data"
	"go.viam.com/rdk/referenceframe"
	"go.viam.com/rdk/utils"
)

type method int64

const (
	endPosition method = iota
	jointPositions
	doCommand
)

func (m method) String() string {
	switch m {
	case endPosition:
		return "EndPosition"
	case jointPositions:
		return "JointPositions"
	case doCommand:
		return "DoCommand"
	}
	return "Unknown"
}

// newEndPositionCollector returns a collector to register an end position method. If one is already registered
// with the same MethodMetadata it will panic.
func newEndPositionCollector(resource interface{}, params data.CollectorParams) (data.Collector, error) {
	arm, err := utils.AssertType[Arm](resource)
	if err != nil {
		return nil, err
	}

	cFunc := data.CaptureFunc(func(ctx context.Context, _ map[string]*anypb.Any) (data.CaptureResult, error) {
		timeRequested := time.Now()
		var res data.CaptureResult
		v, err := arm.EndPosition(ctx, data.FromDMExtraMap)
		if err != nil {
			return res, formatErr(err, endPosition, params)
		}
		o := v.Orientation().OrientationVectorDegrees()
		ts := data.Timestamps{TimeRequested: timeRequested, TimeReceived: time.Now()}
		return data.NewTabularCaptureResult(ts, pb.GetEndPositionResponse{
			Pose: &v1.Pose{
				X:     v.Point().X,
				Y:     v.Point().Y,
				Z:     v.Point().Z,
				OX:    o.OX,
				OY:    o.OY,
				OZ:    o.OZ,
				Theta: o.Theta,
			},
		})
	})
	return data.NewCollector(cFunc, params)
}

// newJointPositionsCollector returns a collector to register a joint positions method. If one is already registered
// with the same MethodMetadata it will panic.
func newJointPositionsCollector(resource interface{}, params data.CollectorParams) (data.Collector, error) {
	arm, err := utils.AssertType[Arm](resource)
	if err != nil {
		return nil, err
	}

	cFunc := data.CaptureFunc(func(ctx context.Context, _ map[string]*anypb.Any) (data.CaptureResult, error) {
		timeRequested := time.Now()
		var res data.CaptureResult
		v, err := arm.JointPositions(ctx, data.FromDMExtraMap)
		if err != nil {
			return res, formatErr(err, jointPositions, params)
		}
		// its ok to be ignoring the error from this function because the appropriate warning will have been
		// logged with the above JointPositions call
		//nolint:errcheck
		k, _ := arm.Kinematics(ctx)
		jp, err := referenceframe.JointPositionsFromInputs(k, v)
		if err != nil {
			return res, data.NewFailedToReadError(params.ComponentName, jointPositions.String(), err)
		}
		ts := data.Timestamps{TimeRequested: timeRequested, TimeReceived: time.Now()}
		return data.NewTabularCaptureResult(ts, pb.GetJointPositionsResponse{Positions: jp})
	})
	return data.NewCollector(cFunc, params)
}

// newDoCommandCollector returns a collector to register a doCommand action. If one is already registered
// with the same MethodMetadata it will panic.
func newDoCommandCollector(resource interface{}, params data.CollectorParams) (data.Collector, error) {
	arm, err := utils.AssertType[Arm](resource)
	if err != nil {
		return nil, err
	}

	cFunc := data.NewDoCommandCaptureFunc(arm, params)
	return data.NewCollector(cFunc, params)
}

// A modular filter component can be created to filter the readings from a component. The error ErrNoCaptureToStore
// is used in the datamanager to exclude readings from being captured and stored.
func formatErr(err error, m method, params data.CollectorParams) error {
	if errors.Is(err, data.ErrNoCaptureToStore) {
		return err
	}
	return data.NewFailedToReadError(params.ComponentName, m.String(), err)
}
