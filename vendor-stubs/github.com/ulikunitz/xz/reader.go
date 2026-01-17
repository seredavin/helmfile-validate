package xz

import (
	"io"
	"errors"
)

// Reader is a stub reader that does nothing
type Reader struct {
	r io.Reader
}

// NewReader creates a stub reader
func NewReader(r io.Reader) (*Reader, error) {
	return nil, errors.New("xz decompression is disabled in this build")
}

// Read is a stub implementation
func (r *Reader) Read(p []byte) (n int, err error) {
	return 0, errors.New("xz decompression is disabled in this build")
}

// Close is a stub implementation
func (r *Reader) Close() error {
	return nil
}
