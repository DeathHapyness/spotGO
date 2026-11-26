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
	AccessToken  string `json:"access_token"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
}

type tokenError struct {
	Error       string `json:"error"`
	Description string `json:"error_description"`
}

// RequestToken retorna ErrInvalidCredentials só para 400/401. Falhas de rede
// e 5xx são erros comuns, para que o chamador não peça credenciais novas
// quando o problema é a conexão.
func RequestToken(ctx context.Context, hc *http.Client, tokenURL string, c config.Credentials) (Token, error) {
	tok, _, err := postToken(ctx, hc, tokenURL, url.Values{"grant_type": {"client_credentials"}}, &c)
	return tok, err
}

// ExchangeCode troca o código do login (PKCE) pelo token e pelo refresh token.
func ExchangeCode(ctx context.Context, hc *http.Client, tokenURL, clientID, code, verifier, redirect string) (Token, string, error) {
	return postToken(ctx, hc, tokenURL, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirect},
		"client_id":     {clientID},
		"code_verifier": {verifier},
	}, nil)
}

// Refresh renova o access token. Com PKCE o Spotify pode devolver um refresh
// token novo (o antigo deixa de valer); vazio significa que o antigo continua.
func Refresh(ctx context.Context, hc *http.Client, tokenURL, clientID, refresh string) (Token, string, error) {
	return postToken(ctx, hc, tokenURL, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refresh},
		"client_id":     {clientID},
	}, nil)
}

func postToken(ctx context.Context, hc *http.Client, tokenURL string, form url.Values, basic *config.Credentials) (Token, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return Token{}, "", err
	}
	if basic != nil {
		req.SetBasicAuth(basic.ClientID, basic.ClientSecret)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := hc.Do(req)
	if err != nil {
		return Token{}, "", fmt.Errorf("falha ao contatar o Spotify: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxTokenBody))
	if err != nil {
		return Token{}, "", fmt.Errorf("lendo resposta do Spotify: %w", err)
	}

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusBadRequest, http.StatusUnauthorized:
		var te tokenError
		_ = json.Unmarshal(body, &te)
		if te.Description != "" {
			return Token{}, "", fmt.Errorf("%w: %s", ErrInvalidCredentials, te.Description)
		}
		if te.Error != "" {
			return Token{}, "", fmt.Errorf("%w: %s", ErrInvalidCredentials, te.Error)
		}
		return Token{}, "", ErrInvalidCredentials
	default:
		return Token{}, "", fmt.Errorf("Spotify respondeu %s ao pedir token", resp.Status)
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return Token{}, "", fmt.Errorf("resposta de token inválida: %w", err)
	}
	if tr.AccessToken == "" {
		return Token{}, "", errors.New("resposta de token sem access_token")
	}
	return Token{
		AccessToken: tr.AccessToken,
		ExpiresAt:   time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second),
	}, tr.RefreshToken, nil
}
