package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeBundleArgumentsAllowsSourceFirst(t *testing.T) {
	got := normalizeBundleArguments([]string{"trace.ndjson", "--out", "reviewed.rlviz", "--title=Reviewed", "--license", "MIT", "--reviewed", "--redaction-confirmed", "--json"})
	want := []string{"--out", "reviewed.rlviz", "--title=Reviewed", "--license", "MIT", "--reviewed", "--redaction-confirmed", "--json", "trace.ndjson"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeBundleArguments() = %#v, want %#v", got, want)
	}
}

func TestWriteNewBundleIsCreateOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reviewed.rlviz")
	written, err := writeNewBundle(path, []byte("first"))
	if err != nil || written != path {
		t.Fatalf("writeNewBundle() = %q, %v", written, err)
	}
	if _, err := writeNewBundle(path, []byte("second")); err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("overwrite error = %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "first" {
		t.Fatalf("content = %q, %v", content, err)
	}
}

func TestWriteNewBundleRejectsMissingParent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "reviewed.rlviz")
	if _, err := writeNewBundle(path, []byte("data")); err == nil {
		t.Fatal("writeNewBundle accepted a missing parent")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("partial file exists: %v", err)
	}
}
