package main

import (
	"context"
	"fmt"
	"github.com/Matrix86/cloudwatcher"
	"github.com/dropbox/dropbox-sdk-go-unofficial/dropbox"
	"golang.org/x/oauth2"
	"time"
)

func getAuthCode() (string, error) {
	conf := &oauth2.Config{
		ClientID:     "CLIENT_ID_HERE",
		ClientSecret: "CLIENT_SECRET_HERE",
		Endpoint:     dropbox.OAuthEndpoint(""),
	}

	fmt.Printf("1. Go to %v\n", conf.AuthCodeURL("state"))
	fmt.Printf("2. Click \"Allow\" (you might have to log in first).\n")
	fmt.Printf("3. Copy the authorization code.\n")
	fmt.Printf("Enter the authorization code here: ")

	var code string
	if _, err := fmt.Scan(&code); err != nil {
		return "", err
	}

	ctx := context.Background()
	token, err := conf.Exchange(ctx, code)
	if err != nil {
		return "", err
	}
	return token.AccessToken, nil
}
func main() {
	s, err := cloudwatcher.New("dropbox", "", 2*time.Second)
	if err != nil {
		fmt.Printf("ERROR: %s", err)
		return
	}

	config := map[string]string{
		"debug":     "true",
		"token":     "",
	}

	if v, ok := config["token"]; !ok ||  v == "" {
		token, err := getAuthCode()
		if err != nil {
			fmt.Printf("ERROR: %s", err)
			return
		}
		fmt.Printf("NEW TOKEN: %s\n", token)
		config["token"] = token
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
