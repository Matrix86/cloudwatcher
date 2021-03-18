package main

import (
	"fmt"
	"github.com/Matrix86/cloudwatcher"
	"time"
)

func main() {
	s, err := cloudwatcher.New("git", "", 2*time.Second)
	if err != nil {
		fmt.Printf("ERROR: %s", err)
		return
	}

	config := map[string]string{
		"debug":        "true",
		"monitor_type": "file",
		"repo_url":     "git@github.com:Matrix86/cloudwatcher.git",
		"repo_branch":  "main",
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
			fmt.Printf("ERROR: %s\n", e)
		}
	}
}
