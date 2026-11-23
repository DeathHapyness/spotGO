package fetch

import (
	"context"
	"net/url"
	"sync"
	"time"
)

var defaultLimits = map[string]time.Duration{
	"api.deezer.com":    100 * time.Millisecond,
	"open.spotify.com":  250 * time.Millisecond,
	"api.spotify.com":   100 * time.Millisecond,
	"music.youtube.com": 200 * time.Millisecond,
}

type limiter struct {
	mu       sync.Mutex
	next     time.Time
	interval time.Duration
}

// wait reserva o próximo horário livre dentro do lock e dorme fora dele,
// para que uma goroutine esperando não bloqueie as outras de reservar.
func (l *limiter) wait(ctx context.Context, now func() time.Time, sleep func(context.Context, time.Duration) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	l.mu.Lock()
	t := now()
	slot := l.next
	if slot.Before(t) {
		slot = t
	}
	l.next = slot.Add(l.interval)
	l.mu.Unlock()

	if d := slot.Sub(t); d > 0 {
		return sleep(ctx, d)
	}
	return nil
}

func newLimiters(limits map[string]time.Duration) map[string]*limiter {
	if limits == nil {
		limits = defaultLimits
	}
	m := make(map[string]*limiter, len(limits))
	for h, d := range limits {
		m[h] = &limiter{interval: d}
	}
	return m
}

func (c *Client) waitHost(ctx context.Context, rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil
	}
	l, ok := c.limits[u.Host]
	if !ok {
		return nil
	}
	return l.wait(ctx, c.now, c.sleep)
}
