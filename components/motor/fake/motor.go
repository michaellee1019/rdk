// Package fake implements a fake motor.
package fake

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/pkg/errors"

	"go.viam.com/rdk/components/board"
	fakeboard "go.viam.com/rdk/components/board/fake"
	"go.viam.com/rdk/components/encoder"
	"go.viam.com/rdk/components/encoder/fake"
	"go.viam.com/rdk/components/motor"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/operation"
	"go.viam.com/rdk/resource"
)

var (
	// Model is the fake motor's model.
	Model         = resource.DefaultModelFamily.WithModel("fake")
	fakeBoardConf = resource.Config{
		Name: "fakeboard",
		API:  board.API,
		ConvertedAttributes: &fakeboard.Config{
			FailNew: false,
		},
	}
)

const (
	defaultMaxRpm = 100
)

// PinConfig defines the mapping of where motor are wired.
type PinConfig struct {
	Direction string `json:"dir"`
	PWM       string `json:"pwm"`
}

// Config describes the configuration of a motor.
type Config struct {
	Pins             PinConfig `json:"pins,omitempty"`
	BoardName        string    `json:"board,omitempty"`
	MinPowerPct      float64   `json:"min_power_pct,omitempty"`
	MaxPowerPct      float64   `json:"max_power_pct,omitempty"`
	PWMFreq          uint      `json:"pwm_freq,omitempty"`
	Encoder          string    `json:"encoder,omitempty"`
	MaxRPM           float64   `json:"max_rpm,omitempty"`
	TicksPerRotation int       `json:"ticks_per_rotation,omitempty"`
	DirectionFlip    bool      `json:"direction_flip,omitempty"`
}

// Validate ensures all parts of the config are valid.
func (cfg *Config) Validate(path string) ([]string, []string, error) {
	var deps []string
	if cfg.BoardName != "" {
		deps = append(deps, cfg.BoardName)
	}
	if cfg.Encoder != "" {
		if cfg.TicksPerRotation <= 0 {
			return nil, nil, resource.NewConfigValidationError(path, errors.New("need nonzero TicksPerRotation for encoded motor"))
		}
		deps = append(deps, cfg.Encoder)
	}
	return deps, nil, nil
}

func init() {
	resource.RegisterComponent(motor.API, Model, resource.Registration[motor.Motor, *Config]{
		Constructor: NewMotor,
	})
}

// A Motor allows setting and reading a set power percentage and
// direction.
type Motor struct {
	resource.Named
	resource.TriviallyCloseable

	mu                sync.Mutex
	powerPct          float64
	Board             string
	PWM               board.GPIOPin
	PositionReporting bool
	Encoder           fake.Encoder
	MaxRPM            float64
	DirFlip           bool
	TicksPerRotation  int

	OpMgr  *operation.SingleOperationManager
	Logger logging.Logger
}

// NewMotor creates a new fake motor.
func NewMotor(ctx context.Context, deps resource.Dependencies, conf resource.Config, logger logging.Logger) (motor.Motor, error) {
	m := &Motor{
		Named:  conf.ResourceName().AsNamed(),
		Logger: logger,
		OpMgr:  operation.NewSingleOperationManager(),
	}
	if err := m.Reconfigure(ctx, deps, conf); err != nil {
		return nil, err
	}
	return m, nil
}

