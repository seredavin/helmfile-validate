package tracking

import (
	"io/fs"
	"path/filepath"
	"sync"

	"github.com/seredavin/helmfile-validate/pkg/helmfile/filesystem"
)

// TrackingFileSystem wraps a FileSystem and tracks all files that are read or globbed
type TrackingFileSystem struct {
	*filesystem.FileSystem
	mu       sync.Mutex
	allFiles map[string]bool
}

// NewTrackingFileSystem creates a new TrackingFileSystem that wraps the given FileSystem
func NewTrackingFileSystem(baseFs *filesystem.FileSystem) *TrackingFileSystem {
	tfs := &TrackingFileSystem{
		FileSystem: baseFs,
		allFiles:   make(map[string]bool),
	}

	// Wrap ReadFile to track files
	originalReadFile := baseFs.ReadFile
	tfs.ReadFile = func(path string) ([]byte, error) {
		tfs.trackFile(path)
		return originalReadFile(path)
	}

	// Wrap Glob to track files
	originalGlob := baseFs.Glob
	tfs.Glob = func(pattern string) ([]string, error) {
		matches, err := originalGlob(pattern)
		if err == nil {
			for _, match := range matches {
				tfs.trackFile(match)
			}
		}
		return matches, err
	}

	// Wrap ReadDir to track directories (and potentially files within)
	originalReadDir := baseFs.ReadDir
	tfs.ReadDir = func(path string) ([]fs.DirEntry, error) {
		entries, err := originalReadDir(path)
		if err == nil {
			for _, entry := range entries {
				fullPath := filepath.Join(path, entry.Name())
				tfs.trackFile(fullPath)
			}
		}
		return entries, err
	}

	return tfs
}

// trackFile adds a file to the tracking set
func (tfs *TrackingFileSystem) trackFile(path string) {
	tfs.mu.Lock()
	defer tfs.mu.Unlock()
	// Normalize path
	absPath, err := filepath.Abs(path)
	if err == nil {
		tfs.allFiles[absPath] = true
	} else {
		// If abs fails, use the original path
		tfs.allFiles[path] = true
	}
}

// GetAllFiles returns all files that have been tracked
func (tfs *TrackingFileSystem) GetAllFiles() []string {
	tfs.mu.Lock()
	defer tfs.mu.Unlock()
	files := make([]string, 0, len(tfs.allFiles))
	for file := range tfs.allFiles {
		files = append(files, file)
	}
	return files
}
