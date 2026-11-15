package main

import (
	"testing"
)

func TestFakeExecutorRecordsDir(t *testing.T) {
	f := newFakeExecutor()
	f.swap(t)
	dir := t.TempDir()
	if err := runScriptInDir("svc.sh", []string{"install"}, dir); err != nil {
		t.Fatalf("runScriptInDir install: %v", err)
	}
	if err := runScriptInDir("svc.sh", []string{"start"}, dir); err != nil {
		t.Fatalf("runScriptInDir start: %v", err)
	}
	calls := f.Calls()
	if len(calls) != 2 {
		t.Fatalf("calls = %v, want 2", calls)
	}
	if calls[1].Name != "svc.sh" || calls[1].Dir != dir {
		t.Fatalf("call = %+v, want name svc.sh dir %q", calls[1], dir)
	}
	if f.SvcState() != "running" {
		t.Fatalf("svcState = %q, want running", f.SvcState())
	}
}

func TestFakeExecutorFailureInjection(t *testing.T) {
	f := newFakeExecutor()
	f.swap(t)
	f.FailSvcInstall = errTest
	dir := t.TempDir()
	if err := runScriptInDir("svc.sh", []string{"install"}, dir); err == nil {
		t.Fatal("expected injected failure")
	}
	if f.SvcState() != "not-installed" {
		t.Fatalf("svcState = %q, want not-installed after failed install", f.SvcState())
	}
}

func TestFakeExecutorCrashAfterConfig(t *testing.T) {
	f := newFakeExecutor()
	f.swap(t)
	f.CrashConfig = errTest
	dir := t.TempDir()
	err := runScriptInDir(dir+"/config.sh", []string{"--unattended"}, dir)
	if err == nil {
		t.Fatal("expected injected crash")
	}
	// The crash happens *after* the .runner file is written, so the tool
	// must observe a configured-but-incomplete runner on the next run.
	if !isConfigured(dir) {
		t.Fatal(".runner missing after crash-configure; want it present")
	}
}

var errTest = errTestT("test failure")

type errTestT string

func (e errTestT) Error() string { return string(e) }
