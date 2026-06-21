package main

import (
	"bytes"
	"compress/flate"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	zip "github.com/sprungknoedl/zip"
)

// Loggers for different log levels
var (
	WarnLogger *log.Logger
	InfoLogger *log.Logger
	ErrLogger  *log.Logger
)

// Configuration struct holds parameters for the application
type Configuration struct {
	OutputDir  string // Directory for the generated zip archive
	OutputFile string // Name of the generated zip archive file
	ZipPass    string // Password for the output zip archive (AES-256 if set)

	CompressionLevel int // Zip compression level: -1 (unset, stdlib default), 0 (store), 1..9 (deflate)

	SftpAddr string // SFTP server address
	SftpUser string // SFTP server username
	SftpPass string // SFTP server password
	SftpDir  string // Target directory on the SFTP server
	SftpFile string // Target filename on the SFTP server

	DagobertAddr string // Dagobert server URL
	DagobertCase string // Dagobert case id
	DagobertKey  string // Dagobert API key
	DagobertFile string // Target filename on Dagobert

	CollectionRoots []string // Search root paths
	QuackTargets    string   // Quack collection paths file
	KapeTargets     string   // Kape target name
	KapeFiles       string   // Directory with Kape target and module files

	RawAccess bool // Use raw NTFS access (Windows only)

	SkipTraversal  bool
	SkipCollection bool
	SkipUpload     bool
	SkipCleanup    bool
}

func main() {
	// Initialize loggers, teed into an in-memory transcript buffer so the
	// console output of stages 1-2 can be embedded in the output archive.
	var transcript bytes.Buffer
	out := io.MultiWriter(os.Stdout, &transcript)
	InfoLogger = log.New(out, "INFO: ", log.Ldate|log.Ltime)
	WarnLogger = log.New(out, "WARN: ", log.Ldate|log.Ltime)
	ErrLogger = log.New(out, "ERRR: ", log.Ldate|log.Ltime)
	Jrnl = NewJournal(&transcript)

	// Record the start time
	begin := time.Now()
	defer func() {
		// Log the total execution time
		InfoLogger.Printf("donald finished in %v.", time.Since(begin))
	}()

	// Parse command line parameters
	cfg, err := ParseConfig()
	if err != nil {
		ErrLogger.Fatalf("Coud not parse command line parameters: %q", err)
	}

	paths, err := step1TraverseFS(cfg)
	if err != nil {
		ErrLogger.Fatalf("Stage 1: Unrecoverable error: %v", err)
	}

	sum, err := step2CollectFiles(cfg, paths)
	if err != nil {
		ErrLogger.Fatalf("Stage 2: Unrecoverable error: %v", err)
	}

	err = step3UploadSFTP(cfg, sum)
	if err != nil {
		ErrLogger.Fatalf("Stage 3 (SFTP): Unrecoverable error: %v", err)
	}

	err = step3UploadDagobert(cfg, sum)
	if err != nil {
		ErrLogger.Fatalf("Stage 3 (Dagobert): Unrecoverable error: %v", err)
	}

	err = step4CleanUp(cfg)
	if err != nil {
		ErrLogger.Fatalf("Stage 4: Unrecoverable error: %v", err)
	}
}

// step1TraverseFS traverses the file system based on the provided Configuration,
// collects file paths, and logs the progress.
func step1TraverseFS(cfg Configuration) ([]CollectTarget, error) {
	Jrnl.started = time.Now()

	if cfg.SkipTraversal {
		InfoLogger.Println("Stage 1: Traversing file tree skipped.")
		return []CollectTarget{}, nil
	}

	// Log the start of Stage 1
	InfoLogger.Println("Stage 1: Traversing file tree ...")
	start := time.Now()

	var paths []CollectTarget
	var err error
	if cfg.RawAccess {
		paths, err = GetPathsRaw(cfg)
	} else {
		paths, err = GetPaths(cfg)
	}

	if err != nil {
		return nil, err
	}

	// Log the completion of Stage 1 along with the elapsed time
	InfoLogger.Printf("Stage 1 finished in %v", time.Since(start))
	return paths, nil
}

