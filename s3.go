package cloudwatcher

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type objectInfo = minio.ObjectInfo
type s3Configuration struct {
	BucketName      string `json:"bucket_name"`
	Endpoint        string `json:"endpoint"`
	AccessKey       string `json:"access_key"`
	SecretAccessKey string `json:"secret_key"`
	SessionToken    string `json:"token"`
	Region          string `json:"region"`
	SSLEnabled      Bool   `json:"ssl_enabled"`
}

// S3Watcher is the specialized watcher for Amazon S3 service
type S3Watcher struct {
	WatcherBase

	syncing uint32

	ticker *time.Ticker
	stop   chan bool
	config *s3Configuration
	client IMinio
	cache  map[string]*S3Object
}

// S3Object is the object that contains the info of the file
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

// SetConfig is used to configure the S3Watcher
func (u *S3Watcher) SetConfig(m map[string]string) error {
	j, err := json.Marshal(m)
	if err != nil {
		return err
	}

	config := s3Configuration{}
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

// Start launches the polling process
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
		u.sync(true)
		for {
			select {
			case <-u.ticker.C:
				u.sync(false)

			case <-u.stop:
				close(u.Events)
				close(u.Errors)
				return
			}
		}
	}()
	return nil
}

// Close stop the polling process
func (u *S3Watcher) Close() {
	if u.stop != nil {
		u.stop <- true
	}
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

func (u *S3Watcher) sync(firstSync bool) {
	// allow only one sync at same time
	if !atomic.CompareAndSwapUint32(&u.syncing, 0, 1) {
		return
	}
	defer atomic.StoreUint32(&u.syncing, 0)

	if found, err := u.bucketExists(u.config.BucketName); found == false || err != nil {
		u.Errors <- fmt.Errorf("bucket '%s' not found: %s", u.config.BucketName, err)
		return
	}

	fileList := make(map[string]*S3Object, 0)

	err := u.enumerateFiles(u.config.BucketName, u.watchDir, func(page int64, obj *objectInfo) bool {
		// Get Info from S3 object
		upd, err := u.getInfoFromObject(obj)
		if err != nil {
			return true // continue
		}

		if !firstSync {
			// Store the files to check the deleted one
			fileList[upd.Key] = upd

			// Check if the object is cached by Key
			cached := u.getCachedObject(upd)
			// Object has been cached previously by Key
			if cached != nil {
				// Check if the LastModified has been changed
				if !cached.LastModified.Equal(upd.LastModified) || cached.Size != upd.Size {
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
	options := minio.ListObjectsOptions{
		WithVersions: false,
		WithMetadata: false,
		Prefix:       prefix,
		Recursive:    true,
		MaxKeys:      0,
		UseV1:        false,
	}

	// List all objects from a bucket-name with a matching prefix.
	for object := range u.client.ListObjects(context.Background(), bucket, options) {
		if object.Err != nil {
			continue
		}

		if callback(0, &object) == false {
			break
		}
	}
	return nil
}

func init() {
	supportedServices["s3"] = newS3Watcher
}
