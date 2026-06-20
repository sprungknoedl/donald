package main

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	zip "github.com/yeka/zip"
	"www.velocidex.com/golang/go-ntfs/parser"

	_ "embed"
)

//go:embed targets/default_windows.quack
var defaultQuack []byte

func DefaulRootPaths() []string {
	return []string{
		"C:\\",
	}
}

func GetPathsRaw(cfg Configuration) ([]CollectTarget, error) {
	scanned := 0
	matchers, forced, err := LoadMatchers(cfg)
	if err != nil {
		return nil, fmt.Errorf("load matchers: %w", err)
	}

	var targets []CollectTarget
	for _, p := range forced {
		targets = append(targets, CollectTarget{Path: p, Source: "force"})
	}

	roots := cfg.CollectionRoots
	if len(roots) == 0 {
		roots = DefaulRootPaths()
	}

	for _, root := range roots {
		driveLetter := filepath.VolumeName(root)
		fd, err := os.Open("\\\\.\\" + driveLetter)
		if err != nil {
			return nil, fmt.Errorf("open drive: %s: %w", root, err)
		}

		defer fd.Close()
		drive := NewSectorReaderAt(fd, 512)
		ntfs, err := parser.GetNTFSContext(drive, 0)
		if err != nil {
			return nil, fmt.Errorf("get ntfs context: %w", err)
		}

		mft, err := ntfs.GetMFT(5)
		if err != nil {
			return nil, fmt.Errorf("get mft 5: %w", err)
		}

		mft, err = mft.Open(ntfs, "")
		if err != nil {
			return nil, fmt.Errorf("open mft: %w", err)
		}

		err = walkDirRaw(ntfs, root, mft, func(path string, info *parser.FileInfo, err error) error {
			if err != nil {
				WarnLogger.Printf("traverse | %v", err)
				Jrnl.RecordDirSkipped(path, err)
				return fs.SkipDir
			}

			scanned++
			pathTrimmed := strings.TrimPrefix(path, root)
			if !info.IsDir {
				for _, match := range matchers {
					if match(path) || match(pathTrimmed) {
						targets = append(targets, CollectTarget{Path: path, Source: "match"})
						break
					}
				}
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

func CollectFileRaw(cfg Configuration, archive *zip.Writer, path string) (string, int64, string, string, error) {
	rel, err := filepath.Rel(filepath.VolumeName(path)+"/", filepath.ToSlash(path))
	if err != nil {
		return "", 0, "", "", err
	}

	driveLetter := filepath.VolumeName(path)
	fd, err := os.Open("\\\\.\\" + driveLetter)
	if err != nil {
		return rel, 0, "", "", fmt.Errorf("open drive: %s: %w", path, err)
	}

	defer fd.Close()
	drive := NewSectorReaderAt(fd, 512)
	ntfsCtx, err := parser.GetNTFSContext(drive, 0)
	if err != nil {
		return rel, 0, "", "", fmt.Errorf("get ntfs context: %w", err)
	}

	r, err := parser.GetDataForPath(ntfsCtx, rel)
	if err != nil {
		return rel, 0, "", "", fmt.Errorf("get data stream: %w", err)
	}

	fh, err := archiveEntry(cfg, archive, rel)
	if err != nil {
		return rel, 0, "", "", fmt.Errorf("add file to archive: %w", err)
	}

	// Tap the digests off the streaming read: hash the source bytes as they
	// are written to the archive, with no second read.
	h256 := sha256.New()
	hmd5 := md5.New()
	buf := make([]byte, 1024*1024*10)
	offset := int64(0)
	size := int64(0)
	for {
		n, err := r.ReadAt(buf, offset)
		if n == 0 || err != nil {
			if err == nil || errors.Is(err, io.EOF) {
				return rel, size, hex.EncodeToString(h256.Sum(nil)), hex.EncodeToString(hmd5.Sum(nil)), nil
			}
			return rel, 0, "", "", fmt.Errorf("read from disk: %w", err)
		}

		_, err = fh.Write(buf[:n])
		if err != nil {
			return rel, 0, "", "", fmt.Errorf("write to archive: %w", err)
		}
		h256.Write(buf[:n])
		hmd5.Write(buf[:n])
		size += int64(n)

		offset += int64(n)
	}
}

type SectorReaderAt struct {
	r          io.ReaderAt
	sectorSize int
}

func NewSectorReaderAt(r io.ReaderAt, sectorSize int) *SectorReaderAt {
	return &SectorReaderAt{r: r, sectorSize: sectorSize}
}

func (r *SectorReaderAt) ReadAt(p []byte, off int64) (int, error) {
	sector := int(off) / r.sectorSize
	sectorOff := int64(sector) * int64(r.sectorSize)
	misalignment := int(off) % r.sectorSize
	size := roundUp(len(p)+int(misalignment), r.sectorSize)

	buf := make([]byte, size)
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
