package models

import (
	"database/sql/driver"
	"fmt"
)

type State string

const (
	Provisioning   State = "provisioning"
	Provisioned    State = "provisioned"
	Deprovisioning State = "deprovisioning"
	Deprovisioned  State = "deprovisioned"
	Conflict       State = "conflict"
	Failed         State = "failed"
	TimedOut       State = "timedout"
)

// Marshal a `State` to a `string` when saving to the database
func (s State) Value() (driver.Value, error) {
	return string(s), nil
}

// Unmarshal an `interface{}` to a `State` when reading from the database
func (s *State) Scan(value interface{}) error {
	switch value.(type) {
	case string:
		*s = State(value.(string))
	case []byte:
		*s = State(value.([]byte))
	default:
		err := fmt.Errorf("%s-is-incompatible", value)
		helperLogger.Session("state-scan").Error("scan-switch", err)
		return err
	}
	return nil
}
