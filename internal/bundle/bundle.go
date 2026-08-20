// Package bundle defines RLViz's portable, read-only trajectory bundle.
package bundle

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/TheSnakeFang/rlviz/internal/model"
)

const (
	Format          = "rlviz-portable-bundle-v1"
	Schema          = "rlviz.dev/bundle/v1"
	ManifestPath    = "manifest.json"
	TracePath       = "trace.ndjson"
	MaxBundleBytes  = 32 << 20
	maxManifestSize = 64 << 10
)

type Blob struct {
	Path      string `json:"path"`
	MediaType string `json:"media_type"`
	SHA256    string `json:"sha256"`
	Size      int64  `json:"size_bytes"`
}

type Source struct {
	Name        string `json:"name"`
	Format      string `json:"format"`
	Fingerprint string `json:"fingerprint"`
}

type Review struct {
	Status    string `json:"status"`
	Redaction string `json:"redaction"`
}

type Limitations struct {
	PathBackedArtifacts string `json:"path_backed_artifacts,omitempty"`
}

type Manifest struct {
	Schema      string      `json:"schema"`
	Title       string      `json:"title"`
	CreatedAt   string      `json:"created_at"`
	License     string      `json:"license"`
	ExpiresAt   string      `json:"expires_at,omitempty"`
	Source      Source      `json:"source"`
	Trace       Blob        `json:"trace"`
	Review      Review      `json:"review"`
	Limitations Limitations `json:"limitations,omitempty"`
}

type CreateOptions struct {
	Title               string
	License             string
	ExpiresAt           time.Time
	CreatedAt           time.Time
	SourceName          string
	SourceFormat        string
	SourceFingerprint   string
	PathBackedArtifacts int
}

// Create validates canonical NDJSON and returns a new portable bundle. It does
// not read artifacts or publish anything.
func Create(canonical []byte, options CreateOptions) ([]byte, Manifest, error) {
	if strings.TrimSpace(options.Title) == "" {
		return nil, Manifest{}, errors.New("bundle title is required")
	}
	if strings.TrimSpace(options.License) == "" {
		return nil, Manifest{}, errors.New("bundle license is required")
	}
	if strings.TrimSpace(options.SourceName) == "" || strings.TrimSpace(options.SourceFormat) == "" || strings.TrimSpace(options.SourceFingerprint) == "" {
		return nil, Manifest{}, errors.New("bundle source name, format, and fingerprint are required")
	}
	if len(canonical) > MaxBundleBytes {
		return nil, Manifest{}, fmt.Errorf("canonical trace is %d bytes; portable bundle maximum is %d bytes", len(canonical), MaxBundleBytes)
	}
	if err := model.Decode(bytes.NewReader(canonical), nil); err != nil {
		return nil, Manifest{}, fmt.Errorf("validate canonical trace: %w", err)
	}
	createdAt := options.CreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	traceDigest := sha256.Sum256(canonical)
	manifest := Manifest{
		Schema: Schema, Title: strings.TrimSpace(options.Title), CreatedAt: createdAt.Format(time.RFC3339), License: strings.TrimSpace(options.License),
		Source: Source{Name: filepath.Base(options.SourceName), Format: options.SourceFormat, Fingerprint: options.SourceFingerprint},
		Trace:  Blob{Path: TracePath, MediaType: "application/x-ndjson", SHA256: hex.EncodeToString(traceDigest[:]), Size: int64(len(canonical))},
		Review: Review{Status: "reviewed", Redaction: "confirmed"},
	}
	if !options.ExpiresAt.IsZero() {
		if !options.ExpiresAt.After(createdAt) {
			return nil, Manifest{}, errors.New("bundle expiration must be after creation time")
		}
		manifest.ExpiresAt = options.ExpiresAt.UTC().Format(time.RFC3339)
	}
	if options.PathBackedArtifacts > 0 {
		manifest.Limitations.PathBackedArtifacts = fmt.Sprintf("%d path-backed artifact(s) are referenced for provenance but not embedded", options.PathBackedArtifacts)
	}
	if err := validateManifest(manifest); err != nil {
		return nil, Manifest{}, err
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, Manifest{}, err
	}
	manifestBytes = append(manifestBytes, '\n')

	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	for _, entry := range []struct {
		name string
		body []byte
	}{{ManifestPath, manifestBytes}, {TracePath, canonical}} {
		header := &zip.FileHeader{Name: entry.name, Method: zip.Deflate}
		header.Modified = time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC)
		header.SetMode(0o644)
		file, createErr := writer.CreateHeader(header)
		if createErr != nil {
			return nil, Manifest{}, createErr
		}
		if _, writeErr := file.Write(entry.body); writeErr != nil {
			return nil, Manifest{}, writeErr
		}
	}
	if err := writer.Close(); err != nil {
		return nil, Manifest{}, err
	}
	if output.Len() > MaxBundleBytes {
		return nil, Manifest{}, fmt.Errorf("portable bundle is %d bytes; maximum is %d bytes", output.Len(), MaxBundleBytes)
	}
	return output.Bytes(), manifest, nil
}

