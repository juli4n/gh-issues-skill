package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/v69/github"
)

func TestRenderIssue(t *testing.T) {
	ts := func(s string) *github.Timestamp {
		tm, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatal(err)
		}
		return &github.Timestamp{Time: tm}
	}
	issue := &github.Issue{
		Number:      github.Ptr(7),
		Title:       github.Ptr(`Crash on "empty" config`),
		State:       github.Ptr("closed"),
		StateReason: github.Ptr("completed"),
		Labels:      []*github.Label{{Name: github.Ptr("bug")}, {Name: github.Ptr("area/config")}},
		User:        &github.User{Login: github.Ptr("alice")},
		CreatedAt:   ts("2026-01-01T00:00:00Z"),
		UpdatedAt:   ts("2026-01-02T00:00:00Z"),
		ClosedAt:    ts("2026-01-02T00:00:00Z"),
		HTMLURL:     github.Ptr("https://github.com/o/r/issues/7"),
		Body:        github.Ptr("  It crashes.\r\nEvery time.\r\n"),
	}
	comments := []*github.IssueComment{{
		User:      &github.User{Login: github.Ptr("bob")},
		CreatedAt: ts("2026-01-01T12:00:00Z"),
		Body:      github.Ptr("Fixed in #8."),
	}}

	want := `---
number: 7
title: "Crash on \"empty\" config"
state: closed
state_reason: completed
labels: ["bug", "area/config"]
author: alice
created: 2026-01-01T00:00:00Z
updated: 2026-01-02T00:00:00Z
closed: 2026-01-02T00:00:00Z
url: https://github.com/o/r/issues/7
---

# Crash on "empty" config

It crashes.
Every time.

## Comments

### bob at 2026-01-01T12:00:00Z

Fixed in #8.
`
	if got := renderIssue(issue, comments); got != want {
		t.Errorf("renderIssue() =\n%s\nwant:\n%s", got, want)
	}
}

func TestParseLabels(t *testing.T) {
	got := parseLabels(" kind/bug, area/net,,kind/bug ")
	if want := []string{"area/net", "kind/bug"}; !slices.Equal(got, want) {
		t.Errorf("parseLabels() = %q, want %q", got, want)
	}
	if got := parseLabels(""); len(got) != 0 {
		t.Errorf("parseLabels(\"\") = %q, want none", got)
	}
}

func TestSkillName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"cli-issues", "cli-issues"},
		{"My_Repo.JS-issues", "my-repo-js-issues"},
		{"--weird--", "weird"},
		{"...", "issues"},
		{strings.Repeat("a", 70), strings.Repeat("a", 64)},
	}
	for _, tt := range tests {
		if got := skillName(tt.in); got != tt.want {
			t.Errorf("skillName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSyncCommand(t *testing.T) {
	opts := options{owner: "o", name: "r", labels: []string{"good first issue"}, state: "open"}
	want := "gh-issues-skill -repo o/r -label 'good first issue' -state open"
	if got := syncCommand(opts); got != want {
		t.Errorf("syncCommand() = %q, want %q", got, want)
	}
}

func TestManifestRoundTrip(t *testing.T) {
	opts := options{owner: "o", name: "r", labels: []string{"bug"}, state: "all", outDir: t.TempDir()}

	if m, err := readManifest(opts.outDir); err != nil || m != nil {
		t.Fatalf("readManifest() on a fresh directory = %v, %v; want nil, nil", m, err)
	}
	syncedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := writeManifest(opts, syncedAt); err != nil {
		t.Fatal(err)
	}
	m, err := readManifest(opts.outDir)
	if err != nil {
		t.Fatal(err)
	}
	if !m.SyncedAt.Equal(syncedAt) {
		t.Errorf("SyncedAt = %v, want %v", m.SyncedAt, syncedAt)
	}
	if !m.matches(opts) {
		t.Errorf("manifest %+v does not match the options it was written with", m)
	}
	other := opts
	other.labels = nil
	if m.matches(other) {
		t.Errorf("manifest %+v matches options with different labels", m)
	}
}

func TestWriteSkillKeepsEdits(t *testing.T) {
	opts := options{owner: "o", name: "r", state: "all", outDir: filepath.Join(t.TempDir(), "r-issues")}
	if err := os.MkdirAll(opts.outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeSkill(opts); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(opts.outDir, "SKILL.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "---\nname: r-issues\n") {
		t.Errorf("SKILL.md does not start with the expected frontmatter:\n%s", data)
	}

	if err := os.WriteFile(path, []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeSkill(opts); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); string(data) != "edited" {
		t.Errorf("writeSkill overwrote an existing SKILL.md")
	}
}

func TestProgressOnTerminal(t *testing.T) {
	var out strings.Builder
	p := startProgress(&out, true, "Syncing o/r issues")
	p.issueSynced()
	p.issueSynced()
	p.setWaiting("rate limited, resuming at 12:00:00")
	time.Sleep(250 * time.Millisecond)
	p.finish()

	got := out.String()
	if want := "Syncing o/r issues: 2 synced (rate limited, resuming at 12:00:00)"; !strings.Contains(got, want) {
		t.Errorf("status line %q not found in output %q", want, got)
	}
	if !strings.HasSuffix(got, "\r\033[K") {
		t.Errorf("finish did not clear the status line: %q", got)
	}
}

func TestProgressWithoutTerminal(t *testing.T) {
	var out strings.Builder
	p := startProgress(&out, false, "Syncing o/r issues")
	p.issueSynced()
	p.setWaiting("rate limited, resuming at 12:00:00")
	p.setWaiting("")
	p.finish()

	if want := "Syncing o/r issues\nrate limited, resuming at 12:00:00\n"; out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
}
