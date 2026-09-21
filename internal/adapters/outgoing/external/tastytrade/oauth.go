package tastytrade

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

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

	// breaker: ventana compartida entre TODAS las conexiones del proceso
	// cuando el limite de sesiones de TastyTrade esta saturado -- ver
	// session_breaker.go para el porque hace falta ademas de resetGroup.
	breaker SessionBreaker
}

// MarkSessionsSaturated extiende la ventana compartida de espera -- llamado
// por cualquier DxLinkConn (en vivo o efimera del barrido) que detecta un
// rechazo por limite de sesiones.
func (o *OAuth) MarkSessionsSaturated() {
	o.breaker.MarkSaturated()
}

// WaitForSessionCooldown bloquea hasta que la ventana compartida termine, si
// esta activa -- llamado antes de CUALQUIER intento de dial nuevo.
func (o *OAuth) WaitForSessionCooldown(ctx context.Context) error {
	return o.breaker.Wait(ctx)
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

// ResetSessions rota el access token, colapsando llamadas concurrentes en
// una sola (ver el comentario de resetGroup). DELETE /sessions ya no se
// llama: TastyTrade retiro las sesiones legacy (feb-2026), siempre da 403, y
// las sesiones huerfanas de DxLink solo se liberan por expiracion del lado
// servidor (docs/dxlink-session-incidents.md).
func (o *OAuth) ResetSessions(ctx context.Context) error {
	_, err, _ := o.resetGroup.Do("reset", func() (interface{}, error) {
		return nil, o.resetSessionsOnce(ctx)
	})
	return err
}

func (o *OAuth) resetSessionsOnce(ctx context.Context) error {
	_, err := o.RefreshAccessToken(ctx)
	return err
}
