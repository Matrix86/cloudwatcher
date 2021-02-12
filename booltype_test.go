package cloudwatcher

import (
	"encoding/json"
	"testing"
)

func TestBool_MarshalJSON(t *testing.T) {
	type X struct {
		B Bool
	}

	x := X{}
	err := json.Unmarshal([]byte("{\"B\":\"true\"}"), &x)
	if err != nil {
		t.Errorf("error during the unmarshalling : %s", err)
	}

	if x.B == false {
		t.Errorf("it should be true")
	}

	err = json.Unmarshal([]byte("{\"B\":\"1\"}"), &x)
	if err != nil {
		t.Errorf("error during the unmarshalling : %s", err)
	}

	if x.B == false {
		t.Errorf("it should be true")
	}

	err = json.Unmarshal([]byte("{\"B\":\"false\"}"), &x)
	if err != nil {
		t.Errorf("error during the unmarshalling : %s", err)
	}

	if x.B == true {
		t.Errorf("it should be false")
	}

	err = json.Unmarshal([]byte("{\"B\":false}"), &x)
	if err == nil {
		t.Errorf("it should return an error")
	}
}

func TestBool_UnmarshalJSON(t *testing.T) {
	type X struct {
		B Bool
	}

	x := X{true}
	j, err := json.Marshal(x)
	if err != nil {
		t.Errorf("error during the marshalling : %s", err)
	}

	if string(j) != "{\"B\":true}" {
		t.Errorf("marshalling didn't work correctly")
	}

	x.B = false
	j, err = json.Marshal(x)
	if err != nil {
		t.Errorf("error during the marshalling : %s", err)
	}

	if string(j) != "{\"B\":false}" {
		t.Errorf("marshalling didn't work correctly")
	}
}
