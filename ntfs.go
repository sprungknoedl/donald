package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	zip "github.com/sprungknoedl/zip"
	"www.velocidex.com/golang/go-ntfs/parser"
)

// This file holds the platform-independent raw-NTFS logic. It operates purely on
// a *parser.NTFSContext (an io.ReaderAt under the hood), so it compiles and runs
// on every platform and is exercised by ntfs_test.go against a vendored image.
// The Windows-only device layer (opening \\.\C:) lives in collection_windows.go,
// which acquires the context and then calls into the cores below.

// walkNTFS walks one NTFS volume (already opened as ntfs) rooted at root,
// appending a "match" target for every non-directory path that matches. It
// returns the grown target slice and the number of paths scanned. Mirrors the
// per-root body of the plain GetPaths walk, but over the raw MFT.
func walkNTFS(ntfs *parser.NTFSContext, root string, matchers []Matcher, set map[string]bool, targets []CollectTarget) ([]CollectTarget, int, error) {
	mft, err := ntfs.GetMFT(5)
	if err != nil {
		return targets, 0, fmt.Errorf("get mft 5: %w", err)
	}

	mft, err = mft.Open(ntfs, "")
	if err != nil {
		return targets, 0, fmt.Errorf("open mft: %w", err)
	}

	scanned := 0
	err = walkDirRaw(ntfs, root, mft, func(path string, info *parser.FileInfo, err error) error {
		if err != nil {
			WarnLogger.Printf("traverse | %v", err)
			Jrnl.RecordDirSkipped(path, err)
			return fs.SkipDir
		}

		if info.IsDir && shouldSkipDir(set, path) {
			return fs.SkipDir
		}

		scanned++
		if !info.IsDir {
			targets = appendIfMatch(targets, matchers, path, root)
		}

		return nil
	})
	return targets, scanned, err
}

// collectFromNTFS streams the data for rel out of the already-opened context
// into the archive, returning the entry name, byte size, and SHA-256/MD5 of the
// source bytes. rel is the volume-relative path (forward slashes, optionally
// with an :ADS suffix); the caller is responsible for stripping the volume name.
// Digests are tapped off the streaming read with no second pass.
// buf is the caller-supplied read buffer the stream is copied through; it is
// reused across files and is NOT safe for concurrent use.
func collectFromNTFS(cfg Configuration, archive *zip.Writer, ntfs *parser.NTFSContext, rel string, buf []byte) (string, int64, string, string, error) {
	r, err := parser.GetDataForPath(ntfs, rel)
	if err != nil {
		return rel, 0, "", "", fmt.Errorf("get data stream: %w", err)
	}

	fh, err := createNamedEntry(cfg, archive, rel, ntfsMtime(ntfs, rel))
	if err != nil {
		return rel, 0, "", "", fmt.Errorf("add file to archive: %w", err)
	}

	// Tap the digests off the streaming read: hash the source bytes as they
	// are written to the archive, with no second read.
	hashes, digests := NewHashers()
	offset := int64(0)
	size := int64(0)
	for {
		n, err := r.ReadAt(buf, offset)
		if n == 0 || err != nil {
			if err == nil || errors.Is(err, io.EOF) {
				sha256sum, md5sum := digests()
				return rel, size, sha256sum, md5sum, nil
			}
			return rel, 0, "", "", fmt.Errorf("read from disk: %w", err)
		}

		_, err = fh.Write(buf[:n])
		if err != nil {
			return rel, 0, "", "", fmt.Errorf("write to archive: %w", err)
		}
		hashes.Write(buf[:n])
		size += int64(n)

		offset += int64(n)
	}
}

// ntfsMtime resolves rel to its MFT entry and returns the file's
// $STANDARD_INFORMATION modified time, so a raw-collected entry carries its real
// on-disk timestamp rather than the archive-write time. Best-effort: any
// resolution failure yields the zero time (the entry is written without a source
// mod-time). Any :ADS suffix is stripped — the timestamp belongs to the base record.
func ntfsMtime(ntfs *parser.NTFSContext, rel string) time.Time {
	root, err := ntfs.GetMFT(5)
	if err != nil {
		return time.Time{}
	}

	entry, err := root.Open(ntfs, strings.SplitN(rel, ":", 2)[0])
	if err != nil {
		return time.Time{}
	}

	infos := parser.Stat(ntfs, entry)
	if len(infos) == 0 {
		return time.Time{}
	}
	return infos[0].Mtime
}

// SectorReaderAt adapts an io.ReaderAt to the sector-aligned reads a raw NTFS
// volume device requires: every underlying ReadAt starts on a sectorSize
// boundary and spans a whole number of sectors. Harmless over a plain file too,
// which is how the tests exercise it.
type SectorReaderAt struct {
	r          io.ReaderAt
	sectorSize int
	// scratch backs the misaligned read path, grown and reused across calls. It
	// is NOT safe for concurrent ReadAt on the same SectorReaderAt.
	scratch []byte
}

func NewSectorReaderAt(r io.ReaderAt, sectorSize int) *SectorReaderAt {
	return &SectorReaderAt{r: r, sectorSize: sectorSize}
}

func (r *SectorReaderAt) ReadAt(p []byte, off int64) (int, error) {
	// Aligned fast path: the collection loop issues only whole-sector reads on
	// sector boundaries, so read straight into the caller's buffer — no scratch
	// buffer, no copy.
	if off%int64(r.sectorSize) == 0 && len(p)%r.sectorSize == 0 {
		return r.r.ReadAt(p, off)
	}

	sector := int(off) / r.sectorSize
	sectorOff := int64(sector) * int64(r.sectorSize)
	misalignment := int(off) % r.sectorSize
	size := roundUp(len(p)+int(misalignment), r.sectorSize)

	if cap(r.scratch) < size {
		r.scratch = make([]byte, size)
	}
	buf := r.scratch[:size]
	n, err := r.r.ReadAt(buf, sectorOff)
	copy(p, buf[misalignment:])
	return n, err
}

func roundUp(num int, multiple int) int {
	if multiple == 0 {
		return num
	}

	remainder := num % multiple
	if remainder == 0 {
		return num
	}

	return num + multiple - remainder
}

// walkDirRaw recursively walks the MFT directory tree from mft, calling
// walkDirFn for each entry (fs.SkipDir on a directory prunes it). It guards
// against the self-referential entries that would otherwise loop forever.
func walkDirRaw(ctx *parser.NTFSContext, path string, mft *parser.MFT_ENTRY, walkDirFn func(path string, d *parser.FileInfo, err error) error) error {
	fi := parser.Stat(ctx, mft)[0]
	if err := walkDirFn(path, fi, nil); err != nil || !fi.IsDir {
		if err == fs.SkipDir && fi.IsDir {
			// Successfully skipped directory.
			err = nil
		}
		return err
	}

	records := mft.Dir(ctx)
	for _, r1 := range records {
		path1 := filepath.Join(path, r1.File().Name())
		if r1.File().Name() == "" || path == path1 {
			// avoid infite loop
			continue
		}

		mft1, err := ctx.GetMFT(int64(r1.MftReference()))
		if err != nil {
			return err
		}

		if err := walkDirRaw(ctx, path1, mft1, walkDirFn); err != nil {
			if err == fs.SkipDir {
				break
			}
			return err
		}
	}

	return nil
}
