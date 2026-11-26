package spotify

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestLoginURL(t *testing.T) {
	l, err := NewLogin("cid")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(l.URL())
	q := u.Query()
	if q.Get("client_id") != "cid" || q.Get("code_challenge_method") != "S256" || q.Get("redirect_uri") != RedirectURI ||
		q.Get("state") != l.State || q.Get("code_challenge") != Challenge(l.Verifier) || q.Get("scope") != Scopes {
		t.Errorf("URL = %s", u)
	}
	// SHA-256 em base64url sem padding, conferido com o hashlib do Python.
	if got := Challenge("dBjftJeZ4CVP-mJ0kzi9mLj9kqDDtA7dnB5PUc6FZ0k"); got != "hVrocD1Xmt9gsDryirZkRgrax2XEGIfq2rwbcG2SPfA" {
		t.Errorf("Challenge = %s", got)
	}
}

func startLogin(t *testing.T) (*Login, string, chan [2]any) {
	t.Helper()
	l, _ := NewLogin("cid")
	l.Addr = "127.0.0.1:0"
	ln, err := l.Listen()
	if err != nil {
		t.Fatal(err)
	}
	base := "http://" + ln.Addr().String() + "/callback"
	done := make(chan [2]any, 1)
	go func() {
		code, err := l.Wait(context.Background(), ln, 5*time.Second)
		done <- [2]any{code, err}
	}()
	return l, base, done
}

func TestLoginWait(t *testing.T) {
	l, base, done := startLogin(t)
	resp, err := http.Get(base + "?state=errado&code=x")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("state errado: %d", resp.StatusCode)
	}
	resp, err = http.Get(base + "?state=" + l.State + "&code=abc")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	r := <-done
	if r[0] != "abc" || r[1] != nil {
		t.Errorf("Wait = %v", r)
	}
}

func TestLoginDenied(t *testing.T) {
	l, base, done := startLogin(t)
	resp, err := http.Get(base + "?state=" + l.State + "&error=access_denied")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if r := <-done; !errors.Is(r[1].(error), ErrLoginDenied) {
		t.Errorf("err = %v", r[1])
	}
}

func TestLoginTimeoutAndCancel(t *testing.T) {
	l, _ := NewLogin("cid")
	l.Addr = "127.0.0.1:0"
	ln, _ := l.Listen()
	if _, err := l.Wait(context.Background(), ln, 10*time.Millisecond); err == nil || !strings.Contains(err.Error(), "não voltou") {
		t.Errorf("timeout: %v", err)
	}
	ln, _ = l.Listen()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := l.Wait(ctx, ln, time.Minute); !errors.Is(err, context.Canceled) {
		t.Errorf("cancel: %v", err)
	}
}
