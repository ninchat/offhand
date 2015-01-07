package offhand

import (
	"log"
)

// Logger is used to log Binder, Endpoint and Link errors.  Compatible with
// log.Logger.
type Logger interface {
	// Printf logs a single error message.
	Printf(format string, v ...interface{})
}

// NullLogger discards messages.
var NullLogger Logger = nullLogger{}

type nullLogger struct{}

func (nullLogger) Printf(string, ...interface{}) {
}

// defaultLogger
type defaultLogger struct{}

func (defaultLogger) Printf(format string, v ...interface{}) {
	log.Printf(format, v...)
}
