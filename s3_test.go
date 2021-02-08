package cloudwatcher

import (
	"testing"
	"time"
)

func TestS3Watcher_Create(t *testing.T) {
	_, err := New("s3", "/", 10*time.Second)
	if err != nil {
		t.Errorf("error during creation: %s", err)
	}
}

func TestS3Watcher_SetConfig(t *testing.T) {
	s, err := New("s3", "/", 10*time.Second)
	if err != nil {
		t.Errorf("error during creation: %s", err)
	}

	m := map[string]string{
		"this": "is wrong",
	}

	err = s.SetConfig(m)
	if err == nil {
		t.Errorf("it should return an error")
	}

	m2 := map[string]string{
		"bucketname":      "test.storage",
		"Endpoint":        "minio",
		"AccessKey":       "minio",
		"SecretAccessKey": "minio123",
		"SessionToken":    "",
		"Region":          "",
		"SSLEnabled":      "false",
	}
	err = s.SetConfig(m2)
	if err != nil {
		t.Errorf("%s", err)
	}
}

func TestS3Watcher_Start(t *testing.T) {
	s, err := New("s3", "/", 10*time.Second)
	if err != nil {
		t.Errorf("error during creation: %s", err)
	}

	config := map[string]string{
		"bucketname":      "wrong-test.storage",
		"Endpoint":        "127.0.0.1:9000",
		"AccessKey":       "minio",
		"SecretAccessKey": "minio123",
		"SessionToken":    "",
		"Region":          "",
		"SSLEnabled":      "false",
	}
	err = s.SetConfig(config)
	if err != nil {
		t.Errorf("%s", err)
	}

	err = s.Start()
	if err == nil {
		t.Errorf("it should return an error")
	}

	config["bucketname"] = "test.storage"
	err = s.SetConfig(config)
	if err != nil {
		t.Errorf("%s", err)
	}

	err = s.Start()
	if err != nil {
		t.Errorf("%s", err)
	}
}

func TestS3Watcher_Close(t *testing.T) {
	s, err := New("s3", "/", 10*time.Second)
	if err != nil {
		t.Errorf("error during creation: %s", err)
	}

	config := map[string]string{
		"bucketname":      "test.storage",
		"Endpoint":        "127.0.0.1:9000",
		"AccessKey":       "minio",
		"SecretAccessKey": "minio123",
		"SessionToken":    "",
		"Region":          "",
		"SSLEnabled":      "false",
	}
	err = s.SetConfig(config)
	if err != nil {
		t.Errorf("%s", err)
	}

	err = s.Start()
	if err != nil {
		t.Errorf("%s", err)
	}

	s.Close()
}