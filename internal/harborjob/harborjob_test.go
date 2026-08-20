package harborjob

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/TheSnakeFang/rlviz/internal/model"
)

func TestNormalizeCurrentHarborJob(t *testing.T) {
	root := writeJob(t, false)
	snapshot, err := Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.trials) != 2 {
		t.Fatalf("trials = %d, want 2", len(snapshot.trials))
	}

	first, err := Normalize(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Normalize(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("normalization is not deterministic")
	}

	var runs, cases, groups, trajectories, graders, failures int
	signals := map[string]*model.Signal{}
	artifacts := map[string]bool{}
	if err := model.Decode(bytes.NewReader(first), func(record *model.Record) error {
		switch value := record.Value.(type) {
		case *model.Run:
			runs++
			if value.Name != "harbor-smoke" || value.Metadata["source_format"] != Format {
				t.Fatalf("unexpected run: %#v", value)
			}
		case *model.Case:
			cases++
		case *model.Group:
			groups++
		case *model.Trajectory:
			trajectories++
			if value.Status == "failed" {
				failures++
			}
		case *model.Event:
			if value.Kind == "grader" {
				graders++
				output, _ := value.Output.(map[string]any)
				if output["verdict"] != "pass" {
					t.Fatalf("grader verdict = %#v", output["verdict"])
				}
			}
			if value.Kind == "error" {
				failures++
			}
		case *model.Signal:
			signals[value.Name] = value
		case *model.Artifact:
			artifacts[value.Path] = true
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if runs != 1 || cases != 2 || groups != 2 || trajectories != 2 || graders != 1 || failures != 2 {
		t.Fatalf("runs=%d cases=%d groups=%d trajectories=%d graders=%d failures=%d", runs, cases, groups, trajectories, graders, failures)
	}
	if signals["reward"] == nil || signals["reward"].Value != json.Number("1") {
		t.Fatalf("reward signal = %#v", signals["reward"])
	}
	if signals["input_tokens"] == nil || signals["input_tokens"].Metadata["provenance"] != "adapter_derived_sum" {
		t.Fatalf("input token signal = %#v", signals["input_tokens"])
	}
	if signals["input_tokens"].Value != json.Number("100") {
		t.Fatalf("input tokens = %#v, aggregate and step contexts were both counted", signals["input_tokens"].Value)
	}
	if signals["uncached_input_tokens"].Value != json.Number("75") || signals["total_tokens"].Value != json.Number("110") {
		t.Fatalf("derived token signals = uncached %#v total %#v", signals["uncached_input_tokens"], signals["total_tokens"])
	}
	if signals["agent_execution_duration_ms"] == nil || signals["agent_execution_duration_ms"].Value != json.Number("2000") {
		t.Fatalf("agent execution duration = %#v", signals["agent_execution_duration_ms"])
	}
	for _, path := range []string{"config.json", "result.json", "trial-pass/verifier/ctrf.json", "trial-pass/artifacts/report.txt"} {
		if !artifacts[path] {
			t.Errorf("missing artifact %q", path)
		}
	}
}

func TestInspectSupportsTrialsContainerAndRefreshesFingerprint(t *testing.T) {
	root := writeJob(t, true)
	first, err := Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.trials) != 2 || !strings.Contains(first.trials[0].directory, string(filepath.Separator)+"trials"+string(filepath.Separator)) {
		t.Fatalf("legacy trial inventory = %#v", first.trials)
	}

	writeJSON(t, filepath.Join(root, "trials", "trial-pass", "result.json"), trialResult("trial-pass", "task-pass", nil, 0.75))
	second, err := Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	if first.Fingerprint == second.Fingerprint {
		t.Fatal("fingerprint did not change after a recognized file changed")
	}
}

func TestInspectRejectsSymlinkedTrajectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions vary on Windows")
	}
	root := writeJob(t, false)
	trajectory := filepath.Join(root, "trial-pass", "agent", "trajectory.json")
	if err := os.Remove(trajectory); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "trajectory.json")
	writeJSON(t, outside, atifDocument("outside"))
	if err := os.Symlink(outside, trajectory); err != nil {
		t.Fatal(err)
	}
	if _, err := Inspect(root); err == nil || !strings.Contains(err.Error(), "must not contain a symlink") {
		t.Fatalf("Inspect error = %v, want symlink rejection", err)
	}
}

func TestInspectRejectsSymlinkedTrajectoryParent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions vary on Windows")
	}
	root := writeJob(t, false)
	agentDirectory := filepath.Join(root, "trial-pass", "agent")
	if err := os.RemoveAll(agentDirectory); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	writeJSON(t, filepath.Join(outside, "trajectory.json"), atifDocument("outside"))
	if err := os.Symlink(outside, agentDirectory); err != nil {
		t.Fatal(err)
	}
	if _, err := Inspect(root); err == nil || !strings.Contains(err.Error(), "must not contain a symlink") {
		t.Fatalf("Inspect error = %v, want parent-symlink rejection", err)
	}
}

