package fetch

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"
)

func echoBody(t *testing.T, got *[]string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Method = %q, quero POST", r.Method)
		}
		b, _ := io.ReadAll(r.Body)
		*got = append(*got, string(b))
		w.Write([]byte("ok:" + string(b)))
	}
}

func TestPostSendsBody(t *testing.T) {
	var got []string
	srv, _ := sequence(t, echoBody(t, &got))
	c, _ := newTestClient(Options{})

	body, err := c.Get(context.Background(), Request{URL: srv.URL, Method: http.MethodPost, Body: []byte(`{"q":1}`)})
	if err != nil || string(body) != `ok:{"q":1}` {
		t.Fatalf("Get = %q, %v", body, err)
	}
	if len(got) != 1 || got[0] != `{"q":1}` {
		t.Fatalf("servidor recebeu %q", got)
	}
}

func TestPostRetrySendsBodyAgain(t *testing.T) {
	var got []string
	fail := func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = append(got, string(b))
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	srv, n := sequence(t, fail, echoBody(t, &got))
	c, _ := newTestClient(Options{})

	if _, err := c.Get(context.Background(), Request{URL: srv.URL, Method: http.MethodPost, Body: []byte("abc")}); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if n.Load() != 2 || len(got) != 2 || got[0] != "abc" || got[1] != "abc" {
		t.Fatalf("tentativas = %d, corpos = %q", n.Load(), got)
	}
}

func TestCacheKeyIncludesBody(t *testing.T) {
	var got []string
	srv, n := sequence(t, echoBody(t, &got))
	c, _ := newTestClient(Options{CacheDir: t.TempDir()})
	get := func(b string) string {
		body, err := c.Get(context.Background(), Request{URL: srv.URL, Method: http.MethodPost, Body: []byte(b), CacheTTL: time.Hour})
		if err != nil {
			t.Fatalf("Get(%q): %v", b, err)
		}
		return string(body)
	}

	if get("a") != "ok:a" || get("b") != "ok:b" {
		t.Fatalf("corpos trocados no cache")
	}
	if get("a") != "ok:a" || n.Load() != 2 {
		t.Fatalf("requisições = %d, quero 2 (a terceira vem do cache)", n.Load())
	}
}
