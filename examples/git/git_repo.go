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
		"debug":           "true",
		"monitor_type":    "repo",
		"repo_url":        "git@github.com:Matrix86/cloudwatcher.git",
		"assemble_events": "true",
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
			if v.Key == "commit" {
				fmt.Println("New commits:")
				for _, c := range v.Object.(*cloudwatcher.GitObject).Commits {
					fmt.Printf("- hash=%s branch=%s : %s\n", c.Hash, c.Branch, c.Message)
				}
			} else if v.Key == "tag" {
				fmt.Println("New tags:")
				for _, c := range v.Object.(*cloudwatcher.GitObject).Commits {
					fmt.Printf("- %s\n", c.Message)
				}
			}

		case e := <-s.GetErrors():
			fmt.Printf("ERROR: %s\n", e)
		}
	}
}
