// Command gh-issues-skill mirrors the issues of a GitHub repository into a local
// directory, one markdown file per issue, and generates a Claude Code skill
// that explains how to search them.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/google/go-github/v69/github"
)

// options are the settings for one sync, taken from the command line.
type options struct {
	owner, name string
	labels      []string // only issues carrying all of these are synced; sorted
	state       string   // "open", "closed" or "all"
	outDir      string
	full        bool
}

func (o options) repo() string { return o.owner + "/" + o.name }

func main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: gh-issues-skill -repo owner/name [flags]\n\nFlags:\n")
		flag.PrintDefaults()
	}
	repo := flag.String("repo", "", "repository to mirror, as `owner/name` (required)")
	outDir := flag.String("out", "", "output `directory` (default \".claude/skills/<name>-issues\")")
	labels := flag.String("label", "", "comma-separated `labels`; only issues with all of them are synced")
	state := flag.String("state", "all", "which issues to sync: open, closed or all")
	full := flag.Bool("full", false, "refetch all issues, not just those updated since the last sync")
	flag.Parse()

	owner, name, ok := strings.Cut(*repo, "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") {
		usageError("-repo owner/name is required")
	}
	if flag.NArg() > 0 {
		usageError("unexpected argument %q", flag.Arg(0))
	}
	if !slices.Contains([]string{"open", "closed", "all"}, *state) {
		usageError("-state must be open, closed or all, not %q", *state)
	}

	opts := options{
		owner:  owner,
		name:   name,
		labels: parseLabels(*labels),
		state:  *state,
		outDir: *outDir,
		full:   *full,
	}
	if opts.outDir == "" {
		opts.outDir = filepath.Join(".claude", "skills", skillName(name+"-issues"))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, opts); err != nil {
		fmt.Fprintf(os.Stderr, "gh-issues-skill: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, opts options) error {
	prev, err := readManifest(opts.outDir)
	if err != nil {
		return err
	}
	var since time.Time
	if prev != nil {
		if !prev.matches(opts) {
			return fmt.Errorf("%s mirrors %s; use another -out directory or delete it first",
				opts.outDir, prev.describe())
		}
		if !opts.full {
			since = prev.SyncedAt
		}
	}

	token, err := authToken()
	if err != nil {
		return err
	}
	client := github.NewClient(nil).WithAuthToken(token)

	issuesDir := filepath.Join(opts.outDir, "issues")
	if err := os.MkdirAll(issuesDir, 0o755); err != nil {
		return err
	}
	status := fmt.Sprintf("Syncing %s issues (full sync)", opts.repo())
	if !since.IsZero() {
		status = fmt.Sprintf("Syncing %s issues updated since %s", opts.repo(), since.Local().Format("2006-01-02 15:04 MST"))
	}
	p := startProgress(os.Stderr, isTerminal(os.Stderr), status)
	syncedAt, written, err := syncIssues(ctx, client, opts, since, issuesDir, p)
	p.finish()
	if err != nil {
		return err
	}
	if err := writeSkill(opts); err != nil {
		return err
	}
	if err := writeManifest(opts, syncedAt); err != nil {
		return err
	}
	fmt.Printf("wrote %d issues from %s into %s\n", written, opts.repo(), issuesDir)
	return nil
}

// parseLabels splits a comma-separated list into sorted, de-duplicated labels.
func parseLabels(s string) []string {
	var labels []string
	for _, l := range strings.Split(s, ",") {
		if l = strings.TrimSpace(l); l != "" {
			labels = append(labels, l)
		}
	}
	slices.Sort(labels)
	return slices.Compact(labels)
}

func usageError(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "gh-issues-skill: "+format+"\n\n", args...)
	flag.Usage()
	os.Exit(2)
}
