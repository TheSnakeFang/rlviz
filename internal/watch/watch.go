// Package watch provides a small polling watcher for local rollout sources.
// Polling is intentionally used instead of platform-specific APIs so one
// release binary behaves consistently on macOS, Linux, and Windows.
package watch

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"
)

type Signature struct {
	Size       int64
	ModifiedAt time.Time
}

type Change struct {
	Path     string
	Previous Signature
	Current  Signature
}

type Callback func(context.Context, Change) error

type Watcher struct {
	Interval time.Duration

	mu      sync.Mutex
	entries map[string]*entry
}

type entry struct {
	path      string
	signature Signature
	callback  Callback
	cancel    context.CancelFunc
}

func New(interval time.Duration) *Watcher {
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	return &Watcher{Interval: interval, entries: make(map[string]*entry)}
}

// Add starts watching path under a caller-chosen stable key. Replacing a key
// cancels its previous watch. The callback runs serially for that key.
func (watcher *Watcher) Add(parent context.Context, key, path string, callback Callback) error {
	if key == "" || path == "" || callback == nil {
		return fmt.Errorf("watch key, path, and callback are required")
	}
	signature, err := stat(path)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(parent)
	item := &entry{path: path, signature: signature, callback: callback, cancel: cancel}

	watcher.mu.Lock()
	if previous := watcher.entries[key]; previous != nil {
		previous.cancel()
	}
	watcher.entries[key] = item
	watcher.mu.Unlock()
	go watcher.run(ctx, key, item)
	return nil
}

func (watcher *Watcher) Remove(key string) {
	watcher.mu.Lock()
	item := watcher.entries[key]
	delete(watcher.entries, key)
	watcher.mu.Unlock()
	if item != nil {
		item.cancel()
	}
}

func (watcher *Watcher) Close() {
	watcher.mu.Lock()
	entries := watcher.entries
	watcher.entries = make(map[string]*entry)
	watcher.mu.Unlock()
	for _, item := range entries {
		item.cancel()
	}
}

func (watcher *Watcher) run(ctx context.Context, key string, item *entry) {
	ticker := time.NewTicker(watcher.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			current, err := stat(item.path)
			if err != nil || current == item.signature {
				continue
			}
			change := Change{Path: item.path, Previous: item.signature, Current: current}
			if err := item.callback(ctx, change); err != nil {
				continue
			}
			item.signature = current
			watcher.mu.Lock()
			active := watcher.entries[key]
			watcher.mu.Unlock()
			if active != item {
				return
			}
		}
	}
}

func stat(path string) (Signature, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Signature{}, err
	}
	if !info.Mode().IsRegular() {
		return Signature{}, fmt.Errorf("watched source %s is not a regular file", path)
	}
	return Signature{Size: info.Size(), ModifiedAt: info.ModTime()}, nil
}
