package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-github/v69/github"
)

// renderIssue formats an issue and its comments as markdown with YAML
// frontmatter.
func renderIssue(issue *github.Issue, comments []*github.IssueComment) string {
	// strconv.Quote output is a valid YAML double-quoted string.
	labels := make([]string, len(issue.Labels))
	for i, l := range issue.Labels {
		labels[i] = strconv.Quote(l.GetName())
	}

	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "number: %d\n", issue.GetNumber())
	fmt.Fprintf(&b, "title: %s\n", strconv.Quote(issue.GetTitle()))
	fmt.Fprintf(&b, "state: %s\n", issue.GetState())
	if reason := issue.GetStateReason(); reason != "" {
		fmt.Fprintf(&b, "state_reason: %s\n", reason)
	}
	fmt.Fprintf(&b, "labels: [%s]\n", strings.Join(labels, ", "))
	fmt.Fprintf(&b, "author: %s\n", issue.GetUser().GetLogin())
	fmt.Fprintf(&b, "created: %s\n", formatTime(issue.GetCreatedAt()))
	fmt.Fprintf(&b, "updated: %s\n", formatTime(issue.GetUpdatedAt()))
	if closed := issue.GetClosedAt(); !closed.IsZero() {
		fmt.Fprintf(&b, "closed: %s\n", formatTime(closed))
	}
	fmt.Fprintf(&b, "url: %s\n", issue.GetHTMLURL())
	b.WriteString("---\n\n")

	fmt.Fprintf(&b, "# %s\n\n", issue.GetTitle())
	b.WriteString(normalizeMarkdown(issue.GetBody()))
	b.WriteString("\n")

	if len(comments) > 0 {
		b.WriteString("\n## Comments\n")
		for _, c := range comments {
			fmt.Fprintf(&b, "\n### %s at %s\n\n%s\n",
				c.GetUser().GetLogin(), formatTime(c.GetCreatedAt()), normalizeMarkdown(c.GetBody()))
		}
	}
	return b.String()
}

func formatTime(t github.Timestamp) string { return t.UTC().Format(time.RFC3339) }

func normalizeMarkdown(s string) string {
	return strings.ReplaceAll(strings.TrimSpace(s), "\r\n", "\n")
}
