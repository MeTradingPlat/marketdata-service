package secedgar

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEnsureCachedZip_DownloadFailsOlderZipExists(t *testing.T) {
	tmpDir := t.TempDir()
	olderZip1 := filepath.Join(tmpDir, "companyfacts_2026-11-01.zip")
	olderZip2 := filepath.Join(tmpDir, "companyfacts_2026-11-05.zip")
	tmpFile := filepath.Join(tmpDir, "companyfacts_2026-11-10.zip.tmp")

	if err := os.WriteFile(olderZip1, []byte("zip1"), 0o644); err != nil {
		t.Fatalf("writing olderZip1: %v", err)
	}
	if err := os.WriteFile(olderZip2, []byte("zip2"), 0o644); err != nil {
		t.Fatalf("writing olderZip2: %v", err)
	}
	if err := os.WriteFile(tmpFile, []byte("partial"), 0o644); err != nil {
		t.Fatalf("writing tmpFile: %v", err)
	}

	origDownload := downloadCompanyFacts
	defer func() { downloadCompanyFacts = origDownload }()
	downloadCompanyFacts = func(ctx context.Context, url, target string) error {
		return errors.New("simulated download network failure")
	}

	client := NewEdgarClient(nil, tmpDir)
	got, err := client.ensureCachedZip(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != olderZip2 {
		t.Fatalf("got %q, want %q", got, olderZip2)
	}
}

func TestEnsureCachedZip_DownloadFailsNoZipExists(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "companyfacts_2026-11-10.zip.tmp")
	if err := os.WriteFile(tmpFile, []byte("partial"), 0o644); err != nil {
		t.Fatalf("writing tmpFile: %v", err)
	}

	origDownload := downloadCompanyFacts
	defer func() { downloadCompanyFacts = origDownload }()
	downloadCompanyFacts = func(ctx context.Context, url, target string) error {
		return errors.New("simulated download network failure")
	}

	client := NewEdgarClient(nil, tmpDir)
	got, err := client.ensureCachedZip(context.Background())
	if err == nil {
		t.Fatalf("expected error, got path: %q", got)
	}
}

func TestEnsureCachedZip_TodaysZipExists(t *testing.T) {
	tmpDir := t.TempDir()
	today := filepath.Join(tmpDir, "companyfacts_"+time.Now().Format("2006-01-02")+".zip")
	if err := os.WriteFile(today, []byte("today_zip"), 0o644); err != nil {
		t.Fatalf("writing today zip: %v", err)
	}

	origDownload := downloadCompanyFacts
	defer func() { downloadCompanyFacts = origDownload }()
	downloadCompanyFacts = func(ctx context.Context, url, target string) error {
		t.Fatal("downloadCompanyFacts should not be called when today's zip exists")
		return nil
	}

	client := NewEdgarClient(nil, tmpDir)
	got, err := client.ensureCachedZip(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != today {
		t.Fatalf("got %q, want %q", got, today)
	}
}
