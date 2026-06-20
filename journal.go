package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"time"

	zip "github.com/sprungknoedl/zip"
)

// Version is the donald build version recorded in the manifest summary.
// Default "dev"; injectable via -ldflags "-X main.Version=..." later.
var Version = "dev"

// CollectTarget is a path queued for collection, tagged with how it was
// selected: "match" (matched a matcher) or "force" (a force-target).
type CollectTarget struct {
	Path   string
	Source string // "match" | "force"
}

// fileRecord is one collection attempt (manifest "file" record).
type fileRecord struct {
	Type   string `json:"type"`
	TS     string `json:"ts"`
	Path   string `json:"path"`
	Entry  string `json:"entry"`
	Source string `json:"source"`
	Status string `json:"status"`
	Size   *int64 `json:"size,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
	MD5    string `json:"md5,omitempty"`
	Error  string `json:"error,omitempty"`
}

// dirSkippedRecord is one traversal directory-skip (manifest "dir_skipped" record).
type dirSkippedRecord struct {
	Type  string `json:"type"`
	TS    string `json:"ts"`
	Path  string `json:"path"`
	Error string `json:"error"`
}

// summaryRecord is the final manifest line.
type summaryRecord struct {
	Type          string   `json:"type"`
	TS            string   `json:"ts"`
	Host          string   `json:"host"`
	DonaldVersion string   `json:"donald_version"`
	Targets       string   `json:"targets"`
	Roots         []string `json:"roots"`
	Raw           bool     `json:"raw"`
	Scanned       int      `json:"scanned"`
	Matched       int      `json:"matched"`
	Collected     int      `json:"collected"`
	Errors        int      `json:"errors"`
	DirsSkipped   int      `json:"dirs_skipped"`
	BytesTotal    int64    `json:"bytes_total"`
	Started       string   `json:"started"`
	Finished      string   `json:"finished"`
	Duration      string   `json:"duration"`
}

// Journal is a package-level recorder of the collection, mirroring the
// existing logger pattern. It accumulates manifest records as the pipeline
// runs and flushes them into the output archive just before it is sealed.
type Journal struct {
	transcript *bytes.Buffer // tee target for the loggers
	records    []any         // file / dir_skipped objects, marshaled to JSONL at flush

	scanned     int
	collected   int
	errors      int
	dirsSkipped int
	bytesTotal  int64

	started time.Time
}

// Jrnl is the package-level recorder, initialized in main().
var Jrnl *Journal

// NewJournal creates a Journal that tees the loggers through transcript.
func NewJournal(transcript *bytes.Buffer) *Journal {
	return &Journal{transcript: transcript}
}

func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// RecordFile records one collection attempt. A non-nil err produces an
// "error" record (no size/digests); otherwise a "collected" record.
func (j *Journal) RecordFile(t CollectTarget, entry string, size int64, sha256, md5 string, err error) {
	rec := fileRecord{
		Type:   "file",
		TS:     nowUTC(),
		Path:   t.Path,
		Entry:  entry,
		Source: t.Source,
	}

	if err != nil {
		rec.Status = "error"
		rec.Error = err.Error()
		j.errors++
	} else {
		n := size
		rec.Status = "collected"
		rec.Size = &n
		rec.SHA256 = sha256
		rec.MD5 = md5
		j.collected++
		j.bytesTotal += size
	}

	j.records = append(j.records, rec)
}

// RecordDirSkipped records a directory that could not be enumerated.
func (j *Journal) RecordDirSkipped(path string, err error) {
	j.records = append(j.records, dirSkippedRecord{
		Type:  "dir_skipped",
		TS:    nowUTC(),
		Path:  path,
		Error: err.Error(),
	})
	j.dirsSkipped++
}

// SetScanned records the total number of paths enumerated during traversal.
func (j *Journal) SetScanned(n int) {
	j.scanned = n
}

// targetsSource describes where the matchers came from, for the summary.
func targetsSource(cfg Configuration) string {
	if cfg.QuackTargets != "" {
		return cfg.QuackTargets
	}
	if cfg.KapeTargets != "" {
		return "kape:" + cfg.KapeTargets
	}
	return "default_" + runtime.GOOS + ".quack"
}

// Flush writes the metadata files into the archive, in order:
// _donald/collection.log, _donald/manifest.jsonl (records + summary), and the
// two coreutils checksum files _donald/sha256sums.txt and _donald/md5sums.txt.
// A returned error is logged as WARN by the caller; the archive still closes.
func (j *Journal) Flush(cfg Configuration, archive *zip.Writer) error {
	// The _donald/ files are synthesized at flush time (no on-disk source), so
	// they are all stamped with a single collection timestamp.
	now := time.Now()

	// _donald/collection.log — verbatim transcript of stages 1-2. Encrypted
	// alongside the evidence when -zip-pass is set (it carries host paths/log lines).
	w, err := archiveEntry(cfg, archive, "_donald/collection.log", now)
	if err != nil {
		return err
	}
	if _, err := w.Write(j.transcript.Bytes()); err != nil {
		return err
	}

	// _donald/manifest.jsonl — one JSON object per line, summary last.
	w, err = archiveEntry(cfg, archive, "_donald/manifest.jsonl", now)
	if err != nil {
		return err
	}
	for _, rec := range j.records {
		b, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		if _, err := w.Write(append(b, '\n')); err != nil {
			return err
		}
	}

	roots := cfg.CollectionRoots
	if len(roots) == 0 {
		roots = DefaulRootPaths()
	}
	host, _ := os.Hostname()
	finished := time.Now().UTC()

	summary := summaryRecord{
		Type:          "summary",
		TS:            finished.Format(time.RFC3339),
		Host:          host,
		DonaldVersion: Version,
		Targets:       targetsSource(cfg),
		Roots:         roots,
		Raw:           cfg.RawAccess,
		Scanned:       j.scanned,
		Matched:       j.collected + j.errors,
		Collected:     j.collected,
		Errors:        j.errors,
		DirsSkipped:   j.dirsSkipped,
		BytesTotal:    j.bytesTotal,
		Started:       j.started.UTC().Format(time.RFC3339),
		Finished:      finished.Format(time.RFC3339),
		Duration:      finished.Sub(j.started).String(),
	}
	b, err := json.Marshal(summary)
	if err != nil {
		return err
	}
	if _, err := w.Write(append(b, '\n')); err != nil {
		return err
	}

	// _donald/sha256sums.txt — coreutils format, one line per collected file.
	sha, err := archiveEntry(cfg, archive, "_donald/sha256sums.txt", now)
	if err != nil {
		return err
	}
	for _, rec := range j.records {
		if fr, ok := rec.(fileRecord); ok && fr.Status == "collected" {
			if _, err := fmt.Fprintf(sha, "%s  %s\n", fr.SHA256, fr.Entry); err != nil {
				return err
			}
		}
	}

	// _donald/md5sums.txt — coreutils format, one line per collected file.
	m, err := archiveEntry(cfg, archive, "_donald/md5sums.txt", now)
	if err != nil {
		return err
	}
	for _, rec := range j.records {
		if fr, ok := rec.(fileRecord); ok && fr.Status == "collected" {
			if _, err := fmt.Fprintf(m, "%s  %s\n", fr.MD5, fr.Entry); err != nil {
				return err
			}
		}
	}

	return nil
}
