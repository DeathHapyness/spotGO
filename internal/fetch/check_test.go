package fetch

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func body(s string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte(s)) }
}

func TestCheckErrorNotCached(t *testing.T) {
	srv, n := sequence(t, body(`{"error":800}`))
	c, _ := newTestClient(Options{CacheDir: t.TempDir()})
	errRuim := errors.New("corpo ruim")
	req := Request{
		URL:      srv.URL,
		CacheTTL: time.Hour,
		Check:    func([]byte) error { return errRuim },
	}

	for i := 0; i < 2; i++ {
		if _, err := c.Get(context.Background(), req); !errors.Is(err, errRuim) {
			t.Fatalf("chamada %d: err = %v, quero %v", i, err, errRuim)
		}
	}
	if n.Load() != 2 {
		t.Fatalf("chamadas ao servidor = %d, quero 2 (erro não pode ir para o cache)", n.Load())
	}
}

func TestCheckRetryAfter(t *testing.T) {
	srv, n := sequence(t, body(`{"error":4}`), body(`{"ok":1}`))
	c, rec := newTestClient(Options{CacheDir: t.TempDir()})
	req := Request{
		URL:      srv.URL,
		CacheTTL: time.Hour,
		Check: func(b []byte) error {
			if strings.Contains(string(b), "error") {
				return &RetryAfterError{After: 5 * time.Second, Err: errors.New("cota")}
			}
			return nil
		},
	}

	got, err := c.Get(context.Background(), req)
	if err != nil || string(got) != `{"ok":1}` {
		t.Fatalf("Get = %q, %v", got, err)
	}
	if n.Load() != 2 {
		t.Fatalf("chamadas ao servidor = %d, quero 2", n.Load())
	}
	if len(rec.calls) != 1 || rec.calls[0] != 5*time.Second {
		t.Fatalf("sleeps = %v, quero [5s]", rec.calls)
	}
}

func TestCheckRetryAfterGivesUp(t *testing.T) {
	srv, n := sequence(t, body(`{"error":4}`))
	c, _ := newTestClient(Options{CacheDir: t.TempDir(), MaxRetries: 2})
	cota := errors.New("cota")
	req := Request{
		URL:   srv.URL,
		Check: func([]byte) error { return &RetryAfterError{After: time.Second, Err: cota} },
	}

	_, err := c.Get(context.Background(), req)
	if !errors.Is(err, cota) {
		t.Fatalf("err = %v, quero que envolva %v", err, cota)
	}
	if n.Load() != 3 {
		t.Fatalf("chamadas ao servidor = %d, quero 3 (1 + 2 retries)", n.Load())
	}
}

func TestCheckRetryAfterTooLongFailsFast(t *testing.T) {
	srv, n := sequence(t, body(`{"error":4}`))
	c, rec := newTestClient(Options{MaxWait: time.Second})
	req := Request{
		URL:   srv.URL,
		Check: func([]byte) error { return &RetryAfterError{After: time.Minute, Err: errors.New("cota")} },
	}

	var ra *RetryAfterError
	if _, err := c.Get(context.Background(), req); !errors.As(err, &ra) {
		t.Fatalf("err = %v, quero *RetryAfterError", err)
	}
	if n.Load() != 1 || len(rec.calls) != 0 {
		t.Fatalf("chamadas = %d, sleeps = %v; quero 1 chamada e nenhuma espera", n.Load(), rec.calls)
	}
}

func TestCheckPassesThrough(t *testing.T) {
	srv, n := sequence(t, body("ok"))
	c, _ := newTestClient(Options{CacheDir: t.TempDir()})
	req := Request{URL: srv.URL, CacheTTL: time.Hour, Check: func([]byte) error { return nil }}

	for i := 0; i < 2; i++ {
		if got, err := c.Get(context.Background(), req); err != nil || string(got) != "ok" {
			t.Fatalf("Get = %q, %v", got, err)
		}
	}
	if n.Load() != 1 {
		t.Fatalf("chamadas ao servidor = %d, quero 1 (resposta válida vai para o cache)", n.Load())
	}
}
