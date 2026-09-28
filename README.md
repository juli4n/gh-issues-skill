# gh-issues-skill

Mirrors the issues of a GitHub repository into a local directory, one markdown file per issue, and generates a [Claude Code skill](https://docs.claude.com/en/docs/claude-code/skills) that tells Claude how to search them.

With the issues on disk, finding existing reports of a bug is a `grep` away, for you or for Claude. Syncs are incremental, so keeping the copy up to date is cheap.

## Install

Requires Go 1.22 or later.

```
go install github.com/juli4n/gh-issues-skill@latest
```

This puts `gh-issues-skill` in `$(go env GOPATH)/bin`, usually `~/go/bin`. Make sure that directory is on your `PATH`.

## Authentication

gh-issues-skill uses the first of these that is available:

1. the `GITHUB_TOKEN` environment variable
2. the `GH_TOKEN` environment variable
3. the output of `gh auth token`, so being logged in with the [GitHub CLI](https://cli.github.com) is enough

## Usage

```
gh-issues-skill -repo owner/name [flags]
```

| Flag     | Default                        | Description                                                      |
|----------|--------------------------------|------------------------------------------------------------------|
| `-repo`  | required                       | Repository to mirror, as `owner/name`                            |
| `-out`   | `.claude/skills/<name>-issues` | Output directory                                                 |
| `-label` | none                           | Comma-separated labels; only issues with *all* of them are synced |
| `-state` | `all`                          | `open`, `closed` or `all`                                        |
| `-full`  | `false`                        | Refetch all issues, not just those updated since the last sync   |

Run it from the root of a project, and the default output directory makes the issues available to Claude Code as a project skill:

```
cd ~/src/my-project
gh-issues-skill -repo cli/cli
```

Other examples:

```
# A personal skill, available in every project
gh-issues-skill -repo cli/cli -out ~/.claude/skills/cli-issues

# Only open issues labeled "bug"
gh-issues-skill -repo cli/cli -label bug -state open

# Refetch everything
gh-issues-skill -repo cli/cli -full
```

Label names must match exactly, and a label that doesn't exist matches nothing. List a repository's labels with `gh label list -R owner/name`.

A mirror can hold thousands of files. If you keep it inside a project, you probably want to add it to `.gitignore`.

## Output

```
<out>/
├── SKILL.md         # Claude Code skill describing the mirror
├── .gh-issues-skill.json    # repository, filters and time of the last sync
└── issues/
    ├── 1.md
    ├── 2.md
    └── ...
```

Each issue file has YAML frontmatter, the issue body and its comments:

```
---
number: 123
title: "Something is broken"
state: closed
state_reason: completed
labels: ["bug", "area/network"]
author: someone
created: 2026-01-01T00:00:00Z
updated: 2026-01-02T00:00:00Z
closed: 2026-01-02T00:00:00Z
url: https://github.com/owner/name/issues/123
---

# Something is broken

Issue body...

## Comments

### someone-else at 2026-01-01T12:00:00Z

Comment body...
```

Pull requests are not included.

### The skill

`SKILL.md` tells Claude what the mirror contains, how to refresh it, the file format, and how to search for related issues. The skill is named after the output directory. Claude Code loads it automatically when the directory is under `.claude/skills/` in your project or in your home directory. For any other location, point Claude at the file.

`SKILL.md` is only written if it doesn't exist yet, so you can edit it, for example to explain the repository's labels. Delete it to have the next sync regenerate it.

## Incremental sync

After each successful sync, gh-issues-skill records the time in `.gh-issues-skill.json`. The next sync only fetches issues updated since then and rewrites their files. If a sync fails or is interrupted, the next one starts again from the previous successful sync.

An output directory is tied to the repository and filters it was first synced with. To change them, use a new directory, or delete the old one first.

Limitations:

- Issues that are deleted or transferred, or that stop matching `-label` or `-state`, are not removed. To reset, delete the output directory and sync again.
- If GitHub rate-limits a request, gh-issues-skill waits as long as GitHub asks and retries up to three times. An authenticated token allows 5,000 requests per hour. A sync takes one request per 100 issues plus one per issue with comments.

## Development

```
go test ./...
go vet ./...
```