// step2CollectFiles collects files based on the provided file paths, creates a zip archive,
// and logs the progress.
func step2CollectFiles(cfg Configuration, paths []CollectTarget) (string, error) {
	if cfg.SkipCollection {
		InfoLogger.Println("Stage 2: Collecting files skipped.")
		return "", nil
	}

	// Log the start of Stage 2
	InfoLogger.Println("Stage 2: Collecting files ...")
	start := time.Now()

	if cfg.ZipPass != "" {
		InfoLogger.Println("archive encryption enabled (AES-256)")
	}

	// Create the output file for the zip archive
	fh, err := os.Create(filepath.Join(cfg.OutputDir, cfg.OutputFile))
	if err != nil {
		return "", err
	}

	// Tee every byte written to the archive into a SHA-256 hasher so we can
	// emit a sidecar attesting to the final (post-compression, post-encryption)
	// bytes on disk without re-reading the archive.
	h := sha256.New()
	archive := zip.NewWriter(io.MultiWriter(fh, h))

	// Honor -zip-level for the Deflate entries: levels 1..9 register a leveled
	// flate compressor used by every Deflate entry. Level 0 (Store) is set per
	// entry by the collect funcs; -1 (unset) registers nothing, keeping the
	// stdlib default Deflate.
	if cfg.CompressionLevel >= 1 {
		archive.RegisterCompressor(zip.Deflate, func(w io.Writer) (io.WriteCloser, error) {
			return flate.NewWriter(w, cfg.CompressionLevel)
		})
	}

	// Release any cached raw-volume handles when the stage ends, even on a
	// mid-stage fatal error (no-op off Windows).
	defer CloseRawCollectors()

	// Iterate over file paths and collect files into the zip archive
	for _, target := range paths {
		var entry, sha256, md5 string
		var size int64
		if cfg.RawAccess {
			entry, size, sha256, md5, err = CollectFileRaw(cfg, archive, target.Path)
		} else {
			entry, size, sha256, md5, err = CollectFile(cfg, archive, target.Path)
		}

		if err != nil {
			// Log a warning if file collection fails for a specific path
			WarnLogger.Printf("Stage 2: Failed to collect file: %v", err)
		}

		Jrnl.RecordFile(target, entry, size, sha256, md5, err)
	}

	// Write the collection log / manifest / checksum files into the archive
	// as the last entries, before it is sealed. Non-fatal: a metadata-write
	// failure still lets the archive close with the collected evidence intact.
	if err := Jrnl.Flush(cfg, archive); err != nil {
		WarnLogger.Printf("Stage 2: Failed to write collection log: %v", err)
	}

	// Close the zip archive
	err = archive.Close()
	if err != nil {
		return "", err
	}
	fh.Close()

	// Write the hash sidecar next to the archive. Non-fatal: a sidecar failure
	// degrades verifiability but the evidence archive itself is intact.
	sum := hex.EncodeToString(h.Sum(nil))
	if err := writeSidecar(cfg, sum); err != nil {
		WarnLogger.Printf("Stage 2: Failed to write hash sidecar: %v", err)
	}

	// Log the completion of Stage 2 along with the elapsed time
	InfoLogger.Printf("Stage 2 finished in %v", time.Since(start))
	return sum, nil
}

// writeSidecar writes a sha256sum-compatible companion file
// <OutputDir>/<OutputFile>.sha256 holding the digest of the final archive
// bytes against the archive's basename, verifiable with `sha256sum -c`.
func writeSidecar(cfg Configuration, sum string) error {
	line := fmt.Sprintf("%s  %s\n", sum, filepath.Base(cfg.OutputFile))
	path := filepath.Join(cfg.OutputDir, cfg.OutputFile+".sha256")
	return os.WriteFile(path, []byte(line), 0644)
}

// step3UploadSFTP uploads the zip archive to an SFTP server (if configured)
// and logs the progress.
func step3UploadSFTP(cfg Configuration, sum string) error {
	// Check if SFTP address is empty, if so, skip the upload
	if cfg.SkipUpload || cfg.SftpAddr == "" {
		InfoLogger.Println("Stage 3: Uploading archive to SFTP skipped.")
		return nil
	}

	// Log the start of Stage 3
	InfoLogger.Println("Stage 3: Uploading archive to SFTP ...")
	start := time.Now()

	// Upload the zip archive to the SFTP server
	err := UploadSFTP(cfg, sum)
	if err != nil {
		return err
	}

	// Log the completion of Stage 3 along with the elapsed time
	InfoLogger.Printf("Stage 3 (SFTP) finished in %v", time.Since(start))
	return nil
}

// step3UploadDagobert uploads the zip archive to an Dagobert server (if configured)
// and logs the progress.
func step3UploadDagobert(cfg Configuration, sum string) error {
	// Check if Dagobert address is empty, if so, skip the upload
	if cfg.SkipUpload || cfg.DagobertAddr == "" {
		InfoLogger.Println("Stage 3: Uploading archive to Dagobert skipped.")
		return nil
	}

	// Log the start of Stage 3
	InfoLogger.Println("Stage 3: Uploading archive to Dagobert ...")
	start := time.Now()

	// Upload the zip archive to the SFTP server
	err := UploadDagobert(cfg, sum)
	if err != nil {
		return err
	}

	// Log the completion of Stage 3 along with the elapsed time
	InfoLogger.Printf("Stage 3 (Dagobert) finished in %v", time.Since(start))
	return nil
}

