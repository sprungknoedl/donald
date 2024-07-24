package main

import (
	"archive/zip"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
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
	LogFile    string // Log file path

	SftpAddr string // SFTP server address
	SftpUser string // SFTP server username
	SftpPass string // SFTP server password
	SftpDir  string // Target directory on the SFTP server
	SftpFile string // Target filename on the SFTP server

	DagobertAddr string // Dagobert server URL
	DagobertCase string // Dagobert case id
	DagobertKey  string // Dagobert API key
	DagobertFile string // Target filename on Dagobert

	CollectionRoots   []string // Search root paths
	CustomListFile    string   // Custom collection paths file
	CustomListReplace bool     // Replace default collection paths with custom ones

	RawAccess bool // Use raw NTFS access (Windows only)

	SkipTraversal  bool
	SkipCollection bool
	SkipUpload     bool
	SkipCleanup    bool
}

func main() {
	// Initialize loggers
	InfoLogger = log.New(os.Stdout, "INFO: ", log.Ldate|log.Ltime)
	WarnLogger = log.New(os.Stdout, "WARN: ", log.Ldate|log.Ltime)
	ErrLogger = log.New(os.Stdout, "ERRR: ", log.Ldate|log.Ltime)

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

	err = step2CollectFiles(cfg, paths)
	if err != nil {
		ErrLogger.Fatalf("Stage 2: Unrecoverable error: %v", err)
	}

	err = step3UploadSFTP(cfg)
	if err != nil {
		ErrLogger.Fatalf("Stage 3 (SFTP): Unrecoverable error: %v", err)
	}

	err = step3UploadDagobert(cfg)
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
func step1TraverseFS(cfg Configuration) ([]string, error) {
	if cfg.SkipTraversal {
		InfoLogger.Println("Stage 1: Traversing file tree skipped.")
		return []string{}, nil
	}

	// Log the start of Stage 1
	InfoLogger.Println("Stage 1: Traversing file tree ...")
	start := time.Now()

	paths, err := GetPaths(cfg)
	if err != nil {
		return nil, err
	}

	// Log the completion of Stage 1 along with the elapsed time
	InfoLogger.Printf("Stage 1 finished in %v", time.Since(start))
	return paths, nil
}

// step2CollectFiles collects files based on the provided file paths, creates a zip archive,
// and logs the progress.
func step2CollectFiles(cfg Configuration, paths []string) error {
	if cfg.SkipCollection {
		InfoLogger.Println("Stage 2: Collecting files skipped.")
		return nil
	}

	// Log the start of Stage 2
	InfoLogger.Println("Stage 2: Collecting files ...")
	start := time.Now()

	// Create the output file for the zip archive
	fh, err := os.Create(filepath.Join(cfg.OutputDir, cfg.OutputFile))
	if err != nil {
		return err
	}

	// Create a zip archive
	archive := zip.NewWriter(fh)

	// Iterate over file paths and collect files into the zip archive
	for _, path := range paths {
		if cfg.RawAccess {
			err = CollectFileRaw(cfg, archive, path)
		} else {
			err = CollectFile(cfg, archive, path)
		}

		if err != nil {
			// Log a warning if file collection fails for a specific path
			WarnLogger.Printf("Stage 2: Failed to collect file: %v", err)
		}
	}

	// Close the zip archive
	err = archive.Close()
	if err != nil {
		return err
	}

	// Log the completion of Stage 2 along with the elapsed time
	InfoLogger.Printf("Stage 2 finished in %v", time.Since(start))
	return nil
}

// step3UploadSFTP uploads the zip archive to an SFTP server (if configured)
// and logs the progress.
func step3UploadSFTP(cfg Configuration) error {
	// Check if SFTP address is empty, if so, skip the upload
	if cfg.SkipUpload || cfg.SftpAddr == "" {
		InfoLogger.Println("Stage 3: Uploading archive to SFTP skipped.")
		return nil
	}

	// Log the start of Stage 3
	InfoLogger.Println("Stage 3: Uploading archive to SFTP ...")
	start := time.Now()

	// Upload the zip archive to the SFTP server
	err := UploadSFTP(cfg)
	if err != nil {
		return err
	}

	// Log the completion of Stage 3 along with the elapsed time
	InfoLogger.Printf("Stage 3 (SFTP) finished in %v", time.Since(start))
	return nil
}

// step3UploadDagobert uploads the zip archive to an Dagobert server (if configured)
// and logs the progress.
func step3UploadDagobert(cfg Configuration) error {
	// Check if Dagobert address is empty, if so, skip the upload
	if cfg.SkipUpload || cfg.DagobertAddr == "" {
		InfoLogger.Println("Stage 3: Uploading archive to Dagobert skipped.")
		return nil
	}

	// Log the start of Stage 3
	InfoLogger.Println("Stage 3: Uploading archive to Dagobert ...")
	start := time.Now()

	// Upload the zip archive to the SFTP server
	err := UploadDagobert(cfg)
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
	flag.StringVar(&cfg.CustomListFile, "c", "", "Add custom collection paths (one entry per line). NOTE: Please see CUSTOM_PATH_TEMPLATE.txt for an example.")
	flag.BoolVar(&cfg.CustomListReplace, "replace-paths", false, "Replace the default collection paths with those specified via '-c FILE'.")

	flag.BoolVar(&cfg.RawAccess, "raw", true, "Use raw NTFS access. Only supported on Windows.")

	flag.BoolVar(&cfg.SkipTraversal, "skip-traversal", true, "Skip step #1: traversal / enumaration.")
	flag.BoolVar(&cfg.SkipCollection, "skip-collection", true, "Skip step #2: collection.")
	flag.BoolVar(&cfg.SkipUpload, "skip-upload", true, "Skip step #3: upload.")
	flag.BoolVar(&cfg.SkipCleanup, "skip-cleanup", true, "Skip step #4: cleanup.")

	flag.Parse()

	return cfg, nil
}
