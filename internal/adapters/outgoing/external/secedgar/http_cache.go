package secedgar

import (
	"context"
	"fmt"
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

// downloadToFile escribe la respuesta directo a disco (io.Copy, sin
// bufferear el cuerpo entero en memoria) -- necesario para companyfacts.zip
// (~1.5GB) en un contenedor con 512MB de RAM. Escribe a un .tmp y renombra
// al terminar, asi un intento cortado nunca deja un archivo cacheado a medio
// escribir que loadCachedOrDownload de por bueno.
func downloadToFile(ctx context.Context, url, target string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("building request for %s: %w", url, err)
	}
	req.Header.Set("User-Agent", UserAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading %s: unexpected status %d", url, resp.StatusCode)
	}

	tmp := target + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("creating temp file for %s: %w", url, err)
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("writing %s: %w", url, err)
	}
	f.Close()
	return os.Rename(tmp, target)
}

// fetchBody es para los JSON/XML chicos pedidos por-simbolo (submissions.json,
// primary_doc.xml) -- sin cache en disco, se piden al vuelo.
func fetchBody(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building request for %s: %w", url, err)
	}
	req.Header.Set("User-Agent", UserAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching %s: unexpected status %d", url, resp.StatusCode)
	}
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
