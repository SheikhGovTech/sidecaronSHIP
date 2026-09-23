// Package changeset prepares and verifies the immutable Git snapshot that
// Sidecar evaluates and publishes.
package changeset

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var BuiltInExclusions = []string{
	".harness/**", ".coverage", ".coverage.*", ".pytest_cache/**",
	"htmlcov/**", "coverage.xml",
}

type PathChange struct {
	Status string `json:"status"`
	Path   string `json:"path"`
}
type Snapshot struct {
	Base     string       `json:"base"`
	Paths    []PathChange `json:"paths"`
	Excluded []string     `json:"excluded,omitempty"`
	Digest   string       `json:"digest"`
}

func Prepare(repo, base string, configured []string) (Snapshot, error) {
	resolved, err := git(repo, "rev-parse", "--verify", base+"^{commit}")
	if err != nil {
		return Snapshot{}, fmt.Errorf("resolve task base: %w", err)
	}
	base = strings.TrimSpace(resolved)
	if _, err := git(repo, "merge-base", "--is-ancestor", base, "HEAD"); err != nil {
		return Snapshot{}, fmt.Errorf("task base is not an ancestor of HEAD: %w", err)
	}
	if _, err := git(repo, "reset", "--mixed", base); err != nil {
		return Snapshot{}, fmt.Errorf("normalize agent commits: %w", err)
	}
	candidates, err := candidatePaths(repo)
	if err != nil {
		return Snapshot{}, err
	}
	patterns := append(append([]string{}, BuiltInExclusions...), configured...)
	excluded := []string{}
	for _, name := range candidates {
		if matchesAny(name, patterns) {
			excluded = append(excluded, name)
		}
	}
	if _, err := git(repo, "add", "-A", "--", "."); err != nil {
		return Snapshot{}, fmt.Errorf("stage repair: %w", err)
	}
	for _, name := range excluded {
		if _, err := git(repo, "reset", "-q", base, "--", name); err != nil {
			return Snapshot{}, fmt.Errorf("exclude %q: %w", name, err)
		}
	}
	sort.Strings(excluded)
	snap, err := stagedSnapshot(repo, base)
	if err != nil {
		return Snapshot{}, err
	}
	snap.Excluded = excluded
	return snap, nil
}

func VerifyPrepared(repo string, approved Snapshot) error {
	actual, err := stagedSnapshot(repo, approved.Base)
	if err != nil {
		return err
	}
	return compare(approved, actual)
}
func Commit(repo string, approved Snapshot, message string) error {
	if err := VerifyPrepared(repo, approved); err != nil {
		return fmt.Errorf("prepared index changed: %w", err)
	}
	if len(approved.Paths) == 0 {
		return nil
	}
	// Commit identity and signing are deployment policy.  In particular, a
	// downstream runtime may require a verified GitLab email and signed
	// commits, so do not override its Git configuration here.
	if _, err := git(repo, "commit", "-m", message); err != nil {
		return fmt.Errorf("commit prepared repair: %w", err)
	}
	return nil
}
func VerifyCommitted(repo string, approved Snapshot) error {
	actual, err := snapshot(repo, approved.Base, false)
	if err != nil {
		return err
	}
	return compare(approved, actual)
}
func stagedSnapshot(repo, base string) (Snapshot, error) { return snapshot(repo, base, true) }
func snapshot(repo, base string, cached bool) (Snapshot, error) {
	nameArgs := []string{"diff", "--name-status", "--no-renames", "-z"}
	patchArgs := []string{"diff", "--binary", "--full-index", "--no-ext-diff", "--no-renames"}
	if cached {
		nameArgs = append(nameArgs, "--cached", base, "--")
		patchArgs = append(patchArgs, "--cached", base, "--")
	} else {
		selector := base + "..HEAD"
		nameArgs = append(nameArgs, selector, "--")
		patchArgs = append(patchArgs, selector, "--")
	}
	names, err := gitBytes(repo, nameArgs...)
	if err != nil {
		return Snapshot{}, fmt.Errorf("read change manifest: %w", err)
	}
	patch, err := gitBytes(repo, patchArgs...)
	if err != nil {
		return Snapshot{}, fmt.Errorf("read canonical patch: %w", err)
	}
	parts := strings.Split(strings.TrimSuffix(string(names), "\x00"), "\x00")
	paths := []PathChange{}
	for i := 0; i+1 < len(parts); i += 2 {
		paths = append(paths, PathChange{Status: parts[i], Path: parts[i+1]})
	}
	sort.Slice(paths, func(i, j int) bool {
		if paths[i].Path == paths[j].Path {
			return paths[i].Status < paths[j].Status
		}
		return paths[i].Path < paths[j].Path
	})
	sum := sha256.Sum256(patch)
	return Snapshot{Base: base, Paths: paths, Digest: hex.EncodeToString(sum[:])}, nil
}
func compare(want, got Snapshot) error {
	if want.Digest != got.Digest {
		return fmt.Errorf("patch digest mismatch: approved %s, actual %s", want.Digest, got.Digest)
	}
	if fmt.Sprint(want.Paths) != fmt.Sprint(got.Paths) {
		return fmt.Errorf("path manifest mismatch: approved %v, actual %v", want.Paths, got.Paths)
	}
	return nil
}
func candidatePaths(repo string) ([]string, error) {
	out, err := gitBytes(repo, "ls-files", "-m", "-d", "-o", "--exclude-standard", "-z")
	if err != nil {
		return nil, fmt.Errorf("list repair files: %w", err)
	}
	seen := map[string]bool{}
	for _, name := range strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00") {
		if name != "" {
			seen[filepath.ToSlash(name)] = true
		}
	}
	paths := []string{}
	for name := range seen {
		paths = append(paths, name)
	}
	sort.Strings(paths)
	return paths, nil
}
func matchesAny(name string, patterns []string) bool {
	for _, pattern := range patterns {
		quoted := regexp.QuoteMeta(filepath.ToSlash(pattern))
		quoted = strings.ReplaceAll(quoted, `\*\*`, `.*`)
		quoted = strings.ReplaceAll(quoted, `\*`, `[^/]*`)
		quoted = strings.ReplaceAll(quoted, `\?`, `[^/]`)
		if regexp.MustCompile("^" + quoted + "$").MatchString(filepath.ToSlash(name)) {
			return true
		}
	}
	return false
}
func git(repo string, args ...string) (string, error) {
	out, err := gitBytes(repo, args...)
	return string(out), err
}
func gitBytes(repo string, args ...string) ([]byte, error) {
	cmdArgs := append([]string{"-C", repo}, args...)
	out, err := exec.Command("git", cmdArgs...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}
