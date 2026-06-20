package main

import (
	"fmt"
	"os"
	"path/filepath"

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

func CollectFileRaw(cfg Configuration, archive *zip.Writer, path string) (string, int64, string, string, error) {
	rel, err := filepath.Rel(filepath.VolumeName(path)+"/", filepath.ToSlash(path))
	if err != nil {
		return "", 0, "", "", err
	}

	ntfs, closeVol, err := openNTFSVolume(filepath.VolumeName(path))
	if err != nil {
		return rel, 0, "", "", err
	}
	defer closeVol()

	return collectFromNTFS(cfg, archive, ntfs, rel)
}
