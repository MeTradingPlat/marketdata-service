package secedgar

import (
	"archive/zip"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func fastSec(t *testing.T) {
	t.Helper()
	origLimiter, origDelays := secLimiter, secRetryDelays
	secLimiter = rate.NewLimiter(rate.Inf, 1)
	secRetryDelays = []time.Duration{time.Millisecond, time.Millisecond}
	t.Cleanup(func() { secLimiter, secRetryDelays = origLimiter, origDelays })
}

func validZip(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, _ := w.Create("SUBMISSION.tsv")
	f.Write(bytes.Repeat([]byte("ACCESSION_NUMBER\tISSUERTRADINGSYMBOL\n"), 2000))
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

type recordedServer struct {
	*httptest.Server
	mu     sync.Mutex
	ranges []string
}

func newRecordedServer(handler func(call int, w http.ResponseWriter, r *http.Request)) *recordedServer {
	rs := &recordedServer{}
	rs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rs.mu.Lock()
		rs.ranges = append(rs.ranges, r.Header.Get("Range"))
		call := len(rs.ranges)
		rs.mu.Unlock()
		handler(call, w, r)
	}))
	return rs
}

func cutAfter(w http.ResponseWriter, full []byte, n int) {
	conn, buf, _ := w.(http.Hijacker).Hijack()
	buf.WriteString("HTTP/1.1 200 OK\r\nContent-Length: " + strconv.Itoa(len(full)) + "\r\n\r\n")
	buf.Write(full[:n])
	buf.Flush()
	conn.Close()
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return data
}

func TestDownloadToFileWritesValidZipAndLeavesNoTemp(t *testing.T) {
	fastSec(t)
	full := validZip(t)
	srv := newRecordedServer(func(_ int, w http.ResponseWriter, _ *http.Request) { w.Write(full) })
	defer srv.Close()
	target := filepath.Join(t.TempDir(), "facts.zip")

	if err := downloadToFile(context.Background(), srv.URL, target); err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(readFile(t, target), full) {
		t.Fatal("downloaded content differs")
	}
	if _, err := os.Stat(target + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("temp file must be gone after a successful download")
	}
}

func TestDownloadToFileResumesInterruptedDownload(t *testing.T) {
	fastSec(t)
	full := validZip(t)
	cut := len(full) / 2
	srv := newRecordedServer(func(call int, w http.ResponseWriter, r *http.Request) {
		if call == 1 {
			cutAfter(w, full, cut)
			return
		}
		http.ServeContent(w, r, "facts.zip", time.Time{}, bytes.NewReader(full))
	})
	defer srv.Close()
	target := filepath.Join(t.TempDir(), "facts.zip")

	if err := downloadToFile(context.Background(), srv.URL, target); err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(readFile(t, target), full) {
		t.Fatal("resumed download differs from the original")
	}
	if len(srv.ranges) < 2 || srv.ranges[1] == "" {
		t.Fatalf("second request ranges=%v, want a Range header to resume", srv.ranges)
	}
}

func TestDownloadToFileRestartsWhenServerIgnoresRange(t *testing.T) {
	fastSec(t)
	full := validZip(t)
	srv := newRecordedServer(func(_ int, w http.ResponseWriter, _ *http.Request) { w.Write(full) })
	defer srv.Close()
	target := filepath.Join(t.TempDir(), "facts.zip")
	os.WriteFile(target+".tmp", []byte("junk from an older attempt"), 0o644)

	if err := downloadToFile(context.Background(), srv.URL, target); err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(readFile(t, target), full) {
		t.Fatal("a server that ignores Range must produce the full file, not junk plus data")
	}
}

