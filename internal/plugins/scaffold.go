package plugins

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ScaffoldOptions struct {
	Name string
	Kind string
}

// ScaffoldPython writes a minimal, dependency-free adapter project. It refuses
// to overwrite existing files so it is safe for coding agents to invoke.
func ScaffoldPython(destination string, options ScaffoldOptions) error {
	if !pluginName.MatchString(options.Name) {
		return errors.New("invalid plugin name")
	}
	kind := strings.ToLower(options.Kind)
	if kind == "" {
		kind = "adapter"
	}
	if kind != "adapter" && kind != "analyzer" {
		return errors.New("plugin type must be adapter or analyzer")
	}
	abs, err := filepath.Abs(destination)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return err
	}
	files := map[string]string{}
	if kind == "analyzer" {
		files[ManifestName] = strings.ReplaceAll(pythonAnalyzerManifest, "{{NAME}}", options.Name)
		files["analyzer.py"] = pythonAnalyzer
		files["sample-input.json"] = analyzerSampleInput
		files["README.md"] = strings.ReplaceAll(pythonAnalyzerReadme, "{{NAME}}", options.Name)
	} else {
		files[ManifestName] = strings.ReplaceAll(pythonManifest, "{{NAME}}", options.Name)
		files["adapter.py"] = pythonAdapter
		files["README.md"] = strings.ReplaceAll(pythonReadme, "{{NAME}}", options.Name)
	}
	for name := range files {
		path := filepath.Join(abs, name)
		if _, err := os.Lstat(path); err == nil {
			return fmt.Errorf("refusing to overwrite %s", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	for name, contents := range files {
		path := filepath.Join(abs, name)
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return fmt.Errorf("create %s: %w", path, err)
		}
		if _, err := f.WriteString(contents); err != nil {
			f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
	}
	return nil
}

const pythonManifest = `api_version: rlviz.dev/v1alpha1
kind: Adapter
name: {{NAME}}
version: 0.1.0
command:
  - python3
  - adapter.py
capabilities:
  - adapter.probe
  - adapter.stream
`

const pythonAdapter = `#!/usr/bin/env python3
"""Dependency-free RLViz adapter scaffold."""
import argparse
import json
import sys

def load_request(path):
    with open(path, "r", encoding="utf-8") as handle:
        request = json.load(handle)
    if request.get("api_version") != "rlviz.dev/v1alpha1":
        raise ValueError("unsupported api_version")
    return request

def emit(record):
    # stdout is protocol-only. Send diagnostics to stderr.
    print(json.dumps(record, separators=(",", ":"), ensure_ascii=False))

def probe(request):
    # TODO: inspect a bounded prefix and recognize the source format.
    print(json.dumps({"supported": False, "confidence": 0, "reason": "implement format detection"}, separators=(",", ":")))

def stream(request):
    # TODO: emit run/case/group/trajectory/event records with stable IDs.
    # The final count excludes the complete record.
    emit({"record_type": "complete", "records": 0, "warnings": 0})

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("operation", choices=("probe", "stream"))
    parser.add_argument("--request", required=True)
    args = parser.parse_args()
    request = load_request(args.request)
    if request.get("operation") != args.operation:
        raise ValueError("request operation does not match command")
    (probe if args.operation == "probe" else stream)(request)

if __name__ == "__main__":
    try:
        main()
    except Exception as error:
        print(str(error), file=sys.stderr)
        raise SystemExit(1)
`

const pythonReadme = `# {{NAME}}

This is a local RLViz adapter. Implement bounded format detection in
probe and canonical NDJSON emission in stream.

Validate it with:

    rlviz plugin trust .
    rlviz plugin validate . /path/to/sample
`

const pythonAnalyzerManifest = `api_version: rlviz.dev/v1alpha1
kind: Analyzer
name: {{NAME}}
version: 0.1.0
command:
  - python3
  - analyzer.py
capabilities:
  - analyzer.analyze
`

const pythonAnalyzer = `#!/usr/bin/env python3
"""Dependency-free RLViz analyzer scaffold."""
import argparse
import json
import os
import sys

API_VERSION = "rlviz.dev/analyzer/v1alpha1"

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("operation", choices=("analyze",))
    parser.add_argument("--request", required=True)
    args = parser.parse_args()
    with open(args.request, "r", encoding="utf-8") as handle:
        request = json.load(handle)
    if request.get("api_version") != API_VERSION or request.get("operation") != args.operation:
        raise ValueError("unsupported analyzer request")

    # TODO: inspect request["events"] and request.get("signals", []). Findings
    # must use stable IDs and may only reference events in this request.
    output = {
        "api_version": API_VERSION,
        "provenance": {
            "name": os.environ["RLVIZ_ANALYZER_NAME"],
            "version": os.environ["RLVIZ_ANALYZER_VERSION"],
            "digest": os.environ["RLVIZ_ANALYZER_DIGEST"],
            "input_digest": os.environ["RLVIZ_ANALYZER_INPUT_DIGEST"],
        },
        "findings": [],
        "signals": [],
    }
    print(json.dumps(output, separators=(",", ":"), ensure_ascii=False))

if __name__ == "__main__":
    try:
        main()
    except Exception as error:
        print(str(error), file=sys.stderr)
        raise SystemExit(1)
`

const analyzerSampleInput = `{"api_version":"rlviz.dev/analyzer/v1alpha1","operation":"analyze","trajectory_id":"trajectory-1","events":[{"record_type":"event","id":"event-1","trajectory_id":"trajectory-1","sequence":0,"kind":"tool","input":{"name":"example"}}],"signals":[]}
`

const pythonAnalyzerReadme = `# {{NAME}}

This is a local RLViz analyzer. It receives one normalized trajectory and
returns supplemental findings and signals without changing source data.

Validate it with:

    rlviz plugin trust .
    rlviz plugin validate . sample-input.json
`
