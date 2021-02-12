package cloudwatcher

import (
	"fmt"
	"time"
)

type storageFunc func(dir string, interval time.Duration) (Watcher, error)

var supportedServices map[string]storageFunc

// WatcherBase is the struct included in all the specialized watchers
type WatcherBase struct {
	Events chan Event
	Errors chan error

	watchDir    string
	pollingTime time.Duration
}

// Watcher has to be implemented by all the watchers
type Watcher interface {
	Start() error
	SetConfig(c map[string]string) error
	Close()
	GetEvents() chan Event
	GetErrors() chan error
}

// New creates a new instance of a watcher
func New(serviceName string, dir string, interval time.Duration) (Watcher, error) {
	f, ok := supportedServices[serviceName]
	if !ok {
		return nil, fmt.Errorf("service %s is not yet supported", serviceName)
	}
	return f(dir, interval)

}

// GetEvents returns a chan of Event
func (w *WatcherBase) GetEvents() chan Event {
	return w.Events
}

// GetErrors returns a chan of error
func (w *WatcherBase) GetErrors() chan error {
	return w.Errors
}

func init() {
	supportedServices = make(map[string]storageFunc)
}
