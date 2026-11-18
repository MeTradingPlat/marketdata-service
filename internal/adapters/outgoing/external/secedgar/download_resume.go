package secedgar

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/MeTradingPlat/marketdata-service/internal/pkg/httpretry"
)

var errRestartDownload = errors.New("partial download cannot be resumed")

func appendFromURL(ctx context.Context, url, partial string) error {
	offset := partialSize(partial)
	resp, err := secGet(ctx, url, rangeHeader(offset))
	if err != nil {
		if httpretry.StatusCode(err) == http.StatusRequestedRangeNotSatisfiable {
			return errRestartDownload
		}
		return err
	}
	defer resp.Body.Close()

	flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	if resp.StatusCode == http.StatusPartialContent {
		if contentRangeStart(resp) != offset {
			return errRestartDownload
		}
		flags = os.O_CREATE | os.O_WRONLY | os.O_APPEND
	}
	f, err := os.OpenFile(partial, flags, 0o644)
	if err != nil {
		return fmt.Errorf("opening %s: %w", partial, err)
	}
	_, copyErr := io.Copy(f, resp.Body)
	closeErr := f.Close()
	if copyErr != nil {
		return fmt.Errorf("writing %s: %w", partial, copyErr)
	}
	return closeErr
}

func partialSize(partial string) int64 {
	info, err := os.Stat(partial)
	if err != nil {
		return 0
	}
	return info.Size()
}

func rangeHeader(offset int64) http.Header {
	if offset <= 0 {
		return nil
	}
	return http.Header{"Range": {fmt.Sprintf("bytes=%d-", offset)}}
}

func contentRangeStart(resp *http.Response) int64 {
	var start int64 = -1
	fmt.Sscanf(resp.Header.Get("Content-Range"), "bytes %d-", &start)
	return start
}
