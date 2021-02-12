package cloudwatcher

import (
	"encoding/json"
)

// Bool is an alias for the standard boolean type with Unmarshal/Marshal functions
type Bool bool

// UnmarshalJSON allows to unmarshal from string to bool
func (b *Bool) UnmarshalJSON(s []byte) error {
	var txt string
	err := json.Unmarshal(s, &txt)
	if err != nil {
		return err
	}
	if txt == "1" || txt == "true" {
		*b = Bool(true)
	} else {
		*b = Bool(false)
	}
	return nil
}

// MarshalJSON allows the conversion from bool to string
func (b Bool) MarshalJSON() ([]byte, error) {
	if b {
		return []byte("true"), nil
	}
	return []byte("false"), nil
}
