package cloudwatcher

import (
	"github.com/Matrix86/cloudwatcher/mocks"
	"github.com/dropbox/dropbox-sdk-go-unofficial/dropbox/files"
	"github.com/golang/mock/gomock"
	"testing"
	"time"
)

func TestDropboxWatcher_Create(t *testing.T) {
	d, err := New("dropbox", "/", 10*time.Second)
	if err != nil {
		t.Errorf("error during creation: %s", err)
	}

	if _, ok := d.(*DropboxWatcher); !ok {
		t.Errorf("the returned time is not a DropboxWatcher instance")
	}
}

func TestDropboxWatcher_SetConfig(t *testing.T) {
	d, err := newDropboxWatcher("/", 1*time.Second)
	if err != nil {
		t.Errorf("%s", err)
	}

	config := map[string]string {
		"debug": "true",
		//"token": "wrong",
	}
	if err := d.SetConfig(config); err == nil {
		t.Errorf("it has to return the error: unspecified token")
	}

	config["token"] = "bad"
	if err := d.SetConfig(config); err == nil {
		t.Errorf("it has to return the error on unmarshalling")
	}

	config["token"] = "{\"access_token\": \"xxx\"}"
	if err := d.SetConfig(config); err != nil {
		t.Errorf("error returned : %s", err)
	}

	if dw, ok := d.(*DropboxWatcher); ok {
		if dw.config.token.AccessToken != "xxx" {
			t.Errorf("access_token is wrong")
		} else if dw.config.Debug == false {
			t.Errorf("debug is wrong")
		}
	} else {
		t.Errorf("it should be an object of type DropboxWatcher")
	}
}

func TestDropboxWatcher_Start(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mocks.NewMockDropbox(ctrl)

	d, err := newDropboxWatcher("/", 1*time.Second)
	if err != nil {
		t.Errorf("%s", err)
	}

	arg := files.NewListFolderArg("/")
	arg.Recursive = true

	if dw, ok := d.(*DropboxWatcher); !ok {
		t.Errorf("wrong type returned")
	} else {
		conf := map[string]string{
			"debug": "true",
			"token": "{\"access_token\": \"asd\"}",
		}

		err = d.SetConfig(conf)
		if err != nil {
			t.Errorf("error returned")
		}

		dw.client = m

		var event Event

		modtime := time.Now()
		// File created
		m.EXPECT().ListFolder(arg).Return(
			&files.ListFolderResult{
				Entries: []files.IsMetadata{files.NewFileMetadata("name", "Id", modtime, modtime, "1", 120)},
				Cursor:  "",
				HasMore: false,
			}, nil)

		dw.sync()
		event = <-dw.GetEvents()

		if event.Key != "name" {
			t.Errorf("wrong key event received: %s", event.Key)
		} else if event.Type != FileCreated  {
			t.Errorf("wrong type event received: %s", event.TypeString())
		}

		// no event should be sent here! there have been no changes
		m.EXPECT().ListFolder(arg).Return(
			&files.ListFolderResult{
				Entries: []files.IsMetadata{files.NewFileMetadata("name", "Id", modtime, modtime, "1", 120)},
				Cursor:  "",
				HasMore: false,
			}, nil)
		dw.sync()

		// File modified : size changed
		m.EXPECT().ListFolder(arg).Return(
			&files.ListFolderResult{
				Entries: []files.IsMetadata{files.NewFileMetadata("name", "Id", modtime, modtime, "1", 150)},
				Cursor:  "",
				HasMore: false,
			}, nil)
		dw.sync()
		event = <-dw.GetEvents()

		if event.Key != "name" {
			t.Errorf("wrong key event received: %s", event.Key)
		} else if event.Type != FileChanged  {
			t.Errorf("wrong type event received: %s", event.TypeString())
		}

		// File modified: modtime changed
		m.EXPECT().ListFolder(arg).Return(
			&files.ListFolderResult{
				Entries: []files.IsMetadata{files.NewFileMetadata("name", "Id", time.Now(), time.Now(), "1", 150)},
				Cursor:  "",
				HasMore: false,
			}, nil)
		dw.sync()
		event = <-dw.GetEvents()

		if event.Key != "name" {
			t.Errorf("wrong key event received: %s", event.Key)
		} else if event.Type != FileChanged  {
			t.Errorf("wrong type event received: %s", event.TypeString())
		}

		// File deleted
		m.EXPECT().ListFolder(arg).Return(
			&files.ListFolderResult{
				Entries: []files.IsMetadata{},
				Cursor:  "",
				HasMore: false,
			}, nil)
		dw.sync()
		event = <-dw.GetEvents()

		if event.Key != "name" {
			t.Errorf("wrong key event received: %s", event.Key)
		} else if event.Type != FileDeleted {
			t.Errorf("wrong type event received: %s", event.TypeString())
		}
	}
}
