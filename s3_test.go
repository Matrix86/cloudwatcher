package cloudwatcher

import (
	"context"
	"github.com/Matrix86/cloudwatcher/mocks"
	"github.com/golang/mock/gomock"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/tags"
	"testing"
	"time"
)

func TestS3Watcher_Create(t *testing.T) {
	s, err := New("s3", "/", 10*time.Second)
	if err != nil {
		t.Errorf("error during creation: %s", err)
	}

	if _, ok := s.(*S3Watcher); !ok {
		t.Errorf("the returned time is not a S3Watcher instance")
	}
}

func TestS3Watcher_SetConfig(t *testing.T) {
	s, err := New("s3", "/", 10*time.Second)
	if err != nil {
		t.Errorf("error during creation: %s", err)
	}

	m := map[string]string{
		"this": "is wrong",
	}

	err = s.SetConfig(m)
	if err == nil {
		t.Errorf("it should return an error")
	}

	m2 := map[string]string{
		"bucket_name": "test.storage",
		"endpoint":    "endpoint:9000",
		"access_key":  "minio",
		"secret_key":  "minio123",
		"token":       "token",
		"region":      "region",
		"ssl_enabled": "true",
	}
	err = s.SetConfig(m2)
	if err != nil {
		t.Errorf("%s", err)
	}

	if sw, ok := s.(*S3Watcher); !ok {
		t.Errorf("the returned time is not a S3Watcher instance")
	} else {
		if sw.config.SSLEnabled == false {
			t.Errorf("wrong config set: ssl_enabled should be true")
		} else if sw.config.Endpoint != m2["endpoint"] {
			t.Errorf("wrong config set: endpoint should be %s", m2["endpoint"])
		} else if sw.config.AccessKey != m2["access_key"] {
			t.Errorf("wrong config set: AccessKey should be %s", m2["access_key"])
		} else if sw.config.SecretAccessKey != m2["secret_key"] {
			t.Errorf("wrong config set: SecretAccessKey should be %s", m2["secret_key"])
		} else if sw.config.SessionToken != m2["token"] {
			t.Errorf("wrong config set: SessionToken should be %s", m2["token"])
		} else if sw.config.Region != m2["region"] {
			t.Errorf("wrong config set: Region should be %s", m2["region"])
		}
	}
}

