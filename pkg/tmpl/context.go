package tmpl

import (
	"github.com/kirillseredavin/helmfile-validate/pkg/filesystem"
)

type Context struct {
	preRender bool
	basePath  string
	fs        *filesystem.FileSystem
}

func (c *Context) SetBasePath(path string) {
	c.basePath = path
}
