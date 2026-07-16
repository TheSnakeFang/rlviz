package watch

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestWatcherReportsAppendAndStops(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	if err := os.WriteFile(path, []byte("one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	watcher := New(10 * time.Millisecond)
	defer watcher.Close()
	changes := make(chan Change, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := watcher.Add(ctx, "source", path, func(_ context.Context, change Change) error {
		changes <- change
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("two\n"); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case change := <-changes:
		if change.Previous.Size != 4 || change.Current.Size != 8 {
			t.Fatalf("change = %#v", change)
		}
	case <-time.After(time.Second):
		t.Fatal("watcher did not report append")
	}
	watcher.Remove("source")
}

func TestWatcherRetriesFailedCallback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	if err := os.WriteFile(path, []byte("one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	watcher := New(5 * time.Millisecond)
	defer watcher.Close()
	var calls atomic.Int32
	done := make(chan struct{}, 1)
	if err := watcher.Add(context.Background(), "source", path, func(context.Context, Change) error {
		if calls.Add(1) == 1 {
			return context.DeadlineExceeded
		}
		done <- struct{}{}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
		if calls.Load() < 2 {
			t.Fatalf("calls = %d", calls.Load())
		}
	case <-time.After(time.Second):
		t.Fatal("watcher did not retry")
	}
}
