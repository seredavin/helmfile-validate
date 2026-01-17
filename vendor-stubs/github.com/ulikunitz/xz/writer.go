package xz

import (
	"io"
	"errors"
)

// Writer is a stub writer that does nothing
type Writer struct {
	w io.Writer
}

// NewWriter creates a stub writer
func NewWriter(w io.Writer) (*Writer, error) {
	return nil, errors.New("xz compression is disabled in this build")
}

// Write is a stub implementation
func (w *Writer) Write(p []byte) (n int, err error) {
	return 0, errors.New("xz compression is disabled in this build")
}

// Close is a stub implementation
func (w *Writer) Close() error {
	return nil
}
