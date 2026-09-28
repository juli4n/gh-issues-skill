package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const manifestFile = ".gh-issues-skill.json"

// manifest records what an output directory mirrors and when it was last
// synced, so that the next sync only fetches issues updated since then.
type manifest struct {
	Repo     string    `json:"repo"`
	Labels   []string  `json:"labels,omitempty"`
	State    string    `json:"state"`
	SyncedAt time.Time `json:"synced_at"`
}

// readManifest returns the manifest in dir, or nil if dir has never been synced.
func readManifest(dir string) (*manifest, error) {
	path := filepath.Join(dir, manifestFile)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return &m, nil
}

func writeManifest(opts options, syncedAt time.Time) error {
	m := manifest{Repo: opts.repo(), Labels: opts.labels, State: opts.state, SyncedAt: syncedAt}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(opts.outDir, manifestFile), append(data, '\n'), 0o644)
}

// matches reports whether m was synced with the same repository and filters as
// opts. Syncing with different filters into the same directory would mix issues
// that match the new filters with stale ones that don't.
func (m *manifest) matches(opts options) bool {
	return strings.EqualFold(m.Repo, opts.repo()) &&
		slices.Equal(m.Labels, opts.labels) &&
		m.State == opts.state
}

func (m *manifest) describe() string {
	s := fmt.Sprintf("%s (-state %s", m.Repo, m.State)
	if len(m.Labels) > 0 {
		s += " -label " + strings.Join(m.Labels, ",")
	}
	return s + ")"
}
