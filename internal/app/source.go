package app

import (
	"fmt"
	"os"
	"path/filepath"
)

// ResolveSource resolves a source path and verifies that it is a readable
// regular file or directory. RLViz never opens source trajectories for writing.
func ResolveSource(path string) (string, os.FileInfo, error) {
	if path == "" {
		return "", nil, fmt.Errorf("source path is required")
	}

	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", nil, fmt.Errorf("resolve source path: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", nil, fmt.Errorf("resolve source path %q: %w", absolute, err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", nil, fmt.Errorf("inspect source path %q: %w", resolved, err)
	}
	if !info.Mode().IsRegular() && !info.IsDir() {
		return "", nil, fmt.Errorf("source path %q is not a regular file or directory", resolved)
	}

	file, err := os.Open(resolved)
	if err != nil {
		return "", nil, fmt.Errorf("open source path %q read-only: %w", resolved, err)
	}
	if err := file.Close(); err != nil {
		return "", nil, fmt.Errorf("close source path %q: %w", resolved, err)
	}
	return resolved, info, nil
}

// ValidateSource preserves the regular-file boundary used by file-only APIs.
func ValidateSource(path string) (string, error) {
	resolved, info, err := ResolveSource(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("source path %q is not a regular file", resolved)
	}
	return resolved, nil
}
