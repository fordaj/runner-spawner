package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

// platform describes the host's runner support and service mechanics.
type platform struct {
	GOOS   string // darwin, windows, linux
	GOARCH string // amd64, arm64

	// RunnerAsset is the actions/runner release asset stem (without the
	// version + extension), e.g. "actions-runner-osx-x64".
	RunnerAsset string
	// ArchiveExt is ".tar.gz" on Unix, ".zip" on Windows.
	ArchiveExt string
	// ConfigBin is "config.sh" or "config.cmd".
	ConfigBin string
	// ServiceBin is "svc.sh" on Unix; empty on Windows (sc.exe).
	ServiceBin string
	// IsWindows reports the Windows service path.
	IsWindows bool
	// Supported reports whether create/remove/start/stop are implemented.
	Supported bool
	// OSLabel / ArchLabel are the Actions labels (e.g. macos, x64).
	OSLabel   string
	ArchLabel string
}

func currentPlatform() platform {
	return platformFor(runtime.GOOS, runtime.GOARCH, hostname())
}

func platformFor(goos, goarch, hostname string) platform {
	p := platform{GOOS: goos, GOARCH: goarch}
	_ = hostname
	switch goos {
	case "darwin":
		switch goarch {
		case "amd64":
			p.RunnerAsset, p.OSLabel, p.ArchLabel = "actions-runner-osx-x64", "macos", "x64"
		case "arm64":
			p.RunnerAsset, p.OSLabel, p.ArchLabel = "actions-runner-osx-arm64", "macos", "ARM64"
		}
		p.ArchiveExt, p.ConfigBin, p.ServiceBin, p.Supported = ".tar.gz", "config.sh", "svc.sh", true
	case "windows":
		switch goarch {
		case "amd64":
			p.RunnerAsset, p.OSLabel, p.ArchLabel = "actions-runner-win-x64", "windows", "x64"
		case "arm64":
			p.RunnerAsset, p.OSLabel, p.ArchLabel = "actions-runner-win-arm64", "windows", "ARM64"
		}
		p.ArchiveExt, p.ConfigBin, p.IsWindows, p.Supported = ".zip", "config.cmd", true, true
	case "linux":
		// Names are filled in so status/list diagnostics can still describe
		// the host, but the runner verbs refuse with a clear message.
		switch goarch {
		case "amd64":
			p.RunnerAsset, p.OSLabel, p.ArchLabel = "actions-runner-linux-x64", "linux", "x64"
		case "arm64":
			p.RunnerAsset, p.OSLabel, p.ArchLabel = "actions-runner-linux-arm64", "linux", "ARM64"
		}
		p.ArchiveExt, p.ConfigBin, p.ServiceBin = ".tar.gz", "config.sh", "svc.sh"
	default:
		p.Supported = false
	}
	return p
}

// Labels returns the runner labels: self-hosted, <os>, <arch>, <host>,
// <host>-<name>. An explicit cfg.Labels list (from --labels /
// RUNNER_SPAWNER_LABELS / .runner-spawner.json) disables auto-detection.
func (p platform) Labels(cfg *Config, name string) []string {
	if len(cfg.Labels) > 0 {
		return append([]string(nil), cfg.Labels...)
	}
	host := hostname()
	return []string{"self-hosted", p.OSLabel, p.ArchLabel, host, host + "-" + name}
}

// runnerURL is the GitHub repository URL the runner attaches to.
func (cfg *Config) runnerURL() string {
	return "https://github.com/" + cfg.Owner + "/" + cfg.Repo
}

func hostname() string {
	h, err := os.Hostname()
	if err != nil || strings.TrimSpace(h) == "" {
		return "host"
	}
	return strings.TrimSpace(h)
}

// requireSupported refuses the mutating verbs on unsupported platforms.
func (p platform) requireSupported(verb string) error {
	if p.Supported {
		if p.RunnerAsset == "" {
			return fmt.Errorf("%s: unsupported platform %s/%s", verb, p.GOOS, p.GOARCH)
		}
		return nil
	}
	if p.GOOS == "linux" {
		return fmt.Errorf("%s: Linux runners are not yet supported (only macOS and Windows); archive asset %s is resolved but service install is unimplemented", verb, p.RunnerAsset)
	}
	return fmt.Errorf("%s: unsupported platform %s/%s (supported: darwin/amd64, darwin/arm64, windows/amd64, windows/arm64)", verb, p.GOOS, p.GOARCH)
}

// adminCheck enforces the Windows elevation requirement (`net session`),
// mirroring the old _admin_check.sh. No-op on non-Windows.
func adminCheck(p platform) error {
	if !p.IsWindows {
		return nil
	}
	if err := runCmd("net", "session"); err != nil {
		return fmt.Errorf("This script must be run with admin privileges")
	}
	return nil
}
