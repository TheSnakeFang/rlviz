package letta

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TheSnakeFang/rlviz/internal/model"
)

func TestProbeAndNormalizeTrajectoryV1(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "traces", "letta-trajectory-v1.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	supported, source, err := Probe(bytes.NewReader(data))
	if err != nil || !supported || source != "codex" {
		t.Fatalf("probe supported=%v source=%q err=%v", supported, source, err)
	}
	canonical, err := NormalizeBytes(data, path)
	if err != nil {
		t.Fatal(err)
	}
	var events []*model.Event
	err = model.Decode(bytes.NewReader(canonical), func(record *model.Record) error {
		if event, ok := record.Value.(*model.Event); ok {
			events = append(events, event)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 5 {
		t.Fatalf("events = %d", len(events))
	}
	if events[2].Kind != "tool" || events[2].Input.(map[string]any)["name"] != "exec_command" {
		t.Fatalf("tool event = %#v", events[2])
	}
	if events[3].Kind != "observation" || events[3].ParentID != events[2].ID {
		t.Fatalf("tool result = %#v", events[3])
	}
	repeated, err := NormalizeBytes(data, path)
	if err != nil || !bytes.Equal(canonical, repeated) {
		t.Fatal("normalization is not deterministic")
	}
}

func TestRejectsNonTrajectoryArraysAndMalformedRecords(t *testing.T) {
	for name, input := range map[string]struct{ input, message string }{
		"ordinary array":      {`[{"name":"not-a-trajectory"}]`, "leading meta"},
		"unknown field":       {`[{"role":"meta","source":"codex"},{"role":"user","content":"hi","timestamp":"2026-07-24T00:00:00Z","extra":true}]`, "unknown field"},
		"assistant invariant": {`[{"role":"meta","source":"codex"},{"role":"assistant","content":"not null","timestamp":"2026-07-24T00:00:00Z","tool_calls":[{"id":"1","name":"x","args":"{}"}]}]`, "null content"},
		"empty tool calls":    {`[{"role":"meta","source":"codex"},{"role":"assistant","content":"hello","timestamp":"2026-07-24T00:00:00Z","tool_calls":[]}]`, "must not be empty"},
		"wrong role field":    {`[{"role":"meta","source":"codex"},{"role":"user","content":"hello","timestamp":"2026-07-24T00:00:00Z","tool_calls":[]}]`, "cannot contain tool_calls"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := NormalizeBytes([]byte(input.input), "test.json")
			if err == nil || !strings.Contains(err.Error(), input.message) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestNormalizeLongTrajectory(t *testing.T) {
	var source strings.Builder
	source.WriteString(`[{"role":"meta","source":"claude-code","model":"claude"}`)
	for index := 0; index < 2_000; index++ {
		fmt.Fprintf(&source, `,{"role":"assistant","content":"step %d","timestamp":"2026-07-24T00:%02d:%02dZ"}`, index, (index/60)%60, index%60)
	}
	source.WriteByte(']')
	canonical, err := NormalizeBytes([]byte(source.String()), "long.json")
	if err != nil {
		t.Fatal(err)
	}
	events := 0
	if err := model.Decode(bytes.NewReader(canonical), func(record *model.Record) error {
		if _, ok := record.Value.(*model.Event); ok {
			events++
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if events != 2_000 {
		t.Fatalf("events = %d", events)
	}
}
