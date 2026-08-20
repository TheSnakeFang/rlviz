package app

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/TheSnakeFang/rlviz/internal/bundle"
	rolloutindex "github.com/TheSnakeFang/rlviz/internal/index"
	"github.com/TheSnakeFang/rlviz/internal/plugins"
)

func TestIndexSourcePortableBundle(t *testing.T) {
	canonical, err := os.ReadFile(filepath.Join("..", "..", "fixtures", "canonical", "linear.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	data, _, err := bundle.Create(canonical, bundle.CreateOptions{Title: "Reviewed", License: "MIT", CreatedAt: time.Unix(1, 0), SourceName: "linear.ndjson", SourceFormat: "canonical-ndjson", SourceFingerprint: "sha256:test"})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "reviewed.rlviz")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := rolloutindex.Open(filepath.Join(t.TempDir(), "index.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	indexed, err := IndexSource(context.Background(), store, path, "")
	if err != nil {
		t.Fatal(err)
	}
	trajectories, err := store.Trajectories(context.Background(), indexed.Info.ID)
	if err != nil || len(trajectories) != 1 {
		t.Fatalf("trajectories = %#v, err = %v", trajectories, err)
	}
}

func TestIndexSourceCanonicalCachesWholeGroup(t *testing.T) {
	store, err := rolloutindex.Open(filepath.Join(t.TempDir(), "index.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	path := filepath.Join("..", "..", "fixtures", "canonical", "group.ndjson")
	first, err := IndexSource(context.Background(), store, path, "")
	if err != nil {
		t.Fatal(err)
	}
	if !first.Refreshed || first.Info.ID == "" {
		t.Fatalf("first index = %#v", first)
	}
	trajectories, err := store.Trajectories(context.Background(), first.Info.ID)
	if err != nil || len(trajectories) != 2 {
		t.Fatalf("trajectories=%#v err=%v", trajectories, err)
	}
	second, err := IndexSource(context.Background(), store, path, "")
	if err != nil {
		t.Fatal(err)
	}
	if second.Refreshed || second.Info.ID != first.Info.ID {
		t.Fatalf("cached index = %#v", second)
	}
	page, err := store.Events(context.Background(), rolloutindex.EventQuery{SourceID: first.Info.ID, TrajectoryID: "traj-success"})
	if err != nil || len(page.Events) != 2 || page.Events[0].Value.Source == nil || page.Events[0].Value.Source.Path == "" {
		t.Fatalf("events=%#v err=%v", page, err)
	}
}

func TestIndexSourceHarborATIFBuiltIn(t *testing.T) {
	store, err := rolloutindex.Open(filepath.Join(t.TempDir(), "index.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	path := filepath.Join("..", "..", "examples", "traces", "harbor-atif.json")
	indexed, err := IndexSource(context.Background(), store, path, "")
	if err != nil {
		t.Fatal(err)
	}
	if !indexed.Refreshed || indexed.Info.ID == "" {
		t.Fatalf("indexed = %#v", indexed)
	}
	trajectories, err := store.Trajectories(context.Background(), indexed.Info.ID)
	if err != nil || len(trajectories) != 2 {
		t.Fatalf("trajectories=%#v err=%v", trajectories, err)
	}
	page, err := store.Events(context.Background(), rolloutindex.EventQuery{SourceID: indexed.Info.ID, TrajectoryID: trajectories[0].Value.ID})
	if err != nil || page.Total != 5 {
		t.Fatalf("page=%#v err=%v", page, err)
	}
}

func TestIndexSourceHarborJobDirectoryCachesAndRefreshes(t *testing.T) {
	store, err := rolloutindex.Open(filepath.Join(t.TempDir(), "index.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	root := t.TempDir()
	writeAppJSON(t, filepath.Join(root, "config.json"), map[string]any{"job_name": "directory-job"})
	writeAppJSON(t, filepath.Join(root, "result.json"), map[string]any{"id": "job-directory", "n_total_trials": 1})
	trial := filepath.Join(root, "trial-1")
	resultPath := filepath.Join(trial, "result.json")
	result := map[string]any{
		"id": "trial-1", "task_name": "task-1", "finished_at": "2026-08-20T10:00:01Z",
		"agent_info":      map[string]any{"name": "codex", "model_info": map[string]any{"name": "gpt-test"}},
		"verifier_result": map[string]any{"rewards": map[string]any{"reward": 1}},
	}
	writeAppJSON(t, resultPath, result)
	writeAppJSON(t, filepath.Join(trial, "agent", "trajectory.json"), map[string]any{
		"schema_version": "ATIF-v1.7", "session_id": "session-1", "agent": map[string]any{"name": "codex"},
		"steps": []any{map[string]any{"step_id": 1, "source": "user", "message": "Solve it."}},
	})

	first, err := IndexSource(context.Background(), store, root, "")
	if err != nil {
		t.Fatal(err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Refreshed || first.Info.Source.Path != resolvedRoot {
		t.Fatalf("first index = %#v", first)
	}
	trajectories, err := store.Trajectories(context.Background(), first.Info.ID)
	if err != nil || len(trajectories) != 1 {
		t.Fatalf("trajectories=%#v err=%v", trajectories, err)
	}
	signals, err := store.Signals(context.Background(), first.Info.ID, trajectories[0].Value.ID)
	if err != nil || len(signals) != 1 || signals[0].Value.Name != "reward" {
		t.Fatalf("signals=%#v err=%v", signals, err)
	}
	second, err := IndexSource(context.Background(), store, root, "")
	if err != nil {
		t.Fatal(err)
	}
	if second.Refreshed {
		t.Fatal("unchanged Harbor job was reindexed")
	}

	result["updated_at"] = "2026-08-20T10:01:00Z"
	writeAppJSON(t, resultPath, result)
	third, err := IndexSource(context.Background(), store, root, "")
	if err != nil {
		t.Fatal(err)
	}
	if !third.Refreshed || third.Info.ID != first.Info.ID {
		t.Fatalf("refreshed index = %#v", third)
	}
}

func TestIndexSourceHarborJobFixture(t *testing.T) {
	store, err := rolloutindex.Open(filepath.Join(t.TempDir(), "index.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	indexed, err := IndexSource(context.Background(), store, filepath.Join("..", "..", "fixtures", "harbor-job"), "")
	if err != nil {
		t.Fatal(err)
	}
	trajectories, err := store.Trajectories(context.Background(), indexed.Info.ID)
	if err != nil || len(trajectories) != 1 {
		t.Fatalf("trajectories=%#v err=%v", trajectories, err)
	}
	trajectoryID := trajectories[0].Value.ID
	page, err := store.Events(context.Background(), rolloutindex.EventQuery{SourceID: indexed.Info.ID, TrajectoryID: trajectoryID})
	if err != nil || page.Total != 6 {
		t.Fatalf("events=%#v err=%v", page, err)
	}
	artifacts, err := store.Artifacts(context.Background(), indexed.Info.ID, trajectoryID)
	if err != nil || len(artifacts) != 6 {
		t.Fatalf("artifacts=%#v err=%v", artifacts, err)
	}
}

func writeAppJSON(t *testing.T, path string, value any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestIndexSourceDocumentJSONBuiltIns(t *testing.T) {
	for _, name := range []string{"letta-trajectory-v1.json", "inspect-ai-eval.json", "verifiers-generate.json"} {
		t.Run(name, func(t *testing.T) {
			store, err := rolloutindex.Open(filepath.Join(t.TempDir(), "index.sqlite"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			indexed, err := IndexSource(context.Background(), store, filepath.Join("..", "..", "examples", "traces", name), "")
			if err != nil {
				t.Fatal(err)
			}
			trajectories, err := store.Trajectories(context.Background(), indexed.Info.ID)
			if err != nil || len(trajectories) == 0 {
				t.Fatalf("trajectories=%#v err=%v", trajectories, err)
			}
		})
	}
}

func TestIndexSourceAdapterRequiresTrustAndIndexes(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is unavailable")
	}
	t.Setenv("RLVIZ_CONFIG_DIR", t.TempDir())
	store, err := rolloutindex.Open(filepath.Join(t.TempDir(), "index.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	adapter := filepath.Join("..", "..", "examples", "adapters", "simple-jsonl")
	source := filepath.Join("..", "..", "examples", "traces", "simple-agent.jsonl")
	if _, err := IndexSource(context.Background(), store, source, adapter); err == nil {
		t.Fatal("untrusted adapter was indexed")
	}
	plugin, err := plugins.Load(adapter)
	if err != nil {
		t.Fatal(err)
	}
	trust, err := plugins.DefaultTrustStore()
	if err != nil {
		t.Fatal(err)
	}
	if err := trust.Trust(plugin); err != nil {
		t.Fatal(err)
	}
	indexed, err := IndexSource(context.Background(), store, source, adapter)
	if err != nil {
		t.Fatal(err)
	}
	if !indexed.Refreshed || indexed.Info.Adapter != plugin.Path || indexed.Info.Fingerprint != plugin.Digest {
		t.Fatalf("indexed = %#v", indexed)
	}
	trajectories, err := store.Trajectories(context.Background(), indexed.Info.ID)
	if err != nil || len(trajectories) != 1 {
		t.Fatalf("trajectories=%#v err=%v", trajectories, err)
	}
	page, err := store.Events(context.Background(), rolloutindex.EventQuery{SourceID: indexed.Info.ID, TrajectoryID: trajectories[0].Value.ID})
	if err != nil || page.Total != 4 {
		t.Fatalf("page=%#v err=%v", page, err)
	}
}

func TestIndexSourceRefreshesChangedCanonicalFile(t *testing.T) {
	store, err := rolloutindex.Open(filepath.Join(t.TempDir(), "index.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	source := filepath.Join(t.TempDir(), "trace.ndjson")
	fixture, err := os.ReadFile(filepath.Join("..", "..", "fixtures", "canonical", "linear.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, fixture, 0o600); err != nil {
		t.Fatal(err)
	}
	first, err := IndexSource(context.Background(), store, source, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(source, first.Info.ModTime.AddDate(0, 0, 1), first.Info.ModTime.AddDate(0, 0, 1)); err != nil {
		t.Fatal(err)
	}
	second, err := IndexSource(context.Background(), store, source, "")
	if err != nil {
		t.Fatal(err)
	}
	if !second.Refreshed {
		t.Fatal("changed source was not refreshed")
	}
}
