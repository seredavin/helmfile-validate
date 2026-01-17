package tmpl

import (
	"strings"

	"github.com/kirillseredavin/helmfile-validate/pkg/filesystem"
)

type FileRenderer struct {
	fs      *filesystem.FileSystem
	Context *Context
	Data    any
}

func NewFileRenderer(fs *filesystem.FileSystem, basePath string, data any) *FileRenderer {
	return &FileRenderer{
		fs: fs,
		Context: &Context{
			basePath: basePath,
			fs:       fs,
		},
		Data: data,
	}
}

func (r *FileRenderer) RenderToBytes(path string) ([]byte, error) {
	data, err := r.fs.ReadFile(path)
	if err != nil {
		return nil, err
	}

	if !strings.HasSuffix(path, ".gotmpl") {
		return data, nil
	}

	rendered, err := r.Context.RenderTemplateToBuffer(string(data), r.Data)
	if err != nil {
		return nil, err
	}

	return rendered.Bytes(), nil
}

func (r *FileRenderer) RenderTemplateContentToBuffer(content []byte) ([]byte, error) {
	rendered, err := r.Context.RenderTemplateToBuffer(string(content), r.Data)
	if err != nil {
		return nil, err
	}

	return rendered.Bytes(), nil
}