// Reconfigure atomically reconfigures this motor in place based on the new config.
func (m *Motor) Reconfigure(ctx context.Context, deps resource.Dependencies, conf resource.Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	newConf, err := resource.NativeConfig[*Config](conf)
	if err != nil {
		return err
	}

	var b board.Board
	if newConf.BoardName != "" {
		m.Board = newConf.BoardName
		b, err = board.FromDependencies(deps, m.Board)
		if err != nil {
			return err
		}
	} else {
		m.Logger.CInfo(ctx, "board not provided, using a fake board")
		m.Board = "fakeboard"
		b, err = fakeboard.NewBoard(ctx, fakeBoardConf, m.Logger)
		if err != nil {
			return err
		}
	}

	pwmPin := "1"
	if newConf.Pins.PWM != "" {
		pwmPin = newConf.Pins.PWM
	}
	m.PWM, err = b.GPIOPinByName(pwmPin)
	if err != nil {
		return err
	}
	if err = m.PWM.SetPWMFreq(ctx, newConf.PWMFreq, nil); err != nil {
		return err
	}

	m.MaxRPM = newConf.MaxRPM

	if m.MaxRPM == 0 {
		m.Logger.CInfof(ctx, "Max RPM not provided to a fake motor, defaulting to %v", defaultMaxRpm)
		m.MaxRPM = defaultMaxRpm
	}

	if newConf.Encoder != "" {
		m.TicksPerRotation = newConf.TicksPerRotation

		e, err := encoder.FromDependencies(deps, newConf.Encoder)
		if err != nil {
			return err
		}
		fakeEncoder, ok := e.(fake.Encoder)
		if !ok {
			return resource.TypeError[fake.Encoder](e)
		}
		m.Encoder = fakeEncoder
		m.PositionReporting = true
	} else {
		m.PositionReporting = false
	}
	m.DirFlip = false
	if newConf.DirectionFlip {
		m.DirFlip = true
	}
	return nil
}

// Position returns motor position in rotations.
func (m *Motor) Position(ctx context.Context, extra map[string]interface{}) (float64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.Encoder == nil {
		return 0, errors.New("encoder is not defined")
	}

	ticks, _, err := m.Encoder.Position(ctx, encoder.PositionTypeUnspecified, extra)
	if err != nil {
		return 0, err
	}

	if m.TicksPerRotation == 0 {
		return 0, errors.New("need nonzero TicksPerRotation for motor")
	}

	return ticks / float64(m.TicksPerRotation), nil
}

// Properties returns the status of whether the motor supports certain optional properties.
func (m *Motor) Properties(ctx context.Context, extra map[string]interface{}) (motor.Properties, error) {
	return motor.Properties{
		PositionReporting: m.PositionReporting,
	}, nil
}

// SetPower sets the given power percentage.
func (m *Motor) SetPower(ctx context.Context, powerPct float64, extra map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.OpMgr.CancelRunning(ctx)
	m.Logger.CDebugf(ctx, "Motor SetPower %f", powerPct)
	m.setPowerPct(powerPct)

	if m.Encoder != nil {
		if m.TicksPerRotation <= 0 {
			return errors.New("need positive nonzero TicksPerRotation")
		}

		newSpeed := (m.MaxRPM * m.powerPct) * float64(m.TicksPerRotation)
		err := m.Encoder.SetSpeed(ctx, newSpeed)
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *Motor) setPowerPct(powerPct float64) {
	m.powerPct = powerPct
}

// PowerPct returns the set power percentage.
func (m *Motor) PowerPct() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.DirFlip {
		m.powerPct *= -1
	}
	return m.powerPct
}

// Direction returns the set direction.
func (m *Motor) Direction() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch {
	case m.powerPct > 0:
		return 1
	case m.powerPct < 0:
		return -1
	}
	return 0
}

func goForMath(maxRPM, rpm, revolutions float64) (float64, time.Duration, float64) {
	// need to do this so time is reasonable
	if rpm > maxRPM {
		rpm = maxRPM
	} else if rpm < -1*maxRPM {
		rpm = -1 * maxRPM
	}

	dir := motor.GetRequestedDirection(rpm, revolutions)

	powerPct := math.Abs(rpm) / maxRPM * dir
	waitDur := time.Duration(math.Abs(revolutions/rpm)*60*1000) * time.Millisecond
	return powerPct, waitDur, dir
}

// checkSpeed checks if the input rpm is too slow or fast and returns a warning and/or error.
func checkSpeed(rpm, max float64) (string, error) {
	switch speed := math.Abs(rpm); {
	case speed == 0:
		return "motor speed requested is 0 rev_per_min", motor.NewZeroRPMError()
	case speed > 0 && speed < 0.1:
		return "motor speed is nearly 0 rev_per_min", nil
	case max > 0 && speed > max-0.1:
		return fmt.Sprintf("motor speed is nearly the max rev_per_min (%f)", max), nil
	default:
		return "", nil
	}
}

