package main

import (
	"bytes"
	"encoding/json"
	"io"
	"runtime"
	"strings"
	"testing"
	"time"

	zip "github.com/sprungknoedl/zip"
)

func TestTargetsSource(t *testing.T) {
	if got := targetsSource(Configuration{QuackTargets: "custom.quack"}); got != "custom.quack" {
		t.Errorf("quack: got %q, want custom.quack", got)
	}
	if got := targetsSource(Configuration{KapeTargets: "EventLogs"}); got != "kape:EventLogs" {
		t.Errorf("kape: got %q, want kape:EventLogs", got)
	}
	want := "default_" + runtime.GOOS + ".quack"
	if got := targetsSource(Configuration{}); got != want {
		t.Errorf("default: got %q, want %q", got, want)
	}
}

func TestJournalCounters(t *testing.T) {
	j := NewJournal(&bytes.Buffer{})

	j.RecordFile(CollectTarget{Path: "/a", Source: "match"}, "a", 10, "sha-a", "md5-a", nil)
	j.RecordFile(CollectTarget{Path: "/b", Source: "force"}, "b", 5, "sha-b", "md5-b", nil)
	j.RecordFile(CollectTarget{Path: "/c", Source: "match"}, "c", 0, "", "", io.ErrUnexpectedEOF)
	j.RecordDirSkipped("/locked", io.ErrClosedPipe)

	if j.collected != 2 {
		t.Errorf("collected = %d, want 2", j.collected)
	}
	if j.errors != 1 {
		t.Errorf("errors = %d, want 1", j.errors)
	}
	if j.dirsSkipped != 1 {
		t.Errorf("dirsSkipped = %d, want 1", j.dirsSkipped)
	}
	if j.bytesTotal != 15 {
		t.Errorf("bytesTotal = %d, want 15 (errored file contributes nothing)", j.bytesTotal)
	}
}

func TestJournalFlush(t *testing.T) {
	transcript := bytes.NewBufferString("INFO: Stage 1 ...\nINFO: Stage 2 ...\n")
	j := NewJournal(transcript)
	j.started = time.Now()
	j.SetScanned(42)
	j.RecordFile(CollectTarget{Path: "/etc/passwd", Source: "match"}, "etc/passwd", 12, "deadbeef", "cafef00d", nil)
	j.RecordDirSkipped("/proc", io.ErrClosedPipe)

	var buf bytes.Buffer
	archive := zip.NewWriter(&buf)
	if err := j.Flush(Configuration{}, archive); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if err := archive.Close(); err != nil {
		t.Fatalf("archive.Close: %v", err)
	}

	files := readZip(t, buf.Bytes())

	// collection.log is the verbatim transcript.
	if got := files["_donald/collection.log"]; got != transcript.String() {
		t.Errorf("collection.log = %q, want the transcript", got)
	}

	// sha256sums.txt has a coreutils line for the one collected file only.
	wantSums := "deadbeef  etc/passwd\n"
	if got := files["_donald/sha256sums.txt"]; got != wantSums {
		t.Errorf("sha256sums.txt = %q, want %q", got, wantSums)
	}

	// manifest.jsonl: one file record, one dir_skipped record, summary last.
	lines := strings.Split(strings.TrimRight(files["_donald/manifest.jsonl"], "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("manifest has %d lines, want 3", len(lines))
	}

	var summary summaryRecord
	if err := json.Unmarshal([]byte(lines[2]), &summary); err != nil {
		t.Fatalf("unmarshal summary: %v", err)
	}
	if summary.Type != "summary" {
		t.Errorf("last line type = %q, want summary", summary.Type)
	}
	if summary.Scanned != 42 {
		t.Errorf("summary.Scanned = %d, want 42", summary.Scanned)
	}
	if summary.Collected != 1 || summary.Errors != 0 {
		t.Errorf("summary collected/errors = %d/%d, want 1/0", summary.Collected, summary.Errors)
	}
	if summary.DirsSkipped != 1 {
		t.Errorf("summary.DirsSkipped = %d, want 1", summary.DirsSkipped)
	}
	if summary.BytesTotal != 12 {
		t.Errorf("summary.BytesTotal = %d, want 12", summary.BytesTotal)
	}
}

// readZip reads every entry of an unencrypted zip into a name->content map.
func readZip(t *testing.T, data []byte) map[string]string {
	t.Helper()
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}

	out := map[string]string{}
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open entry %s: %v", f.Name, err)
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read entry %s: %v", f.Name, err)
		}
		out[f.Name] = string(b)
	}
	return out
}
