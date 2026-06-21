package main

import (
	"bytes"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/gobwas/glob"
	zip "github.com/sprungknoedl/zip"
)

// Matcher pairs a path predicate with the literal path prefix it can match under.
// Matching is case-insensitive, mirroring CyLR's behaviour, but Match's input is
// expected to be already lowercased by the caller: appendIfMatch folds each path
// once and passes the folded form, so Match must not re-fold its input.
//
// Prefix is the lowercased literal path prefix every match must start with (e.g.
// "/var/log/" for the glob "/var/log/**"), or "" when the matcher is unprunable —
// its matches are not anchored to a literal prefix. The constructors derive it
// alongside Match so the two can never desync. Directory pruning (see prunable /
// shouldDescend) reads it to skip subtrees no matcher could match.
type Matcher struct {
	Match  func(string) bool
	Prefix string
}

func NewStaticMatcher(pattern string) Matcher {
	pattern = strings.ToLower(pattern)
	// A static pattern is a full literal path, so the whole thing is its prefix.
	return Matcher{
		Match:  func(filename string) bool { return pattern == filename },
		Prefix: pattern,
	}
}

func NewGlobMatcher(pattern string) (Matcher, error) {
	// The prefix is the literal run before the first glob meta character, taken
	// from the raw pattern (before backslash-escaping) so it keeps the same
	// separator form the walked directory paths use.
	prefix := strings.ToLower(literalGlobPrefix(pattern))
	pattern = strings.ReplaceAll(pattern, "\\", "\\\\")
	m, err := glob.Compile(strings.ToLower(pattern))
	if err != nil {
		return Matcher{}, err
	}
	return Matcher{
		Match:  func(filename string) bool { return m.Match(filename) },
		Prefix: prefix,
	}, nil
}

func NewRegexpMatcher(pattern string) (Matcher, error) {
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		// regexp wraps its message in "error parsing regexp: "; drop it since
		// the caller already labels the failure as an invalid regex.
		return Matcher{}, fmt.Errorf("%s", strings.TrimPrefix(err.Error(), "error parsing regexp: "))
	}
	// Extracting a literal prefix from a Go regex is not worth it, so a regex is
	// always unprunable. Note the cost: a single regex target leaves an empty
	// Prefix, which (via prunable) turns off directory pruning for the whole
	// collection and forces a full-volume walk, however tightly scoped the other
	// targets are.
	return Matcher{Match: re.MatchString, Prefix: ""}, nil
}

// literalGlobPrefix returns the leading run of pattern that contains no glob meta
// character, i.e. the literal path prefix every match must start with. The meta
// characters are *, ?, [ and { (gobwas/glob's openers); a pattern that starts
// with one yields "" (no usable prefix). Backslashes are treated as ordinary path
// bytes, not glob escapes — NewGlobMatcher escapes them so Windows separators
// match literally, so the extracted prefix keeps them in the same separator form
// the walked directory paths use.
func literalGlobPrefix(pattern string) string {
	if i := strings.IndexAny(pattern, "*?[{"); i >= 0 {
		return pattern[:i]
	}
	return pattern
}

// prunable derives the per-matcher literal prefixes and whether directory pruning
// is enabled. Pruning is a pure function of the loaded matchers, computed once
// before a walk: it is on only when there is at least one matcher and every one
// has a non-empty prefix. A single empty prefix (a leading-** glob or a regex
// matcher) means some matcher could match anywhere, so pruning must be disabled
// entirely to stay result-preserving.
func prunable(matchers []Matcher) (prefixes []string, prune bool) {
	prefixes = make([]string, len(matchers))
	for i, m := range matchers {
		prefixes[i] = m.Prefix
	}
	return prefixes, len(prefixes) > 0 && !slices.Contains(prefixes, "")
}

func LoadMatchers(cfg Configuration) ([]Matcher, []string, error) {
	// parse collector configuration
	if cfg.QuackTargets != "" {
		fh, err := os.Open(cfg.QuackTargets)
		if err != nil {
			return nil, nil, err
		}

		return ParseQuack(fh)
	} else if cfg.KapeTargets != "" {
		return ParseKapeTargets(cfg.KapeTargets, cfg.KapeFiles)
	} else {
		r := bytes.NewReader(defaultQuack)
		return ParseQuack(r)
	}
}

