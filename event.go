package cloudwatcher

type Event struct {
	Key    string      // Path of file
	Type   Op          // File operation
	Object interface{} // Object pointer
}

type Op uint32

const (
	FileCreated = iota
	FileChanged
	FileDeleted
	TagsChanged
)

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