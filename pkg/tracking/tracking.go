package tracking

import (
	"sync"

	helmfileFs "github.com/seredavin/helmfile-validate/pkg/helmfile/filesystem"
)

// TrackingFileSystem wraps a helmfile FileSystem and tracks all files that are read
type TrackingFileSystem struct {
	*helmfileFs.FileSystem
	mu           sync.Mutex
	readFiles    map[string]bool
	globFiles    map[string]bool
	allFiles     map[string]bool
}

// NewTrackingFileSystem creates a new TrackingFileSystem that wraps the provided FileSystem
func NewTrackingFileSystem(base *helmfileFs.FileSystem) *TrackingFileSystem {
	tfs := &TrackingFileSystem{
		FileSystem: base,
		readFiles:  make(map[string]bool),
		globFiles:  make(map[string]bool),
		allFiles:   make(map[string]bool),
	}

	// Wrap ReadFile to track reads
	originalReadFile := base.ReadFile
	tfs.ReadFile = func(path string) ([]byte, error) {
		tfs.mu.Lock()
		tfs.readFiles[path] = true
		tfs.allFiles[path] = true
		tfs.mu.Unlock()
		return originalReadFile(path)
	}

	// Wrap Glob to track globbed files
	originalGlob := base.Glob
	tfs.Glob = func(pattern string) ([]string, error) {
		matches, err := originalGlob(pattern)
		if err == nil {
			tfs.mu.Lock()
			for _, match := range matches {
				tfs.globFiles[match] = true
				tfs.allFiles[match] = true
			}
			tfs.mu.Unlock()
		}
		return matches, err
	}

	return tfs
}

// GetReadFiles returns a copy of all files that were read
func (tfs *TrackingFileSystem) GetReadFiles() []string {
	tfs.mu.Lock()
	defer tfs.mu.Unlock()

	files := make([]string, 0, len(tfs.readFiles))
	for f := range tfs.readFiles {
		files = append(files, f)
	}
	return files
}

// GetGlobFiles returns a copy of all files that were matched by glob patterns
func (tfs *TrackingFileSystem) GetGlobFiles() []string {
	tfs.mu.Lock()
	defer tfs.mu.Unlock()

	files := make([]string, 0, len(tfs.globFiles))
	for f := range tfs.globFiles {
		files = append(files, f)
	}
	return files
}

// GetAllFiles returns a copy of all files (both read and globbed)
func (tfs *TrackingFileSystem) GetAllFiles() []string {
	tfs.mu.Lock()
	defer tfs.mu.Unlock()

	files := make([]string, 0, len(tfs.allFiles))
	for f := range tfs.allFiles {
		files = append(files, f)
	}
	return files
}
