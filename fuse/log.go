package fuse

// Logger allows the use of custom loggers in the FUSE server. The log.Logger
// in the standard library implements this interface.
type Logger interface {
	Println(v ...interface{})
	Printf(format string, v ...interface{})
}