// Open validates a complete bundle and returns its canonical trajectory bytes.
func Open(data []byte) (Manifest, []byte, error) {
	if len(data) > MaxBundleBytes {
		return Manifest{}, nil, fmt.Errorf("portable bundle is %d bytes; maximum is %d bytes", len(data), MaxBundleBytes)
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return Manifest{}, nil, fmt.Errorf("open portable bundle: %w", err)
	}
	if len(reader.File) != 2 {
		return Manifest{}, nil, fmt.Errorf("portable bundle must contain exactly %s and %s", ManifestPath, TracePath)
	}
	entries := map[string]*zip.File{}
	for _, file := range reader.File {
		if file.FileInfo().IsDir() || file.Name != filepath.ToSlash(filepath.Clean(file.Name)) || strings.Contains(file.Name, "..") {
			return Manifest{}, nil, fmt.Errorf("portable bundle contains unsafe entry %q", file.Name)
		}
		if _, exists := entries[file.Name]; exists {
			return Manifest{}, nil, fmt.Errorf("portable bundle contains duplicate entry %q", file.Name)
		}
		entries[file.Name] = file
	}
	manifestBytes, err := readEntry(entries[ManifestPath], maxManifestSize)
	if err != nil {
		return Manifest{}, nil, fmt.Errorf("read %s: %w", ManifestPath, err)
	}
	var manifest Manifest
	decoder := json.NewDecoder(bytes.NewReader(manifestBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, nil, fmt.Errorf("decode %s: %w", ManifestPath, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Manifest{}, nil, fmt.Errorf("decode %s: trailing JSON value", ManifestPath)
	}
	if err := validateManifest(manifest); err != nil {
		return Manifest{}, nil, err
	}
	trace, err := readEntry(entries[TracePath], MaxBundleBytes)
	if err != nil {
		return Manifest{}, nil, fmt.Errorf("read %s: %w", TracePath, err)
	}
	if int64(len(trace)) != manifest.Trace.Size {
		return Manifest{}, nil, fmt.Errorf("trace size mismatch: manifest has %d, bundle has %d", manifest.Trace.Size, len(trace))
	}
	digest := sha256.Sum256(trace)
	if actual := hex.EncodeToString(digest[:]); actual != manifest.Trace.SHA256 {
		return Manifest{}, nil, fmt.Errorf("trace digest mismatch: manifest has %s, bundle has %s", manifest.Trace.SHA256, actual)
	}
	if err := model.Decode(bytes.NewReader(trace), nil); err != nil {
		return Manifest{}, nil, fmt.Errorf("validate bundled trace: %w", err)
	}
	return manifest, trace, nil
}

func validateManifest(manifest Manifest) error {
	if manifest.Schema != Schema {
		return fmt.Errorf("unsupported portable bundle schema %q", manifest.Schema)
	}
	if strings.TrimSpace(manifest.Title) == "" || strings.TrimSpace(manifest.License) == "" {
		return errors.New("portable bundle title and license are required")
	}
	if strings.TrimSpace(manifest.Source.Name) == "" || strings.TrimSpace(manifest.Source.Format) == "" || strings.TrimSpace(manifest.Source.Fingerprint) == "" {
		return errors.New("portable bundle source name, format, and fingerprint are required")
	}
	if strings.ContainsAny(manifest.Source.Name, `/\`) || filepath.Base(manifest.Source.Name) != manifest.Source.Name {
		return errors.New("portable bundle source name must be a basename")
	}
	if len(manifest.Title) > 256 || len(manifest.License) > 128 || len(manifest.Source.Name) > 512 || len(manifest.Source.Format) > 128 || len(manifest.Source.Fingerprint) > 512 {
		return errors.New("portable bundle manifest field exceeds its maximum length")
	}
	createdAt, err := time.Parse(time.RFC3339, manifest.CreatedAt)
	if err != nil {
		return fmt.Errorf("portable bundle created_at: %w", err)
	}
	if manifest.ExpiresAt != "" {
		expiresAt, err := time.Parse(time.RFC3339, manifest.ExpiresAt)
		if err != nil {
			return fmt.Errorf("portable bundle expires_at: %w", err)
		}
		if !expiresAt.After(createdAt) {
			return errors.New("portable bundle expires_at must be after created_at")
		}
	}
	if manifest.Trace.Path != TracePath || manifest.Trace.MediaType != "application/x-ndjson" || manifest.Trace.Size < 0 {
		return errors.New("portable bundle trace descriptor is invalid")
	}
	if len(manifest.Trace.SHA256) != 64 || strings.Trim(manifest.Trace.SHA256, "0123456789abcdef") != "" {
		return errors.New("portable bundle trace sha256 is invalid")
	}
	if manifest.Review.Status != "reviewed" || manifest.Review.Redaction != "confirmed" {
		return errors.New("portable bundle review and redaction confirmation are required")
	}
	return nil
}

func readEntry(file *zip.File, limit int64) ([]byte, error) {
	if file == nil {
		return nil, errors.New("entry is missing")
	}
	if file.UncompressedSize64 > uint64(limit) {
		return nil, fmt.Errorf("entry exceeds %d bytes", limit)
	}
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("entry exceeds %d bytes", limit)
	}
	return data, nil
}
