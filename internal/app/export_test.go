package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TheSnakeFang/rlviz/internal/harborjob"
)

func TestCanonicalExportHarborJobIsStableAndCountsExternalArtifacts(t *testing.T) {
	source, err := CanonicalExport(context.Background(), filepath.Join("..", "..", "fixtures", "harbor-job"), "")
	if err != nil {
		t.Fatal(err)
	}
	if source.Format != harborjob.Format || !strings.HasPrefix(source.Fingerprint, harborjob.Format+":") {
		t.Fatalf("source = %#v", source)
	}
	if source.PathBackedArtifacts == 0 {
		t.Fatal("Harbor export did not disclose path-backed artifacts")
	}
	if strings.Contains(string(source.Canonical), source.SourcePath) {
		t.Fatal("canonical export leaked absolute source path")
	}
}
