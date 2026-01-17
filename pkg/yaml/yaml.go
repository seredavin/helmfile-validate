package yaml

import (
	"bytes"
	"io"

	v3 "go.yaml.in/yaml/v3"
)

type Encoder interface {
	Encode(any) error
	Close() error
}

func NewEncoder(w io.Writer) Encoder {
	v3Encoder := v3.NewEncoder(w)
	v3Encoder.SetIndent(2)
	return v3Encoder
}

func Marshal(v any) ([]byte, error) {
	var b bytes.Buffer
	yamlEncoder := NewEncoder(&b)
	err := yamlEncoder.Encode(v)
	defer func() {
		_ = yamlEncoder.Close()
	}()
	return b.Bytes(), err
}

func Unmarshal(data []byte, v any) error {
	return v3.Unmarshal(data, v)
}
