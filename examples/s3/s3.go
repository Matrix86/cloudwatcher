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
		"bucket_name": "test.storage",
		"endpoint":   "127.0.0.1:9000",
		"access_key":  "minio",
		"secret_key":  "minio123",
		"token":      "",
		"region":     "",
		"ssl_enabled": "false",
	}
	err = s.SetConfig(config)
	if err != nil {
		fmt.Printf("ERROR: %s", err)
		return
	}

	err = s.Start()
	defer s.Close()
	for {
		select {
		case v := <-s.GetEvents():
			fmt.Printf("EVENT: %s %s\n", v.Key, v.TypeString())

		case e := <-s.GetErrors():
			fmt.Printf("ERROR: %#v\n", e)
		}
	}
}