func TestNormalizeMultiStepTrialPreservesParent(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "config.json"), map[string]any{"job_name": "multi-step"})
	writeJSON(t, filepath.Join(root, "result.json"), map[string]any{"id": "job-multi", "n_total_trials": 1})
	trial := filepath.Join(root, "trial-multi")
	result := trialResult("trial-multi", "task-multi", nil, 1)
	delete(result, "agent_result")
	result["step_results"] = []any{map[string]any{
		"step_name": "verify", "exception_info": map[string]any{"exception_type": "VerifierError", "exception_message": "broken verifier", "occurred_at": "2026-08-20T10:00:04Z"},
	}}
	writeJSON(t, filepath.Join(trial, "result.json"), result)
	// Agents may reuse a session identifier across steps. RLViz must still
	// namespace every canonical record under its source document.
	writeJSON(t, filepath.Join(trial, "steps", "1", "agent", "trajectory.json"), atifDocument("shared-session"))
	writeJSON(t, filepath.Join(trial, "steps", "2", "agent", "trajectory.json"), atifDocument("shared-session"))
	writeJSON(t, filepath.Join(trial, "steps", "2", "verifier", "ctrf.json"), map[string]any{"results": map[string]any{"summary": map[string]any{"tests": 1, "failed": 1}}})

	snapshot, err := Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := Normalize(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	parents, childIDs, eventIDs, errors, graders := map[string]bool{}, map[string]bool{}, map[string]bool{}, 0, 0
	if err := model.Decode(bytes.NewReader(canonical), func(record *model.Record) error {
		if trajectory, ok := record.Value.(*model.Trajectory); ok {
			if trajectory.ParentID == "" {
				parents[trajectory.ID] = true
				if trajectory.Status != "failed" || trajectory.Termination != "VerifierError" {
					t.Fatalf("multi-step parent status = %#v", trajectory)
				}
			} else {
				if childIDs[trajectory.ID] {
					t.Fatalf("duplicate child trajectory ID %q", trajectory.ID)
				}
				childIDs[trajectory.ID] = true
				if !parents[trajectory.ParentID] {
					t.Fatalf("child %q references unknown parent %q", trajectory.ID, trajectory.ParentID)
				}
			}
		}
		if event, ok := record.Value.(*model.Event); ok {
			if eventIDs[event.ID] {
				t.Fatalf("duplicate event ID %q", event.ID)
			}
			eventIDs[event.ID] = true
			if event.Kind == "error" {
				errors++
				if event.Metadata["step_name"] != "verify" {
					t.Fatalf("step exception metadata = %#v", event.Metadata)
				}
			}
			if event.Kind == "grader" {
				graders++
				if event.Metadata["step_name"] != "2" || event.AlignmentKey != "grader:ctrf:2" {
					t.Fatalf("step grader = %#v", event)
				}
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(parents) != 1 || len(childIDs) != 2 || errors != 1 || graders != 1 {
		t.Fatalf("parents=%d children=%d errors=%d graders=%d", len(parents), len(childIDs), errors, graders)
	}
}

func writeJob(t *testing.T, legacy bool) string {
	t.Helper()
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "config.json"), map[string]any{"job_name": "harbor-smoke"})
	writeJSON(t, filepath.Join(root, "result.json"), map[string]any{"id": "job-1", "started_at": "2026-08-20T10:00:00Z", "finished_at": "2026-08-20T10:01:00Z", "n_total_trials": 2})
	container := root
	if legacy {
		container = filepath.Join(root, "trials")
	}
	pass := filepath.Join(container, "trial-pass")
	passResult := trialResult("trial-pass", "task-pass", nil, 1)
	passResult["step_results"] = []any{
		map[string]any{"step_name": "prepare", "agent_result": map[string]any{"n_input_tokens": 40}, "verifier_result": map[string]any{"rewards": map[string]any{"policy": 1}}},
		map[string]any{"step_name": "solve", "agent_result": map[string]any{"n_input_tokens": 60}},
	}
	writeJSON(t, filepath.Join(pass, "result.json"), passResult)
	writeJSON(t, filepath.Join(pass, "config.json"), map[string]any{"agent": map[string]any{"name": "codex"}})
	writeJSON(t, filepath.Join(pass, "agent", "trajectory.json"), atifDocument("pass-session"))
	writeJSON(t, filepath.Join(pass, "verifier", "ctrf.json"), map[string]any{"results": map[string]any{"summary": map[string]any{"tests": 2, "failed": 0}}})
	writeText(t, filepath.Join(pass, "artifacts", "report.txt"), "source-backed evidence")

	failed := filepath.Join(container, "trial-failed")
	exception := map[string]any{"exception_type": "AgentTimeoutError", "occurred_at": "2026-08-20T10:00:30Z", "message": "agent timed out"}
	writeJSON(t, filepath.Join(failed, "result.json"), trialResult("trial-failed", "task-failed", exception, 0))
	return root
}

func trialResult(id, task string, exception map[string]any, reward float64) map[string]any {
	result := map[string]any{
		"id": id, "trial_name": id, "task_name": task, "task_id": task + "-id", "source": "terminal-bench",
		"started_at": "2026-08-20T10:00:00Z", "finished_at": "2026-08-20T10:00:10Z",
		"agent_info":      map[string]any{"name": "codex", "model_info": map[string]any{"name": "gpt-test"}},
		"agent_result":    map[string]any{"n_input_tokens": 100, "n_cache_tokens": 25, "n_output_tokens": 10, "cost_usd": 0.01},
		"verifier_result": map[string]any{"rewards": map[string]any{"reward": reward}},
		"agent_execution": map[string]any{"started_at": "2026-08-20T10:00:01Z", "finished_at": "2026-08-20T10:00:03Z"},
	}
	if exception != nil {
		result["exception_info"] = exception
	}
	return result
}

func atifDocument(session string) map[string]any {
	return map[string]any{
		"schema_version": "ATIF-v1.7", "session_id": session,
		"agent": map[string]any{"name": "codex", "model_name": "gpt-test"},
		"steps": []any{map[string]any{"step_id": 1, "source": "user", "message": "Solve the task."}},
	}
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	writeText(t, path, string(raw))
}

func writeText(t *testing.T, path, value string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(value), 0o644); err != nil {
		t.Fatal(err)
	}
}
