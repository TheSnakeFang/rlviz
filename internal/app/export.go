package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/TheSnakeFang/rlviz/internal/browsercore"
	"github.com/TheSnakeFang/rlviz/internal/bundle"
	"github.com/TheSnakeFang/rlviz/internal/harborjob"
	"github.com/TheSnakeFang/rlviz/internal/model"
	"github.com/TheSnakeFang/rlviz/internal/plugins"
)

// ExportSource is a validated canonical snapshot suitable for an explicit
// portable export. SourcePath remains local and is not written to the bundle.
type ExportSource struct {
	SourcePath          string
	SourceName          string
	Format              string
	Fingerprint         string
	Canonical           []byte
	PathBackedArtifacts int
}

// CanonicalExport snapshots a built-in source or trusted adapter output
// without modifying the source or persistent RLViz index.
func CanonicalExport(ctx context.Context, path, adapterPath string) (ExportSource, error) {
	resolved, info, err := ResolveSource(path)
	if err != nil {
		return ExportSource{}, err
	}
	if adapterPath != "" {
		return canonicalAdapterExport(ctx, resolved, adapterPath)
	}
	if info.IsDir() {
		return canonicalHarborExport(resolved)
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return ExportSource{}, fmt.Errorf("read source: %w", err)
	}
	if len(data) > bundle.MaxBundleBytes {
		return ExportSource{}, fmt.Errorf("source is %d bytes; portable export maximum is %d bytes", len(data), bundle.MaxBundleBytes)
	}
	canonical, format, err := browsercore.Normalize(data, resolved)
	if err != nil {
		return ExportSource{}, &UnsupportedFormatError{Path: resolved, Cause: err}
	}
	digest := sha256.Sum256(data)
	return finishExport(resolved, format, "sha256:"+hex.EncodeToString(digest[:]), canonical)
}

func canonicalHarborExport(resolved string) (ExportSource, error) {
	snapshot, err := harborjob.Inspect(resolved)
	if err != nil {
		return ExportSource{}, &UnsupportedFormatError{Path: resolved, Cause: err}
	}
	for attempt := 0; attempt < 3; attempt++ {
		canonical, normalizeErr := harborjob.Normalize(snapshot)
		confirmed, inspectErr := harborjob.Inspect(resolved)
		if inspectErr != nil {
			return ExportSource{}, &UnsupportedFormatError{Path: resolved, Cause: inspectErr}
		}
		if confirmed.Fingerprint == snapshot.Fingerprint {
			if normalizeErr != nil {
				return ExportSource{}, &UnsupportedFormatError{Path: resolved, Cause: normalizeErr}
			}
			return finishExport(resolved, harborjob.Format, snapshot.Fingerprint, canonical)
		}
		snapshot = confirmed
	}
	return ExportSource{}, errors.New("harbor job changed repeatedly while it was being exported; retry after the current write finishes")
}

func canonicalAdapterExport(ctx context.Context, resolved, adapterPath string) (ExportSource, error) {
	plugin, err := plugins.Load(adapterPath)
	if err != nil {
		return ExportSource{}, fmt.Errorf("load adapter: %w", err)
	}
	trust, err := plugins.DefaultTrustStore()
	if err != nil {
		return ExportSource{}, fmt.Errorf("locate adapter trust store: %w", err)
	}
	host := plugins.NewHost(trust)
	probeRequest, err := plugins.NewRequest("probe", resolved, "")
	if err != nil {
		return ExportSource{}, err
	}
	probe, diagnostics, err := host.Probe(ctx, plugin, probeRequest)
	if err != nil {
		if errors.Is(err, plugins.ErrUntrusted) {
			return ExportSource{}, &PluginUntrustedError{Path: plugin.Path, Digest: plugin.Digest, Cause: err}
		}
		return ExportSource{}, withDiagnostics(err, diagnostics)
	}
	if !probe.Supported {
		return ExportSource{}, fmt.Errorf("adapter %q does not support source: %s", plugin.Manifest.Name, probe.Reason)
	}
	request, err := plugins.NewRequest("stream", resolved, probeRequest.Source.Root)
	if err != nil {
		return ExportSource{}, err
	}
	var canonical bytes.Buffer
	diagnostics, err = host.Stream(ctx, plugin, request, func(record *model.Record) error {
		raw, marshalErr := json.Marshal(record.Value)
		if marshalErr != nil {
			return marshalErr
		}
		if canonical.Len()+len(raw)+1 > bundle.MaxBundleBytes {
			return fmt.Errorf("adapter output exceeds portable export maximum of %d bytes", bundle.MaxBundleBytes)
		}
		canonical.Write(raw)
		canonical.WriteByte('\n')
		return nil
	})
	if err != nil {
		return ExportSource{}, withDiagnostics(err, diagnostics)
	}
	fingerprint := "adapter-sha256:" + plugin.Digest
	return finishExport(resolved, probe.Format, fingerprint, canonical.Bytes())
}

func finishExport(resolved, format, fingerprint string, canonical []byte) (ExportSource, error) {
	pathBacked := 0
	if err := model.Decode(bytes.NewReader(canonical), func(record *model.Record) error {
		if artifact, ok := record.Value.(*model.Artifact); ok && artifact.Path != "" && artifact.Text == "" && artifact.JSON == nil {
			pathBacked++
		}
		return nil
	}); err != nil {
		return ExportSource{}, fmt.Errorf("validate export: %w", err)
	}
	return ExportSource{SourcePath: resolved, SourceName: filepath.Base(resolved), Format: format, Fingerprint: fingerprint, Canonical: canonical, PathBackedArtifacts: pathBacked}, nil
}
