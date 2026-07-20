//go:build windows

package main

import (
	"strings"
	"testing"
)

// TestRelSlashWindows confirms relSlash never emits a backslash for
// Windows-shaped inputs, including ADS-suffixed paths raw NTFS collection
// produces (e.g. $Extend\$UsnJrnl:$J).
func TestRelSlashWindows(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{`C:\Windows\System32\Tasks\foo.job`, "Windows/System32/Tasks/foo.job"},
		{`C:\$Extend\$UsnJrnl:$J`, "$Extend/$UsnJrnl:$J"},
	}

	for _, c := range cases {
		rel, err := relSlash(c.path)
		if err != nil {
			t.Fatalf("relSlash(%q): %v", c.path, err)
		}
		if rel != c.want {
			t.Errorf("relSlash(%q) = %q, want %q", c.path, rel, c.want)
		}
		if strings.Contains(rel, `\`) {
			t.Errorf("relSlash(%q) = %q, contains backslash", c.path, rel)
		}
	}
}
