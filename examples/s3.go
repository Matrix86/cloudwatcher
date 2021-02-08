package main

import (
	"fmt"
	"github.com/Matrix86/cloudwatcher"
	"time"
)

func main() {
	s, err := cloudwatcher.New("s3", "/", 1*time.Second)
	if err != nil {
		fmt.Printf("ERROR: %s", err)
		return
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
		fmt.Printf("ERROR: %s", err)
		return
	}

	err = s.SetConfig(config)
	if err != nil {
		fmt.Printf("ERROR: %s", err)
		return
	}

	err = s.Start()
	defer s.Close()

	go func() {
		for v := range s.GetEvents() {
			fmt.Printf("EVENT: %s %s\n", v.Key, v.TypeString())
		}
	}()

	go func() {
		for e := range s.GetErrors() {
			fmt.Printf("ERROR: %#v\n", e)
		}
	}()

	time.Sleep(30*time.Second)
}
