package main

import (
	"fmt"
	"os"
	"path/filepath"

	zip "github.com/sprungknoedl/zip"
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

// openNTFSVolume opens the raw volume backing root (e.g. \\.\C:) and builds an
// NTFS context over it. This is the only genuinely Windows-specific step of the
// raw codepath; everything downstream is platform-independent (see ntfs.go).
// The returned closer must be called to release the device handle.
func openNTFSVolume(root string) (*parser.NTFSContext, func() error, error) {
	driveLetter := filepath.VolumeName(root)
	fd, err := os.Open("\\\\.\\" + driveLetter)
	if err != nil {
		return nil, nil, fmt.Errorf("open drive: %s: %w", root, err)
	}

	drive := NewSectorReaderAt(fd, 512)
	ntfs, err := parser.GetNTFSContext(drive, 0)
	if err != nil {
		fd.Close()
		return nil, nil, fmt.Errorf("get ntfs context: %w", err)
	}

	return ntfs, fd.Close, nil
}

func GetPathsRaw(cfg Configuration) ([]CollectTarget, error) {
	matchers, targets, roots, err := loadTargetsAndRoots(cfg)
	if err != nil {
		return nil, err
	}

	scanned := 0
	for _, root := range roots {
		ntfs, closeVol, err := openNTFSVolume(root)
		if err != nil {
			return nil, err
		}

		var n int
		targets, n, err = walkNTFS(ntfs, root, matchers, targets)
		scanned += n
		closeVol()
		if err != nil {
			return targets, err
		}
	}

	Jrnl.SetScanned(scanned)
	InfoLogger.Printf("traverse | scanned %d paths, resulted in %d files to collect", scanned, len(targets))
	return targets, nil
}

// rawCollector holds an open volume handle and its parsed NTFS context, reused
// across all files collected from one drive during stage 2.
type rawCollector struct {
	fd  *os.File
	ctx *parser.NTFSContext
	// buf is the 10 MB read buffer collectFromNTFS streams each file through,
	// reused across every file from this drive. Stage 2 collects sequentially,
	// so this is NOT safe for concurrent use.
	buf []byte
}

// rawCollectors caches one open volume + NTFS context per drive letter for the
// duration of stage 2. Stage 2 collects sequentially, so no locking is needed.
var rawCollectors = map[string]*rawCollector{}

// rawCollectorFor returns the cached collector for driveLetter, opening the raw
// volume and parsing its NTFS context on first use. Reusing the context avoids
// re-parsing the whole volume (boot sector + MFT bootstrap) for every file.
func rawCollectorFor(driveLetter string) (*rawCollector, error) {
	if c, ok := rawCollectors[driveLetter]; ok {
		return c, nil
	}

	fd, err := os.Open("\\\\.\\" + driveLetter)
	if err != nil {
		return nil, fmt.Errorf("open drive: %s: %w", driveLetter, err)
	}

	drive := NewSectorReaderAt(fd, 512)
	ntfs, err := parser.GetNTFSContext(drive, 0)
	if err != nil {
		fd.Close()
		return nil, fmt.Errorf("get ntfs context: %w", err)
	}

	c := &rawCollector{fd: fd, ctx: ntfs, buf: make([]byte, 1024*1024*10)}
	rawCollectors[driveLetter] = c
	return c, nil
}

// CloseRawCollectors releases every cached volume handle and clears the cache.
func CloseRawCollectors() {
	for k, c := range rawCollectors {
		c.fd.Close()
		delete(rawCollectors, k)
	}
}

func CollectFileRaw(cfg Configuration, archive *zip.Writer, path string) (string, int64, string, string, error) {
	rel, err := filepath.Rel(filepath.VolumeName(path)+"/", filepath.ToSlash(path))
	if err != nil {
		return "", 0, "", "", err
	}

	c, err := rawCollectorFor(filepath.VolumeName(path))
	if err != nil {
		return rel, 0, "", "", err
	}

	return collectFromNTFS(cfg, archive, c.ctx, rel, c.buf)
}
