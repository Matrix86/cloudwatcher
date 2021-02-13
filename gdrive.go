package cloudwatcher

import (
	"context"
	"encoding/json"
	"fmt"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"strings"
	"sync/atomic"
	"time"

	"google.golang.org/api/drive/v3"
)

// GDriveWatcher is the specialized watcher for Google Drive service
type GDriveWatcher struct {
	WatcherBase

	syncing uint32

	ticker *time.Ticker
	stop   chan bool
	config *gDriveConfiguration
	cache  map[string]*GDriveObject
	client *drive.Service
}

// GDriveObject is the object that contains the info of the file
type GDriveObject struct {
	ID           string
	Key          string
	Size         int64
	LastModified time.Time
	Hash         string
}

type gDriveConfiguration struct {
	Debug        Bool   `json:"debug"`
	JToken       string `json:"token"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	APIKey       string `json:"api_key"`

	token *oauth2.Token
}

func newGDriveWatcher(dir string, interval time.Duration) (Watcher, error) {
	w := &GDriveWatcher{
		cache:  make(map[string]*GDriveObject),
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

// SetConfig is used to configure the GDriveWatcher
func (w *GDriveWatcher) SetConfig(m map[string]string) error {
	j, err := json.Marshal(m)
	if err != nil {
		return err
	}

	config := gDriveConfiguration{}
	if err := json.Unmarshal(j, &config); err != nil {
		return err
	}

	if config.JToken == "" && config.APIKey == "" {
		return fmt.Errorf("token or api_key have to be set")
	}
	w.config = &config

	tok := &oauth2.Token{}
	if err := json.Unmarshal([]byte(config.JToken), tok); err != nil {
		return err
	}
	w.config.token = tok
	return nil
}

// Start launches the polling process
func (w *GDriveWatcher) Start() error {
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

// Close stop the polling process
func (w *GDriveWatcher) Close() {
	if w.stop != nil {
		w.stop <- true
	}
}

func (w *GDriveWatcher) sync() {
	// allow only one sync at same time
	if !atomic.CompareAndSwapUint32(&w.syncing, 0, 1) {
		return
	}
	defer atomic.StoreUint32(&w.syncing, 0)

	fileList := make(map[string]*GDriveObject, 0)

	err := w.enumerateFiles(w.watchDir, func(obj *GDriveObject) bool {
		// Store the files to check the deleted one
		fileList[obj.ID] = obj
		// Check if the object is cached by Key
		cached := w.getCachedObject(obj)
		// Object has been cached previously by Key
		if cached != nil {
			// Check if the LastModified has been changed
			if !cached.LastModified.Equal(obj.LastModified) || cached.Hash != obj.Hash {
				fmt.Printf("cached %s obj %s\n", cached.LastModified, obj.LastModified)
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
		w.cache[obj.ID] = obj
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
				Object: o,
			}
			w.Events <- event
		}
	}
}

func (w *GDriveWatcher) resolveParents(file *drive.File, list map[string]*drive.File) [][]string {
	paths := make([][]string, len(file.Parents))
	for i := range paths {
		paths[i] = make([]string, 0)
		paths[i] = append(paths[i], file.Name)
	}
	for i, parentID := range file.Parents {
		if v, ok := list[parentID]; !ok {
			continue
		} else {
			for _, ppath := range w.resolveParents(v, list) {
				paths[i] = append(ppath, paths[i]...)
			}
		}
	}
	return paths
}

func (w *GDriveWatcher) getFullPaths(file *drive.File, list map[string]*drive.File) []string {
	tpaths := w.resolveParents(file, list)
	paths := make([]string, 0)
	for _, path := range tpaths {
		paths = append(paths, strings.Join(path, "/"))
	}
	return paths
}

func (w *GDriveWatcher) initDriverClient() {
	var err error
	config := &oauth2.Config{
		ClientID:     w.config.ClientID,
		ClientSecret: w.config.ClientSecret,
		Scopes:       []string{drive.DriveMetadataReadonlyScope},
		Endpoint:     google.Endpoint,
	}

	var opt option.ClientOption
	if w.config.token != nil {
		opt = option.WithTokenSource(config.TokenSource(context.Background(), w.config.token))
	} else if w.config.APIKey != "" {
		opt = option.WithAPIKey(w.config.APIKey)
	}

	w.client, err = drive.NewService(context.Background(), opt)
	if err != nil {
		w.Errors <- fmt.Errorf("unable to retrieve Drive client: %v", err)
	}
}

func (w *GDriveWatcher) enumerateFiles(prefix string, callback func(object *GDriveObject) bool) error {
	if w.client == nil {
		w.initDriverClient()
	}

	err := w.client.Files.List().Fields("nextPageToken, files(id, name, mimeType, modifiedTime, parents, size, md5Checksum, trashed)").Pages(context.Background(), func(files *drive.FileList) error {
		fileList := make(map[string]*drive.File)

		// we need to map all the files with their id to construct the file tree
		for _, f := range files.Files {
			fileList[f.Id] = f
		}

		for _, file := range fileList {
			if file.MimeType != "application/vnd.google-apps.folder" && !file.Trashed {
				for _, name := range w.getFullPaths(file, fileList) {
					mt, err := time.Parse(time.RFC3339, file.ModifiedTime)
					if err != nil {
						w.Errors <- err
						continue
					}
					if strings.HasPrefix(name, prefix) {
						o := &GDriveObject{
							ID:           file.Id,
							Key:          name,
							Size:         file.Size,
							LastModified: mt,
							Hash:         file.Md5Checksum,
						}
						if callback(o) == false {
							break
						}
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("unable to retrieve files: %v", err)
	}
	return nil
}

func (w *GDriveWatcher) getCachedObject(o *GDriveObject) *GDriveObject {
	if cachedObject, ok := w.cache[o.ID]; ok {
		return cachedObject
	}
	return nil
}

func init() {
	supportedServices["gdrive"] = newGDriveWatcher
}
