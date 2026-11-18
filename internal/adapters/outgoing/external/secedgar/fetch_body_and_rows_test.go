package secedgar

import (
	"archive/zip"
	"bytes"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFetchBodyRetriesServerErrors(t *testing.T) {
	fastSec(t)
	srv := newRecordedServer(func(call int, w http.ResponseWriter, _ *http.Request) {
		if call < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte(`{"filings":[]}`))
	})
	defer srv.Close()

	body, err := fetchBody(context.Background(), srv.URL)

	if err != nil || string(body) != `{"filings":[]}` || len(srv.ranges) != 3 {
		t.Fatalf("body=%q err=%v calls=%d, want success on the 3rd call", body, err, len(srv.ranges))
	}
}

func TestFetchBodyDoesNotRetryClientErrors(t *testing.T) {
	fastSec(t)
	srv := newRecordedServer(func(_ int, w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) })
	defer srv.Close()

	_, err := fetchBody(context.Background(), srv.URL)

	if err == nil || len(srv.ranges) != 1 {
		t.Fatalf("err=%v calls=%d, want one call and an error", err, len(srv.ranges))
	}
}

func zipWithEntry(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data.zip")
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, _ := w.Create(name)
	f.Write([]byte(content))
	w.Close()
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestForEachRowReadsEveryRow(t *testing.T) {
	path := zipWithEntry(t, "SUBMISSION.tsv", "A\tB\n1\t2\n3\t4\n")
	var rows [][]string

	err := forEachRow(path, "SUBMISSION.tsv", func(_, cols []string) { rows = append(rows, cols) })

	if err != nil || len(rows) != 2 {
		t.Fatalf("err=%v rows=%d, want 2 rows and no error", err, len(rows))
	}
}

func TestForEachRowReportsRowsTooLongForTheScanner(t *testing.T) {
	path := zipWithEntry(t, "SUBMISSION.tsv", "A\tB\n1\t2\n"+strings.Repeat("x", 2*1024*1024)+"\n5\t6\n")
	count := 0

	err := forEachRow(path, "SUBMISSION.tsv", func(_, _ []string) { count++ })

	if err == nil {
		t.Fatal("a truncated read must be reported, not silently ignored")
	}
	if count != 1 {
		t.Fatalf("rows before the failure = %d, want 1", count)
	}
}

func TestForEachRowReportsMissingEntryAndBadZip(t *testing.T) {
	path := zipWithEntry(t, "OTHER.tsv", "A\n1\n")
	if err := forEachRow(path, "SUBMISSION.tsv", func(_, _ []string) {}); err == nil {
		t.Fatal("a missing entry must be reported")
	}
	bad := filepath.Join(t.TempDir(), "bad.zip")
	os.WriteFile(bad, []byte("not a zip"), 0o644)
	if err := forEachRow(bad, "SUBMISSION.tsv", func(_, _ []string) {}); err == nil {
		t.Fatal("an unreadable zip must be reported")
	}
}
