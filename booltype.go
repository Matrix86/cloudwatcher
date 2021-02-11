package cloudwatcher

import (
	"encoding/json"
)

type Bool bool

func (bit *Bool) UnmarshalJSON(b []byte) error {
	var txt string
	err := json.Unmarshal(b, &txt)
	if err != nil {
		return err
	}
	*bit = Bool(txt == "1" || txt == "true")
	return nil
}

func (bit *Bool) MarshalJSON() ([]byte, error) {
	if *bit {
		return []byte("true"), nil
	}
	return []byte("false"), nil
}
