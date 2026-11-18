package secedgar

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/pkg/httpretry"
)

const partialMaxAge = 24 * time.Hour

var errInvalidDownload = errors.New("downloaded file is not a valid zip")

// downloadToFile escribe la respuesta directo a disco (io.Copy, sin
// bufferear el cuerpo entero en memoria) -- necesario para companyfacts.zip
// (~1.5GB) en un contenedor con 512MB de RAM. Escribe a un .tmp y renombra
// al terminar, asi un intento cortado nunca deja un archivo cacheado a medio
// escribir que loadCachedOrDownload de por bueno. Si el intento se corta, el
// .tmp se conserva y el siguiente intento lo reanuda con Range en vez de
// empezar de cero; un ZIP que no abre se descarta y se baja completo una vez.
func downloadToFile(ctx context.Context, url, target string) error {
	partial := target + ".tmp"
	removeStalePartials(filepath.Dir(target), partialMaxAge)

	restarts := 0
	for attempt := 0; ; attempt++ {
		err := appendFromURL(ctx, url, partial)
		if err == nil {
			err = validateDownload(partial, target)
		}
		if err == nil {
			return commitDownload(partial, target)
		}
		if errors.Is(err, errRestartDownload) || errors.Is(err, errInvalidDownload) {
			os.Remove(partial)
			if restarts++; restarts > 1 {
				return err
			}
			continue
		}
		if ctx.Err() != nil || !httpretry.IsTransient(err) || attempt >= len(secRetryDelays) {
			return err
		}
		if httpretry.Sleep(ctx, secRetryDelays[attempt]) != nil {
			return err
		}
	}
}

func validateDownload(partial, target string) error {
	if filepath.Ext(target) != ".zip" {
		return nil
	}
	r, err := zip.OpenReader(partial)
	if err != nil {
		return fmt.Errorf("%w: %v", errInvalidDownload, err)
	}
	return r.Close()
}

func commitDownload(partial, target string) error {
	if err := os.Rename(partial, target); err != nil {
		os.Remove(partial)
		return fmt.Errorf("renaming temp file for %s: %w", target, err)
	}
	return nil
}
