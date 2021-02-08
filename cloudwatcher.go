package cloudwatcher

import (
	"fmt"
	"time"
)

type storageFunc func(config interface{}) (Watcher, error)
var supportedServices map[string]storageFunc

type WatcherBase struct {
	Events chan Event
	Errors chan error

	watchDir string
	pollingTime time.Duration
}

type Watcher interface {
	Start()
	Close()
}

func New(service string, config interface{}) (Watcher, error) {
	if f, ok := supportedServices[service]; !ok {
		return nil, fmt.Errorf("service %s is not yet supported")
	} else {
		return f(config)
	}
}

func init() {
	supportedServices = make(map[string]storageFunc)
}