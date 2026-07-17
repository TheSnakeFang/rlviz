package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAgentSetup(t *testing.T) {
	tests := []struct {
		name        string
		destination string
		marker      string
	}{
		{name: "codex", destination: "AGENTS.md", marker: "# RLViz trace workflow"},
		{name: "claude-code", destination: "CLAUDE.md", marker: "# RLViz trace workflow"},
		{name: "cursor", destination: ".cursor/rules/rlviz.mdc", marker: "alwaysApply: false"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := loadAgentSetup(test.name)
			if err != nil {
				t.Fatalf("loadAgentSetup() error = %v", err)
			}
			if result.SchemaVersion != "1" || result.Command != "setup_agent" || result.Mode != "print" || result.Status != "ready" {
				t.Fatalf("unexpected stable metadata: %#v", result)
			}
			if result.WritePolicy != "read_only" || len(result.ContentSHA256) != 64 {
				t.Fatalf("unexpected content metadata: %#v", result)
			}
			if result.Agent != test.name || result.SuggestedDestination != test.destination {
				t.Fatalf("unexpected agent metadata: %#v", result)
			}
			if result.Source == "" || !strings.Contains(result.Content, test.marker) {
				t.Fatalf("missing bundled instructions: %#v", result)
			}
		})
	}
}

func TestLoadAgentSetupRejectsUnknownAgent(t *testing.T) {
	_, err := loadAgentSetup("other")
	if err == nil || !strings.Contains(err.Error(), "choose codex, claude-code, or cursor") {
		t.Fatalf("loadAgentSetup() error = %v", err)
	}
}

func TestAgentSetupJSONContract(t *testing.T) {
	result, err := loadAgentSetup("codex")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	wantFields := []string{
		`"schema_version":"1"`,
		`"command":"setup_agent"`,
		`"mode":"print"`,
		`"status":"ready"`,
		`"agent":"codex"`,
		`"source":"integrations/codex/AGENTS.md"`,
		`"suggested_destination":"AGENTS.md"`,
		`"write_policy":"read_only"`,
		`"content_sha256":`,
		`"content":`,
	}
	for _, field := range wantFields {
		if !strings.Contains(string(encoded), field) {
			t.Fatalf("JSON %s does not contain %s", encoded, field)
		}
	}
}

func TestAgentSetupDryRunJSONContract(t *testing.T) {
	t.Chdir(t.TempDir())
	result, err := prepareAgentSetup("codex", "dry_run", ".agents/rlviz.md")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	wantFields := []string{
		`"schema_version":"1"`,
		`"command":"setup_agent"`,
		`"mode":"dry_run"`,
		`"status":"would_create"`,
		`"destination":".agents/rlviz.md"`,
		`"write_policy":"create_only"`,
		`"content_sha256":`,
		`"content":`,
	}
	for _, field := range wantFields {
		if !strings.Contains(string(encoded), field) {
			t.Fatalf("JSON %s does not contain %s", encoded, field)
		}
	}
}

func TestNormalizeSetupAgentArguments(t *testing.T) {
	got := normalizeSetupAgentArguments([]string{"codex", "--dry-run", "--destination", ".agent/rlviz.md", "--json"})
	want := []string{"--dry-run", "--destination", ".agent/rlviz.md", "--json", "codex"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("normalizeSetupAgentArguments() = %q, want %q", got, want)
	}
}

func TestParseAgentSetupOptionsReturnsStableJSONErrors(t *testing.T) {
	tests := []struct {
		name      string
		arguments []string
		message   string
	}{
		{name: "missing mode", arguments: []string{"codex", "--json"}, message: "choose exactly one setup mode"},
		{name: "conflicting modes", arguments: []string{"codex", "--print", "--write", "--destination", "rules.md", "--json"}, message: "choose exactly one setup mode"},
		{name: "missing destination", arguments: []string{"codex", "--write", "--json"}, message: "require --destination"},
		{name: "print destination", arguments: []string{"codex", "--print", "--destination", "rules.md", "--json"}, message: "does not accept"},
		{name: "unknown flag", arguments: []string{"codex", "--print", "--unknown", "--json"}, message: "flag provided but not defined"},
		{name: "unknown flag before json value", arguments: []string{"codex", "--print", "--unknown", "--json=1"}, message: "flag provided but not defined"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options, err := parseAgentSetupOptions(test.arguments)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("error = %v, want %q", err, test.message)
			}
			if !options.JSON {
				t.Fatal("JSON intent was lost on invalid arguments")
			}
		})
	}
}

