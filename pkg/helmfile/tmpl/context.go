package tmpl

import (
	"github.com/seredavin/helmfile-validate/pkg/helmfile/filesystem"
)

type Context struct {
	preRender bool
	basePath  string
	fs        *filesystem.FileSystem
}

// SetBasePath sets the base path for the template
func (c *Context) SetBasePath(path string) {
	c.basePath = path
}

func (c *Context) SetFileSystem(fs *filesystem.FileSystem) {
	c.fs = fs
}
