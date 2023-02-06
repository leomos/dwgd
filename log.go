package dwgd

import (
	"encoding/json"
	"log"
	"os"
)

// used for everything that can be considered a "result"
// and should be printed to standard output
var EventsLog = log.New(os.Stdout, "", log.Lmsgprefix)

// used for messages that can give the user a context of
// what the software is doing
var DiagnosticsLog = log.New(os.Stderr, "", log.LstdFlags|log.LUTC)

// used for very detailed messages, should not be used
// in a production environment.
// Disabled by default.
var TraceLog = log.New(&EmptyWriter{}, "", log.LstdFlags|log.LUTC)

type EmptyWriter struct{}

func (e *EmptyWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func Jsonify(data interface{}) string {
	j, err := json.Marshal(data)
	if err != nil {
		return ""
	}
	return string(j)
}