func TestExecuteAgentSetupWriteJSONContractAndCreateOnlyFailure(t *testing.T) {
	t.Chdir(t.TempDir())
	options := agentSetupOptions{Agent: "cursor", Mode: "write", Destination: ".cursor/rules/rlviz.mdc", JSON: true}
	result, err := executeAgentSetup(options)
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != "write" || result.Status != "created" || result.WritePolicy != "create_only" || result.Destination != options.Destination {
		t.Fatalf("write result = %#v", result)
	}
	content, err := os.ReadFile(options.Destination)
	if err != nil || string(content) != result.Content {
		t.Fatalf("created content mismatch: %v", err)
	}

	before := append([]byte(nil), content...)
	if _, err := executeAgentSetup(options); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("second write error = %v", err)
	}
	after, err := os.ReadFile(options.Destination)
	if err != nil || string(after) != string(before) {
		t.Fatalf("existing destination changed: %v", err)
	}
}

func TestPrepareAgentSetupDryRunIsStableAndReadOnly(t *testing.T) {
	t.Chdir(t.TempDir())

	result, err := prepareAgentSetup("cursor", "dry_run", ".cursor/rules/rlviz.mdc")
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != "dry_run" || result.Status != "would_create" || result.WritePolicy != "create_only" {
		t.Fatalf("unexpected dry-run metadata: %#v", result)
	}
	if result.Destination != ".cursor/rules/rlviz.mdc" {
		t.Fatalf("Destination = %q", result.Destination)
	}
	if _, err := os.Stat(".cursor"); !os.IsNotExist(err) {
		t.Fatalf("dry-run changed the filesystem: %v", err)
	}
}

func TestPrepareAgentSetupRejectsUnsafeOrExistingDestinations(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	if err := os.WriteFile("AGENTS.md", []byte("project rules\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), "linked"); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		destination string
		message     string
	}{
		{destination: "AGENTS.md", message: "already exists"},
		{destination: "../AGENTS.md", message: "stay within"},
		{destination: filepath.Join(root, "other.md"), message: "relative"},
		{destination: "linked/rlviz.md", message: "symbolic link"},
	}
	for _, test := range tests {
		t.Run(test.destination, func(t *testing.T) {
			_, err := prepareAgentSetup("codex", "dry_run", test.destination)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("prepareAgentSetup(%q) error = %v, want %q", test.destination, err, test.message)
			}
		})
	}
}

func TestCreateAgentSetupFileCreatesNestedFileExactlyOnce(t *testing.T) {
	t.Chdir(t.TempDir())
	const destination = ".cursor/rules/rlviz.mdc"
	const content = "exact bundled content\n"

	if err := createAgentSetupFile(destination, content); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != content {
		t.Fatalf("content = %q, want %q", got, content)
	}
	if err := createAgentSetupFile(destination, "replacement\n"); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("second create error = %v", err)
	}
	got, err = os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != content {
		t.Fatalf("existing file was changed: %q", got)
	}
}

func TestCreateAgentSetupFileRejectsSymlinkParent(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	t.Chdir(root)
	if err := os.Symlink(outside, "linked"); err != nil {
		t.Fatal(err)
	}

	err := createAgentSetupFile("linked/rlviz.md", "content\n")
	if err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("createAgentSetupFile() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "rlviz.md")); !os.IsNotExist(err) {
		t.Fatalf("wrote through symlink: %v", err)
	}
}

func TestCreateAgentSetupFileRejectsNonDirectoryAndLeafSymlink(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(t.TempDir(), "outside.md")
	t.Chdir(root)
	if err := os.WriteFile("file", []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(out, "linked.md"); err != nil {
		t.Fatal(err)
	}

	if err := createAgentSetupFile("file/rules.md", "content\n"); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("non-directory error = %v", err)
	}
	if err := createAgentSetupFile("linked.md", "content\n"); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("leaf symlink error = %v", err)
	}
	content, err := os.ReadFile(out)
	if err != nil || string(content) != "outside" {
		t.Fatalf("symlink target changed: %q %v", content, err)
	}
}
