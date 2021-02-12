package cloudwatcher

import "testing"

func TestEvent_TypeString(t *testing.T) {
	e := Event{
		Type: FileCreated,
	}
	if e.TypeString() != "FileCreated" {
		t.Errorf("FileCreated has been wrongly translated ")
	}

	e.Type = FileChanged
	if e.TypeString() != "FileChanged" {
		t.Errorf("FileChanged has been wrongly translated ")
	}

	e.Type = FileDeleted
	if e.TypeString() != "FileDeleted" {
		t.Errorf("FileDeleted has been wrongly translated ")
	}

	e.Type = TagsChanged
	if e.TypeString() != "TagsChanged" {
		t.Errorf("TagsChanged has been wrongly translated ")
	}

	e.Type = 999
	if e.TypeString() != "unknown" {
		t.Errorf("unknown event has been wrongly translated ")
	}
}
