package main

import (
	"os"
	"path/filepath"
	"testing"
)

// writeKapeFixtures writes the given name->content .tkape files into a fresh
// temp dir and returns the dir.
func writeKapeFixtures(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write fixture %s: %v", name, err)
		}
	}
	return dir
}

func TestParseKapeRecursiveTarget(t *testing.T) {
	dir := writeKapeFixtures(t, map[string]string{
		"evt.tkape": "" +
			"Description: Event logs\n" +
			"Targets:\n" +
			"  - Name: EventLogs\n" +
			"    Path: C:\\Windows\\System32\\winevt\\Logs\n" +
			"    Recursive: true\n",
	})

	matchers, paths, err := ParseKapeTargets("evt", dir)
	if err != nil {
		t.Fatalf("ParseKapeTargets: %v", err)
	}
	if len(paths) != 0 {
		t.Errorf("paths: got %d, want 0", len(paths))
	}
	if len(matchers) != 1 {
		t.Fatalf("matchers: got %d, want 1", len(matchers))
	}
	// Recursive + no FileMask -> "<path>\**", which should match nested files.
	if !matchers[0]("c:\\windows\\system32\\winevt\\logs\\system.evtx") {
		t.Error("recursive target should match a nested log file")
	}
}

// The command-line target name must be given without the .tkape extension
// (matching KAPE and the -kt flag help); passing the extension does not resolve.
func TestParseKapeNameWithExtensionRejected(t *testing.T) {
	dir := writeKapeFixtures(t, map[string]string{
		"evt.tkape": "" +
			"Targets:\n" +
			"  - Name: EventLogs\n" +
			"    Path: C:\\Windows\\System32\\winevt\\Logs\n" +
			"    Recursive: true\n",
	})

	if _, _, err := ParseKapeTargets("evt.tkape", dir); err == nil {
		t.Fatal("expected an error when the -kt name includes the .tkape extension, got nil")
	}
}

func TestParseKapeFileMaskTarget(t *testing.T) {
	dir := writeKapeFixtures(t, map[string]string{
		"pf.tkape": "" +
			"Targets:\n" +
			"  - Name: Prefetch\n" +
			"    Path: C:\\Windows\\Prefetch\n" +
			"    FileMask: '*.pf'\n",
	})

	matchers, _, err := ParseKapeTargets("pf", dir)
	if err != nil {
		t.Fatalf("ParseKapeTargets: %v", err)
	}
	if len(matchers) != 1 {
		t.Fatalf("matchers: got %d, want 1", len(matchers))
	}
	if !matchers[0]("c:\\windows\\prefetch\\foo.pf") {
		t.Error("FileMask target should match a *.pf file")
	}
	if matchers[0]("c:\\windows\\prefetch\\foo.txt") {
		t.Error("FileMask target should not match a non-.pf file")
	}
}

func TestParseKapeUserPlaceholder(t *testing.T) {
	dir := writeKapeFixtures(t, map[string]string{
		"reg.tkape": "" +
			"Targets:\n" +
			"  - Name: NTUSER\n" +
			"    Path: C:\\Users\\%user%\n" +
			"    FileMask: NTUSER.DAT\n",
	})

	matchers, _, err := ParseKapeTargets("reg", dir)
	if err != nil {
		t.Fatalf("ParseKapeTargets: %v", err)
	}
	if len(matchers) != 1 {
		t.Fatalf("matchers: got %d, want 1", len(matchers))
	}
	// %user% is rewritten to * so any user's hive matches.
	if !matchers[0]("c:\\users\\alice\\ntuser.dat") {
		t.Error("%user% should expand to match any username")
	}
}

func TestParseKapeNestedReference(t *testing.T) {
	dir := writeKapeFixtures(t, map[string]string{
		"evt.tkape": "" +
			"Targets:\n" +
			"  - Name: EventLogs\n" +
			"    Path: C:\\Windows\\System32\\winevt\\Logs\n" +
			"    Recursive: true\n",
		"pf.tkape": "" +
			"Targets:\n" +
			"  - Name: Prefetch\n" +
			"    Path: C:\\Windows\\Prefetch\n" +
			"    FileMask: '*.pf'\n",
		"compound.tkape": "" +
			"Targets:\n" +
			"  - Path: evt.tkape\n" +
			"  - Path: pf.tkape\n",
	})

	matchers, _, err := ParseKapeTargets("compound", dir)
	if err != nil {
		t.Fatalf("ParseKapeTargets: %v", err)
	}
	// The compound target flattens both referenced targets' matchers.
	if len(matchers) != 2 {
		t.Fatalf("matchers: got %d, want 2", len(matchers))
	}

	matchAny := func(path string) bool {
		for _, m := range matchers {
			if m(path) {
				return true
			}
		}
		return false
	}
	if !matchAny("c:\\windows\\system32\\winevt\\logs\\system.evtx") {
		t.Error("compound should include the recursive event-log matcher")
	}
	if !matchAny("c:\\windows\\prefetch\\foo.pf") {
		t.Error("compound should include the prefetch matcher")
	}
}

func TestParseKapeMissingTarget(t *testing.T) {
	dir := writeKapeFixtures(t, map[string]string{})

	_, _, err := ParseKapeTargets("does-not-exist", dir)
	if err == nil {
		t.Fatal("expected an error for an unknown target name, got nil")
	}
}