func TestDownloadToFileRestartsOnRangeNotSatisfiable(t *testing.T) {
	fastSec(t)
	full := validZip(t)
	srv := newRecordedServer(func(_ int, w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "" {
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		w.Write(full)
	})
	defer srv.Close()
	target := filepath.Join(t.TempDir(), "facts.zip")
	os.WriteFile(target+".tmp", []byte("stale partial"), 0o644)

	if err := downloadToFile(context.Background(), srv.URL, target); err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(readFile(t, target), full) {
		t.Fatal("expected a clean full download after 416")
	}
}

func TestDownloadToFileRestartsWhenContentRangeDoesNotMatch(t *testing.T) {
	fastSec(t)
	full := validZip(t)
	srv := newRecordedServer(func(_ int, w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "" {
			w.Header().Set("Content-Range", "bytes 0-99/1000")
			w.WriteHeader(http.StatusPartialContent)
			w.Write(full[:100])
			return
		}
		w.Write(full)
	})
	defer srv.Close()
	target := filepath.Join(t.TempDir(), "facts.zip")
	os.WriteFile(target+".tmp", []byte("12345"), 0o644)

	if err := downloadToFile(context.Background(), srv.URL, target); err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(readFile(t, target), full) {
		t.Fatal("a mismatched Content-Range must not be appended")
	}
}

func TestDownloadToFileRejectsCorruptZipAndCleansUp(t *testing.T) {
	fastSec(t)
	srv := newRecordedServer(func(_ int, w http.ResponseWriter, _ *http.Request) { w.Write([]byte("this is not a zip")) })
	defer srv.Close()
	target := filepath.Join(t.TempDir(), "facts.zip")

	err := downloadToFile(context.Background(), srv.URL, target)

	if err == nil {
		t.Fatal("expected an error for a corrupt zip")
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Fatal("a corrupt zip must never be left as the cached file")
	}
	if _, statErr := os.Stat(target + ".tmp"); !os.IsNotExist(statErr) {
		t.Fatal("the corrupt partial must be removed")
	}
}

func TestDownloadToFileDoesNotValidateNonZipTargets(t *testing.T) {
	fastSec(t)
	srv := newRecordedServer(func(_ int, w http.ResponseWriter, _ *http.Request) { w.Write([]byte(`{"ok":true}`)) })
	defer srv.Close()
	target := filepath.Join(t.TempDir(), "company_tickers.json")

	if err := downloadToFile(context.Background(), srv.URL, target); err != nil {
		t.Fatal(err)
	}

	if string(readFile(t, target)) != `{"ok":true}` {
		t.Fatal("json content differs")
	}
}

func TestDownloadToFileSendsIdentifyingUserAgent(t *testing.T) {
	fastSec(t)
	var agent string
	srv := newRecordedServer(func(_ int, w http.ResponseWriter, r *http.Request) {
		agent = r.Header.Get("User-Agent")
		w.Write([]byte("{}"))
	})
	defer srv.Close()

	downloadToFile(context.Background(), srv.URL, filepath.Join(t.TempDir(), "x.json"))

	if agent != UserAgent {
		t.Fatalf("User-Agent = %q, want %q", agent, UserAgent)
	}
}

func TestRemoveStalePartialsOnlyRemovesOldTempFiles(t *testing.T) {
	dir := t.TempDir()
	oldTmp, freshTmp, keep := filepath.Join(dir, "old.zip.tmp"), filepath.Join(dir, "fresh.zip.tmp"), filepath.Join(dir, "good.zip")
	for _, p := range []string{oldTmp, freshTmp, keep} {
		os.WriteFile(p, []byte("x"), 0o644)
	}
	past := time.Now().Add(-48 * time.Hour)
	os.Chtimes(oldTmp, past, past)
	os.Chtimes(keep, past, past)

	removeStalePartials(dir, 24*time.Hour)

	for path, wantExists := range map[string]bool{oldTmp: false, freshTmp: true, keep: true} {
		if _, err := os.Stat(path); (err == nil) != wantExists {
			t.Fatalf("%s exists=%v, want %v", filepath.Base(path), err == nil, wantExists)
		}
	}
}
