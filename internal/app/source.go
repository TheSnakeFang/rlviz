package app

import (
	"fmt"
	"os"
	"path/filepath"
)

// ValidateSource resolves a source path and verifies that it can be opened as a
// regular file. RLViz never opens source trajectories for writing.
func ValidateSource(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("source path is required")
	}

	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve source path: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve source path %q: %w", absolute, err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("inspect source path %q: %w", resolved, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("source path %q is not a regular file", resolved)
	}

	file, err := os.Open(resolved)
	if err != nil {
		return "", fmt.Errorf("open source path %q read-only: %w", resolved, err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close source path %q: %w", resolved, err)
	}
	return resolved, nil
}