// skipDirSet builds the lowercased set of directories to prune during traversal.
// It uses cfg.SkipDirs when given, otherwise the per-OS DefaultSkipDirs() — same
// replace-not-add precedence as -root / DefaultRootPaths.
func skipDirSet(cfg Configuration) map[string]bool {
	dirs := cfg.SkipDirs
	if len(dirs) == 0 {
		dirs = DefaultSkipDirs()
	}

	set := make(map[string]bool, len(dirs))
	for _, d := range dirs {
		set[strings.ToLower(d)] = true
	}
	return set
}

// shouldSkipDir reports whether path is an exact (case-insensitive) member of the
// skip set, mirroring the static-matcher convention. The match is exact, not a
// prefix: a "/dev" entry does not skip "/devices".
func shouldSkipDir(set map[string]bool, path string) bool {
	return set[strings.ToLower(path)]
}

// shouldDescend reports whether dir is worth walking given the literal prefixes:
// true if dir shares a path with some prefix p — either dir is on the way down to
// p (dir is a path-prefix of p) or dir already sits inside p's subtree (p is a
// path-prefix of dir). Comparison is on segment boundaries so /var does not match
// /variant. dir and the prefixes are assumed already lowercased; the caller folds
// dir before calling, mirroring appendIfMatch.
func shouldDescend(prefixes []string, dir string) bool {
	for _, p := range prefixes {
		if pathHasPrefix(dir, p) || pathHasPrefix(p, dir) {
			return true
		}
	}
	return false
}

// pathHasPrefix reports whether path b lies at or below path a, comparing on
// segment boundaries: true if b equals a or b continues a right after a
// separator. Trailing separators on either operand are ignored, and both / and \
// count as separators, so it works for the unix and Windows path forms alike.
func pathHasPrefix(a, b string) bool {
	a = strings.TrimRight(a, "/\\")
	b = strings.TrimRight(b, "/\\")
	if !strings.HasPrefix(b, a) {
		return false
	}
	rest := b[len(a):]
	return rest == "" || rest[0] == '/' || rest[0] == '\\'
}

// loadTargetsAndRoots performs the shared prologue of both traversal codepaths:
// it loads the matchers, seeds the target list with the unconditional `force`
// paths, resolves the collection roots (falling back to the platform default),
// and resolves the skip-dir set once so both walks share it.
func loadTargetsAndRoots(cfg Configuration) (matchers []Matcher, targets []CollectTarget, roots []string, set map[string]bool, err error) {
	matchers, forced, err := LoadMatchers(cfg)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("load matchers: %w", err)
	}

	for _, p := range forced {
		targets = append(targets, CollectTarget{Path: p, Source: "force"})
	}

	roots = cfg.CollectionRoots
	if len(roots) == 0 {
		roots = DefaultRootPaths()
	}

	return matchers, targets, roots, skipDirSet(cfg), nil
}

// appendIfMatch tests path against every matcher, appending a "match" target on
// the first hit. path is matched as the full absolute path only — the form both
// walkers emit — so a pattern written relative to a non-root -root no longer
// matches; write it absolute or with a leading **. Shared by the normal and raw
// walks.
func appendIfMatch(targets []CollectTarget, matchers []Matcher, path string) []CollectTarget {
	// Fold the path once; every matcher reuses the lowercased form.
	pathLower := strings.ToLower(path)
	for _, m := range matchers {
		if m.Match(pathLower) {
			return append(targets, CollectTarget{Path: path, Source: "match"})
		}
	}
	return targets
}

