package main

import (
	"archive/zip"
)

func DefaulRootPaths() []string {
	return []string{
		"/",
	}
}

func DefaultCollection() []Matcher {
	return []Matcher{
		// == static matchers
		NewStaticMatcher("/etc/hosts.allow"),
		NewStaticMatcher("/etc/hosts.deny"),
		NewStaticMatcher("/etc/hosts"),
		NewStaticMatcher("/private/etc/hosts.allow"),
		NewStaticMatcher("/private/etc/hosts.deny"),
		NewStaticMatcher("/private/etc/hosts"),
		NewStaticMatcher("/etc/passwd"),
		NewStaticMatcher("/etc/group"),
		NewStaticMatcher("/private/etc/passwd"),
		NewStaticMatcher("/private/etc/group"),

		// == glob matchers
		NewGlobMatcher("**/Library/*Support/Google/Chrome/Default/*"),
		NewGlobMatcher("**/Library/*Support/Google/Chrome/Default/History*"),
		NewGlobMatcher("**/Library/*Support/Google/Chrome/Default/Cookies*"),
		NewGlobMatcher("**/Library/*Support/Google/Chrome/Default/Bookmarks*"),
		NewGlobMatcher("**/Library/*Support/Google/Chrome/Default/Extensions/**"),
		NewGlobMatcher("**/Library/*Support/Google/Chrome/Default/Last*"),
		NewGlobMatcher("**/Library/*Support/Google/Chrome/Default/Shortcuts*"),
		NewGlobMatcher("**/Library/*Support/Google/Chrome/Default/Top*"),
		NewGlobMatcher("**/Library/*Support/Google/Chrome/Default/Visited*"),
		NewGlobMatcher("**/places.sqlite*"),
		NewGlobMatcher("**/downloads.sqlite*"),
		NewGlobMatcher("**/*.plist"),
		NewGlobMatcher("/Users/*/.*history"),
		NewGlobMatcher("/root/.*history"),
		NewGlobMatcher("/System/Library/StartupItems/**"),
		NewGlobMatcher("/System/Library/LaunchAgents/**"),
		NewGlobMatcher("/System/Library/LaunchDaemons/**"),
		NewGlobMatcher("/Library/LaunchAgents/**"),
		NewGlobMatcher("/Library/LaunchDaemons/**"),
		NewGlobMatcher("/Library/StartupItems/**"),
		NewGlobMatcher("/var/log/**"),
		NewGlobMatcher("/private/var/log/**"),
		NewGlobMatcher("/private/etc/rc.d/**"),
		NewGlobMatcher("/etc/rc.d/**"),
		NewGlobMatcher("/.fseventsd/**"),

		// == regexp matchers
		// nil
	}
}

func ForcedFiles() []string {
	return []string{}
}

func CollectFileRaw(cfg *Configuration, archive *zip.Writer, path string) error {
	return CollectFile(cfg, archive, path)
}
