package main

import (
	_ "embed"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
)

//go:embed skill.md.tmpl
var skillTemplateText string

var skillTemplate = template.Must(template.New("SKILL.md").Parse(skillTemplateText))

// writeSkill writes SKILL.md into the output directory, unless it already
// exists, so that edits made to it are kept across syncs.
func writeSkill(opts options) error {
	f, err := os.OpenFile(filepath.Join(opts.outDir, "SKILL.md"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	absOut, err := filepath.Abs(opts.outDir)
	if err != nil {
		return errors.Join(err, f.Close())
	}
	err = skillTemplate.Execute(f, map[string]string{
		"Name":    skillName(filepath.Base(absOut)),
		"Repo":    opts.repo(),
		"Scope":   describeScope(opts),
		"Command": syncCommand(opts),
	})
	return errors.Join(err, f.Close())
}

var invalidSkillNameChars = regexp.MustCompile(`[^a-z0-9]+`)

// skillName turns s into a valid skill name: at most 64 lowercase letters,
// digits and hyphens.
func skillName(s string) string {
	name := invalidSkillNameChars.ReplaceAllString(strings.ToLower(s), "-")
	name = strings.Trim(name[:min(len(name), 64)], "-")
	if name == "" {
		return "issues"
	}
	return name
}

// describeScope says which issues the mirror holds, e.g. "open issues labeled
// `bug`".
func describeScope(opts options) string {
	s := "issues"
	if opts.state != "all" {
		s = opts.state + " issues"
	}
	if len(opts.labels) > 0 {
		s += " labeled `" + strings.Join(opts.labels, "` and `") + "`"
	}
	return s
}

// syncCommand returns the gh-issues-skill command line, minus -out, that
// syncs the same issues as opts.
func syncCommand(opts options) string {
	args := []string{"gh-issues-skill", "-repo", opts.repo()}
	if len(opts.labels) > 0 {
		args = append(args, "-label", shellQuote(strings.Join(opts.labels, ",")))
	}
	if opts.state != "all" {
		args = append(args, "-state", opts.state)
	}
	return strings.Join(args, " ")
}

var shellSafe = regexp.MustCompile(`^[A-Za-z0-9_./:,=+-]+$`)

func shellQuote(s string) string {
	if shellSafe.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
