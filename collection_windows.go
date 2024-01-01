package main

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"www.velocidex.com/golang/go-ntfs/parser"
)

func DefaulRootPaths() []string {
	return []string{
		"C:\\",
	}
}

func DefaultCollection() []Matcher {
	return []Matcher{
		// == static matchers
		NewStaticMatcher("Windows\\SchedLgU.Txt"),
		NewStaticMatcher("Windows\\inf\\setupapi.dev.log"),
		NewStaticMatcher("Windows\\System32\\drivers\\etc\\hosts"),
		NewStaticMatcher("Windows\\System32\\config\\SAM"),
		NewStaticMatcher("Windows\\System32\\config\\SYSTEM"),
		NewStaticMatcher("Windows\\System32\\config\\SOFTWARE"),
		NewStaticMatcher("Windows\\System32\\config\\SECURITY"),
		NewStaticMatcher("Windows\\System32\\config\\SAM.LOG1"),
		NewStaticMatcher("Windows\\System32\\config\\SYSTEM.LOG1"),
		NewStaticMatcher("Windows\\System32\\config\\SOFTWARE.LOG1"),
		NewStaticMatcher("Windows\\System32\\config\\SECURITY.LOG1"),
		NewStaticMatcher("Windows\\System32\\config\\SAM.LOG2"),
		NewStaticMatcher("Windows\\System32\\config\\SYSTEM.LOG2"),
		NewStaticMatcher("Windows\\System32\\config\\SOFTWARE.LOG2"),
		NewStaticMatcher("Windows\\System32\\config\\SECURITY.LOG2"),

		// == glob matchers
		NewGlobMatcher("Windows\\Tasks\\**"),
		NewGlobMatcher("Windows\\Prefetch\\**"),
		NewGlobMatcher("Windows\\System32\\sru\\**"),
		NewGlobMatcher("Windows\\System32\\winevt\\Logs\\**"),
		NewGlobMatcher("Windows\\System32\\Tasks\\**"),
		NewGlobMatcher("Windows\\System32\\LogFiles\\W3SVC1\\**"),
		NewGlobMatcher("Windows\\Appcompat\\Programs\\**"),
		NewGlobMatcher("ProgramData\\Microsoft\\Windows\\Start Menu\\Programs\\Startup\\**"),
		NewGlobMatcher("$Recycle.Bin\\**\\$I*"),
		NewGlobMatcher("$Recycle.Bin\\$I*"),

		NewGlobMatcher("Users\\*\\NTUSER.DAT"),
		NewGlobMatcher("Users\\*\\NTUSER.DAT.LOG1"),
		NewGlobMatcher("Users\\*\\NTUSER.DAT.LOG2"),
		NewGlobMatcher("Users\\*\\AppData\\Local\\Microsoft\\Windows\\UsrClass.dat"),
		NewGlobMatcher("Users\\*\\AppData\\Local\\Microsoft\\Windows\\UsrClass.dat.LOG1"),
		NewGlobMatcher("Users\\*\\AppData\\Local\\Microsoft\\Windows\\UsrClass.dat.LOG2"),
		NewGlobMatcher("Users\\*\\AppData\\Local\\Google\\Chrome\\User Data\\Default\\History"),
		NewGlobMatcher("Users\\*\\AppData\\Local\\Microsoft\\Edge\\User Data\\Default\\History"),
		NewGlobMatcher("Users\\*\\AppData\\Roaming\\Microsoft\\Windows\\PowerShell\\PSReadline\\ConsoleHost_history.txt"),
		NewGlobMatcher("Users\\*\\AppData\\Roaming\\Microsoft\\Windows\\Recent\\**"),
		NewGlobMatcher("Users\\*\\AppData\\Local\\Microsoft\\Windows\\WebCache\\**"),
		NewGlobMatcher("Users\\*\\AppData\\Roaming\\Microsoft\\Windows\\Recent\\AutomaticDestinations\\**"),
		NewGlobMatcher("Users\\*\\AppData\\Roaming\\Mozilla\\Firefox\\Profiles\\**"),
		NewGlobMatcher("Users\\*\\AppData\\Local\\ConnectedDevicesPlatform\\**"),
		NewGlobMatcher("Users\\*\\AppData\\Local\\Microsoft\\Windows\\Explorer\\**"),

		// == regexp matchers
		// nil
	}
}

func ForcedFiles() []string {
	return []string{
		"C:\\$LogFile",
		"C:\\$MFT",
		"C:\\$Extend\\$UsnJrnl:$J",
	}
}

func CollectFileRaw(cfg Configuration, archive *zip.Writer, path string) error {
	rel, err := filepath.Rel(filepath.VolumeName(path)+"/", filepath.ToSlash(path))
	if err != nil {
		return err
	}

	driveLetter := filepath.VolumeName(path)
	fd, err := os.Open("\\\\.\\" + driveLetter)
	if err != nil {
		return fmt.Errorf("open drive: %s: %w", path, err)
	}

	drive := NewSectorReaderAt(fd, 512)
	ntfsCtx, err := parser.GetNTFSContext(drive, 0)
	if err != nil {
		return fmt.Errorf("get ntfs context: %w", err)
	}

	r, err := parser.GetDataForPath(ntfsCtx, rel)
	if err != nil {
		return fmt.Errorf("get data stream: %w", err)
	}

	fh, err := archive.Create(rel)
	if err != nil {
		return fmt.Errorf("add file to archive: %w", err)
	}

	buf := make([]byte, 1024*1024*10)
	offset := int64(0)
	for {
		n, err := r.ReadAt(buf, offset)
		if n == 0 || err != nil {
			if err == nil || errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("read from disk: %w", err)
		}

		_, err = fh.Write(buf[:n])
		if err != nil {
			return fmt.Errorf("write to archive: %w", err)
		}

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
