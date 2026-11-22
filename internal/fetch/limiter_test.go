package fetch

import (
	"context"
	"errors"
	"net/url"
	"sync"
	"testing"
	"time"
)

func TestLimiterSpacing(t *testing.T) {
	l := &limiter{interval: 100 * time.Millisecond}
	frozen := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	now := func() time.Time { return frozen }
	rec := &sleepRecorder{}

	for i := 0; i < 3; i++ {
		if err := l.wait(context.Background(), now, rec.sleep); err != nil {
			t.Fatal(err)
		}
	}
	want := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond}
	if len(rec.calls) != len(want) || rec.calls[0] != want[0] || rec.calls[1] != want[1] {
		t.Fatalf("sleeps = %v, quero %v (a primeira chamada não espera)", rec.calls, want)
	}
}

func TestLimiterConcurrent(t *testing.T) {
	l := &limiter{interval: 5 * time.Millisecond}
	var wg sync.WaitGroup
	start := time.Now()

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := l.wait(context.Background(), time.Now, sleepCtx); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()

	if elapsed := time.Since(start); elapsed < 95*time.Millisecond {
		t.Fatalf("20 chamadas com intervalo de 5ms levaram só %v", elapsed)
	}
}

func TestLimiterCanceled(t *testing.T) {
	l := &limiter{interval: time.Hour}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := l.wait(ctx, time.Now, sleepCtx); !errors.Is(err, context.Canceled) {
		t.Fatalf("quero context.Canceled, veio %v", err)
	}
}

func TestGetUsesHostLimit(t *testing.T) {
	srv, _ := sequence(t, status(200))
	u, _ := url.Parse(srv.URL)
	c, rec := newTestClient(Options{Limits: map[string]time.Duration{u.Host: 50 * time.Millisecond}})
	frozen := time.Now()
	c.now = func() time.Time { return frozen }

	for i := 0; i < 2; i++ {
		if _, err := c.Get(context.Background(), Request{URL: srv.URL}); err != nil {
			t.Fatal(err)
		}
	}
	if len(rec.calls) != 1 || rec.calls[0] != 50*time.Millisecond {
		t.Fatalf("sleeps = %v, quero [50ms]", rec.calls)
	}
}

func TestDefaultLimits(t *testing.T) {
	c := New(Options{})
	for host, want := range map[string]time.Duration{
		"api.deezer.com":   100 * time.Millisecond,
		"open.spotify.com": 250 * time.Millisecond,
		"api.spotify.com":  100 * time.Millisecond,
	} {
		l, ok := c.limits[host]
		if !ok || l.interval != want {
			t.Errorf("limite de %s = %+v, quero %v", host, l, want)
		}
	}
}
