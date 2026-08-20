package bundle

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestCreateAndOpenRoundTripWithoutLeakingSourcePath(t *testing.T) {
	canonical, err := os.ReadFile(filepath.Join("..", "..", "fixtures", "canonical", "linear.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	created := time.Date(2026, 8, 20, 20, 0, 0, 0, time.UTC)
	data, manifest, err := Create(canonical, CreateOptions{Title: "Reviewed rollout", License: "CC-BY-4.0", CreatedAt: created, ExpiresAt: created.Add(24 * time.Hour), SourceName: "/private/customer/run.ndjson", SourceFormat: "canonical-ndjson", SourceFingerprint: "sha256:source", PathBackedArtifacts: 2})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("/private/customer")) {
		t.Fatal("bundle leaked the local source path")
	}
	if manifest.Source.Name != "run.ndjson" || !strings.Contains(manifest.Limitations.PathBackedArtifacts, "2 path-backed") {
		t.Fatalf("manifest = %#v", manifest)
	}
	opened, trace, err := Open(data)
	if err != nil {
		t.Fatal(err)
	}
	if opened != manifest || !bytes.Equal(trace, canonical) {
		t.Fatalf("round trip changed bundle: manifest=%#v trace_equal=%v", opened, bytes.Equal(trace, canonical))
	}
}

func TestOpenRejectsChangedTraceAndUnexpectedEntries(t *testing.T) {
	canonical, err := os.ReadFile(filepath.Join("..", "..", "fixtures", "canonical", "linear.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	data, _, err := Create(canonical, testOptions("Reviewed", "proprietary", time.Unix(1, 0)))
	if err != nil {
		t.Fatal(err)
	}

	tampered := rewriteZip(t, data, func(name string, body []byte) (string, []byte) {
		if name == TracePath {
			body = append(append([]byte{}, body...), '\n')
		}
		return name, body
	}, nil)
	if _, _, err := Open(tampered); err == nil || !strings.Contains(err.Error(), "size mismatch") {
		t.Fatalf("Open(tampered) error = %v", err)
	}

	extra := rewriteZip(t, data, func(name string, body []byte) (string, []byte) { return name, body }, map[string][]byte{"notes.txt": []byte("unexpected")})
	if _, _, err := Open(extra); err == nil || !strings.Contains(err.Error(), "exactly") {
		t.Fatalf("Open(extra) error = %v", err)
	}
}

func TestCreateRequiresReviewMetadataAndFutureExpiration(t *testing.T) {
	canonical, err := os.ReadFile(filepath.Join("..", "..", "fixtures", "canonical", "linear.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	missingTitle := testOptions("", "MIT", time.Unix(1, 0))
	if _, _, err := Create(canonical, missingTitle); err == nil || !strings.Contains(err.Error(), "title") {
		t.Fatalf("missing title error = %v", err)
	}
	created := time.Unix(10, 0)
	options := testOptions("x", "MIT", created)
	options.ExpiresAt = created
	if _, _, err := Create(canonical, options); err == nil || !strings.Contains(err.Error(), "after") {
		t.Fatalf("expiration error = %v", err)
	}
}

func TestBundleDigestIsStableForFixedMetadata(t *testing.T) {
	canonical, err := os.ReadFile(filepath.Join("..", "..", "fixtures", "canonical", "linear.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	options := testOptions("Stable", "MIT", time.Unix(10, 0))
	left, _, err := Create(canonical, options)
	if err != nil {
		t.Fatal(err)
	}
	right, _, err := Create(canonical, options)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(left, right) {
		leftDigest, rightDigest := sha256.Sum256(left), sha256.Sum256(right)
		t.Fatalf("bundle is not deterministic: %s != %s", hex.EncodeToString(leftDigest[:]), hex.EncodeToString(rightDigest[:]))
	}
}

func TestCreatedManifestMatchesPublicSchema(t *testing.T) {
	canonical, err := os.ReadFile(filepath.Join("..", "..", "fixtures", "canonical", "linear.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	_, manifest, err := Create(canonical, testOptions("Reviewed", "MIT", time.Unix(10, 0)))
	if err != nil {
		t.Fatal(err)
	}
	schemaData, err := os.ReadFile(filepath.Join("..", "..", "schemas", "v1alpha1", "bundle-manifest.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaData))
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	location := "https://rlviz.dev/schemas/v1alpha1/bundle-manifest.schema.json"
	if err := compiler.AddResource(location, document); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile(location)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(instance); err != nil {
		t.Fatal(err)
	}
}

func testOptions(title, license string, created time.Time) CreateOptions {
	return CreateOptions{Title: title, License: license, CreatedAt: created, SourceName: "trace.ndjson", SourceFormat: "canonical-ndjson", SourceFingerprint: "sha256:source"}
}

func rewriteZip(t *testing.T, input []byte, change func(string, []byte) (string, []byte), extra map[string][]byte) []byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(input), int64(len(input)))
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	for _, file := range reader.File {
		opened, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(opened)
		_ = opened.Close()
		if err != nil {
			t.Fatal(err)
		}
		name, body := change(file.Name, body)
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	for name, body := range extra {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}
