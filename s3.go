package cloudwatcher

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/minio/minio-go"
	"github.com/minio/minio-go/pkg/credentials"
)

type objectInfo = minio.ObjectInfo
type S3Configuration struct {
	BucketName      string `json:"bucket_name"`
	Endpoint        string `json:"endpoint"`
	AccessKey       string `json:"access_key"`
	SecretAccessKey string `json:"secret_key"`
	SessionToken    string `json:"token"`
	Region          string `json:"region"`
	SSLEnabled      Bool   `json:"ssl_enabled"`
}

type S3Watcher struct {
	WatcherBase

	syncing uint32

	ticker *time.Ticker
	stop   chan bool
	config *S3Configuration
	client *minio.Client
	cache  map[string]*S3Object
}

type S3Object struct {
	Key          string
	Etag         string
	Size         int64
	Tags         map[string]string
	LastModified time.Time
}

func newS3Watcher(dir string, interval time.Duration) (Watcher, error) {

	upd := &S3Watcher{
		cache:  make(map[string]*S3Object),
		config: nil,
		stop:   make(chan bool, 1),
		WatcherBase: WatcherBase{
			Events:      make(chan Event, 100),
			Errors:      make(chan error, 100),
			watchDir:    dir,
			pollingTime: interval,
		},
	}
	return upd, nil
}

func (u *S3Watcher) SetConfig(m map[string]string) error {
	j, err := json.Marshal(m)
	if err != nil {
		return err
	}

	config := S3Configuration{}
	if err := json.Unmarshal(j, &config); err != nil {
		return err
	}
	u.config = &config

	client, err := minio.New(u.config.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(u.config.AccessKey, u.config.SecretAccessKey, u.config.SessionToken),
		Secure: bool(u.config.SSLEnabled),
	})
	if err != nil {
		return err
	}
	u.client = client
	return nil
}

func (u *S3Watcher) Start() error {
	if u.config == nil {
		return fmt.Errorf("configuration for S3 needed")
	}

	if ok, err := u.bucketExists(u.config.BucketName); err != nil {
		return fmt.Errorf("error on checking the bucket: %s", err)
	} else if !ok {
		return fmt.Errorf("error on checking the bucket: bucket %s not exists", u.config.BucketName)
	}

	u.ticker = time.NewTicker(u.pollingTime)
	go func() {
		// launch synchronization also the first time
		u.sync()
		for {
			select {
			case <-u.ticker.C:
				u.sync()

			case <-u.stop:
				close(u.Events)
				close(u.Errors)
				return
			}
		}
	}()
	return nil
}

func (u *S3Watcher) Close() {
	u.stop <- true
}

func (u *S3Watcher) getCachedObject(o *S3Object) *S3Object {
	if cachedObject, ok := u.cache[o.Key]; ok {
		return cachedObject
	}
	return nil
}

func (u *S3Object) areTagsChanged(new *S3Object) bool {
	// Check if tags are changed
	if len(u.Tags) != len(new.Tags) {
		return true
	}
	for k, v := range u.Tags {
		if nv, ok := new.Tags[k]; !ok || v != nv {
			return true
		}
	}
	for k, v := range new.Tags {
		if nv, ok := u.Tags[k]; !ok || v != nv {
			return true
		}
	}
	return false
}

func (u *S3Watcher) sync() {
	// allow only one sync at same time
	if !atomic.CompareAndSwapUint32(&u.syncing, 0, 1) {
		return
	}
	defer atomic.StoreUint32(&u.syncing, 0)

	// Avoid to delete all the things if the updater env is not ready...
	if u.isConnected() == false {
		return
	}

	fileList := make(map[string]*S3Object, 0)

	err := u.enumerateFiles(u.config.BucketName, u.watchDir, func(page int64, obj *objectInfo) bool {

		// Get Info from S3 object
		upd, err := u.getInfoFromObject(obj)
		if err != nil {
			return true // continue
		}

		// Store the files to check the deleted one
		fileList[upd.Key] = upd

		// Check if the object is cached by Key
		cached := u.getCachedObject(upd)
		// Object has been cached previously by Key
		if cached != nil {
			// Check if the LastModified has been changed
			if !cached.LastModified.Equal(upd.LastModified) {
				event := Event{
					Key:    upd.Key,
					Type:   FileChanged,
					Object: upd,
				}
				u.Events <- event
			}
			// Check if the tags have been updated
			if cached.areTagsChanged(upd) {
				event := Event{
					Key:    upd.Key,
					Type:   TagsChanged,
					Object: upd,
				}
				u.Events <- event
			}
		} else {
			event := Event{
				Key:    upd.Key,
				Type:   FileCreated,
				Object: upd,
			}
			u.Events <- event
		}
		u.cache[upd.Key] = upd

		return true
	})
	if err != nil {
		u.Errors <- err
		return
	}

	for k, o := range u.cache {
		if _, found := fileList[k]; !found {
			// file not found in the list...deleting it
			delete(u.cache, k)
			event := Event{
				Key:    o.Key,
				Type:   FileDeleted,
				Object: o,
			}
			u.Events <- event
		}
	}
}

func (u *S3Watcher) bucketExists(bucket string) (bool, error) {
	found, err := u.client.BucketExists(context.Background(), bucket)
	if err != nil {
		return false, err
	}
	return found, nil
}

func (u *S3Watcher) getTags(key string, bucket string) (map[string]string, error) {
	t, err := u.client.GetObjectTagging(context.Background(), bucket, key, minio.GetObjectTaggingOptions{})
	if err != nil {
		return nil, err
	}

	tags := make(map[string]string)
	for key, tag := range t.ToMap() {
		tags[key] = tag
	}
	return tags, nil
}

func (u *S3Watcher) isConnected() bool {
	found, err := u.bucketExists(u.config.BucketName)
	if err != nil {
		return false
	}
	return found
}

func (u *S3Watcher) getInfoFromObject(obj *objectInfo) (*S3Object, error) {
	var upd *S3Object

	tags, err := u.getTags(obj.Key, u.config.BucketName)
	if err != nil {
		return nil, fmt.Errorf("getting tags from key '%s': %s", obj.Key, err)
	}
	//log.Debug("s3 watcher: get tags from key '%s': %v", obj.Key, tags)

	upd = &S3Object{
		Key:          obj.Key,
		Etag:         strings.ToLower(strings.Trim(obj.ETag, "\"")), // ETag contains double quotes
		Size:         obj.Size,
		LastModified: obj.LastModified,
		Tags:         make(map[string]string),
	}
	for k, v := range tags {
		upd.Tags[k] = v
	}
	return upd, nil
}

func (u *S3Watcher) enumerateFiles(bucket, prefix string, callback func(page int64, object *objectInfo) bool) error {
	doneCh := make(chan struct{})
	defer close(doneCh)

	// List all objects from a bucket-name with a matching prefix.
	for object := range u.client.ListObjects(context.Background(), bucket, minio.ListObjectsOptions{
		WithVersions: false,
		WithMetadata: false,
		Prefix:       prefix,
		Recursive:    true,
		MaxKeys:      0,
		UseV1:        false,
	}) {
		if object.Err != nil {
			continue
		}

		obj := objectInfo(object)
		if callback(0, &obj) == false {
			break
		}
	}
	return nil
}

func init() {
	supportedServices["s3"] = newS3Watcher
}
