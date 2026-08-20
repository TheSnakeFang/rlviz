package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/TheSnakeFang/rlviz/internal/app"
	"github.com/TheSnakeFang/rlviz/internal/bundle"
)

type bundleCreateResult struct {
	Status               string `json:"status"`
	Path                 string `json:"path"`
	SHA256               string `json:"sha256"`
	SizeBytes            int    `json:"size_bytes"`
	TraceSHA256          string `json:"trace_sha256"`
	Title                string `json:"title"`
	License              string `json:"license"`
	ExpiresAt            string `json:"expires_at,omitempty"`
	PathArtifactsOmitted int    `json:"path_artifacts_omitted"`
}

func runBundle(arguments []string) {
	if len(arguments) == 0 || arguments[0] == "help" || arguments[0] == "-h" || arguments[0] == "--help" {
		printBundleHelp()
		return
	}
	if arguments[0] != "create" {
		fmt.Fprintf(os.Stderr, "unknown bundle command %q\n", arguments[0])
		os.Exit(2)
	}
	runBundleCreate(arguments[1:])
}

func runBundleCreate(arguments []string) {
	flags := flag.NewFlagSet("bundle create", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	out := flags.String("out", "", "new .rlviz destination")
	title := flags.String("title", "", "human-readable bundle title")
	license := flags.String("license", "", "declared content license or proprietary")
	expires := flags.String("expires", "", "optional advisory expiration in RFC3339")
	reviewed := flags.Bool("reviewed", false, "confirm the trajectory content was reviewed")
	redaction := flags.Bool("redaction-confirmed", false, "confirm secrets and private data were removed")
	adapter := flags.String("adapter", "", "trusted adapter plugin path")
	jsonOutput := flags.Bool("json", false, "print machine-readable output")
	flags.Usage = func() { printBundleHelpTo(flags.Output()) }
	if err := flags.Parse(normalizeBundleArguments(arguments)); err != nil {
		os.Exit(2)
	}
	if flags.NArg() != 1 || *out == "" || *title == "" || *license == "" {
		flags.Usage()
		os.Exit(2)
	}
	if !*reviewed || !*redaction {
		fatalError("bundle_create", *jsonOutput, errors.New("portable export requires both --reviewed and --redaction-confirmed"))
	}
	if !strings.EqualFold(filepath.Ext(*out), ".rlviz") {
		fatalError("bundle_create", *jsonOutput, errors.New("bundle destination must use the .rlviz extension"))
	}
	var expiresAt time.Time
	if *expires != "" {
		var err error
		expiresAt, err = time.Parse(time.RFC3339, *expires)
		if err != nil {
			fatalError("bundle_create", *jsonOutput, fmt.Errorf("parse --expires: %w", err))
		}
	}
	source, err := app.CanonicalExport(context.Background(), flags.Arg(0), *adapter)
	if err != nil {
		fatalError("bundle_create", *jsonOutput, err)
	}
	data, manifest, err := bundle.Create(source.Canonical, bundle.CreateOptions{
		Title: *title, License: *license, ExpiresAt: expiresAt, SourceName: source.SourceName,
		SourceFormat: source.Format, SourceFingerprint: source.Fingerprint, PathBackedArtifacts: source.PathBackedArtifacts,
	})
	if err != nil {
		fatalError("bundle_create", *jsonOutput, err)
	}
	path, err := writeNewBundle(*out, data)
	if err != nil {
		fatalError("bundle_create", *jsonOutput, err)
	}
	digest := sha256.Sum256(data)
	result := bundleCreateResult{Status: "created", Path: path, SHA256: hex.EncodeToString(digest[:]), SizeBytes: len(data), TraceSHA256: manifest.Trace.SHA256, Title: manifest.Title, License: manifest.License, ExpiresAt: manifest.ExpiresAt, PathArtifactsOmitted: source.PathBackedArtifacts}
	human := fmt.Sprintf("Created reviewed portable bundle %s\nSHA-256: %s", result.Path, result.SHA256)
	if result.PathArtifactsOmitted > 0 {
		human += fmt.Sprintf("\nNote: %d path-backed artifact(s) remain provenance references and were not embedded.", result.PathArtifactsOmitted)
	}
	writeOutput(result, *jsonOutput, human)
}

func normalizeBundleArguments(arguments []string) []string {
	valueFlags := map[string]bool{"--out": true, "--title": true, "--license": true, "--expires": true, "--adapter": true}
	booleanFlags := map[string]bool{"--reviewed": true, "--redaction-confirmed": true, "--json": true}
	flags, positions := make([]string, 0, len(arguments)), make([]string, 0, 1)
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if valueFlags[argument] {
			flags = append(flags, argument)
			if index+1 < len(arguments) {
				index++
				flags = append(flags, arguments[index])
			}
			continue
		}
		matched := booleanFlags[argument]
		for name := range valueFlags {
			matched = matched || strings.HasPrefix(argument, name+"=")
		}
		if matched || strings.HasPrefix(argument, "--") {
			flags = append(flags, argument)
		} else {
			positions = append(positions, argument)
		}
	}
	return append(flags, positions...)
}

func writeNewBundle(destination string, data []byte) (string, error) {
	absolute, err := filepath.Abs(destination)
	if err != nil {
		return "", err
	}
	file, err := os.OpenFile(absolute, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("refusing to overwrite existing bundle %s", absolute)
		}
		return "", fmt.Errorf("create bundle: %w", err)
	}
	written := false
	defer func() {
		_ = file.Close()
		if !written {
			_ = os.Remove(absolute)
		}
	}()
	if _, err := file.Write(data); err != nil {
		return "", fmt.Errorf("write bundle: %w", err)
	}
	if err := file.Sync(); err != nil {
		return "", fmt.Errorf("sync bundle: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close bundle: %w", err)
	}
	written = true
	return absolute, nil
}

func printBundleHelp() { printBundleHelpTo(os.Stdout) }

func printBundleHelpTo(output io.Writer) {
	fmt.Fprint(output, `RLViz portable bundles

Usage:
  rlviz bundle create --out FILE --title TITLE --license LICENSE --reviewed --redaction-confirmed [--expires RFC3339] [--adapter PATH] [--json] SOURCE

Creates a new local .rlviz file. It never uploads or overwrites a file.
`)
}
