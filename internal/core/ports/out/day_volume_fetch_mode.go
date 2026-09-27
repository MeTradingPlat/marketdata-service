package out

import "context"

type fastFetchKey struct{}

// WithFastFetch pide una sola pasada rapida de FetchDayVolumes: las fronteras
// de sesion (9:30 y 16:00 ET) necesitan que la captura termine en un minuto o
// dos, aunque queden simbolos sin resolver -- una captura lenta ya incluye
// volumen de la sesion siguiente y se descarta.
func WithFastFetch(ctx context.Context) context.Context {
	return context.WithValue(ctx, fastFetchKey{}, true)
}

func IsFastFetch(ctx context.Context) bool {
	fast, _ := ctx.Value(fastFetchKey{}).(bool)
	return fast
}