// step4CleanUp removes temporary files created during the process
// and logs the progress.
func step4CleanUp(cfg Configuration) error {
	if cfg.SkipCleanup {
		InfoLogger.Println("Stage 4: Cleanup of temporary files skipped.")
		return nil
	}

	// Log the start of Stage 4
	InfoLogger.Println("Stage 4: Cleanup of temporary files ...")
	start := time.Now()

	// Remove the zip archive file
	err := os.Remove(filepath.Join(cfg.OutputDir, cfg.OutputFile))
	if err != nil {
		return err
	}

	// Remove the hash sidecar alongside the archive (best-effort).
	os.Remove(filepath.Join(cfg.OutputDir, cfg.OutputFile+".sha256"))

	// Log the completion of Stage 4 along with the elapsed time
	InfoLogger.Printf("Stage 4  finished in %v", time.Since(start))
	return nil
}

// ParseConfig parses command line parameters and returns a Configuration struct
func ParseConfig() (Configuration, error) {
	// Initialize Configuration struct
	cfg := Configuration{}

	now := time.Now().Format("20060102150405")
	hostname, err := os.Hostname()
	if err != nil {
		return cfg, err
	}

	// Set command line flags and default values
	flag.StringVar(&cfg.OutputDir, "od", ".", "Defines the directory that the zip archive will be created in.")
	flag.StringVar(&cfg.OutputFile, "of", hostname+"-"+now+".zip", "Defines the name of the zip archive created.")
	flag.StringVar(&cfg.ZipPass, "zip-pass", "", "Password for the output zip archive. If set, the archive is AES-256 encrypted (WinZip AES). If empty, the archive is not encrypted.")
	flag.IntVar(&cfg.CompressionLevel, "zip-level", -1, "Zip compression level: 0 = store (no compression), 1 (fastest) .. 9 (smallest). If unset, the standard Deflate default is used.")

	flag.StringVar(&cfg.SftpAddr, "sftp-addr", "", "SFTP server address")
	flag.StringVar(&cfg.SftpUser, "sftp-user", "", "SFTP username")
	flag.StringVar(&cfg.SftpPass, "sftp-pass", "", "SFTP password")
	flag.StringVar(&cfg.SftpDir, "sftp-dir", ".", "Defines the output directory on the SFTP server, as it may be a different location than the archive generated on disk.")
	flag.StringVar(&cfg.SftpFile, "sftp-file", hostname+"-"+now+".zip", "Defines the name of the zip archive created on the SFTP server.")

	flag.StringVar(&cfg.DagobertAddr, "dagobert-addr", "", "Dagobert URL")
	flag.StringVar(&cfg.DagobertCase, "dagobert-case", "", "Dagobert case id")
	flag.StringVar(&cfg.DagobertKey, "dagobert-key", "", "Dagobert API Key")
	flag.StringVar(&cfg.DagobertFile, "dagobert-file", hostname+"-"+now+".zip", "Defines the name of the zip archive created on the SFTP server.")

	defaultRoots := strings.Join(DefaulRootPaths(), ", ")
	usageRoots := fmt.Sprintf("Defines the search root path(s). If multiple root paths are given, they are traversed in order. (default %q)", defaultRoots)
	flag.Func("root", usageRoots, func(s string) error {
		cfg.CollectionRoots = append(cfg.CollectionRoots, s)
		return nil
	})
	flag.StringVar(&cfg.QuackTargets, "c", "", "Add custom collection paths (one entry per line). NOTE: Please see example.quack for the syntax.")

	flag.StringVar(&cfg.KapeTargets, "kt", "", "The KAPE target configuration to collect, without the extension. ")
	flag.StringVar(&cfg.KapeFiles, "kf", "KapeFiles", "Directory containing targets intended for use with KAPE.")

	flag.BoolVar(&cfg.RawAccess, "raw", true, "Use raw NTFS access. Only supported on Windows.")

	flag.BoolVar(&cfg.SkipTraversal, "skip-traversal", false, "Skip step #1: traversal / enumaration.")
	flag.BoolVar(&cfg.SkipCollection, "skip-collection", false, "Skip step #2: collection.")
	flag.BoolVar(&cfg.SkipUpload, "skip-upload", false, "Skip step #3: upload.")
	flag.BoolVar(&cfg.SkipCleanup, "skip-cleanup", false, "Skip step #4: cleanup.")

	flag.Parse()

	// Sanity checks & modifications
	if cfg.CompressionLevel < -1 || cfg.CompressionLevel > 9 {
		return cfg, fmt.Errorf("invalid -zip-level %d: want 0..9", cfg.CompressionLevel)
	}

	cfg.SkipTraversal = cfg.SkipTraversal || cfg.SkipCollection
	cfg.SkipCleanup = cfg.SkipCleanup || cfg.SkipUpload || (cfg.SftpAddr == "" && cfg.DagobertAddr == "")

	return cfg, nil
}
