package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/unlatch-ai/rolloutviz/internal/analyzers"
	"github.com/unlatch-ai/rolloutviz/internal/model"
)

type ValidationReport struct {
	Plugin        string `json:"plugin"`
	Digest        string `json:"digest"`
	Format        string `json:"format"`
	Records       int    `json:"records"`
	Warnings      int64  `json:"warnings"`
	Deterministic bool   `json:"deterministic"`
}

type AnalyzerValidationReport struct {
	Plugin        string `json:"plugin"`
	Digest        string `json:"digest"`
	Findings      int    `json:"findings"`
	Signals       int    `json:"signals"`
	Deterministic bool   `json:"deterministic"`
}

// LoadAnalyzerInput reads one strict, bounded analyzer v1alpha1 request.
func LoadAnalyzerInput(path string) (analyzers.Input, error) {
	var input analyzers.Input
	file, err := os.Open(path)
	if err != nil {
		return input, err
	}
	defer file.Close()
	reader := io.LimitReader(file, analyzers.MaxInputBytes+1)
	data, err := io.ReadAll(reader)
	if err != nil {
		return input, err
	}
	if len(data) > analyzers.MaxInputBytes {
		return input, fmt.Errorf("analyzer input exceeds %d bytes", analyzers.MaxInputBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return input, fmt.Errorf("invalid analyzer input: %w", err)
	}
	if err := ensureSingleJSON(decoder, "analyzer input"); err != nil {
		return input, err
	}
	input = analyzers.NormalizeInput(input)
	if err := analyzers.ValidateInput(input); err != nil {
		return input, err
	}
	return input, nil
}

// ValidateAnalyzer executes the trusted analyzer twice and requires byte-level
// deterministic protocol output in addition to semantic validation.
func (h *Host) ValidateAnalyzer(ctx context.Context, plugin *Plugin, input analyzers.Input) (AnalyzerValidationReport, error) {
	report := AnalyzerValidationReport{}
	if plugin != nil {
		report.Plugin, report.Digest = plugin.Manifest.Name, plugin.Digest
	}
	first, _, err := h.analyzeBytes(ctx, plugin, input)
	if err != nil {
		return report, fmt.Errorf("analyze pass 1: %w", err)
	}
	second, _, err := h.analyzeBytes(ctx, plugin, input)
	if err != nil {
		return report, fmt.Errorf("analyze pass 2: %w", err)
	}
	if !bytes.Equal(first, second) {
		return report, errors.New("analyzer is nondeterministic: repeated output differs")
	}
	output, err := decodeAnalyzerOutput(plugin, input, first)
	if err != nil {
		return report, fmt.Errorf("validate analyzer output: %w", err)
	}
	report.Findings, report.Signals, report.Deterministic = len(output.Findings), len(output.Signals), true
	return report, nil
}

// ValidateAdapter probes and streams the same source twice. Exact stdout
// equality enforces stable IDs, ordering, payloads, and completion counts.
func (h *Host) ValidateAdapter(ctx context.Context, plugin *Plugin, sourcePath, root string) (ValidationReport, error) {
	report := ValidationReport{Deterministic: false}
	if plugin != nil {
		report.Plugin = plugin.Manifest.Name
		report.Digest = plugin.Digest
	}
	probeReq, err := NewRequest("probe", sourcePath, root)
	if err != nil {
		return report, err
	}
	p1, _, err := h.Probe(ctx, plugin, probeReq)
	if err != nil {
		return report, fmt.Errorf("probe pass 1: %w", err)
	}
	p2, _, err := h.Probe(ctx, plugin, probeReq)
	if err != nil {
		return report, fmt.Errorf("probe pass 2: %w", err)
	}
	if p1 != p2 {
		return report, fmt.Errorf("probe is nondeterministic: first=%+v second=%+v", p1, p2)
	}
	if !p1.Supported {
		return report, fmt.Errorf("adapter does not support source: %s", p1.Reason)
	}
	report.Format = p1.Format
	streamReq, err := NewRequest("stream", sourcePath, root)
	if err != nil {
		return report, err
	}
	first, err := h.run(ctx, plugin, streamReq)
	if err != nil {
		return report, fmt.Errorf("stream pass 1: %w", err)
	}
	second, err := h.run(ctx, plugin, streamReq)
	if err != nil {
		return report, fmt.Errorf("stream pass 2: %w", err)
	}
	if !bytes.Equal(first.Stdout, second.Stdout) {
		return report, fmt.Errorf("stream is nondeterministic: repeated output differs")
	}
	err = decodeReport(first.Stdout, streamReq.Source.Root, &report)
	if err != nil {
		return report, err
	}
	report.Deterministic = true
	return report, nil
}

func decodeReport(data []byte, root string, report *ValidationReport) error {
	return model.Decode(bytes.NewReader(data), func(record *model.Record) error {
		if err := validateRecordProvenance(record, root); err != nil {
			return err
		}
		if record.Type == model.RecordComplete {
			report.Warnings = record.Value.(*model.Complete).Warnings
		} else {
			report.Records++
		}
		return nil
	})
}

func validateRecordProvenance(record *model.Record, root string) error {
	event, ok := record.Value.(*model.Event)
	if !ok || event.Source == nil {
		return nil
	}
	path := event.Source.Path
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("event %q source path: %w", event.ID, err)
	}
	if !within(root, resolved) {
		return fmt.Errorf("event %q source path escapes registered root", event.ID)
	}
	if event.Source.ByteOffset == nil {
		return nil
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return err
	}
	end := *event.Source.ByteOffset
	if event.Source.ByteLength != nil {
		length := *event.Source.ByteLength
		if length > info.Size()-end {
			return fmt.Errorf("event %q source byte range exceeds file size %d", event.ID, info.Size())
		}
		end += length
	}
	if end > info.Size() {
		return fmt.Errorf("event %q source byte range ends at %d past file size %d", event.ID, end, info.Size())
	}
	return nil
}
