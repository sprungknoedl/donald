package main

import (
	"strings"
	"testing"
)

func TestParseQuackMatcherTypes(t *testing.T) {
	in := strings.Join([]string{
		"# a comment",
		"",
		"static\tC:\\Windows\\System32\\config\\SAM",
		"glob\tC:\\Users\\*\\NTUSER.DAT",
		"regex\t\\\\\\$MFT$",
		"force\tC:\\$Extend\\$UsnJrnl:$J",
	}, "\n")

	matchers, paths, err := ParseQuack(strings.NewReader(in))
	if err != nil {
		t.Fatalf("ParseQuack: unexpected error: %v", err)
	}

	// static, glob and regex become matchers; force becomes a path.
	if len(matchers) != 3 {
		t.Errorf("matchers: got %d, want 3", len(matchers))
	}
	if len(paths) != 1 {
		t.Fatalf("paths: got %d, want 1", len(paths))
	}
	if paths[0] != "C:\\$Extend\\$UsnJrnl:$J" {
		t.Errorf("force path: got %q", paths[0])
	}
}

func TestParseQuackSkipsBlankAndComments(t *testing.T) {
	// Blank lines and #-comments are skipped; only the static line is a matcher.
	in := "# header\n\nstatic\tC:\\Windows\n\n# trailing comment\n"
	matchers, paths, err := ParseQuack(strings.NewReader(in))
	if err != nil {
		t.Fatalf("ParseQuack: unexpected error: %v", err)
	}
	if len(matchers) != 1 || len(paths) != 0 {
		t.Errorf("got %d matchers, %d paths; want 1, 0", len(matchers), len(paths))
	}
}

func TestParseQuackMissingTab(t *testing.T) {
	_, _, err := ParseQuack(strings.NewReader("static C:\\no\\tab\\here"))
	if err == nil {
		t.Fatal("expected error for missing tab delimiter, got nil")
	}
	if !strings.Contains(err.Error(), "missing tab delimiter") {
		t.Errorf("error = %q, want it to mention the missing tab", err)
	}
}

func TestParseQuackUnknownMatcher(t *testing.T) {
	_, _, err := ParseQuack(strings.NewReader("bogus\tsome\\path"))
	if err == nil {
		t.Fatal("expected error for unknown matcher, got nil")
	}
	if !strings.Contains(err.Error(), "unknown matcher") {
		t.Errorf("error = %q, want it to mention the unknown matcher", err)
	}
}

func TestParseQuackEmptyInput(t *testing.T) {
	matchers, paths, err := ParseQuack(strings.NewReader(""))
	if err != nil {
		t.Fatalf("ParseQuack: unexpected error: %v", err)
	}
	if len(matchers) != 0 || len(paths) != 0 {
		t.Errorf("empty input: got %d matchers, %d paths; want 0, 0", len(matchers), len(paths))
	}
}
