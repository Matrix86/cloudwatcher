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
