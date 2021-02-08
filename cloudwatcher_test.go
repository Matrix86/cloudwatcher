package cloudwatcher

import (
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	_, err := New("wrong", "/", 10*time.Second)
	if err == nil {
		t.Errorf("it should return an error if the service name not exists")
	}

	_, err = New("s3", "/", 10*time.Second)
	if err != nil {
		t.Errorf("error during creation: %s", err)
	}
}
