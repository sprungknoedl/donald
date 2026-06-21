package main

import (
	"os"
	"slices"
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

// exerciseMatchers calls every matcher with a sample path. A matcher built from
// an invalid pattern used to compile to a nil engine whose method value was
// returned as the Matcher, so calling it dereferenced a nil pointer and
// panicked. ParseQuack now rejects such patterns, but this guards against a
// regression: any matcher it does return must be safe to call.
func exerciseMatchers(t *testing.T, matchers []Matcher) {
	t.Helper()
	for _, m := range matchers {
		m.Match("c:\\programdata\\foobarba\\quack.exe")
	}
}

// TestParseQuackInvalidRegex guards against the nil-pointer panic from an
// invalid regex pattern. `\P` is a Unicode-property escape in Go's RE2 engine,
// so `\ProgramData` does not compile. ParseQuack must reject the line with an
// error (or hand back a matcher that is safe to call) -- it must never panic.
func TestParseQuackInvalidRegex(t *testing.T) {
	matchers, _, err := ParseQuack(strings.NewReader("regex\t^\\ProgramData\\x"))
	if err != nil {
		return // expected: the bad pattern is rejected up front
	}
	exerciseMatchers(t, matchers) // must not panic
}

// TestParseQuackInvalidGlob is the glob-pattern counterpart: an unterminated
// character class fails to compile, and ParseQuack must surface that as an
// error rather than yield a matcher backed by a nil glob.
func TestParseQuackInvalidGlob(t *testing.T) {
	matchers, _, err := ParseQuack(strings.NewReader("glob\t[unterminated"))
	if err != nil {
		return // expected: the bad pattern is rejected up front
	}
	exerciseMatchers(t, matchers) // must not panic
}

// TestParseQuackExampleFile guards the originally reported crash: parsing the
// bundled targets/example.quack must not panic. Its regex examples must stay
// valid Go patterns, so ParseQuack is expected to succeed and the matchers to
// be safe to call.
func TestParseQuackExampleFile(t *testing.T) {
	fh, err := os.Open("targets/example.quack")
	if err != nil {
		t.Fatalf("open example.quack: %v", err)
	}
	defer fh.Close()

	matchers, _, err := ParseQuack(fh)
	if err != nil {
		t.Fatalf("ParseQuack(example.quack): %v", err)
	}
	exerciseMatchers(t, matchers) // must not panic
}

// TestParseQuackPrefixes checks that parsed matchers carry the right literal
// prefixes (so prunable can derive them) and that a single unprunable matcher
// disables pruning.
func TestParseQuackPrefixes(t *testing.T) {
	matchers, _, err := ParseQuack(strings.NewReader("static\t/etc/passwd\nglob\t/var/log/**\nglob\t**/*.plist"))
	if err != nil {
		t.Fatalf("ParseQuack: %v", err)
	}
	prefixes, prune := prunable(matchers)
	if want := []string{"/etc/passwd", "/var/log/", ""}; !slices.Equal(prefixes, want) {
		t.Errorf("prefixes = %v, want %v", prefixes, want)
	}
	if prune {
		t.Error("prune should be false: the leading-** glob has no literal prefix")
	}

	// A regex matcher is unprunable too (no literal-prefix extraction from regex),
	// so its empty prefix disables pruning just like the leading-** glob does.
	matchers, _, err = ParseQuack(strings.NewReader("glob\t/var/log/**\nregex\t^/etc/.*$"))
	if err != nil {
		t.Fatalf("ParseQuack: %v", err)
	}
	if prefixes, prune := prunable(matchers); prune {
		t.Errorf("prune should be false: a regex matcher has no literal prefix (prefixes=%v)", prefixes)
	}

	// Drop both unprunable matchers and pruning becomes possible.
	matchers, _, err = ParseQuack(strings.NewReader("static\t/etc/passwd\nglob\t/var/log/**"))
	if err != nil {
		t.Fatalf("ParseQuack: %v", err)
	}
	if prefixes, prune := prunable(matchers); !prune {
		t.Errorf("prune should be true: every matcher has a literal prefix (prefixes=%v)", prefixes)
	}
}

// TestParseQuackForceNoPrefix confirms force lines contribute no matcher (hence
// no prefix) and do not disable pruning.
func TestParseQuackForceNoPrefix(t *testing.T) {
	matchers, forced, err := ParseQuack(strings.NewReader("force\t/some/path\nglob\t/var/log/**"))
	if err != nil {
		t.Fatalf("ParseQuack: %v", err)
	}
	prefixes, prune := prunable(matchers)
	if want := []string{"/var/log/"}; !slices.Equal(prefixes, want) {
		t.Errorf("prefixes = %v, want %v", prefixes, want)
	}
	if want := []string{"/some/path"}; !slices.Equal(forced, want) {
		t.Errorf("forced = %v, want %v", forced, want)
	}
	if !prune {
		t.Error("prune should be true: the force line does not disable pruning")
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
