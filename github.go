package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/go-github/v69/github"
)

// syncOverlap is how far before GitHub's reported time the next incremental
// sync starts. The Date header is set after GitHub runs the query and has
// one-second resolution, so without this an issue updated in between could be
// missed. Refetching an issue just rewrites its file, so overlap is harmless.
const syncOverlap = time.Minute

// maxRateLimitRetries bounds how many times a rate-limited request is retried.
const maxRateLimitRetries = 3

// authToken returns $GITHUB_TOKEN or $GH_TOKEN if set, and otherwise asks the
// GitHub CLI.
func authToken() (string, error) {
	for _, env := range []string{"GITHUB_TOKEN", "GH_TOKEN"} {
		if token := os.Getenv(env); token != "" {
			return token, nil
		}
	}
	out, err := exec.Command("gh", "auth", "token").Output()
	if err != nil {
		return "", fmt.Errorf("GITHUB_TOKEN is not set and `gh auth token` failed: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// syncIssues writes every issue matching opts that was updated since the given
// time (or every issue, if since is zero) into dir. It returns the time to pass
// as since on the next sync and the number of issues written.
func syncIssues(ctx context.Context, client *github.Client, opts options, since time.Time, dir string, p *progress) (nextSince time.Time, written int, err error) {
	listOpts := &github.IssueListByRepoOptions{
		State:  opts.state,
		Labels: opts.labels,
		Since:  since,
		// Paginate in creation order, which updates don't change. In update order,
		// an issue updated mid-sync would jump to the end and shift an issue we
		// haven't seen onto a page we have already read, skipping it for good.
		Sort:        "created",
		Direction:   "asc",
		ListOptions: github.ListOptions{PerPage: 100},
	}
	for {
		issues, resp, err := retryRateLimited(ctx, p, func() ([]*github.Issue, *github.Response, error) {
			return client.Issues.ListByRepo(ctx, opts.owner, opts.name, listOpts)
		})
		if err != nil {
			return time.Time{}, written, fmt.Errorf("listing issues of %s: %w", opts.repo(), err)
		}
		if nextSince.IsZero() {
			// Anything updated after the first page was requested is picked up next time.
			nextSince = serverTime(resp).Add(-syncOverlap)
		}

		for _, issue := range issues {
			if issue.IsPullRequest() { // the issues endpoint returns pull requests too
				continue
			}
			var comments []*github.IssueComment
			if issue.GetComments() > 0 {
				comments, err = listComments(ctx, client, opts, issue.GetNumber(), p)
				if err != nil {
					return time.Time{}, written, fmt.Errorf("listing comments of issue #%d: %w", issue.GetNumber(), err)
				}
			}
			path := filepath.Join(dir, fmt.Sprintf("%d.md", issue.GetNumber()))
			if err := os.WriteFile(path, []byte(renderIssue(issue, comments)), 0o644); err != nil {
				return time.Time{}, written, err
			}
			written++
			p.issueSynced()
		}

		if resp.NextPage == 0 {
			return nextSince, written, nil
		}
		listOpts.Page = resp.NextPage
	}
}

func listComments(ctx context.Context, client *github.Client, opts options, number int, p *progress) ([]*github.IssueComment, error) {
	listOpts := &github.IssueListCommentsOptions{ListOptions: github.ListOptions{PerPage: 100}}
	var all []*github.IssueComment
	for {
		comments, resp, err := retryRateLimited(ctx, p, func() ([]*github.IssueComment, *github.Response, error) {
			return client.Issues.ListComments(ctx, opts.owner, opts.name, number, listOpts)
		})
		if err != nil {
			return nil, err
		}
		all = append(all, comments...)
		if resp.NextPage == 0 {
			return all, nil
		}
		listOpts.Page = resp.NextPage
	}
}

// retryRateLimited runs call, and if GitHub rejects it for exceeding a rate
// limit, waits for as long as GitHub asks and tries again.
func retryRateLimited[T any](ctx context.Context, p *progress, call func() (T, *github.Response, error)) (T, *github.Response, error) {
	for attempt := 0; ; attempt++ {
		result, resp, err := call()
		wait, limited := rateLimitWait(err)
		if !limited || attempt == maxRateLimitRetries {
			return result, resp, err
		}
		p.setWaiting("rate limited, resuming at " + time.Now().Add(wait).Format("15:04:05"))
		select {
		case <-time.After(wait):
			p.setWaiting("")
		case <-ctx.Done():
			return result, resp, ctx.Err()
		}
	}
}

// rateLimitWait reports whether err is a rate-limit error and, if so, how
// long to wait before retrying.
func rateLimitWait(err error) (time.Duration, bool) {
	var primary *github.RateLimitError
	var secondary *github.AbuseRateLimitError
	switch {
	case errors.As(err, &primary):
		// The floor guards against a local clock that is ahead of GitHub's.
		return max(time.Until(primary.Rate.Reset.Time), 0) + 5*time.Second, true
	case errors.As(err, &secondary) && secondary.RetryAfter != nil:
		return *secondary.RetryAfter, true
	default:
		return 0, false
	}
}

// serverTime returns the time in resp's Date header, so that sync times come
// from GitHub's clock rather than the local one, which may be off.
func serverTime(resp *github.Response) time.Time {
	if t, err := http.ParseTime(resp.Header.Get("Date")); err == nil {
		return t.UTC()
	}
	return time.Now().UTC()
}
