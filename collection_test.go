package main

import (
	"io"
	"testing"
)

func TestStaticMatcherCaseInsensitive(t *testing.T) {
	m := NewStaticMatcher("C:\\Windows\\System32\\config\\SAM")

	if !m("c:\\windows\\system32\\config\\sam") {
		t.Error("static matcher should match the same path in different case")
	}
	if m("C:\\Windows\\System32\\config\\SAM.LOG") {
		t.Error("static matcher must be exact, not a prefix/substring match")
	}
}

func TestGlobMatcherWindowsBackslashes(t *testing.T) {
	// Backslashes are escaped by NewGlobMatcher so Windows separators are
	// treated literally rather than as glob escapes.
	m := NewGlobMatcher("C:\\Users\\*\\NTUSER.DAT")

	if !m("c:\\users\\alice\\ntuser.dat") {
		t.Error("glob should match a Windows path case-insensitively")
	}
	if m("c:\\programdata\\foo.dat") {
		t.Error("glob should not match a path outside the pattern")
	}
}

func TestRegexpMatcherSubstringInsensitive(t *testing.T) {
	m := NewRegexpMatcher("system32")

	if !m("C:\\Windows\\System32\\cmd.exe") {
		t.Error("regex should match as a case-insensitive substring")
	}
	if m("C:\\Windows\\explorer.exe") {
		t.Error("regex should not match a non-matching path")
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
	w, finish := newHashers()
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
