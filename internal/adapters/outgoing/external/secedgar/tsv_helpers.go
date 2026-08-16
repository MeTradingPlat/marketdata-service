package secedgar

import (
	"archive/zip"
	"bufio"
	"strconv"
	"strings"
	"time"
)

// forEachRow busca entryName (ej. "SUBMISSION.tsv") dentro del ZIP y llama
// rowHandler por cada fila de datos -- entra una sola entrada a memoria a
// la vez via bufio.Scanner, nunca el ZIP ni el TSV completo.
func forEachRow(zipPath, entryName string, rowHandler func(headers, cols []string)) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return
	}
	defer r.Close()

	for _, entry := range r.File {
		if entry.Name != entryName {
			continue
		}
		f, err := entry.Open()
		if err != nil {
			return
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		if !scanner.Scan() {
			return
		}
		headers := strings.Split(scanner.Text(), "\t")
		for scanner.Scan() {
			rowHandler(headers, strings.Split(scanner.Text(), "\t"))
		}
		return
	}
}

func value(headers, cols []string, columnName string) string {
	for i, h := range headers {
		if h == columnName && i < len(cols) {
			return cols[i]
		}
	}
	return ""
}

func parseDate(v string) time.Time {
	if v == "" {
		return time.Time{}
	}
	t, err := time.Parse("2006-01-02", v)
	if err != nil {
		return time.Time{}
	}
	return t
}

func parseSharesInt(v string) int64 {
	if v == "" {
		return 0
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0
	}
	return int64(f)
}
