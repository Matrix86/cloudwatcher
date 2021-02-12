package cloudwatcher

// Event is the struct that contains the info about the changed file
type Event struct {
	Key    string      // Path of file
	Type   Op          // File operation
	Object interface{} // Object pointer
}

// Op defines the event's type
type Op uint32

// event's types
const (
	FileCreated = iota
	FileChanged
	FileDeleted
	TagsChanged
)

// TypeString returns a text version of the event's type
func (e *Event) TypeString() string {
	switch e.Type {
	case FileCreated:
		return "FileCreated"
	case FileChanged:
		return "FileChanged"
	case FileDeleted:
		return "FileDeleted"
	case TagsChanged:
		return "TagsChanged"
	default:
		return "unknown"
	}
}