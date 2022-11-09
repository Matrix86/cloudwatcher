# Cloudwatcher
![GitHub](https://img.shields.io/github/license/Matrix86/cloudwatcher)
![GitHub release (latest by date)](https://img.shields.io/github/v/release/Matrix86/cloudwatcher)
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/Matrix86/cloudwatcher)
![Build and Test](https://github.com/Matrix86/cloudwatcher/workflows/Build%20and%20Test/badge.svg) 
![Codecov](https://img.shields.io/codecov/c/github/Matrix86/cloudwatcher)

File system notification for Cloud platforms (and not) in Golang.

cloudwatcher is a file system notification library for cloud platforms (and not) in Go. 
Currently it implements the watchers for the following services:
- Amazon S3
- Google Drive
- Dropbox
- Git
- local filesystem (fsnotify/polling)

It is possible specify the directory to monitor and the polling time (how much often the watcher should check that directory), 
and every time a new file is added, removed or changed, an event is generated and sent to the `Events` channel.

## Usage

```go
package main

import (
	"fmt"
	"time"

	"github.com/Matrix86/cloudwatcher"
)

func main() {
    // the first parameter is the type of service to use: local, dropbox, gdrive or s3
	s, err := cloudwatcher.New("local", "/home/user/tests", time.Second)
	if err != nil {
		fmt.Printf("ERROR: %s", err)
		return
	}

	config := map[string]string{
		"disable_fsnotify": "false",
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
```

The channel returned by `GetEvents()` function, will return an [Event](event.go) struct that contains the event type, the Key
with the name of the file that generates the event and the object itself.

> :warning: check the Event.Object field before use it...in some cases it could be nil (FileDelete event with fsnotify)  

## Amazon S3

The config of the S3 watcher is the following:

```go
config := map[string]string{
    "bucket_name": "storage",
    "endpoint":   "s3-us-west-2.amazonaws.com",
    "access_key":  "user",
    "secret_key":  "secret",
    "token":      "",
    "region":     "us-west-2",
    "ssl_enabled": "true",
}
```

To use [AWS IAM credentials or AWS file](https://docs.aws.amazon.com/sdk-for-go/v1/developer-guide/configuring-sdk.html#specifying-credentials), it is possible to set one of the following vars to "`true`": `aws_iam_credentials`, `aws_file`.
It is also possible to specify the IAM endpoint (`aws_file_profile`) or the path of the file to use (`aws_file_profile`), if not specified it will use the default endpoint and file ($HOME/.aws/credentials).

```go
config := map[string]string{
    "aws_file_profile": "true",
}
```

An example can be found [here](examples/s3/s3.go).

> :gem: [minio](https://docs.min.io/docs/minio-quickstart-guide.html) can be used for testing purposes

## Google Drive

In order to use it you need to enable the drive API from [here](https://developers.google.com/drive/api/v3/enable-drive-api) .
After that you can specify the `client-id` and the `client-secret` on the config file. 
The logic to retrieve the token has to be handled outside the library and the `token` field should contain the json. 

You can find an example in the [examples directory](examples/gdrive/gdrive.go). 

```go
config := map[string]string{
    "debug":         "true",
    "token":         Token,
    "client_id":     ClientId,
    "client_secret": ClientSecret,
}
```

## Dropbox

First, you need to register a new app from the [developer console](https://www.dropbox.com/developers/).
Use the `client-id` and the `client-secret` to retrieve the user token and use it on the `token` field of the config.

You can find an example in the [examples directory](examples/dropbox/dropbox.go). 

```go
config := map[string]string{
    "debug": "true",
    "token": Token,
}
```

## Local filesystem

It is based on [fsnotify](https://github.com/fsnotify/fsnotify) library, so it is cross platform: Windows, Linux, BSD and macOS.
It is not mandatory to call the `SetConfig()` function, and the polling time argument of `cloudwatcher.New` is not used.

Setting `disable_fsnotify` parameter on config to "true" the watcher doesn't use fsnotify and use the listing approach instead.

> :warning: not set `disable_fsnotify` to "true" if you plan to use it on a big directory!!! It could increase the I/O on disk

## Git

Git watcher has the following configurations:

| Name | Description 
| --- | --- |
| `debug` | if "true" the debug mode is enabled (default "false") |
| `monitor_type` | it can be "file" or "repo" (default is "repo") |
| `auth_type` | authentication type to use: "none", "ssh", "http_token", "http_user_pass" (default "none") |
| `ssh_pkey` | path of the ssh private key (required if auth_type = "ssh") |
| `ssh_pkey_password` | password of the private key if set |
| `http_token` | token to use if auth_type = "http_token" |
| `http_username` | username of github account (auth_type = "http_user_pass") |
| `http_password` | password of github account (auth_type = "http_user_pass") |
| `repo_url` | url of the repository |
| `repo_branch` | branch to watch (if `monitor_type` is "repo" you can leave it empty to watch all the branches) |
| `assemble_events` | if "true" the events could contain one or more commit events (only if `monitor_type` = "repo") |
| `temp_dir` | temporary directory to use for clone the repo: if empty the tmp dir will be used |

If `monitor_type` is set to "repo", the event channel will receive an event with the `Object` field filled with commits or tags.
If `assemble_events` is "true" the `Object` field could contains one or more commits.