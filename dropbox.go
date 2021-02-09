package cloudwatcher

import (
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/dropbox/dropbox-sdk-go-unofficial/dropbox"
	"github.com/dropbox/dropbox-sdk-go-unofficial/dropbox/files"
)

type DropboxWatcher struct {
	WatcherBase

	syncing uint32

	ticker *time.Ticker
	stop   chan bool
	config *DropboxConfiguration
	cache  map[string]*DropboxObject
}

type DropboxObject struct {
	Key          string
	Size         int64
	LastModified time.Time
	Hash         string
}

type DropboxConfiguration struct {
	Debug     Bool   `json:"debug"`
	Token     string `json:"token"`
}

func newDropboxWatcher(dir string, interval time.Duration) (Watcher, error) {
	w := &DropboxWatcher{
		cache:  make(map[string]*DropboxObject),
		config: nil,
		stop:   make(chan bool, 1),
		WatcherBase: WatcherBase{
			Events:      make(chan Event, 100),
			Errors:      make(chan error, 100),
			watchDir:    dir,
			pollingTime: interval,
		},
	}

	return w, nil
}

func (w *DropboxWatcher) SetConfig(m map[string]string) error {
	j, err := json.Marshal(m)
	if err != nil {
		return err
	}

	config := DropboxConfiguration{}
	if err := json.Unmarshal(j, &config); err != nil {
		return err
	}

	if config.Token == "" {
		return fmt.Errorf("token not specified")
	}
	w.config = &config


	fmt.Printf("%v", config)
	return nil
}

func (w *DropboxWatcher) Start() error {
	if w.config == nil {
		return fmt.Errorf("configuration for Dropbox needed")
	}

	w.ticker = time.NewTicker(w.pollingTime)
	go func() {
		// launch synchronization also the first time
		w.sync()
		for {
			select {
			case <-w.ticker.C:
				w.sync()

			case <-w.stop:
				close(w.Events)
				close(w.Errors)
				return
			}
		}
	}()
	return nil
}

func (w *DropboxWatcher) Close() {
	w.stop <- true
}

func (w *DropboxWatcher) sync() {
	// allow only one sync at same time
	if !atomic.CompareAndSwapUint32(&w.syncing, 0, 1) {
		return
	}
	defer atomic.StoreUint32(&w.syncing, 0)

	fileList := make(map[string]*DropboxObject, 0)

	err := w.enumerateFiles(w.watchDir, func(obj *DropboxObject) bool {
		// Store the files to check the deleted one
		fileList[obj.Key] = obj
		// Check if the object is cached by Key
		cached := w.getCachedObject(obj)
		// Object has been cached previously by Key
		if cached != nil {
			// Check if the LastModified has been changed
			if !cached.LastModified.Equal(obj.LastModified) || cached.Hash != obj.Hash {
				event := Event{
					Key:    obj.Key,
					Type:   FileChanged,
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
		w.cache[obj.Key] = obj
		return true
	})
	if err != nil {
		w.Errors <- err
		return
	}

	for k, o := range w.cache {
		if _, found := fileList[k]; !found {
			// file not found in the list...deleting it
			delete(w.cache, k)
			event := Event{
				Key:    o.Key,
				Type:   FileDeleted,
				Object: nil,
			}
			w.Events <- event
		}
	}
}

func (w *DropboxWatcher) enumerateFiles(prefix string, callback func(object *DropboxObject) bool) error {
	logLevel := dropbox.LogOff
	if w.config.Debug {
		logLevel = dropbox.LogDebug
	}

	config := dropbox.Config{
		Token:           w.config.Token,
		LogLevel:        logLevel,
		Logger:          nil,
		AsMemberID:      "",
		Domain:          "",
		Client:          nil,
		HeaderGenerator: nil,
		URLGenerator:    nil,
	}
	dbx := files.New(config)
	arg := files.NewListFolderArg(prefix)
	arg.Recursive = true

	var entries []files.IsMetadata
	res, err := dbx.ListFolder(arg)
	if err != nil {
		listRevisionError, ok := err.(files.ListRevisionsAPIError)
		if ok {
			// Don't treat a "not_folder" error as fatal; recover by sending a
			// get_metadata request for the same path and using that response instead.
			if listRevisionError.EndpointError.Path.Tag == files.LookupErrorNotFolder {
				var metaRes files.IsMetadata
				metaRes, err = w.getFileMetadata(dbx, prefix)
				entries = []files.IsMetadata{metaRes}
			} else {
				// Return if there's an error other than "not_folder" or if the follow-up
				// metadata request fails.
				return err
			}
		} else {
			return err
		}
	} else {
		entries = res.Entries

		for res.HasMore {
			arg := files.NewListFolderContinueArg(res.Cursor)

			res, err = dbx.ListFolderContinue(arg)
			if err != nil {
				return err
			}

			entries = append(entries, res.Entries...)
		}
	}

	for _, entry := range entries {
		o := &DropboxObject{}
		switch f := entry.(type) {
		case *files.FileMetadata:
			o.Key = f.PathDisplay
			o.Size = int64(f.Size)
			o.LastModified = f.ServerModified
			o.Hash = f.ContentHash
			callback(o)
			//case *files.FolderMetadata:
			//	o.Key = f.PathDisplay
			//	o.Size = 0
			//	o.LastModified = 0
		}
	}

	return nil
}

func (w *DropboxWatcher) getFileMetadata(c files.Client, path string) (files.IsMetadata, error) {
	arg := files.NewGetMetadataArg(path)

	arg.IncludeDeleted = true

	res, err := c.GetMetadata(arg)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (w *DropboxWatcher) getCachedObject(o *DropboxObject) *DropboxObject {
	if cachedObject, ok := w.cache[o.Key]; ok {
		return cachedObject
	}
	return nil
}

func init() {
	supportedServices["dropbox"] = newDropboxWatcher
}
