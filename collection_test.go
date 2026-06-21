package main

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"testing"
)

// mustGlob / mustRegexp build a matcher from a pattern expected to be valid,
// failing the test if compilation errors.
func mustGlob(t *testing.T, pattern string) Matcher {
	t.Helper()
	m, err := NewGlobMatcher(pattern)
	if err != nil {
		t.Fatalf("NewGlobMatcher(%q): %v", pattern, err)
	}
	return m
}

func mustRegexp(t *testing.T, pattern string) Matcher {
	t.Helper()
	m, err := NewRegexpMatcher(pattern)
	if err != nil {
		t.Fatalf("NewRegexpMatcher(%q): %v", pattern, err)
	}
	return m
}

func TestLiteralGlobPrefix(t *testing.T) {
	cases := []struct {
		pattern string
		want    string
	}{
		{"/var/log/**", "/var/log/"},
		{"/var/log/*", "/var/log/"},
		{"/users/*/.*history", "/users/"},
		{"/a/b", "/a/b"},
		{"/a/b/", "/a/b/"},
		{"**/library/foo", ""},
		{"*", ""},
		{"?foo", ""},
		{"[abc]/x", ""},
		{"{a,b}/x", ""},
		{"/foo[0-9]/bar", "/foo"},
		{"/path with spaces/*", "/path with spaces/"},
		// Backslashes are literal path separators, not glob escapes (NewGlobMatcher
		// escapes them to match literally), so the extracted prefix keeps them.
		{"c:\\windows\\system32\\*", "c:\\windows\\system32\\"},
		{"c:\\users\\*\\ntuser.dat", "c:\\users\\"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := literalGlobPrefix(tc.pattern); got != tc.want {
			t.Errorf("literalGlobPrefix(%q) = %q, want %q", tc.pattern, got, tc.want)
		}
	}
}

// TestMatcherPrefix locks in the literal prefix each constructor derives — the
// case-folded prefix that travels with the matcher and drives directory pruning.
func TestMatcherPrefix(t *testing.T) {
	cases := []struct {
		name    string
		matcher Matcher
		want    string
	}{
		{"static literal", NewStaticMatcher("/etc/passwd"), "/etc/passwd"},
		{"static lowercased", NewStaticMatcher("C:\\Windows\\System32\\config\\SAM"), "c:\\windows\\system32\\config\\sam"},
		{"glob prefix", mustGlob(t, "/var/log/**"), "/var/log/"},
		{"glob lowercased", mustGlob(t, "/Users/*/NTUSER.DAT"), "/users/"},
		{"leading-** glob unprunable", mustGlob(t, "**/*.plist"), ""},
		{"regex unprunable", mustRegexp(t, "system32"), ""},
		{"anchored regex unprunable", mustRegexp(t, "^/var/.*$"), ""},
	}
	for _, tc := range cases {
		if got := tc.matcher.Prefix; got != tc.want {
			t.Errorf("%s: Prefix = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestShouldDescend(t *testing.T) {
	p := []string{"/var/log/", "/library/launchdaemons/"}
	cases := []struct {
		dir  string
		want bool
	}{
		{"/var", true},                       // on the path toward /var/log/
		{"/var/log", true},                   // equals a prefix sans trailing sep
		{"/var/log/nginx", true},             // inside the subtree
		{"/var/log/nginx/sites", true},       // deeper inside
		{"/library", true},                   // toward /library/launchdaemons/
		{"/library/launchdaemons/sub", true}, // inside the second prefix
		{"/applications", false},             // outside every prefix
		{"/usr", false},                      // outside every prefix
		{"/variant", false},                  // must not match /var
		{"/var/logger", false},               // not a segment prefix of /var/log
		{"/library/launchagents", false},     // sibling of the second prefix
		{"/", true},                          // root is toward every absolute prefix
	}
	for _, tc := range cases {
		if got := shouldDescend(p, tc.dir); got != tc.want {
			t.Errorf("shouldDescend(%v, %q) = %v, want %v", p, tc.dir, got, tc.want)
		}
	}

	// Boundary direction toward a prefix.
	if !shouldDescend([]string{"/var/log/"}, "/var") {
		t.Error("/var should descend toward /var/log/")
	}
	if shouldDescend([]string{"/var/log/"}, "/va") {
		t.Error("/va is not a segment boundary of /var/log/ and must not descend")
	}

	// Exact-equality boundary: the dir equals the prefix with or without a sep.
	if !shouldDescend([]string{"/var/log/"}, "/var/log") {
		t.Error("/var/log should descend (equals prefix sans trailing sep)")
	}
	if !shouldDescend([]string{"/var/log/"}, "/var/log/") {
		t.Error("/var/log/ should descend (trailing sep tolerated)")
	}

	// shouldDescend assumes lowercased input (the caller folds the dir first).
	if !shouldDescend([]string{"/var/log/"}, "/var/log/nginx") {
		t.Error("already-lowercased path inside the prefix should descend")
	}

	// Windows separators.
	win := []string{"c:\\users\\"}
	if !shouldDescend(win, "c:\\users") {
		t.Error("c:\\users should descend toward c:\\users\\")
	}
	if !shouldDescend(win, "c:\\users\\alice") {
		t.Error("c:\\users\\alice should descend (inside the prefix)")
	}
	if shouldDescend(win, "c:\\windows") {
		t.Error("c:\\windows is outside c:\\users\\ and must not descend")
	}
}

// TestPrunable checks that pruning is enabled only when every loaded matcher
// carries a non-empty literal prefix, and that the collected prefixes stay
// index-aligned with the matchers.
func TestPrunable(t *testing.T) {
	prefixes, prune := prunable([]Matcher{NewStaticMatcher("/etc/passwd"), mustGlob(t, "/var/log/**")})
	if want := []string{"/etc/passwd", "/var/log/"}; !slices.Equal(prefixes, want) {
		t.Errorf("prefixes = %v, want %v", prefixes, want)
	}
	if !prune {
		t.Error("all-prefixed matchers should enable pruning")
	}

	if _, prune := prunable([]Matcher{mustGlob(t, "/var/log/**"), mustRegexp(t, "x")}); prune {
		t.Error("a regex matcher (empty prefix) must disable pruning")
	}
	if _, prune := prunable([]Matcher{mustGlob(t, "/var/log/**"), mustGlob(t, "**/*.plist")}); prune {
		t.Error("a leading-** glob (empty prefix) must disable pruning")
	}
	if _, prune := prunable(nil); prune {
		t.Error("no matchers must disable pruning")
	}
}

func TestShouldSkipDir(t *testing.T) {
	set := skipDirSet(Configuration{SkipDirs: []string{"/System/Volumes/Data", "/dev"}})

	cases := []struct {
		path string
		want bool
	}{
		{"/dev", true},
		{"/DEV", true},                 // case-insensitive
		{"/System/Volumes/Data", true}, // exact multi-segment
		{"/system/volumes/data", true}, // case-insensitive multi-segment
		{"/devices", false},            // exact, not prefix
		{"/dev/null", false},           // children are not the dir itself
		{"/System/Volumes", false},     // a parent is not skipped
		{"/Users", false},              // unrelated
	}
	for _, tc := range cases {
		if got := shouldSkipDir(set, tc.path); got != tc.want {
			t.Errorf("shouldSkipDir(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestStaticMatcherCaseInsensitive(t *testing.T) {
	// Matchers receive already-lowercased input; the constructor folds the
	// pattern so a mixed-case pattern still matches the lowercased path.
	m := NewStaticMatcher("C:\\Windows\\System32\\config\\SAM")

	if !m.Match("c:\\windows\\system32\\config\\sam") {
		t.Error("static matcher should match the lowercased path")
	}
	if m.Match("c:\\windows\\system32\\config\\sam.log") {
		t.Error("static matcher must be exact, not a prefix/substring match")
	}
}

func TestGlobMatcherWindowsBackslashes(t *testing.T) {
	// Backslashes are escaped by NewGlobMatcher so Windows separators are
	// treated literally rather than as glob escapes. Input is pre-lowercased.
	m := mustGlob(t, "C:\\Users\\*\\NTUSER.DAT")

	if !m.Match("c:\\users\\alice\\ntuser.dat") {
		t.Error("glob should match a lowercased Windows path")
	}
	if m.Match("c:\\programdata\\foo.dat") {
		t.Error("glob should not match a path outside the pattern")
	}
}

func TestRegexpMatcherSubstringInsensitive(t *testing.T) {
	m := mustRegexp(t, "system32")

	if !m.Match("c:\\windows\\system32\\cmd.exe") {
		t.Error("regex should match as a substring of the lowercased path")
	}
	if m.Match("c:\\windows\\explorer.exe") {
		t.Error("regex should not match a non-matching path")
	}
}

func TestAppendIfMatchCaseInsensitive(t *testing.T) {
	// Case-insensitivity lives in appendIfMatch: a mixed-case path must still
	// match, since appendIfMatch folds it before testing the matchers.
	matchers := []Matcher{mustGlob(t, "C:\\Users\\*\\NTUSER.DAT")}

	got := appendIfMatch(nil, matchers, "C:\\Users\\Alice\\NTUSER.DAT")
	if len(got) != 1 {
		t.Fatalf("got %d targets, want 1", len(got))
	}
	if got[0].Path != "C:\\Users\\Alice\\NTUSER.DAT" {
		t.Errorf("Path = %q, want the original-case full path", got[0].Path)
	}
}

func TestAppendIfMatchFullPath(t *testing.T) {
	matchers := []Matcher{NewStaticMatcher("/etc/passwd")}

	got := appendIfMatch(nil, matchers, "/etc/passwd")
	if len(got) != 1 {
		t.Fatalf("got %d targets, want 1", len(got))
	}
	if got[0].Path != "/etc/passwd" || got[0].Source != "match" {
		t.Errorf("target = %+v, want {Path:/etc/passwd Source:match}", got[0])
	}
}

// TestAppendIfMatchCustomRootRelative locks in the documented behavior change:
// a pattern written relative to a non-root -root (here "/log/syslog" under root
// "/var") used to match via the trimmed form and now does not — only the full
// absolute path is tested.
func TestAppendIfMatchCustomRootRelative(t *testing.T) {
	matchers := []Matcher{NewStaticMatcher("/log/syslog")}

	got := appendIfMatch(nil, matchers, "/var/log/syslog")
	if len(got) != 0 {
		t.Errorf("got %d targets, want 0: a root-relative pattern must no longer match the full path", len(got))
	}
}

func TestAppendIfMatchNoMatch(t *testing.T) {
	matchers := []Matcher{NewStaticMatcher("/etc/shadow")}

	got := appendIfMatch(nil, matchers, "/etc/passwd")
	if len(got) != 0 {
		t.Errorf("got %d targets, want 0 (no matcher hit)", len(got))
	}
}

// benchPaths is a representative slice exercised by BenchmarkAppendIfMatch: a
// couple of static hits, a couple of glob hits, and several non-matching paths
// (the overwhelmingly common case on a real volume).
var benchPaths = []string{
	"/etc/hosts",  // static hit (darwin/linux defaults)
	"/etc/passwd", // static hit
	"/Users/alice/Library/Application Support/Google/Chrome/Default/History", // glob hit
	"/var/log/system.log",                          // glob hit (/var/log/**)
	"/Users/alice/Documents/notes.txt",             // no match
	"/usr/local/bin/tool",                          // no match
	"/System/Library/Frameworks/Foo.framework/Foo", // no match
	"/private/var/db/locate.database",              // no match
}

// BenchmarkAppendIfMatch measures the per-path matching hot path against the
// current-OS default matcher set. b.ReportAllocs() makes the allocation cost of
// the path fold(s) visible so the single-form change can be compared before/after.
func BenchmarkAppendIfMatch(b *testing.B) {
	matchers, _, err := LoadMatchers(Configuration{})
	if err != nil {
		b.Fatalf("LoadMatchers: %v", err)
	}

	b.ReportAllocs()
	for b.Loop() {
		for _, p := range benchPaths {
			appendIfMatch(nil, matchers, p)
		}
	}
}

func TestNewHashers(t *testing.T) {
	w, finish := NewHashers()
	if _, err := io.WriteString(w, "abc"); err != nil {
		t.Fatalf("write: %v", err)
	}

	sha, md5 := finish()
	wantSHA := "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	wantMD5 := "900150983cd24fb0d6963f7d28e17f72"
	if sha != wantSHA {
		t.Errorf("sha256(abc) = %q, want %q", sha, wantSHA)
	}
	if md5 != wantMD5 {
		t.Errorf("md5(abc) = %q, want %q", md5, wantMD5)
	}
}

// buildPruneTree lays down a small tree for the GetPaths pruning tests:
//
//	<root>/keep/a.txt
//	<root>/keep/sub/c.txt
//	<root>/skip/b.txt
//	<root>/other/d.log
//
// A focused matcher under keep/ leaves skip/ and other/ as prunable siblings.
func buildPruneTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, f := range []string{"keep/a.txt", "keep/sub/c.txt", "skip/b.txt", "other/d.log"} {
		full := filepath.Join(root, f)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o600); err != nil {
			t.Fatalf("write %s: %v", f, err)
		}
	}
	return root
}

// collectQuackPaths runs GetPaths over root with the given quack body and returns
// the sorted collected paths. The quack file lives outside root so it is not
// itself walked.
func collectQuackPaths(t *testing.T, root, quack string) []string {
	t.Helper()
	qf := filepath.Join(t.TempDir(), "targets.quack")
	if err := os.WriteFile(qf, []byte(quack), 0o600); err != nil {
		t.Fatalf("write quack: %v", err)
	}
	targets, err := GetPaths(Configuration{CollectionRoots: []string{root}, QuackTargets: qf})
	if err != nil {
		t.Fatalf("GetPaths: %v", err)
	}
	paths := make([]string, 0, len(targets))
	for _, tg := range targets {
		paths = append(paths, tg.Path)
	}
	sort.Strings(paths)
	return paths
}

// TestGetPathsPruningPreservesResults is the PRIMARY differential test for the
// WalkDir codepath: for each config it walks once with pruning as the matchers
// imply and once with pruning forced off (a leading-** matcher that matches
// nothing in the tree disables pruning without adding any match), and asserts the
// collected sets are identical. Pruning must only change which directories are
// visited, never which files are collected. Adding a config is a one-line change.
func TestGetPathsPruningPreservesResults(t *testing.T) {
	root := buildPruneTree(t)
	// A leading-** glob matching no file in the tree: disables pruning (empty
	// literal prefix) yet contributes no extra match, so the two runs stay equal.
	const disablePruning = "\nglob\t**/__no_such_marker_zzz__/**\n"

	cases := []struct {
		name  string
		quack string
	}{
		{"focused glob", "glob\t" + root + "/keep/**"},
		{"focused static+glob", "static\t" + root + "/keep/a.txt\nglob\t" + root + "/keep/sub/*.txt"},
		{"mixed leading **", "glob\t" + root + "/keep/**\nglob\t**/b.txt"},
		{"force into pruned subtree", "glob\t" + root + "/keep/**\nforce\t" + root + "/skip/b.txt"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pruned := collectQuackPaths(t, root, tc.quack)
			full := collectQuackPaths(t, root, tc.quack+disablePruning)
			if !slices.Equal(pruned, full) {
				t.Errorf("pruned set != unpruned set\n pruned = %v\n full   = %v", pruned, full)
			}
		})
	}
}

// TestGetPathsPrunesSibling mirrors the focused-prune assertion for WalkDir: the
// keep/ match is collected while the skip/ sibling is absent, and the collected
// set equals the unpruned run.
func TestGetPathsPrunesSibling(t *testing.T) {
	root := buildPruneTree(t)
	quack := "glob\t" + root + "/keep/**"

	got := collectQuackPaths(t, root, quack)
	if !slices.Contains(got, filepath.Join(root, "keep", "a.txt")) {
		t.Errorf("keep/a.txt should be collected; got %v", got)
	}
	if slices.Contains(got, filepath.Join(root, "skip", "b.txt")) {
		t.Errorf("skip/b.txt should not be collected; got %v", got)
	}
}

// TestGetPathsForcePathBypassesPruning proves a force path lands even when it
// points into a subtree the focused prefix prunes away.
func TestGetPathsForcePathBypassesPruning(t *testing.T) {
	root := buildPruneTree(t)
	forced := filepath.Join(root, "skip", "b.txt")
	quack := "glob\t" + root + "/keep/**\nforce\t" + forced

	got := collectQuackPaths(t, root, quack)
	if !slices.Contains(got, forced) {
		t.Errorf("forced path %q should be collected despite skip/ being pruned; got %v", forced, got)
	}
}
