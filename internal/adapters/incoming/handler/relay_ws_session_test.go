package handler

import (
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/livecandles"
)

func TestRelayWSSession_SubscribeUnsubscribeTracksBroadcasterSubs(t *testing.T) {
	broadcaster := livecandles.NewBroadcaster[domain.IntradaySnapshot]()
	session := newRelayWSSession(nil, broadcaster, snapshotMessage)

	session.handleSubscribe("AAPL")
	session.mu.Lock()
	_, subscribed := session.subs["AAPL"]
	session.mu.Unlock()
	if !subscribed {
		t.Fatal("expected AAPL to be tracked after handleSubscribe")
	}

	session.handleSubscribe("AAPL")
	session.mu.Lock()
	subCount := len(session.subs)
	session.mu.Unlock()
	if subCount != 1 {
		t.Fatalf("expected a repeated subscribe to be a no-op, got %d subs", subCount)
	}

	session.handleUnsubscribe("AAPL")
	session.mu.Lock()
	_, stillSubscribed := session.subs["AAPL"]
	session.mu.Unlock()
	if stillSubscribed {
		t.Fatal("expected AAPL to be removed after handleUnsubscribe")
	}

	// Sin nadie suscripto, Publish no debe bloquear ni dejar nada pendiente --
	// solo confirma que el cancel() de arriba de verdad cerro la suscripcion.
	broadcaster.Publish("AAPL", domain.IntradaySnapshot{Symbol: "AAPL"})
	time.Sleep(10 * time.Millisecond)
}
