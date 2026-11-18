package tastytrade

import "context"

const staleRefreshKey = "refresh-stale"

func (o *OAuth) RefreshIfStale(ctx context.Context, stale string) error {
	if o.AccessToken() != stale {
		return nil
	}
	_, err, _ := o.resetGroup.Do(staleRefreshKey, func() (interface{}, error) {
		if o.AccessToken() != stale {
			return nil, nil
		}
		_, err := o.RefreshAccessToken(ctx)
		return nil, err
	})
	return err
}
