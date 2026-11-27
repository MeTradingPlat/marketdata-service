package spool

import (
	"errors"
	"os"
	"path/filepath"
)

var errSpoolFull = errors.New("candle spool capacity exceeded")

func checkDirCap(dir string, maxBytes, addBytes int64) error {
	if maxBytes <= 0 {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var total int64
	for _, e := range entries {
		if info, err := e.Info(); err == nil && !e.IsDir() {
			total += info.Size()
		}
	}
	if total+addBytes > maxBytes {
		return errSpoolFull
	}
	return nil
}

func writeAtomicFile(dir, name string, data []byte) error {
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, filepath.Join(dir, name))
}
