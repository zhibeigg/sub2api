package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func extensionSource(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "..", "extensions", "cursor-cookie-importer"))
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPackageExtensionIsStableAndInjectsVersion(t *testing.T) {
	temp := t.TempDir()
	versionFile := filepath.Join(temp, "VERSION")
	if err := os.WriteFile(versionFile, []byte("1.23.456\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	first := filepath.Join(temp, "first.zip")
	second := filepath.Join(temp, "second.zip")

	firstDigest, err := packageExtension(extensionSource(t), versionFile, first)
	if err != nil {
		t.Fatal(err)
	}
	secondDigest, err := packageExtension(extensionSource(t), versionFile, second)
	if err != nil {
		t.Fatal(err)
	}
	firstBytes, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	secondBytes, err := os.ReadFile(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstBytes, secondBytes) || firstDigest != secondDigest {
		t.Fatal("stable packages differ")
	}
	sum := sha256.Sum256(firstBytes)
	if firstDigest != hex.EncodeToString(sum[:]) {
		t.Fatal("reported digest does not match ZIP")
	}
	sidecar, err := os.ReadFile(first + ".sha256")
	if err != nil {
		t.Fatal(err)
	}
	if string(sidecar) != firstDigest+"  first.zip\n" {
		t.Fatalf("unexpected sidecar: %q", sidecar)
	}

	reader, err := zip.NewReader(bytes.NewReader(firstBytes), int64(len(firstBytes)))
	if err != nil {
		t.Fatal(err)
	}
	if len(reader.File) != len(archiveWhitelist) {
		t.Fatalf("got %d archive entries, want %d", len(reader.File), len(archiveWhitelist))
	}
	expectedNames := append([]string(nil), archiveWhitelist...)
	sort.Strings(expectedNames)
	for index, file := range reader.File {
		if file.Name != expectedNames[index] {
			t.Fatalf("entry %d is %q, want %q", index, file.Name, expectedNames[index])
		}
		if !file.Modified.Equal(fixedTimestamp) {
			t.Fatalf("entry %s has unstable timestamp %s", file.Name, file.Modified)
		}
		if file.Name == "manifest.json" {
			stream, err := file.Open()
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(stream)
			stream.Close()
			if err != nil {
				t.Fatal(err)
			}
			var value manifest
			if err := json.Unmarshal(data, &value); err != nil {
				t.Fatal(err)
			}
			if value.Version != "1.23.456" {
				t.Fatalf("manifest version is %q", value.Version)
			}
		}
	}
}

func TestPackageExtensionRejectsInvalidVersion(t *testing.T) {
	for _, version := range []string{"v1.2.3", "1.2.65536"} {
		t.Run(version, func(t *testing.T) {
			temp := t.TempDir()
			versionFile := filepath.Join(temp, "VERSION")
			if err := os.WriteFile(versionFile, []byte(version), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := packageExtension(extensionSource(t), versionFile, filepath.Join(temp, "out.zip")); err == nil {
				t.Fatal("expected invalid VERSION to be rejected")
			}
		})
	}
}

func TestManifestRejectsNonWhitelistedReference(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(extensionSource(t), "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	raw["options_page"] = "unexpected.html"
	data, err = json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	allowed := make(map[string]struct{}, len(archiveWhitelist))
	for _, name := range archiveWhitelist {
		allowed[name] = struct{}{}
	}
	_, err = injectAndValidateManifest(data, "1.2.3", allowed)
	if err == nil || !strings.Contains(err.Error(), "non-whitelisted") {
		t.Fatalf("expected whitelist error, got %v", err)
	}
}
