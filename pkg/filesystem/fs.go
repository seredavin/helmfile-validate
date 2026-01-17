package filesystem

import (
	"io/fs"
	"os"
	"path/filepath"
)

type FileSystem struct {
	ReadFile          func(string) ([]byte, error)
	ReadDir           func(string) ([]fs.DirEntry, error)
	Glob              func(string) ([]string, error)
	FileExistsAt      func(string) bool
	DirectoryExistsAt func(string) bool
	Stat              func(string) (os.FileInfo, error)
	Getwd             func() (string, error)
	EvalSymlinks      func(string) (string, error)
}

func DefaultFileSystem() *FileSystem {
	dfs := FileSystem{
		ReadDir:      os.ReadDir,
		Glob:         filepath.Glob,
		Getwd:        os.Getwd,
		EvalSymlinks: filepath.EvalSymlinks,
	}

	dfs.Stat = dfs.stat
	dfs.ReadFile = dfs.readFile
	dfs.FileExistsAt = dfs.fileExistsAtDefault
	dfs.DirectoryExistsAt = dfs.directoryExistsDefault
	return &dfs
}

func (filesystem *FileSystem) stat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}

func (filesystem *FileSystem) readFile(name string) ([]byte, error) {
	return os.ReadFile(name)
}

func (filesystem *FileSystem) fileExistsAtDefault(path string) bool {
	fileInfo, err := filesystem.Stat(path)
	return err == nil && fileInfo.Mode().IsRegular()
}

func (filesystem *FileSystem) directoryExistsDefault(path string) bool {
	fileInfo, err := filesystem.Stat(path)
	return err == nil && fileInfo.Mode().IsDir()
}
