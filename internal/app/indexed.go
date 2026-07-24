package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/TheSnakeFang/rlviz/internal/atif"
	"github.com/TheSnakeFang/rlviz/internal/browsercore"
	rolloutindex "github.com/TheSnakeFang/rlviz/internal/index"
	"github.com/TheSnakeFang/rlviz/internal/letta"
	"github.com/TheSnakeFang/rlviz/internal/model"
	"github.com/TheSnakeFang/rlviz/internal/plugins"
	"github.com/TheSnakeFang/rlviz/internal/server"
)

type IndexedSource struct {
	Info      rolloutindex.SourceInfo
	Refreshed bool
}

// IndexSource validates a source and transactionally refreshes its persistent
// canonical index when the source or selected adapter changed.
func IndexSource(ctx context.Context, store *rolloutindex.Index, path, adapterPath string) (IndexedSource, error) {
	if store == nil {
		return IndexedSource{}, errors.New("rollout index is required")
	}
	resolved, err := ValidateSource(path)
	if err != nil {
		return IndexedSource{}, err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return IndexedSource{}, err
	}

	if adapterPath == "" {
		format, err := detectBuiltInFormat(resolved)
		if err != nil {
			return IndexedSource{}, err
		}
		if format != "canonical-ndjson" {
			source := rolloutindex.Source{
				ID: server.SourceID(resolved + "\x00builtin:" + format), Path: resolved,
				Fingerprint: format + ":" + plugins.APIVersion, Size: info.Size(), ModTime: info.ModTime(),
			}
			if cached, ok, err := freshSource(ctx, store, source); err != nil {
				return IndexedSource{}, err
			} else if ok {
				return IndexedSource{Info: cached}, nil
			}
			var canonical []byte
			if format == letta.Format {
				file, openErr := os.Open(resolved)
				if openErr != nil {
					return IndexedSource{}, fmt.Errorf("read %s source: %w", format, openErr)
				}
				canonical, err = letta.Normalize(file, resolved)
				_ = file.Close()
			} else {
				data, readErr := os.ReadFile(resolved)
				if readErr != nil {
					return IndexedSource{}, fmt.Errorf("read %s source: %w", format, readErr)
				}
				if format == atif.Format {
					canonical, err = atif.NormalizeBytes(data, resolved)
				} else {
					canonical, _, err = browsercore.Normalize(data, resolved)
				}
			}
			if err != nil {
				return IndexedSource{}, &UnsupportedFormatError{Path: resolved, Cause: err}
			}
			indexed, err := store.Replace(ctx, source, bytes.NewReader(canonical))
			if err != nil {
				return IndexedSource{}, fmt.Errorf("index %s source: %w", format, err)
			}
			return IndexedSource{Info: indexed, Refreshed: true}, nil
		}
		source := rolloutindex.Source{
			ID: server.SourceID(resolved + "\x00builtin:canonical"), Path: resolved,
			Fingerprint: "canonical:" + plugins.APIVersion, Size: info.Size(), ModTime: info.ModTime(),
		}
		if cached, ok, err := freshSource(ctx, store, source); err != nil {
			return IndexedSource{}, err
		} else if ok {
			return IndexedSource{Info: cached}, nil
		}
		file, err := os.Open(resolved)
		if err != nil {
			return IndexedSource{}, fmt.Errorf("open canonical source: %w", err)
		}
		defer file.Close()
		indexed, err := store.Replace(ctx, source, file)
		if err != nil {
			return IndexedSource{}, &UnsupportedFormatError{Path: resolved, Cause: err}
		}
		return IndexedSource{Info: indexed, Refreshed: true}, nil
	}

	plugin, err := plugins.Load(adapterPath)
	if err != nil {
		return IndexedSource{}, fmt.Errorf("load adapter: %w", err)
	}
	trust, err := plugins.DefaultTrustStore()
	if err != nil {
		return IndexedSource{}, fmt.Errorf("locate adapter trust store: %w", err)
	}
	host := plugins.NewHost(trust)
	probeRequest, err := plugins.NewRequest("probe", resolved, "")
	if err != nil {
		return IndexedSource{}, err
	}
	probe, diagnostics, err := host.Probe(ctx, plugin, probeRequest)
	if err != nil {
		if errors.Is(err, plugins.ErrUntrusted) {
			return IndexedSource{}, &PluginUntrustedError{Path: plugin.Path, Digest: plugin.Digest, Cause: err}
		}
		return IndexedSource{}, withDiagnostics(err, diagnostics)
	}
	if !probe.Supported {
		return IndexedSource{}, fmt.Errorf("adapter %q does not support source: %s", plugin.Manifest.Name, probe.Reason)
	}
	source := rolloutindex.Source{
		ID: server.SourceID(resolved + "\x00adapter:" + plugin.Path), Path: resolved,
		Adapter: plugin.Path, Fingerprint: plugin.Digest, Size: info.Size(), ModTime: info.ModTime(),
	}
	if cached, ok, err := freshSource(ctx, store, source); err != nil {
		return IndexedSource{}, err
	} else if ok {
		return IndexedSource{Info: cached}, nil
	}

	streamRequest, err := plugins.NewRequest("stream", resolved, probeRequest.Source.Root)
	if err != nil {
		return IndexedSource{}, err
	}
	indexed, err := store.ReplaceRecords(ctx, source, func(yield func(*model.Record) error) error {
		var streamErr error
		diagnostics, streamErr = host.Stream(ctx, plugin, streamRequest, yield)
		if streamErr == nil {
			return nil
		}
		return withDiagnostics(streamErr, diagnostics)
	})
	if err != nil {
		return IndexedSource{}, fmt.Errorf("stream and index adapter output: %w", err)
	}
	return IndexedSource{Info: indexed, Refreshed: true}, nil
}

func detectBuiltInFormat(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	supported, _, err := atif.Probe(io.LimitReader(file, 1<<20))
	if err == nil && supported {
		return atif.Format, nil
	}
	if _, seekErr := file.Seek(0, io.SeekStart); seekErr == nil {
		supported, _, err = letta.Probe(io.LimitReader(file, 1<<20))
		if err == nil && supported {
			return letta.Format, nil
		}
	}
	info, statErr := file.Stat()
	if statErr == nil && info.Size() <= browsercore.MaxRecommendedBytes && strings.EqualFold(filepath.Ext(path), ".json") {
		if _, seekErr := file.Seek(0, io.SeekStart); seekErr == nil {
			data, readErr := io.ReadAll(file)
			if readErr == nil {
				if _, format, normalizeErr := browsercore.Normalize(data, path); normalizeErr == nil {
					return format, nil
				}
			}
		}
	}
	// A bounded probe can end inside a large field before the ATIF header. Such
	// documents are not auto-detected; explicit adapters remain available.
	return "canonical-ndjson", nil
}

func freshSource(ctx context.Context, store *rolloutindex.Index, source rolloutindex.Source) (rolloutindex.SourceInfo, bool, error) {
	status, err := store.Status(ctx, source)
	if err != nil {
		return rolloutindex.SourceInfo{}, false, err
	}
	if status.State == rolloutindex.CacheFresh && status.Cached != nil {
		return *status.Cached, true, nil
	}
	return rolloutindex.SourceInfo{}, false, nil
}