func TestS3Watcher_Start(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mocks.NewMockMinio(ctrl)

	d, err := newS3Watcher("/", 1*time.Second)
	if err != nil {
		t.Errorf("%s", err)
	}

	// Bucket exists
	m.EXPECT().BucketExists(context.Background(), gomock.Eq("test.storage")).Return(true, nil).AnyTimes()
	// Bucket doesn't exist
	m.EXPECT().BucketExists(context.Background(), gomock.Not("test.storage")).Return(false, nil).AnyTimes()

	// Tags
	tag, _ := tags.NewTags(map[string]string{ "key": "value" }, true)

	options := minio.ListObjectsOptions{
		WithVersions: false,
		WithMetadata: false,
		Prefix:       "/",
		Recursive:    true,
		MaxKeys:      0,
		UseV1:        false,
	}

	if sw, ok := d.(*S3Watcher); !ok {
		t.Errorf("wrong type returned")
	} else {
		var event Event

		config := map[string]string{
			"bucket_name": "test.storage.wrong",
			"endpoint":    "endpoint:9000",
			"access_key":  "minio",
			"secret_key":  "minio123",
			"token":       "token",
			"region":      "region",
			"ssl_enabled": "true",
		}

		err = sw.SetConfig(config)
		if err != nil {
			t.Errorf("%s", err)
		}
		// we need to overwrite the client after the call to SetConfig
		sw.client = m

		sw.sync()

		// wrong bucket
		select {
		case <-d.GetErrors():
		default:
			t.Errorf("an error should be returned since the bucket not exist")
		}

		config["bucket_name"] = "test.storage"
		err = sw.SetConfig(config)
		if err != nil {
			t.Errorf("%s", err)
		}

		// we need to overwrite the client after the call to SetConfig
		sw.client = m

		// File created
		m.EXPECT().ListObjects(
			gomock.Any(),
			gomock.Eq("test.storage"),
			gomock.Eq(options),
		).DoAndReturn(
			func(_ context.Context, _ string, _ minio.ListObjectsOptions) <-chan minio.ObjectInfo {
				out := make(chan minio.ObjectInfo)
				go func() {
					out <- minio.ObjectInfo{
						ETag:         "xxx",
						Key:          "filename.test",
						LastModified: time.Now(),
						Size:         100,
					}
					close(out)
				}()
				return out
			},
		)
		m.EXPECT().GetObjectTagging(
			gomock.Any(),
			gomock.Eq("test.storage"),
			gomock.Eq("filename.test"),
			minio.GetObjectTaggingOptions{},
		).Return(
			tag,
			nil,
		)

		sw.sync()
		select {
		case event = <-d.GetEvents():
		case <-time.After(1 * time.Second):
			t.Errorf("FileCreated event not received")
		}

		if event.Key != "filename.test" {
			t.Errorf("wrong key event received: %s", event.Key)
		} else if event.Type != FileCreated {
			t.Errorf("wrong type event received: %s", event.TypeString())
		}

		// File changed : Lastmodified changes
		lastmod := time.Now()
		m.EXPECT().ListObjects(
			gomock.Any(),
			gomock.Eq("test.storage"),
			gomock.Eq(options),
		).DoAndReturn(
			func(_ context.Context, _ string, _ minio.ListObjectsOptions) <-chan minio.ObjectInfo {
				out := make(chan minio.ObjectInfo)
				go func() {
					out <- minio.ObjectInfo{
						ETag:         "xxx",
						Key:          "filename.test",
						LastModified: lastmod,
						Size:         100,
					}
					close(out)
				}()
				return out
			},
		)
		m.EXPECT().GetObjectTagging(
			gomock.Any(),
			gomock.Eq("test.storage"),
			gomock.Eq("filename.test"),
			minio.GetObjectTaggingOptions{},
		).Return(
			tag,
			nil,
		)

		sw.sync()
		select {
		case event = <-d.GetEvents():
		case <-time.After(1 * time.Second):
			t.Errorf("FileCreated event not received")
		}

		if event.Key != "filename.test" {
			t.Errorf("wrong key event received: %s", event.Key)
		} else if event.Type != FileChanged {
			t.Errorf("wrong type event received: %s", event.TypeString())
		}

		// File changed : size changes
		m.EXPECT().ListObjects(
			gomock.Any(),
			gomock.Eq("test.storage"),
			gomock.Eq(options),
		).DoAndReturn(
			func(_ context.Context, _ string, _ minio.ListObjectsOptions) <-chan minio.ObjectInfo {
				out := make(chan minio.ObjectInfo)
				go func() {
					out <- minio.ObjectInfo{
						ETag:         "xxx",
						Key:          "filename.test",
						LastModified: lastmod,
						Size:         150,
					}
					close(out)
				}()
				return out
			},
		)
		m.EXPECT().GetObjectTagging(
			gomock.Any(),
			gomock.Eq("test.storage"),
			gomock.Eq("filename.test"),
			minio.GetObjectTaggingOptions{},
		).Return(
			tag,
			nil,
		)
		sw.sync()
		select {
		case event = <-d.GetEvents():
		case <-time.After(1 * time.Second):
			t.Errorf("FileCreated event not received")
		}

		if event.Key != "filename.test" {
			t.Errorf("wrong key event received: %s", event.Key)
		} else if event.Type != FileChanged {
			t.Errorf("wrong type event received: %s", event.TypeString())
		}

		// Tags changed
		m.EXPECT().ListObjects(
			gomock.Any(),
			gomock.Eq("test.storage"),
			gomock.Eq(options),
		).DoAndReturn(
			func(_ context.Context, _ string, _ minio.ListObjectsOptions) <-chan minio.ObjectInfo {
				out := make(chan minio.ObjectInfo)
				go func() {
					out <- minio.ObjectInfo{
						ETag:         "xxx",
						Key:          "filename.test",
						LastModified: lastmod,
						Size:         150,
					}
					close(out)
				}()
				return out
			},
		)
		tag, _ := tags.NewTags(map[string]string{ "key": "newvalue" }, true)
		m.EXPECT().GetObjectTagging(
			gomock.Any(),
			gomock.Eq("test.storage"),
			gomock.Eq("filename.test"),
			minio.GetObjectTaggingOptions{},
		).Return(
			tag,
			nil,
		)
		sw.sync()
		select {
		case event = <-d.GetEvents():
		case <-time.After(1 * time.Second):
			t.Errorf("FileCreated event not received")
		}

		if event.Key != "filename.test" {
			t.Errorf("wrong key event received: %s", event.Key)
		} else if event.Type != TagsChanged {
			t.Errorf("wrong type event received: %s", event.TypeString())
		}

		// File deleted
		m.EXPECT().ListObjects(
			gomock.Any(),
			gomock.Eq("test.storage"),
			gomock.Eq(options),
		).DoAndReturn(
			func(_ context.Context, _ string, _ minio.ListObjectsOptions) <-chan minio.ObjectInfo {
				out := make(chan minio.ObjectInfo)
				go func() {
					close(out)
				}()
				return out
			},
		)

		sw.sync()
		select {
		case event = <-d.GetEvents():
		case <-time.After(1 * time.Second):
			t.Errorf("FileDeleted event not received")
		}

		if event.Key != "filename.test" {
			t.Errorf("wrong key event received: %s", event.Key)
		} else if event.Type != FileDeleted {
			t.Errorf("wrong type event received: %s", event.TypeString())
		}
	}

}
