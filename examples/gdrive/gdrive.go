package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Matrix86/cloudwatcher"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"time"
)

const (
	ClientId     = "XXXXX.apps.googleusercontent.com"
	ClientSecret = "X-XXXX-XX"
	Token        = "{\"access_token\":\"XXXXX\",\"token_type\":\"Bearer\",\"refresh_token\":\"XXXX\",\"expiry\":\"2021-02-09T20:11:25.925429905+01:00\"}"
)

func getToken() (string, error) {
	config := &oauth2.Config{
		ClientID:     ClientId,
		ClientSecret: ClientSecret,
		Scopes:       []string{drive.DriveMetadataReadonlyScope},
		Endpoint:     google.Endpoint,
	}

	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		return "", err
	}

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		return "", err
	}

	b, err := json.Marshal(tok)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
func main() {
	s, err := cloudwatcher.New("gdrive", "", 2*time.Second)
	if err != nil {
		fmt.Printf("ERROR: %s", err)
		return
	}

	config := map[string]string{
		"debug":         "true",
		"token":         Token,
		"client_id":     ClientId,
		"client_secret": ClientSecret,
	}

	if v, ok := config["token"]; !ok || v == "" {
		token, err := getToken()
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
