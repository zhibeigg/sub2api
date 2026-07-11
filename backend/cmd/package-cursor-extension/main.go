package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]{0,4})\.(0|[1-9][0-9]{0,4})\.(0|[1-9][0-9]{0,4})$`)

var archiveWhitelist = []string{
	"icons/icon16.png",
	"icons/icon32.png",
	"icons/icon48.png",
	"icons/icon128.png",
	"manifest.json",
	"options.css",
	"options.html",
	"options.js",
	"page-bridge.js",
	"service-worker.js",
	"shared.js",
}

var fixedTimestamp = time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC)

type manifest struct {
	ManifestVersion         int               `json:"manifest_version"`
	Name                    string            `json:"name"`
	Description             string            `json:"description"`
	Version                 string            `json:"version"`
	Permissions             []string          `json:"permissions"`
	HostPermissions         []string          `json:"host_permissions"`
	OptionalHostPermissions []string          `json:"optional_host_permissions"`
	Background              map[string]any    `json:"background"`
	OptionsPage             string            `json:"options_page"`
	Icons                   map[string]string `json:"icons"`
	ContentScripts          []manifestContent `json:"content_scripts"`
}

type manifestContent struct {
	Matches []string `json:"matches"`
	JS      []string `json:"js"`
	RunAt   string   `json:"run_at"`
}

func main() {
	source := flag.String("source", "../extensions/cursor-cookie-importer", "extension source directory")
	versionFile := flag.String("version-file", "cmd/server/VERSION", "file containing the extension version")
	output := flag.String("output", "cursor-cookie-importer.zip", "output ZIP path")
	flag.Parse()

	digest, err := packageExtension(*source, *versionFile, *output)
	if err != nil {
		fmt.Fprintln(os.Stderr, "package cursor extension:", err)
		os.Exit(1)
	}
	fmt.Printf("%s  %s\n", digest, filepath.Base(*output))
	fmt.Println(*output + ".sha256")
}

func packageExtension(source, versionFile, output string) (string, error) {
	versionBytes, err := os.ReadFile(versionFile)
	if err != nil {
		return "", fmt.Errorf("read VERSION: %w", err)
	}
	version := strings.TrimSpace(string(versionBytes))
	if !validExtensionVersion(version) {
		return "", fmt.Errorf("VERSION %q must be A.B.C with numeric components up to 65535", version)
	}

	files, err := loadWhitelistedFiles(source, version)
	if err != nil {
		return "", err
	}
	archive, err := buildStableZIP(files)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
		return "", fmt.Errorf("create output directory: %w", err)
	}
	if err := os.WriteFile(output, archive, 0o644); err != nil {
		return "", fmt.Errorf("write ZIP: %w", err)
	}
	sum := sha256.Sum256(archive)
	digest := hex.EncodeToString(sum[:])
	sidecar := fmt.Sprintf("%s  %s\n", digest, filepath.Base(output))
	if err := os.WriteFile(output+".sha256", []byte(sidecar), 0o644); err != nil {
		return "", fmt.Errorf("write sha256: %w", err)
	}
	return digest, nil
}

func loadWhitelistedFiles(source, version string) (map[string][]byte, error) {
	allowed := make(map[string]struct{}, len(archiveWhitelist))
	files := make(map[string][]byte, len(archiveWhitelist))
	for _, name := range archiveWhitelist {
		allowed[name] = struct{}{}
		fullPath := filepath.Join(source, filepath.FromSlash(name))
		info, err := os.Lstat(fullPath)
		if err != nil {
			return nil, fmt.Errorf("required file %s: %w", name, err)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("required file %s is not a regular file", name)
		}
		data, err := os.ReadFile(fullPath)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		files[name] = data
	}

	injected, err := injectAndValidateManifest(files["manifest.json"], version, allowed)
	if err != nil {
		return nil, err
	}
	files["manifest.json"] = injected
	return files, nil
}

func injectAndValidateManifest(data []byte, version string, allowed map[string]struct{}) ([]byte, error) {
	var value manifest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}
	if value.ManifestVersion != 3 {
		return nil, fmt.Errorf("manifest_version must be 3")
	}
	if !equalStrings(value.Permissions, []string{"alarms", "cookies", "scripting", "storage"}) {
		return nil, fmt.Errorf("manifest permissions differ from the approved whitelist")
	}
	if !equalStrings(value.HostPermissions, []string{"https://cursor.com/*"}) {
		return nil, fmt.Errorf("manifest host_permissions differ from the approved whitelist")
	}
	if !equalStrings(value.OptionalHostPermissions, []string{"https://*/*"}) {
		return nil, fmt.Errorf("manifest optional_host_permissions differ from the approved whitelist")
	}

	references := []string{value.OptionsPage}
	if worker, ok := value.Background["service_worker"].(string); ok {
		references = append(references, worker)
	} else {
		return nil, fmt.Errorf("manifest background.service_worker is missing")
	}
	for _, icon := range value.Icons {
		references = append(references, icon)
	}
	for _, script := range value.ContentScripts {
		references = append(references, script.JS...)
	}
	for _, reference := range references {
		if _, ok := allowed[reference]; !ok {
			return nil, fmt.Errorf("manifest references non-whitelisted file %q", reference)
		}
	}

	value.Version = version
	result, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode manifest: %w", err)
	}
	return append(result, '\n'), nil
}

func validExtensionVersion(version string) bool {
	if !versionPattern.MatchString(version) {
		return false
	}
	for _, component := range strings.Split(version, ".") {
		value, err := strconv.Atoi(component)
		if err != nil || value > 65535 {
			return false
		}
	}
	return true
}

func equalStrings(actual, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	for index := range expected {
		if actual[index] != expected[index] {
			return false
		}
	}
	return true
}

func buildStableZIP(files map[string][]byte) ([]byte, error) {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, name := range names {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetModTime(fixedTimestamp)
		header.SetMode(0o644)
		entry, err := writer.CreateHeader(header)
		if err != nil {
			return nil, fmt.Errorf("create ZIP entry %s: %w", name, err)
		}
		if _, err := io.Copy(entry, bytes.NewReader(files[name])); err != nil {
			return nil, fmt.Errorf("write ZIP entry %s: %w", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close ZIP: %w", err)
	}
	return buffer.Bytes(), nil
}
