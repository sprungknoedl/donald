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

var (
	WarnLogger *log.Logger
	InfoLogger *log.Logger
	ErrrLogger *log.Logger
)

type Configuration struct {
	OutputDir  string
	OutputFile string
	LogFile    string

	SftpAddr string
	SftpUser string
	SftpPass string
	SftpDir  string
	SftpFile string

	CollectionRoots   []string
	CustomListFile    string
	CustomListReplace bool

	RawAccess bool
}

func main() {
	InfoLogger = log.New(os.Stdout, "INFO: ", log.Ldate|log.Ltime)
	WarnLogger = log.New(os.Stdout, "WARN: ", log.Ldate|log.Ltime)
	ErrrLogger = log.New(os.Stdout, "ERRR: ", log.Ldate|log.Ltime)

	begin := time.Now()
	defer func() {
		InfoLogger.Printf("nut finished in %v.", time.Since(begin))
	}()

	cfg, err := ParseConfig()
	if err != nil {
		ErrrLogger.Fatalf("Coud not parse command line parameters: %q", err)
	}

	// ----------------------------------------------------
	// step 1: traverse filesystem
	// ----------------------------------------------------
	InfoLogger.Println("Stage 1: Traversing file tree ...")
	start := time.Now()
	paths, err := GetPaths(cfg)
	if err != nil {
		ErrrLogger.Fatalf("Stage 1: Unrecoverable error: %v", err)
	}
	InfoLogger.Printf("Stage 1 finished in %v", time.Since(start))

	// ----------------------------------------------------
	// step 2: collect files
	// ----------------------------------------------------
	InfoLogger.Println("Stage 2: Collecting files ...")
	start = time.Now()

	fh, err := os.Create(filepath.Join(cfg.OutputDir, cfg.OutputFile))
	if err != nil {
		ErrrLogger.Fatalf("Stage 2: Unrecoverable error: %v", err)
	}

	archive := zip.NewWriter(fh)
	for _, path := range paths {
		if cfg.RawAccess {
			err = CollectFileRaw(cfg, archive, path)
		} else {
			err = CollectFile(cfg, archive, path)
		}

		if err != nil {
			WarnLogger.Printf("Stage 2: Failed to collect file: %v", err)
		}
	}

	err = archive.Close()
	if err != nil {
		ErrrLogger.Fatalf("Stage 2: Unrecoverable error: %v", err)
	}
	InfoLogger.Printf("Stage 2 finished in %v", time.Since(start))

	// ----------------------------------------------------
	// step 3: upload files
	// ----------------------------------------------------
	if cfg.SftpAddr == "" {
		InfoLogger.Println("Stage 3: Uploading archive to SFTP skipped.")
		InfoLogger.Println("Stage 4: Cleanup skipped.")
		return
	}

	InfoLogger.Println("Stage 3: Uploading archive to SFTP ...")
	start = time.Now()
	err = Upload(cfg)
	if err != nil {
		ErrrLogger.Fatalf("Stage 3: Unrecoverable error: %v", err)
	}
	InfoLogger.Printf("Stage 3 finished in %v", time.Since(start))

	// ----------------------------------------------------
	// step 4: clean up
	// ----------------------------------------------------
	InfoLogger.Println("Stage 4: Cleanup of temporary files ...")
	start = time.Now()
	err = os.Remove(filepath.Join(cfg.OutputDir, cfg.OutputFile))
	if err != nil {
		ErrrLogger.Fatalf("Stage 4: Unrecoverable error: %v", err)
	}
	InfoLogger.Printf("Stage 4  finished in %v", time.Since(start))
}

func ParseConfig() (*Configuration, error) {
	// get default values
	hostname, err := os.Hostname()
	if err != nil {
		return nil, err
	}

	cfg := Configuration{}
	flag.StringVar(&cfg.OutputDir, "od", ".", "Defines the directory that the zip archive will be created in.")
	flag.StringVar(&cfg.OutputFile, "of", hostname+".zip", "Defines the name of the zip archive created.")

	flag.StringVar(&cfg.SftpAddr, "sftp-addr", "", "SFTP server address")
	flag.StringVar(&cfg.SftpUser, "sftp-user", "", "SFTP username")
	flag.StringVar(&cfg.SftpPass, "sftp-pass", "", "SFTP password")
	flag.StringVar(&cfg.SftpDir, "sftp-dir", ".", "Defines the output directory on the SFTP server, as it may be a different location than the archive generated on disk.")
	flag.StringVar(&cfg.SftpFile, "sftp-file", hostname+".zip", "Defines the name of the zip archive created on the SFTP server.")

	defaultRoots := strings.Join(DefaulRootPaths(), ", ")
	usageRoots := fmt.Sprintf("Defines the search root path(s). If multiple root paths are given, they are traversed in order. (default %q)", defaultRoots)
	flag.Func("root", usageRoots, func(s string) error {
		cfg.CollectionRoots = append(cfg.CollectionRoots, s)
		return nil
	})
	flag.StringVar(&cfg.CustomListFile, "c", "", "Add custom collection paths (one entry per line). NOTE: Please see CUSTOM_PATH_TEMPLATE.txt for an example.")
	flag.BoolVar(&cfg.CustomListReplace, "replace-paths", false, "Replace the default collection paths with those specified via '-c FILE'.")

	flag.BoolVar(&cfg.RawAccess, "raw", true, "Use raw NTFS access. Only supported on Windows.")
	flag.Parse()

	return &cfg, nil
}
