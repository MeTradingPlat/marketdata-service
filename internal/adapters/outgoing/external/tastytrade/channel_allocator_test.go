package tastytrade

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func newTestConnFactory(t *testing.T, wsURL string) (func(ctx context.Context) (*DxLinkConn, error), *int32) {
	t.Helper()
	var calls int32
	factory := func(ctx context.Context) (*DxLinkConn, error) {
		atomic.AddInt32(&calls, 1)
		return newConnectedDxLinkConn(t, wsURL), nil
	}
	return factory, &calls
}

func TestChannelAllocator_ReusesChannelWithRoom(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	factory, calls := newTestConnFactory(t, wsURL)
	a := newChannelAllocator(factory, nil, nil, 40)

	first, err := a.allocate(context.Background())
	if err != nil {
		t.Fatalf("first allocate: %v", err)
	}
	second, err := a.allocate(context.Background())
	if err != nil {
		t.Fatalf("second allocate: %v", err)
	}

	if first != second {
		t.Fatal("expected the second allocate to reuse the same channel while it still has room")
	}
	if got := atomic.LoadInt32(calls); got != 1 {
		t.Fatalf("expected exactly 1 connection to be opened, got %d", got)
	}
}

func TestChannelAllocator_OpensNewChannelOnSameConnectionWhenFull(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	factory, calls := newTestConnFactory(t, wsURL)
	a := newChannelAllocator(factory, nil, nil, 40)

	first, err := a.allocate(context.Background())
	if err != nil {
		t.Fatalf("first allocate: %v", err)
	}
	// Llena el canal a capacidad (channelCapacity, ver pooled_channel.go) --
	// el proximo allocate ya no debe poder reusarlo. Cada simbolo necesita
	// una key distinta: occupy() es un set por key, la misma key repetida
	// no suma ocupacion.
	for i := 0; i < channelCapacity; i++ {
		first.occupy(candleKey(fmt.Sprintf("SYM%d", i), domain.M1))
	}

	second, err := a.allocate(context.Background())
	if err != nil {
		t.Fatalf("second allocate: %v", err)
	}
	if first == second {
		t.Fatal("expected a new channel once the first one is at capacity")
	}
	if got := atomic.LoadInt32(calls); got != 1 {
		t.Fatalf("expected the new channel to reuse the SAME connection (still under maxChannelsPerConnection), got %d connections opened", got)
	}
}

func TestChannelAllocator_EnforcesConnectionCeiling(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	factory, calls := newTestConnFactory(t, wsURL)
	a := newChannelAllocator(factory, nil, nil, 1)

	// Agota los maxChannelsPerConnection canales de la unica conexion
	// permitida, cada uno lleno a capacidad, para forzar el camino de
	// "necesita una conexion nueva" -- y confirmar que el techo lo bloquea.
	for i := 0; i < maxChannelsPerConnection; i++ {
		ch, err := a.allocate(context.Background())
		if err != nil {
			t.Fatalf("allocate channel %d within the same connection: %v", i, err)
		}
		for j := 0; j < channelCapacity; j++ {
			ch.occupy(candleKey(fmt.Sprintf("SYM%d-%d", i, j), domain.M1))
		}
	}
	if got := atomic.LoadInt32(calls); got != 1 {
		t.Fatalf("expected all %d channels on a single connection, got %d connections opened", maxChannelsPerConnection, got)
	}

	if _, err := a.allocate(context.Background()); err == nil {
		t.Fatal("expected allocate to fail once at the connection ceiling with no room left")
	}
	if got := atomic.LoadInt32(calls); got != 1 {
		t.Fatalf("expected the ceiling check to reject BEFORE opening a new connection, got %d connections opened", got)
	}
}

func TestChannelAllocator_DrainAllResetsPool(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	factory, calls := newTestConnFactory(t, wsURL)
	a := newChannelAllocator(factory, nil, nil, 40)

	if _, err := a.allocate(context.Background()); err != nil {
		t.Fatalf("allocate: %v", err)
	}
	if n := a.connectionCount(); n != 1 {
		t.Fatalf("expected 1 connection before drain, got %d", n)
	}

	drained := a.drainAll()
	if len(drained) != 1 {
		t.Fatalf("expected drainAll to return the 1 connection, got %d", len(drained))
	}
	if n := a.connectionCount(); n != 0 {
		t.Fatalf("expected 0 connections after drain, got %d", n)
	}

	if _, err := a.allocate(context.Background()); err != nil {
		t.Fatalf("allocate after drain: %v", err)
	}
	if got := atomic.LoadInt32(calls); got != 2 {
		t.Fatalf("expected a brand new connection after drain (2 total), got %d", got)
	}
}

// staggerBeforeNewConnection espacia la apertura de conexiones nuevas para
// no disparar el rechazo de autenticacion de TastyTrade por rafagas rapidas
// (ver el comentario de connectionStaggerDelay) -- confirma que el segundo
// allocate que necesita una conexion nueva de verdad espera.
func TestChannelAllocator_StaggersNewConnections(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	factory, _ := newTestConnFactory(t, wsURL)
	// maxConnections alto y sin ocupar los canales -- forzamos una conexion
	// nueva por llamada manipulando el allocator manualmente entre medio no
	// hace falta: alcanza con medir el tiempo entre dos llamadas que SI
	// necesitan abrir conexion, usando maxChannelsPerConnection=1 efectivo
	// vaciando el canal previo a capacidad como en el test de arriba.
	a := newChannelAllocator(factory, nil, nil, 40)

	if _, err := a.allocate(context.Background()); err != nil {
		t.Fatalf("first allocate: %v", err)
	}
	// Fuerza que la conexion existente ya no tenga lugar, sin llenar canales
	// uno por uno: drainAll() simula que la anterior ya no sirve y el
	// siguiente allocate debe abrir una conexion nueva de cero.
	a.drainAll()

	start := time.Now()
	if _, err := a.allocate(context.Background()); err != nil {
		t.Fatalf("second allocate: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < connectionStaggerDelay/2 {
		t.Fatalf("expected the second connection open to be staggered by ~%v, only waited %v", connectionStaggerDelay, elapsed)
	}
}
