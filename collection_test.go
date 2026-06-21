package main

import (
	"io"
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

	if !m("c:\\windows\\system32\\config\\sam") {
		t.Error("static matcher should match the lowercased path")
	}
	if m("c:\\windows\\system32\\config\\sam.log") {
		t.Error("static matcher must be exact, not a prefix/substring match")
	}
}

func TestGlobMatcherWindowsBackslashes(t *testing.T) {
	// Backslashes are escaped by NewGlobMatcher so Windows separators are
	// treated literally rather than as glob escapes. Input is pre-lowercased.
	m := mustGlob(t, "C:\\Users\\*\\NTUSER.DAT")

	if !m("c:\\users\\alice\\ntuser.dat") {
		t.Error("glob should match a lowercased Windows path")
	}
	if m("c:\\programdata\\foo.dat") {
		t.Error("glob should not match a path outside the pattern")
	}
}

func TestRegexpMatcherSubstringInsensitive(t *testing.T) {
	m := mustRegexp(t, "system32")

	if !m("c:\\windows\\system32\\cmd.exe") {
		t.Error("regex should match as a substring of the lowercased path")
	}
	if m("c:\\windows\\explorer.exe") {
		t.Error("regex should not match a non-matching path")
	}
}

func TestAppendIfMatchCaseInsensitive(t *testing.T) {
	// Case-insensitivity now lives in appendIfMatch: a mixed-case path must
	// still match, since appendIfMatch folds it before testing the matchers.
	matchers := []Matcher{mustGlob(t, "C:\\Users\\*\\NTUSER.DAT")}

	got := appendIfMatch(nil, matchers, "C:\\Users\\Alice\\NTUSER.DAT", "C:\\")
	if len(got) != 1 {
		t.Fatalf("got %d targets, want 1", len(got))
	}
	if got[0].Path != "C:\\Users\\Alice\\NTUSER.DAT" {
		t.Errorf("Path = %q, want the original-case full path", got[0].Path)
	}
}

func TestAppendIfMatchFullPath(t *testing.T) {
	matchers := []Matcher{NewStaticMatcher("/etc/passwd")}

	got := appendIfMatch(nil, matchers, "/etc/passwd", "/")
	if len(got) != 1 {
		t.Fatalf("got %d targets, want 1", len(got))
	}
	if got[0].Path != "/etc/passwd" || got[0].Source != "match" {
		t.Errorf("target = %+v, want {Path:/etc/passwd Source:match}", got[0])
	}
}

func TestAppendIfMatchTrimmedPath(t *testing.T) {
	// The matcher only matches the root-trimmed form; appendIfMatch must still
	// hit, and the recorded Path is the full path (not the trimmed one).
	matchers := []Matcher{NewStaticMatcher("/log/syslog")}

	got := appendIfMatch(nil, matchers, "/var/log/syslog", "/var")
	if len(got) != 1 {
		t.Fatalf("got %d targets, want 1", len(got))
	}
	if got[0].Path != "/var/log/syslog" {
		t.Errorf("Path = %q, want the full path /var/log/syslog", got[0].Path)
	}
}

func TestAppendIfMatchNoMatch(t *testing.T) {
	matchers := []Matcher{NewStaticMatcher("/etc/shadow")}

	got := appendIfMatch(nil, matchers, "/etc/passwd", "/")
	if len(got) != 0 {
		t.Errorf("got %d targets, want 0 (no matcher hit)", len(got))
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
