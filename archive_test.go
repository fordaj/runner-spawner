package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func writeTarGz(t *testing.T, path string, entries []tarEntry) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		hdr := &tar.Header{
			Name:     e.name,
			Mode:     0o755,
			Typeflag: e.typeflag,
			Size:     int64(len(e.body)),
		}
		if e.typeflag == tar.TypeDir {
			hdr.Size = 0
			hdr.Mode = 0o755
		} else if e.typeflag == tar.TypeReg {
			hdr.Mode = 0o755
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("WriteHeader(%q): %v", e.name, err)
		}
		if e.typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatalf("Write(%q): %v", e.name, err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

type tarEntry struct {
	name     string
	typeflag byte
	body     string
}

// TestExtractTarGz_DotPrefix reproduces the actions/runner tarball layout:
// the first member is "." and subsequent members use "./" prefixes.
func TestExtractTarGz_DotPrefix(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "actions-runner-osx-x64-2.338.2.tar.gz")
	writeTarGz(t, archive, []tarEntry{
		{name: ".", typeflag: tar.TypeDir},
		{name: "./safe_sleep.sh", typeflag: tar.TypeReg, body: "#!/bin/sh\n"},
		{name: "./bin", typeflag: tar.TypeDir},
		{name: "./bin/runner", typeflag: tar.TypeReg, body: "binary"},
	})

	dst := filepath.Join(dir, "runner01")
	if err := extractArchive(archive, dst); err != nil {
		t.Fatalf("extractArchive: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dst, "safe_sleep.sh"))
	if err != nil {
		t.Fatalf("ReadFile safe_sleep.sh: %v", err)
	}
	if string(got) != "#!/bin/sh\n" {
		t.Fatalf("safe_sleep.sh = %q, want %q", got, "#!/bin/sh\n")
	}
	got, err = os.ReadFile(filepath.Join(dst, "bin", "runner"))
	if err != nil {
		t.Fatalf("ReadFile bin/runner: %v", err)
	}
	if string(got) != "binary" {
		t.Fatalf("bin/runner = %q, want %q", got, "binary")
	}
	// Executable bit must survive.
	fi, err := os.Stat(filepath.Join(dst, "safe_sleep.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm()&0o100 == 0 {
		t.Fatalf("safe_sleep.sh mode = %o, want executable bit set", fi.Mode().Perm())
	}
}

// TestExtractTarGz_DotSlashDirEntry covers the "./" spelling of the root entry.
func TestExtractTarGz_DotSlashDirEntry(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "a.tar.gz")
	writeTarGz(t, archive, []tarEntry{
		{name: "./", typeflag: tar.TypeDir},
		{name: "./config.sh", typeflag: tar.TypeReg, body: "cfg"},
	})
	dst := filepath.Join(dir, "out")
	if err := extractArchive(archive, dst); err != nil {
		t.Fatalf("extractArchive: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dst, "config.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "cfg" {
		t.Fatalf("config.sh = %q, want %q", got, "cfg")
	}
}

// TestExtractTarGz_EscapingPath checks that ".." entries are still rejected.
func TestExtractTarGz_EscapingPath(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "a.tar.gz")
	writeTarGz(t, archive, []tarEntry{
		{name: ".", typeflag: tar.TypeDir},
		{name: "../evil.txt", typeflag: tar.TypeReg, body: "evil"},
	})
	dst := filepath.Join(dir, "out")
	if err := extractArchive(archive, dst); err == nil {
		t.Fatal("expected error for ../evil.txt, got nil")
	}
	if _, err := os.Stat(filepath.Join(dir, "evil.txt")); err == nil {
		t.Fatal("evil.txt escaped the destination dir")
	}
}

// TestExtractZip_DotPrefix covers zip entries with "./" prefixes and a "./" dir entry.
func TestExtractZip_DotPrefix(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "a.zip")

	f, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for _, name := range []string{"./", "./config.cmd", "./sub/file.txt"} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("Create(%q): %v", name, err)
		}
		if name == "./sub/file.txt" {
			if _, err := io.WriteString(w, "hello"); err != nil {
				t.Fatal(err)
			}
		} else if name != "./" {
			if _, err := io.WriteString(w, "cfg"); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(dir, "out")
	if err := extractArchive(archive, dst); err != nil {
		t.Fatalf("extractArchive: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dst, "config.cmd"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "cfg" {
		t.Fatalf("config.cmd = %q, want %q", got, "cfg")
	}
	got, err = os.ReadFile(filepath.Join(dst, "sub", "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello" {
		t.Fatalf("sub/file.txt = %q, want %q", got, "hello")
	}
}
