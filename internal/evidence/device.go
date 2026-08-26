package evidence

import "rockwool-facade-render-handover/internal/domain"

// DeviceOutcome is a scripted device result.
type DeviceOutcome string

const (
	OutcomeSuccess    DeviceOutcome = "success"
	OutcomeTimeout    DeviceOutcome = "timeout"
	OutcomeMalformat  DeviceOutcome = "malformat"
	OutcomeRefuse     DeviceOutcome = "refuse"
	OutcomeDisconnect DeviceOutcome = "disconnect"
)

// ScriptedDevice is a deterministic device adapter driven by an explicit script
// of outcomes. Each call consumes the next scripted outcome and its associated
// value, so tests never depend on real time or random backoff.
type ScriptedDevice struct {
	Script []DeviceOutcome
	Values []int64
	idx    int
}

// NewScriptedDevice builds a scripted device from a sequence of outcomes and
// matching values (values are only meaningful for success outcomes).
func NewScriptedDevice(script []DeviceOutcome, values []int64) *ScriptedDevice {
	return &ScriptedDevice{Script: script, Values: values}
}

// Call returns the next scripted outcome and its value.
func (d *ScriptedDevice) Call() (DeviceOutcome, int64) {
	if d.idx >= len(d.Script) {
		return OutcomeDisconnect, 0
	}
	outcome := d.Script[d.idx]
	value := int64(0)
	if d.idx < len(d.Values) {
		value = d.Values[d.idx]
	}
	d.idx++
	return outcome, value
}

// DeviceCall is a persisted device request record with attempt count, result
// status, a stable fault code and a raw-format validation summary.
type DeviceCall struct {
	Device      string
	Attempt     int
	Outcome     DeviceOutcome
	FaultCode   domain.ErrorCode
	LogicalTime domain.LogicalTime
	Value       int64
	RawValid    bool
}

// RetryLimit is the deterministic retry ceiling for device calls.
const RetryLimit = 3

// FaultCodeFor maps a device outcome to a stable fault code (empty for success).
func FaultCodeFor(o DeviceOutcome) domain.ErrorCode {
	switch o {
	case OutcomeTimeout:
		return domain.CodeDeviceError
	case OutcomeMalformat:
		return domain.CodeInvalid
	case OutcomeRefuse:
		return domain.CodeDeviceError
	case OutcomeDisconnect:
		return domain.CodeDeviceError
	default:
		return ""
	}
}
