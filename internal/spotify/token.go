// Package spotify é o cliente da Web API do Spotify usado pelo spotgo.
package spotify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/DeathHapyness/spotGO/internal/config"
)

const TokenURL = "https://accounts.spotify.com/api/token"

const maxTokenBody = 64 << 10

var ErrInvalidCredentials = errors.New("credenciais do Spotify inválidas")

type Token struct {
	AccessToken string
	ExpiresAt   time.Time
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

type tokenError struct {
	Error       string `json:"error"`
	Description string `json:"error_description"`
}

// RequestToken retorna ErrInvalidCredentials só para 400/401. Falhas de rede
// e 5xx são erros comuns, para que o chamador não peça credenciais novas
// quando o problema é a conexão.
func RequestToken(ctx context.Context, hc *http.Client, tokenURL string, c config.Credentials) (Token, error) {
	form := url.Values{"grant_type": {"client_credentials"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return Token{}, err
	}
	req.SetBasicAuth(c.ClientID, c.ClientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := hc.Do(req)
	if err != nil {
		return Token{}, fmt.Errorf("falha ao contatar o Spotify: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxTokenBody))
	if err != nil {
		return Token{}, fmt.Errorf("lendo resposta do Spotify: %w", err)
	}

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusBadRequest, http.StatusUnauthorized:
		var te tokenError
		_ = json.Unmarshal(body, &te)
		if te.Description != "" {
			return Token{}, fmt.Errorf("%w: %s", ErrInvalidCredentials, te.Description)
		}
		return Token{}, ErrInvalidCredentials
	default:
		return Token{}, fmt.Errorf("Spotify respondeu %s ao pedir token", resp.Status)
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return Token{}, fmt.Errorf("resposta de token inválida: %w", err)
	}
	if tr.AccessToken == "" {
		return Token{}, errors.New("resposta de token sem access_token")
	}
	return Token{
		AccessToken: tr.AccessToken,
		ExpiresAt:   time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second),
	}, nil
}
