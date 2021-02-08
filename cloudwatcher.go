package cloudwatcher

import (
	"fmt"
	"time"
)

type storageFunc func(dir string, interval time.Duration) (Watcher, error)
var supportedServices map[string]storageFunc

type WatcherBase struct {
	Events chan Event
	Errors chan error

	watchDir string
	pollingTime time.Duration
}

type Watcher interface {
	Start() error
	SetConfig(c map[string]string) error
	Close()
	GetEvents() chan Event
	GetErrors() chan error
}

func New(service string, dir string, interval time.Duration) (Watcher, error) {
	if f, ok := supportedServices[service]; !ok {
		return nil, fmt.Errorf("service %s is not yet supported", service)
	} else {
		return f(dir, interval)
	}
}

func (w *WatcherBase) GetEvents() chan Event {
	return w.Events
}

func (w *WatcherBase) GetErrors() chan error {
	return w.Errors
}

func init() {
	supportedServices = make(map[string]storageFunc)
}