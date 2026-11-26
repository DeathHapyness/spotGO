package spotify

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/url"
	"time"
)

const (
	AuthorizeURL = "https://accounts.spotify.com/authorize"
	// RedirectURI é a que o README manda cadastrar no app.
	RedirectURI = "http://127.0.0.1:8888/callback"
	Scopes      = "playlist-read-private playlist-read-collaborative"
	// LoginTimeout é quanto o servidor local espera a volta do navegador.
	LoginTimeout = 5 * time.Minute
)

// Login guarda o que o fluxo PKCE precisa entre abrir o navegador e receber o código.
type Login struct {
	ClientID string
	Verifier string
	State    string
	// Addr é onde o servidor escuta; vazio = o host:porta da RedirectURI.
	Addr     string
	Redirect string
}

func NewLogin(clientID string) (*Login, error) {
	v, err := randomString(64)
	if err != nil {
		return nil, err
	}
	st, err := randomString(24)
	if err != nil {
		return nil, err
	}
	return &Login{ClientID: clientID, Verifier: v, State: st, Redirect: RedirectURI}, nil
}

func randomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Challenge é o code_challenge S256 do verifier.
func Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func (l *Login) URL() string {
	q := url.Values{
		"client_id":             {l.ClientID},
		"response_type":         {"code"},
		"redirect_uri":          {l.Redirect},
		"code_challenge_method": {"S256"},
		"code_challenge":        {Challenge(l.Verifier)},
		"state":                 {l.State},
		"scope":                 {Scopes},
	}
	return AuthorizeURL + "?" + q.Encode()
}

var ErrLoginDenied = errors.New("o login foi recusado no Spotify")

// Listen abre o servidor local antes de abrir o navegador: se a porta estiver
// ocupada, o erro aparece sem o usuário ter feito login à toa.
func (l *Login) Listen() (net.Listener, error) {
	addr := l.Addr
	if addr == "" {
		u, err := url.Parse(l.Redirect)
		if err != nil {
			return nil, err
		}
		addr = u.Host
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("não consegui abrir %s para receber o login (%w); feche o que estiver usando a porta", addr, err)
	}
	return ln, nil
}

// Wait atende o primeiro callback com o state certo e devolve o código.
// Fecha o servidor no fim, com sucesso, erro, cancelamento ou timeout.
func (l *Login) Wait(ctx context.Context, ln net.Listener, timeout time.Duration) (string, error) {
	path := "/callback"
	if u, err := url.Parse(l.Redirect); err == nil && u.Path != "" {
		path = u.Path
	}
	type result struct {
		code string
		err  error
	}
	done := make(chan result, 1)
	mux := http.NewServeMux()
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		// Um state errado é outra aba ou outra página: ignora sem encerrar o login.
		if subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(l.State)) != 1 {
			http.Error(w, "state inválido", http.StatusBadRequest)
			return
		}
		var res result
		switch {
		case q.Get("error") != "":
			res.err = fmt.Errorf("%w (%s)", ErrLoginDenied, q.Get("error"))
		case q.Get("code") == "":
			res.err = errors.New("o Spotify voltou sem o código do login")
		default:
			res.code = q.Get("code")
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		msg := "Login feito. Pode fechar esta aba e voltar ao terminal."
		if res.err != nil {
			msg = "O login falhou: " + res.err.Error()
		}
		fmt.Fprintf(w, "<!doctype html><meta charset=utf-8><title>spotgo</title><p style=\"font:16px sans-serif\">%s</p>", html.EscapeString(msg))
		select {
		case done <- res:
		default:
		}
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go srv.Serve(ln)
	defer func() {
		sctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		srv.Shutdown(sctx)
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case r := <-done:
		return r.code, r.err
	case <-ctx.Done():
		return "", ctx.Err()
	case <-timer.C:
		return "", fmt.Errorf("o login não voltou do navegador em %s", timeout)
	}
}