// GoFor sets the given direction and an arbitrary power percentage.
// If rpm is 0, the motor should immediately move to the final position.
func (m *Motor) GoFor(ctx context.Context, rpm, revolutions float64, extra map[string]interface{}) error {
	warning, err := checkSpeed(rpm, m.MaxRPM)
	if warning != "" {
		m.Logger.CWarn(ctx, warning)
	}
	if err != nil {
		return err
	}

	if err := motor.CheckRevolutions(revolutions); err != nil {
		return err
	}

	powerPct, waitDur, dir := goForMath(m.MaxRPM, rpm, revolutions)

	var finalPos float64
	if m.Encoder != nil {
		curPos, err := m.Position(ctx, nil)
		if err != nil {
			return err
		}
		finalPos = curPos + dir*math.Abs(revolutions)
	}

	err = m.SetPower(ctx, powerPct, nil)
	if err != nil {
		return err
	}

	if m.OpMgr.NewTimedWaitOp(ctx, waitDur) {
		err = m.Stop(ctx, nil)
		if err != nil {
			return err
		}

		if m.Encoder != nil {
			return m.Encoder.SetPosition(ctx, finalPos*float64(m.TicksPerRotation))
		}
	}
	return nil
}

// GoTo sets the given direction and an arbitrary power percentage for now.
func (m *Motor) GoTo(ctx context.Context, rpm, pos float64, extra map[string]interface{}) error {
	if m.Encoder == nil {
		return errors.New("encoder is not defined")
	}

	warning, err := checkSpeed(rpm, m.MaxRPM)
	if warning != "" {
		m.Logger.CWarn(ctx, warning)
	}
	if err != nil {
		return err
	}

	curPos, err := m.Position(ctx, nil)
	if err != nil {
		return err
	}

	if err := motor.CheckRevolutions(pos - curPos); err != nil {
		return err
	}

	revolutions := pos - curPos

	powerPct, waitDur, _ := goForMath(m.MaxRPM, math.Abs(rpm), revolutions)

	err = m.SetPower(ctx, powerPct, nil)
	if err != nil {
		return err
	}

	if m.OpMgr.NewTimedWaitOp(ctx, waitDur) {
		err = m.Stop(ctx, nil)
		if err != nil {
			return err
		}

		return m.Encoder.SetPosition(ctx, pos*float64(m.TicksPerRotation))
	}

	return nil
}

// SetRPM instructs the motor to move at the specified RPM indefinitely.
func (m *Motor) SetRPM(ctx context.Context, rpm float64, extra map[string]interface{}) error {
	warning, err := checkSpeed(rpm, m.MaxRPM)
	if warning != "" {
		m.Logger.CWarn(ctx, warning)
	}
	if err != nil {
		return err
	}

	powerPct := rpm / m.MaxRPM
	return m.SetPower(ctx, powerPct, nil)
}

// Stop has the motor pretend to be off.
func (m *Motor) Stop(ctx context.Context, extra map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Logger.CDebug(ctx, "Motor Stopped")
	m.setPowerPct(0.0)
	if m.Encoder != nil {
		err := m.Encoder.SetSpeed(ctx, 0.0)
		if err != nil {
			return errors.Wrapf(err, "error in Stop from motor (%s)", m.Name())
		}
	}
	return nil
}

// ResetZeroPosition resets the zero position.
func (m *Motor) ResetZeroPosition(ctx context.Context, offset float64, extra map[string]interface{}) error {
	if m.Encoder == nil {
		return errors.New("encoder is not defined")
	}

	if m.TicksPerRotation == 0 {
		return errors.New("need nonzero TicksPerRotation for motor")
	}

	err := m.Encoder.SetPosition(ctx, -1*offset)
	if err != nil {
		return errors.Wrapf(err, "error in ResetZeroPosition from motor (%s)", m.Name())
	}

	return nil
}

// IsPowered returns if the motor is pretending to be on or not, and its power level.
func (m *Motor) IsPowered(ctx context.Context, extra map[string]interface{}) (bool, float64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return math.Abs(m.powerPct) >= 0.005, m.powerPct, nil
}

// IsMoving returns if the motor is pretending to be moving or not.
func (m *Motor) IsMoving(ctx context.Context) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return math.Abs(m.powerPct) >= 0.005, nil
}