func GetPaths(cfg Configuration) ([]CollectTarget, error) {
	scanned := 0
	matchers, targets, roots, set, err := loadTargetsAndRoots(cfg)
	if err != nil {
		return nil, err
	}

	// Decide pruning once: it is a pure function of the loaded matchers.
	prefixes, prune := prunable(matchers)

	for _, root := range roots {
		err = filepath.WalkDir(root, func(path string, info fs.DirEntry, err error) error {
			if err != nil {
				WarnLogger.Printf("traverse | %v", err)
				Jrnl.RecordDirSkipped(path, err)
				return fs.SkipDir
			}

			if info.IsDir() {
				if shouldSkipDir(set, path) {
					return fs.SkipDir
				}
				if prune && !shouldDescend(prefixes, strings.ToLower(path)) {
					return fs.SkipDir
				}
			}

			scanned++
			if !info.IsDir() {
				targets = appendIfMatch(targets, matchers, path)
			}

			return nil
		})
		if err != nil {
			return targets, err
		}
	}

	Jrnl.SetScanned(scanned)
	InfoLogger.Printf("traverse | scanned %d paths, resulted in %d files to collect", scanned, len(targets))
	return targets, err
}

// zipMethod returns the storage method for an entry given the configured
// -zip-level: Store for level 0 (true no-op container entries) and Deflate
// otherwise. Levels 1..9 are applied via the leveled compressor registered in
// step2CollectFiles; level -1 (unset) keeps the stdlib default Deflate.
func zipMethod(cfg Configuration) uint16 {
	if cfg.CompressionLevel == 0 {
		return zip.Store
	}
	return zip.Deflate
}

// createEntry stamps the cfg-driven compression method and encryption onto fh,
// then creates the archive entry. It is the single place the method/encrypt
// decision lives, so every entry — FileInfo-derived evidence and bare-header
// `_donald/` metadata alike — is compressed and protected identically. It builds
// from a caller-supplied FileHeader (rather than the Encrypt/Create helpers) so
// the source mod-time and mode survive onto the entry even for encrypted output.
func createEntry(cfg Configuration, archive *zip.Writer, fh *zip.FileHeader) (io.Writer, error) {
	fh.Method = zipMethod(cfg)
	if cfg.ZipPass != "" {
		fh.SetPassword(cfg.ZipPass)
		fh.SetEncryptionMethod(zip.AES256Encryption)
	}
	return archive.CreateHeader(fh)
}

// createNamedEntry creates an entry from just a name and modTime, for callers
// with no os.FileInfo to derive a header from: raw-NTFS evidence (mod-time from
// the MFT) and synthesized `_donald/` metadata.
func createNamedEntry(cfg Configuration, archive *zip.Writer, name string, modTime time.Time) (io.Writer, error) {
	return createEntry(cfg, archive, &zip.FileHeader{Name: name, Modified: modTime})
}

// NewHashers returns a writer feeding both a SHA-256 and an MD5 hasher and a
// finish func returning their hex digests. Collection tees the source→archive
// copy through the writer so the digests cover the plaintext source bytes with
// no extra read.
func NewHashers() (w io.Writer, finish func() (sha256sum, md5sum string)) {
	h256 := sha256.New()
	hmd5 := md5.New()
	return io.MultiWriter(h256, hmd5), func() (string, string) {
		return hex.EncodeToString(h256.Sum(nil)), hex.EncodeToString(hmd5.Sum(nil))
	}
}

func CollectFile(cfg Configuration, archive *zip.Writer, path string) (string, int64, string, string, error) {
	rel, _ := filepath.Rel(filepath.VolumeName(path)+"/", filepath.ToSlash(path))
	r, err := os.Open(path)
	if err != nil {
		return rel, 0, "", "", err
	}
	defer r.Close()

	// Build the entry header from the source FileInfo so it carries the file's
	// mod-time and mode, then layer encryption on top when configured — so
	// encrypted entries keep their mod-time too (a plain Encrypt() would drop it).
	fi, err := r.Stat()
	if err != nil {
		return rel, 0, "", "", err
	}

	fh, err := zip.FileInfoHeader(fi)
	if err != nil {
		return rel, 0, "", "", err
	}

	fh.Name = rel

	w, err := createEntry(cfg, archive, fh)
	if err != nil {
		return rel, 0, "", "", err
	}

	// Tee the source→archive copy through the hashers: digests cover the
	// plaintext source bytes, with no extra read. size is the bytes streamed.
	hashes, digests := NewHashers()
	size, err := io.Copy(io.MultiWriter(w, hashes), r)
	if err != nil {
		return rel, 0, "", "", err
	}

	sha256sum, md5sum := digests()
	return rel, size, sha256sum, md5sum, nil
}
