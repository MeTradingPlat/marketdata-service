package tastytrade

import (
	"context"
	"runtime"
	"sync"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/rs/zerolog/log"
)

const closeAllConnectionsLimit = 5 * time.Second

// StopAllLive desuscribe TODAS las suscripciones M1 en vivo del pool -- se
// usa antes del barrido pesado de D1/H1 sobre el universo completo (en una
// hora sin movimiento de mercado, ver runOnce), para que ese barrido no
// compita por conexiones/canales con miles de suscripciones en vivo. Es
// seguro (no repite la colision que causaba el congelamiento) porque el
// hueco entre este remove y el proximo SubscribeLive va a ser de horas,
// no milisegundos -- dxFeed ya proceso el remove de sobra para cuando
// llegue el add. Cada simbolo retoma solo desde su propio watermark al
// resuscribirse, sin perder nada (el mercado estuvo cerrado mientras tanto).
func (p *CandlePool) StopAllLive(ctx context.Context) {
	p.allocator.mu.Lock()
	conns := append([]*pooledConnection(nil), p.allocator.connections...)
	p.allocator.mu.Unlock()

	var stopped []string
	for _, pc := range conns {
		pc.mu.Lock()
		channels := append([]*pooledChannel(nil), pc.channels...)
		pc.mu.Unlock()
		for _, ch := range channels {
			for _, symbol := range ch.liveSymbols() {
				_ = ch.channel.unsubscribe(symbol, domain.M1)
				ch.release(candleKey(symbol, domain.M1))
				stopped = append(stopped, symbol)
			}
		}
	}

	p.dispatchMu.Lock()
	for _, symbol := range stopped {
		delete(p.dispatch, candleKey(symbol, domain.M1))
	}
	p.dispatchMu.Unlock()

	// Antes de descartar cualquier estado -- ver el comentario de
	// flushFormingCandles.
	p.flushFormingCandles()

	p.liveMu.Lock()
	p.liveSubs = make(map[string]func(domain.Candle))
	p.liveMu.Unlock()

	p.currentMu.Lock()
	p.current = make(map[string]domain.Candle)
	// lastClosed tambien se limpia aca -- el hueco hasta el proximo
	// SubscribeLive es de horas (mercado cerrado de por medio), asi que
	// cualquier tick tardio que llegue del otro lado ya pertenece a un dia
	// distinto y no debe fusionarse contra la vela vieja.
	p.lastClosed = make(map[string]domain.Candle)
	p.currentMu.Unlock()

	log.Info().Int("symbols", len(stopped)).Msg("stopped all live M1 subscriptions for the maintenance window")
}

// CloseAllConnections cierra en paralelo, con un close frame explicito, las
// conexiones fisicas del pool (cerrar la conexion ya descarta sus
// suscripciones, no hace falta desuscribir simbolo por simbolo) -- se usa en las
// fronteras D1->H1->M1 del barrido nocturno para que cada fase arranque
// con cero sesiones abiertas ante TastyTrade, en vez de arrastrar las
// conexiones que uso la fase anterior justo cuando la siguiente intenta
// abrir varias de golpe (confirmado en vivo: asi se supero el limite de
// sesiones de TastyTrade durante un rollout de M1 rapido). DxLinkConn.Close
// marca cada conexion como cierre intencional para que no dispare su
// propia reconexion automatica.
func (p *CandlePool) CloseAllConnections() {
	start := time.Now()
	conns := p.allocator.drainAll()
	closeConnections(conns, closeAllConnectionsLimit)
	closedIn := time.Since(start)

	p.dispatchMu.Lock()
	p.dispatch = make(map[string]dispatchEntry)
	p.dispatchMu.Unlock()

	// Antes de descartar cualquier estado -- ver el comentario de
	// flushFormingCandles. CloseAllConnections corre en CADA reinicio del
	// proceso (ResetLiveConnections en el shutdown, ver cmd/api/main.go) y
	// en cada frontera D1->H1->M1 del barrido -- confirmado en vivo el
	// 2026-09-01: sin esto, la vela en formacion de cada simbolo se perdia
	// en silencio en cada uno de esos reinicios (6+ solo hoy), no solo en
	// StopAllLive una vez por noche.
	p.flushFormingCandles()

	p.liveMu.Lock()
	p.liveSubs = make(map[string]func(domain.Candle))
	p.liveMu.Unlock()

	p.currentMu.Lock()
	p.current = make(map[string]domain.Candle)
	p.currentMu.Unlock()

	log.Info().Int("connections", len(conns)).Int("goroutines", runtime.NumGoroutine()).
		Dur("connectionsClosedIn", closedIn).Dur("total", time.Since(start)).
		Msg("closed all dxlink connections at phase boundary")
}

func closeConnections(conns []*pooledConnection, limit time.Duration) {
	var wg sync.WaitGroup
	for _, pc := range conns {
		wg.Add(1)
		go func(pc *pooledConnection) {
			defer wg.Done()
			pc.conn.Close()
		}(pc)
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(limit):
		log.Warn().Int("connections", len(conns)).Dur("limit", limit).Msg("closing dxlink connections did not finish in time")
	}
}
