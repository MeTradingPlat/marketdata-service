package secedgar

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// httpClient no fija Timeout -- cada caller controla su propio limite via
// context.WithTimeout (15s para JSON/XML chicos, 10-20min para los ZIPs
// bulk), no hay un valor unico que sirva para ambos casos.
var httpClient = &http.Client{}

// fetchBody es para los JSON/XML chicos pedidos por-simbolo (submissions.json,
// primary_doc.xml) -- sin cache en disco, se piden al vuelo. Pasa por secGet:
// limite de ritmo global hacia la SEC (su politica de uso justo permite 10
// req/s) y reintentos de errores transitorios respetando Retry-After.
func fetchBody(ctx context.Context, url string) ([]byte, error) {
	resp, err := secGet(ctx, url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func cleanupOldCacheFiles(cacheDir, prefix, keep string) {
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		full := filepath.Join(cacheDir, e.Name())
		if strings.HasPrefix(e.Name(), prefix) && full != keep {
			os.Remove(full)
		}
	}
}
