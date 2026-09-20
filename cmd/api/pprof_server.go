package main

import (
	"net/http"
	"net/http/pprof"

	"github.com/rs/zerolog/log"
)

const pprofLoopbackAddress = "127.0.0.1:6060"

func StartPprofServer() {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	go func() {
		if err := http.ListenAndServe(pprofLoopbackAddress, mux); err != nil {
			log.Warn().Err(err).Msg("pprof server stopped")
		}
	}()
}
