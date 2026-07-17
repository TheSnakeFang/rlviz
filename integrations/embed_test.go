package integrations

import (
	"strings"
	"testing"
)

func TestAgentInstructionsPreserveReviewBeforeTrustWorkflow(t *testing.T) {
	for _, name := range []string{"codex", "claude-code", "cursor"} {
		t.Run(name, func(t *testing.T) {
			setup, err := Agent(name)
			if err != nil {
				t.Fatal(err)
			}
			content := strings.ToLower(strings.Join(strings.Fields(setup.Content), " "))
			init := strings.Index(content, "plugin init --json")
			edit := strings.Index(content, "edit only the generated adapter")
			if edit < 0 {
				edit = strings.Index(content, "edit only the adapter")
			}
			approval := strings.Index(content, "explicit approval")
			trust := strings.LastIndex(content, "plugin trust --json")
			if init < 0 || edit < init || approval < edit || trust < approval {
				t.Fatalf("unsafe workflow ordering in %s instructions: init=%d edit=%d approval=%d trust=%d", name, init, edit, approval, trust)
			}
			for _, required := range []string{"--from \"<source>\"", "plugin validate --json", "open --json"} {
				if !strings.Contains(content, required) {
					t.Fatalf("%s instructions missing %q", name, required)
				}
			}
		})
	}
}
