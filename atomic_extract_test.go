package main

import (
	"archive/tar"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractArchiveAtomic_Fresh(t *testing.T) {
	root := t.TempDir()
	archive := filepath.Join(root, "actions-runner-osx-x64-2.338.2.tar.gz")
	writeTarGz(t, archive, []tarEntry{
		{name: "./config.sh", typeflag: tar.TypeReg, body: "cfg"},
	})
	dir := filepath.Join(root, "runner01")
	if err := extractArchiveAtomic(archive, dir, []string{".runner"}, "2.338.2"); err != nil {
		t.Fatalf("extractArchiveAtomic: %v", err)
	}
	if got := readFileString(t, filepath.Join(dir, "config.sh")); got != "cfg" {
		t.Fatalf("config.sh = %q, want cfg", got)
	}
	if got := installedRunnerVersion(dir); got != "2.338.2" {
		t.Fatalf("version marker = %q, want 2.338.2", got)
	}
}

func TestExtractArchiveAtomic_StaleFilesRemovedPreservedKept(t *testing.T) {
	root := t.TempDir()
	archive := filepath.Join(root, "actions-runner-osx-x64-2.338.2.tar.gz")
	writeTarGz(t, archive, []tarEntry{
		{name: "./config.sh", typeflag: tar.TypeReg, body: "cfg-new"},
	})
	dir := filepath.Join(root, "runner01")
	// Simulate an old install: stale file, configured runner, old marker.
	mustMkdirAll(t, dir)
	writeFileString(t, filepath.Join(dir, "config.sh"), "cfg-old")
	writeFileString(t, filepath.Join(dir, "stale-old-binary"), "stale")
	writeFileString(t, filepath.Join(dir, ".runner"), "{}")
	writeFileString(t, filepath.Join(dir, versionMarkerFile), "2.300.0\n")

	if err := extractArchiveAtomic(archive, dir, []string{".runner", ".credentials"}, "2.338.2"); err != nil {
		t.Fatalf("extractArchiveAtomic: %v", err)
	}
	if got := readFileString(t, filepath.Join(dir, "config.sh")); got != "cfg-new" {
		t.Fatalf("config.sh = %q, want cfg-new", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "stale-old-binary")); !os.IsNotExist(err) {
		t.Fatalf("stale file survived extraction")
	}
	if got := readFileString(t, filepath.Join(dir, ".runner")); got != "{}" {
		t.Fatalf(".runner = %q, want preserved", got)
	}
	if got := installedRunnerVersion(dir); got != "2.338.2" {
		t.Fatalf("version marker = %q, want 2.338.2", got)
	}
	// No staging or backup dirs left behind.
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := e.Name()
		if name != "runner01" && name != filepath.Base(archive) {
			t.Fatalf("unexpected leftover in root: %q", name)
		}
	}
}

func TestExtractArchiveAtomic_CorruptArchiveKeepsPreviousInstall(t *testing.T) {
	root := t.TempDir()
	archive := filepath.Join(root, "broken.tar.gz")
	if err := os.WriteFile(archive, []byte("not a gzip stream"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "runner01")
	mustMkdirAll(t, dir)
	writeFileString(t, filepath.Join(dir, "config.sh"), "cfg-old")
	writeFileString(t, filepath.Join(dir, ".runner"), "{}")

	if err := extractArchiveAtomic(archive, dir, []string{".runner"}, "2.338.2"); err == nil {
		t.Fatal("expected error for corrupt archive")
	}
	if got := readFileString(t, filepath.Join(dir, "config.sh")); got != "cfg-old" {
		t.Fatalf("config.sh = %q, want cfg-old (previous install intact)", got)
	}
	if got := readFileString(t, filepath.Join(dir, ".runner")); got != "{}" {
		t.Fatalf(".runner = %q, want preserved", got)
	}
	// No staging dirs left behind.
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "runner01" && e.Name() != filepath.Base(archive) {
			t.Fatalf("unexpected leftover in root: %q", e.Name())
		}
	}
}

func mustMkdirAll(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeFileString(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
