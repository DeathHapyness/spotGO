package fetch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type sleepRecorder struct{ calls []time.Duration }

func (s *sleepRecorder) sleep(_ context.Context, d time.Duration) error {
	s.calls = append(s.calls, d)
	return nil
}

func newTestClient(o Options) (*Client, *sleepRecorder) {
	c := New(o)
	rec := &sleepRecorder{}
	c.sleep = rec.sleep
	return c, rec
}

func sequence(t *testing.T, handlers ...http.HandlerFunc) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := int(n.Add(1)) - 1
		if i >= len(handlers) {
			i = len(handlers) - 1
		}
		handlers[i](w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, &n
}

func status(code int, header ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		for i := 0; i+1 < len(header); i += 2 {
			w.Header().Set(header[i], header[i+1])
		}
		w.WriteHeader(code)
		w.Write([]byte("corpo"))
	}
}

func TestGetOK(t *testing.T) {
	srv, _ := sequence(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "spotgo-test" {
			t.Errorf("User-Agent = %q", r.Header.Get("User-Agent"))
		}
		if r.Header.Get("X-Extra") != "1" {
			t.Errorf("header do Request não repassado")
		}
		w.Write([]byte("olá"))
	})
	c, _ := newTestClient(Options{UserAgent: "spotgo-test"})

	body, err := c.Get(context.Background(), Request{URL: srv.URL, Header: http.Header{"X-Extra": {"1"}}})
	if err != nil || string(body) != "olá" {
		t.Fatalf("Get = %q, %v", body, err)
	}
}

func TestRetryAfterSecondsThenOK(t *testing.T) {
	srv, n := sequence(t, status(429, "Retry-After", "2"), status(200))
	c, rec := newTestClient(Options{})

	if _, err := c.Get(context.Background(), Request{URL: srv.URL}); err != nil {
		t.Fatal(err)
	}
	if n.Load() != 2 {
		t.Fatalf("chamadas = %d, quero 2", n.Load())
	}
	if len(rec.calls) != 1 || rec.calls[0] != 2*time.Second {
		t.Fatalf("sleeps = %v, quero [2s]", rec.calls)
	}
}

func TestRetryAfterHTTPDate(t *testing.T) {
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	srv, _ := sequence(t, status(429, "Retry-After", now.Add(5*time.Second).Format(http.TimeFormat)), status(200))
	c, rec := newTestClient(Options{})
	c.now = func() time.Time { return now }

	if _, err := c.Get(context.Background(), Request{URL: srv.URL}); err != nil {
		t.Fatal(err)
	}
	if len(rec.calls) != 1 || rec.calls[0] != 5*time.Second {
		t.Fatalf("sleeps = %v, quero [5s]", rec.calls)
	}
}

func TestRetryAfterTooLongFailsFast(t *testing.T) {
	srv, n := sequence(t, status(429, "Retry-After", "86400"))
	c, rec := newTestClient(Options{})

	_, err := c.Get(context.Background(), Request{URL: srv.URL})
	var rl *RateLimitError
	if !errors.As(err, &rl) {
		t.Fatalf("quero *RateLimitError, veio %v", err)
	}
	if rl.RetryAfter != 24*time.Hour {
		t.Errorf("RetryAfter = %v", rl.RetryAfter)
	}
	if !strings.Contains(err.Error(), "24h") {
		t.Errorf("mensagem sem a duração: %q", err)
	}
	if n.Load() != 1 || len(rec.calls) != 0 {
		t.Fatalf("não deve esperar nem repetir: chamadas=%d sleeps=%v", n.Load(), rec.calls)
	}
}

func TestServerErrorBackoffThenGiveUp(t *testing.T) {
	srv, n := sequence(t, status(503))
	c, rec := newTestClient(Options{MaxRetries: 3})

	_, err := c.Get(context.Background(), Request{URL: srv.URL})
	var se *StatusError
	if !errors.As(err, &se) || se.Code != 503 {
		t.Fatalf("quero *StatusError 503, veio %v", err)
	}
	if n.Load() != 4 {
		t.Fatalf("chamadas = %d, quero 4", n.Load())
	}
	want := []time.Duration{500 * time.Millisecond, time.Second, 2 * time.Second}
	if len(rec.calls) != len(want) {
		t.Fatalf("sleeps = %v", rec.calls)
	}
	for i, w := range want {
		lo, hi := time.Duration(float64(w)*0.8), time.Duration(float64(w)*1.2)
		if rec.calls[i] < lo || rec.calls[i] > hi {
			t.Errorf("sleep[%d] = %v, quero entre %v e %v", i, rec.calls[i], lo, hi)
		}
	}
}

func TestNotFoundNoRetry(t *testing.T) {
	srv, n := sequence(t, status(404))
	c, _ := newTestClient(Options{})

	_, err := c.Get(context.Background(), Request{URL: srv.URL})
	var se *StatusError
	if !errors.As(err, &se) || se.Code != 404 {
		t.Fatalf("quero *StatusError 404, veio %v", err)
	}
	if n.Load() != 1 {
		t.Fatalf("chamadas = %d, quero 1", n.Load())
	}
}

func TestContextCanceledDuringBackoff(t *testing.T) {
	srv, _ := sequence(t, status(503))
	c := New(Options{MaxRetries: 5})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := c.Get(ctx, Request{URL: srv.URL})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("quero erro do contexto, veio %v", err)
	}
	if time.Since(start) > time.Second {
		t.Fatalf("demorou %v para respeitar o cancelamento", time.Since(start))
	}
}

func TestBodyTooLarge(t *testing.T) {
	srv, _ := sequence(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("12345678901"))
	})
	c, _ := newTestClient(Options{MaxBody: 10})

	_, err := c.Get(context.Background(), Request{URL: srv.URL})
	if err == nil || !strings.Contains(err.Error(), "resposta maior que o limite") {
		t.Fatalf("quero erro de tamanho, veio %v", err)
	}
}
