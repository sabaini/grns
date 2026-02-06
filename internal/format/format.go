package format

import (
	"encoding/json"
	"io"
)

// Formatter abstracts output formatting.
type Formatter interface {
	Write(w io.Writer, payload any) error
}

// JSONFormatter writes JSON output.
type JSONFormatter struct{}

// Write writes JSON payload to a writer.
func (f JSONFormatter) Write(w io.Writer, payload any) error {
	enc := json.NewEncoder(w)
	return enc.Encode(payload)
}
