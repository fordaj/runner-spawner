package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// versionedAsset returns the download URL and cache filename for the pinned
// actions/runner release on this host platform.
func versionedAsset(cfg *Config, p platform) (url, filename string, err error) {
	if p.RunnerAsset == "" {
		return "", "", fmt.Errorf("unsupported platform %s/%s: no actions-runner asset", p.GOOS, p.GOARCH)
	}
	ver := strings.TrimPrefix(cfg.RunnerVersion, "v")
	if ver == "" {
		return "", "", fmt.Errorf("runner version must not be empty")
	}
	filename = fmt.Sprintf("%s-%s%s", p.RunnerAsset, ver, p.ArchiveExt)
	url = fmt.Sprintf("https://github.com/actions/runner/releases/download/v%s/%s", ver, filename)
	return url, filename, nil
}

// ensureArchive makes sure the pinned archive exists in cfg's cache dir and
// returns its path. Downloads go to a temporary file in the cache dir and
// are atomically renamed, so interrupted downloads never poison the cache.
func ensureArchive(cfg *Config, p platform, client *http.Client, logf func(string)) (string, error) {
	url, filename, err := versionedAsset(cfg, p)
	if err != nil {
		return "", err
	}
	cacheDir, err := cfg.ArchiveCache()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", err
	}
	dst := filepath.Join(cacheDir, filename)
	if fi, err := os.Stat(dst); err == nil && fi.Size() > 0 {
		logf(fmt.Sprintf("using cached %s", dst))
		return dst, nil
	}
	if client == nil {
		client = http.DefaultClient
	}
	tmp, err := os.CreateTemp(cacheDir, ".download-*.tmp")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after rename
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		tmp.Close()
		return "", err
	}
	logf(fmt.Sprintf("downloading %s", url))
	resp, err := client.Do(req)
	if err != nil {
		tmp.Close()
		return "", fmt.Errorf("downloading %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		tmp.Close()
		return "", fmt.Errorf("downloading %s: %s", url, resp.Status)
	}
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		return "", fmt.Errorf("downloading %s: %w", url, err)
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if fi, err := os.Stat(tmpName); err != nil || fi.Size() == 0 {
		return "", fmt.Errorf("downloaded %s is empty", url)
	}
	if err := os.Rename(tmpName, dst); err != nil {
		return "", err
	}
	logf(fmt.Sprintf("cached %s", dst))
	return dst, nil
}

// versionMarkerFile records which actions/runner version populated the
// install dir. create reads it to decide whether a re-extract is needed.
const versionMarkerFile = ".runner-spawner.version"

// extractArchiveAtomic unpacks the archive into a staging directory
// next to dir, carries over `preserve` files from any previous install,
// stamps the version marker, and then swaps the staging directory into
// place. An interrupted extraction therefore never poisons an existing
// install: the swap only happens after the archive fully unpacked, and
// stale files from an older runner version never survive it.
func extractArchiveAtomic(archivePath, dir string, preserve []string, version string) error {
	parent := filepath.Dir(dir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(parent, "."+filepath.Base(dir)+".extract-*")
	if err != nil {
		return err
	}
	// If anything fails before the swap, the stale staging dir is removed.
	committed := false
	defer func() {
		if !committed {
			_ = os.RemoveAll(staging)
		}
	}()
	if err := extractArchive(archivePath, staging); err != nil {
		return fmt.Errorf("extracting %s: %w", filepath.Base(archivePath), err)
	}
	for _, name := range preserve {
		src := filepath.Join(dir, name)
		if fi, err := os.Stat(src); err == nil && fi.Mode().IsRegular() {
			if err := copyFile(src, filepath.Join(staging, name)); err != nil {
				return fmt.Errorf("preserving %s: %w", name, err)
			}
		}
	}
	if version != "" {
		ver := strings.TrimPrefix(version, "v")
		if err := os.WriteFile(filepath.Join(staging, versionMarkerFile), []byte(ver+"\n"), 0o644); err != nil {
			return fmt.Errorf("writing version marker: %w", err)
		}
	}
	backup := ""
	if runnerExists(dir) {
		backup = dir + ".old-" + time.Now().UTC().Format("20060102150405.000000000")
		if err := os.Rename(dir, backup); err != nil {
			return fmt.Errorf("moving %s aside before install: %w", dir, err)
		}
	}
	if err := os.Rename(staging, dir); err != nil {
		// Roll back: put the previous install back if we created one.
		if backup != "" {
			_ = os.Rename(backup, dir)
		}
		return fmt.Errorf("installing %s: %w", dir, err)
	}
	committed = true
	if backup != "" {
		_ = os.RemoveAll(backup)
	}
	return nil
}

// installedRunnerVersion returns the actions/runner version recorded in
// the version marker, or "" when the marker is absent (fresh or
// hand-installed dir).
func installedRunnerVersion(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, versionMarkerFile))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	fi, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, fi.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// extractArchive unpacks a .tar.gz or .zip actions-runner archive into dir.
// Pure archive/zip + archive/tar replace the old `tar` + 7za shell-outs, so
// no external unzip tool is required. Paths are validated (no ".." escape).
func extractArchive(archivePath, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if strings.HasSuffix(archivePath, ".zip") {
		return extractZip(archivePath, dir)
	}
	return extractTarGz(archivePath, dir)
}

func extractTarGz(archivePath, dir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("opening %s: %w", archivePath, err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		// The published actions/runner tarballs start with a "." / "./"
		// directory entry; the destination dir already exists, so skip it.
		if filepath.Clean(hdr.Name) == "." {
			continue
		}
		target, err := safeJoin(dir, hdr.Name)
		if err != nil {
			return err
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if hdr.Linkname == "" {
				continue
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			_ = os.Remove(target)
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			if hdr.Mode != 0 {
				_ = os.Chmod(target, os.FileMode(hdr.Mode).Perm())
			}
			out.Close()
		}
	}
}

func extractZip(archivePath, dir string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("opening %s: %w", archivePath, err)
	}
	defer r.Close()
	for _, f := range r.File {
		// Skip zip entries for the archive root itself (".", "./").
		if filepath.Clean(f.Name) == "." {
			continue
		}
		target, err := safeJoin(dir, f.Name)
		if err != nil {
			return err
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			rc.Close()
			return err
		}
		_, copyErr := io.Copy(out, rc)
		rc.Close()
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		// Preserve the executable bit from the zip (config.cmd etc.).
		if f.FileInfo().Mode().Perm() != 0 {
			_ = os.Chmod(target, f.FileInfo().Mode().Perm())
		}
	}
	return nil
}

func safeJoin(root, name string) (string, error) {
	// Reject absolute paths and .. escapes.
	if filepath.IsAbs(name) {
		return "", fmt.Errorf("archive contains absolute path %q", name)
	}
	clean := filepath.Clean(name)
	if clean == "." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return "", fmt.Errorf("archive contains escaping path %q", name)
	}
	return filepath.Join(root, clean), nil
}

// defaultHTTPClient bounds downloads so a hung mirror cannot wedge the CLI.
func defaultHTTPClient() *http.Client {
	return &http.Client{Timeout: 10 * time.Minute}
}
