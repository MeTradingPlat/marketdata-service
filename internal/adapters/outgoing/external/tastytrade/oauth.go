package tastytrade

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"golang.org/x/sync/singleflight"
)

type OAuthConfig struct {
	BaseURL      string
	ClientID     string
	ClientSecret string
	RefreshToken string
}

type OAuth struct {
	cfg        OAuthConfig
	httpClient *http.Client

	mu          sync.RWMutex
	accessToken string

	// resetGroup: cada DxLinkConn del pool (30-40 en produccion) llama
	// ResetSessions por su cuenta cuando detecta su propia sesion saturada
	// -- sin coordinacion, si varias lo detectan casi al mismo tiempo (tipico
	// en la apertura del mercado, cuando el host se congela un momento y
	// todas quedan "silenciosas" juntas), cada una manda su propio DELETE
	// /sessions + refresh de token, y el DELETE de una invalida la sesion
	// recien creada por otra -- una tormenta que se auto-alimenta
	// indefinidamente (confirmado en vivo el 2026-08-18 y otra vez el
	// 2026-08-28: silencio total de DxLink por horas en pleno mercado
	// abierto, sin recuperarse solo). singleflight colapsa las llamadas
	// concurrentes en UNA sola ejecucion real -- el resto espera su
	// resultado en vez de pisarlo.
	resetGroup singleflight.Group
}

func NewOAuth(cfg OAuthConfig) *OAuth {
	return &OAuth{cfg: cfg, httpClient: &http.Client{Timeout: 15 * time.Second}}
}

func (o *OAuth) AccessToken() string {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.accessToken
}

type refreshRequest struct {
	GrantType    string `json:"grant_type"`
	RefreshToken string `json:"refresh_token"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// RefreshAccessToken usa el refresh_token grant -- el refresh_token rotado
// que devuelve TastyTrade se guarda en memoria para la proxima llamada, o
// el original (el de la variable de entorno) deja de servir.
func (o *OAuth) RefreshAccessToken(ctx context.Context) (string, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	body, err := json.Marshal(refreshRequest{
		GrantType:    "refresh_token",
		RefreshToken: o.cfg.RefreshToken,
		ClientID:     o.cfg.ClientID,
		ClientSecret: o.cfg.ClientSecret,
	})
	if err != nil {
		return "", fmt.Errorf("encoding oauth refresh request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.cfg.BaseURL+"/oauth/token", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("building oauth refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling oauth refresh: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("oauth refresh returned status %d", resp.StatusCode)
	}

	var tr tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", fmt.Errorf("decoding oauth refresh response: %w", err)
	}
	if tr.AccessToken == "" {
		return "", fmt.Errorf("oauth refresh response missing access_token")
	}

	o.accessToken = tr.AccessToken
	if tr.RefreshToken != "" {
		o.cfg.RefreshToken = tr.RefreshToken
	}
	return o.accessToken, nil
}

// LogoutAllSessions cierra las sesiones de TastyTrade del access token
// actual (incluidas las conexiones dxlink de contenedores anteriores que
// quedaron vivas server-side tras un kill y saturan el limite de sesiones
// del usuario). El access token usado queda invalidado: hay que refrescarlo
// antes del proximo AUTH.
func (o *OAuth) LogoutAllSessions(ctx context.Context) error {
	o.mu.RLock()
	token := o.accessToken
	o.mu.RUnlock()
	if token == "" {
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, o.cfg.BaseURL+"/sessions", nil)
	if err != nil {
		return fmt.Errorf("building sessions logout request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("calling sessions logout: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK {
		return nil
	}
	return fmt.Errorf("sessions logout returned status %d", resp.StatusCode)
}

// ResetSessions cierra todas las sesiones de TastyTrade y rota el access
// token -- la limpieza ordenada que el reconnect reactivo necesita para no
// quedarse atrapado en "sessions exceeded". El logout va primero porque
// DELETE /sessions invalida el token con el que se llama; si falla (403
// tipico cuando el access token ya vencio), se refresca igual y se reintenta
// una vez con el token fresco -- sin ese reintento un token vencido deja las
// sesiones huerfanas saturando el limite indefinidamente (confirmado en vivo
// el 2026-08-25: logout 403 + refill cayendo de 250k a 3k velas/hora).
//
// singleflight.Do colapsa llamadas concurrentes (ver el comentario de
// resetGroup): sin esto, cada DxLinkConn del pool que detecta su sesion
// saturada casi al mismo tiempo manda su propio DELETE /sessions, y cada
// uno invalida la sesion recien creada por el anterior -- confirmado en
// vivo el 2026-08-28: silencio total de DxLink por horas en pleno mercado
// abierto, sin recuperarse solo.
func (o *OAuth) ResetSessions(ctx context.Context) error {
	_, err, _ := o.resetGroup.Do("reset", func() (interface{}, error) {
		return nil, o.resetSessionsOnce(ctx)
	})
	return err
}

func (o *OAuth) resetSessionsOnce(ctx context.Context) error {
	if err := o.LogoutAllSessions(ctx); err != nil {
		log.Warn().Err(err).Msg("dxlink: logout de sesiones falló, refrescando token y reintentando")
		if _, rerr := o.RefreshAccessToken(ctx); rerr != nil {
			return rerr
		}
		if rerr := o.LogoutAllSessions(ctx); rerr != nil {
			log.Warn().Err(rerr).Msg("dxlink: segundo logout también falló, siguiendo con token fresco")
		}
	}
	_, err := o.RefreshAccessToken(ctx)
	return err
}
