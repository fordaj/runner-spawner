package main

import "testing"

func TestParseRemoteURL(t *testing.T) {
	cases := []struct {
		in          string
		owner, repo string
		wantOK      bool
	}{
		{"git@github.com:fordaj/AirFrame.git", "fordaj", "AirFrame", true},
		{"git@github.com:fordaj/AirFrame", "fordaj", "AirFrame", true},
		{"ssh://git@github.com/fordaj/AirFrame.git", "fordaj", "AirFrame", true},
		{"https://github.com/fordaj/AirFrame.git", "fordaj", "AirFrame", true},
		{"https://github.com/fordaj/AirFrame", "fordaj", "AirFrame", true},
		{"https://github.com/fordaj/AirFrame/", "fordaj", "AirFrame", true},
		{"git://github.com/fordaj/AirFrame.git", "fordaj", "AirFrame", true},
		{"", "", "", false},
		{"https://github.com/", "", "", false},
	}
	for _, tc := range cases {
		o, r, err := parseRemoteURL(tc.in)
		if tc.wantOK {
			if err != nil {
				t.Fatalf("parseRemoteURL(%q): unexpected error %v", tc.in, err)
			}
			if o != tc.owner || r != tc.repo {
				t.Fatalf("parseRemoteURL(%q) = %q/%q, want %q/%q", tc.in, o, r, tc.owner, tc.repo)
			}
		} else if err == nil {
			t.Fatalf("parseRemoteURL(%q): expected error, got %q/%q", tc.in, o, r)
		}
	}
}

func TestPlatformFor(t *testing.T) {
	p := platformFor("darwin", "amd64", "maestro")
	if p.RunnerAsset != "actions-runner-osx-x64" || p.ArchiveExt != ".tar.gz" || !p.Supported {
		t.Fatalf("darwin/amd64 = %+v", p)
	}
	if p.Labels(cfgWithLabels(nil), "runner01")[1] != "macos" {
		t.Fatalf("darwin labels = %+v, want os macos", p.Labels(cfgWithLabels(nil), "runner01"))
	}
	if p.Labels(cfgWithLabels([]string{"custom"}), "runner01")[0] != "custom" {
		t.Fatalf("explicit labels not honoured")
	}
	w := platformFor("windows", "amd64", "maestro")
	if w.RunnerAsset != "actions-runner-win-x64" || w.ArchiveExt != ".zip" || !w.Supported || !w.IsWindows {
		t.Fatalf("windows/amd64 = %+v", w)
	}
	l := platformFor("linux", "amd64", "maestro")
	if l.Supported {
		if err := l.requireSupported("create"); err == nil {
			t.Fatal("linux create must be unsupported in v1")
		}
	}
}

func TestVersionedAsset(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RunnerVersion = "2.338.2"
	url, file, err := versionedAsset(cfg, platformFor("darwin", "arm64", "maestro"))
	if err != nil {
		t.Fatal(err)
	}
	if file != "actions-runner-osx-arm64-2.338.2.tar.gz" {
		t.Fatalf("file = %q", file)
	}
	if url != "https://github.com/actions/runner/releases/download/v2.338.2/actions-runner-osx-arm64-2.338.2.tar.gz" {
		t.Fatalf("url = %q", url)
	}
}

func cfgWithLabels(l []string) *Config {
	cfg := DefaultConfig()
	cfg.Labels = l
	return cfg
}

func TestLabelsAuto(t *testing.T) {
	p := platformFor("darwin", "amd64", "maestro")
	labels := p.Labels(DefaultConfig(), "runner01")
	if len(labels) != 5 || labels[0] != "self-hosted" || labels[1] != "macos" || labels[2] != "x64" || labels[3] != "maestro" || labels[4] != "maestro-runner01" {
		t.Fatalf("labels = %+v, want [self-hosted macos x64 maestro maestro-runner01]", labels)
	}
}
