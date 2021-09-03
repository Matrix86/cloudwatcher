package cloudwatcher

import (
	"encoding/json"
	"fmt"
	"github.com/fsnotify/fsnotify"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"
)

// LocalWatcher is the specialized watcher for the local FS
type LocalWatcher struct {
	WatcherBase

	syncing uint32

	watcher *fsnotify.Watcher
	ticker  *time.Ticker
	stop    chan bool
	config  *localConfiguration
	cache   map[string]*LocalObject
}

// LocalObject is the object that contains the info of the file
type LocalObject struct {
	Key          string
	Size         int64
	LastModified time.Time
	FileMode     os.FileMode
}

type localConfiguration struct {
	Debug             Bool `json:"debug"`
	DisableFsNotify   Bool `json:"disable_fsnotify"`
}

func newLocalWatcher(dir string, interval time.Duration) (Watcher, error) {
	w := &LocalWatcher{
		config: &localConfiguration{},
		stop:   make(chan bool, 1),
		cache:  make(map[string]*LocalObject),
		WatcherBase: WatcherBase{
			Events:      make(chan Event, 100),
			Errors:      make(chan error, 100),
			watchDir:    dir,
			pollingTime: interval,
		},
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, fmt.Errorf("directory '%s' not found", dir)
	}

	return w, nil
}

// SetConfig is used to configure the LocalWatcher
func (w *LocalWatcher) SetConfig(m map[string]string) error {
	j, err := json.Marshal(m)
	if err != nil {
		return err
	}

	config := localConfiguration{}
	if err := json.Unmarshal(j, &config); err != nil {
		return err
	}
	w.config = &config

	return nil
}

// Start launches the polling process
func (w *LocalWatcher) Start() error {
	if _, err := os.Stat(w.watchDir); os.IsNotExist(err) {
		return fmt.Errorf("directory '%s' not found", w.watchDir)
	}

	if w.config.DisableFsNotify {
		w.ticker = time.NewTicker(w.pollingTime)
		go func() {
			// launch synchronization also the first time
			w.sync(true)
			for {
				select {
				case <-w.ticker.C:
					w.sync(false)

				case <-w.stop:
					close(w.Events)
					close(w.Errors)
					return
				}
			}
		}()
	} else {
		var err error
		if w.watcher == nil {
			w.watcher, err = fsnotify.NewWatcher()
			if err != nil {
				return err
			}
		}

		go func() {
			for {
				select {
				case event, ok := <-w.watcher.Events:
					if !ok {
						return
					}

					obj := &LocalObject{
						Key:          event.Name,
						Size:         0,
						LastModified: time.Now(),
						FileMode:     0,
					}
					e := Event{}

					var t Op
					if event.Op&fsnotify.Write == fsnotify.Write {
						t = FileChanged
					} else if event.Op&fsnotify.Create == fsnotify.Create {
						t = FileCreated
					} else if event.Op&fsnotify.Remove == fsnotify.Remove {
						t = FileDeleted
					} else if event.Op&fsnotify.Chmod == fsnotify.Chmod {
						t = TagsChanged
					} else {
						// ignoring other events
						continue
					}

					switch t {
					case FileDeleted:
						e = Event{
							Key:    obj.Key,
							Object: obj,
							Type:   t,
						}

						// we don't know if it was a folder...
						w.rmRecursive(event.Name)

					case FileCreated, FileChanged, TagsChanged:
						fi, err := os.Stat(event.Name)
						if err != nil {
							w.Errors <- err
							continue
						}

						// create listener on subfolders
						if fi.IsDir() {
							if err := w.addRecursive(event.Name); err != nil {
								w.Errors <- err
							}
						}

						obj = &LocalObject{
							Key:          event.Name,
							Size:         fi.Size(),
							LastModified: fi.ModTime(),
							FileMode:     fi.Mode(),
						}

						e = Event{
							Key:    obj.Key,
							Object: obj,
							Type:   t,
						}
					}
					w.Events <- e

				case err, ok := <-w.watcher.Errors:
					if !ok {
						return
					}
					w.Errors <- err

				case <-w.stop:
					close(w.Events)
					close(w.Errors)
					w.rmRecursive(w.watchDir)
					w.watcher.Close()
					return
				}
			}
		}()

		if err = w.addRecursive(w.watchDir); err != nil {
			w.Close()
			return err
		}
	}
	return nil
}

// Close stop the polling process
func (w *LocalWatcher) Close() {
	w.stop <- true
}

func (w *LocalWatcher) sync(firstSync bool) {
	// allow only one sync at same time
	if !atomic.CompareAndSwapUint32(&w.syncing, 0, 1) {
		return
	}
	defer atomic.StoreUint32(&w.syncing, 0)

	if _, err := os.Stat(w.watchDir); os.IsNotExist(err) {
		w.Errors <- fmt.Errorf("directory '%s' not found", w.watchDir)
	}

	fileList := make(map[string]*LocalObject, 0)

	err := filepath.Walk(w.watchDir, func(walkPath string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Ignore the changes on the watched dir
		if walkPath == w.watchDir {
			return nil
		}

		obj := &LocalObject{
			Key:          walkPath,
			Size:         fi.Size(),
			LastModified: fi.ModTime(),
			FileMode:     fi.Mode(),
		}

		if !firstSync {
			fileList[walkPath] = obj

			// Check if the object is cached by Key
			cached := w.getCachedObject(obj)
			// Object has been cached previously by Key
			if cached != nil {
				// Check if the LastModified has been changed
				if !cached.LastModified.Equal(obj.LastModified) || (cached.Size != obj.Size) {
					event := Event{
						Key:    obj.Key,
						Type:   FileChanged,
						Object: obj,
					}
					w.Events <- event
				}
				// Check if the file modes have been updated
				if cached.FileMode != obj.FileMode {
					event := Event{
						Key:    obj.Key,
						Type:   TagsChanged,
						Object: obj,
					}
					w.Events <- event
				}
			} else {
				event := Event{
					Key:    obj.Key,
					Type:   FileCreated,
					Object: obj,
				}
				w.Events <- event
			}
		}

		w.cache[obj.Key] = obj
		return nil
	})
	if err != nil {
		w.Errors <- err
		return
	}

	if len(fileList) != 0 {
		for k, o := range w.cache {
			if _, found := fileList[k]; !found {
				// file not found in the list...deleting it
				delete(w.cache, k)
				event := Event{
					Key:    o.Key,
					Type:   FileDeleted,
					Object: o,
				}
				w.Events <- event
			}
		}
	}
}

func (w *LocalWatcher) getCachedObject(o *LocalObject) *LocalObject {
	if cachedObject, ok := w.cache[o.Key]; ok {
		return cachedObject
	}
	return nil
}

func (w *LocalWatcher) addRecursive(dir string) error {
	err := filepath.Walk(dir, func(walkPath string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() {
			if err = w.watcher.Add(walkPath); err != nil {
				return err
			}
		}
		return nil
	})
	return err
}

func (w *LocalWatcher) rmRecursive(dir string) error {
	err := filepath.Walk(dir, func(walkPath string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() {
			if err = w.watcher.Remove(walkPath); err != nil {
				return err
			}
		}
		return nil
	})
	return err
}

func init() {
	supportedServices["local"] = newLocalWatcher
}
