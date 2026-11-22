// Package fetch é o cliente HTTP compartilhado: retry, limitador por host e cache.
package fetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"
)

type Request struct {
	URL      string
	Header   http.Header
	CacheTTL time.Duration
	// Check valida respostas 2xx antes do cache. Um *RetryAfterError faz a
	// requisição ser repetida como um 429; qualquer outro erro volta na hora.
	Check func(body []byte) error
}

type Options struct {
	HTTP       *http.Client
	MaxRetries int
	MaxWait    time.Duration
	MaxBody    int64
	UserAgent  string
	CacheDir   string
	Limits     map[string]time.Duration
	// Sleep substitui a espera real (testes de outros pacotes). nil = espera real.
	Sleep func(ctx context.Context, d time.Duration) error
}

type Client struct {
	http       *http.Client
	maxRetries int
	maxWait    time.Duration
	maxBody    int64
	userAgent  string
	cacheDir   string
	limits     map[string]*limiter

	sleep func(ctx context.Context, d time.Duration) error
	now   func() time.Time
}

const (
	defaultMaxRetries = 3
	defaultMaxWait    = 60 * time.Second
	defaultMaxBody    = 16 << 20
	baseBackoff       = 500 * time.Millisecond
	maxErrorBody      = 512
)

func New(o Options) *Client {
	c := &Client{
		http:       o.HTTP,
		maxRetries: o.MaxRetries,
		maxWait:    o.MaxWait,
		maxBody:    o.MaxBody,
		userAgent:  o.UserAgent,
		cacheDir:   o.CacheDir,
		limits:     newLimiters(o.Limits),
		sleep:      sleepCtx,
		now:        time.Now,
	}
	if o.Sleep != nil {
		c.sleep = o.Sleep
	}
	if c.http == nil {
		c.http = &http.Client{Timeout: 30 * time.Second}
	}
	if c.maxRetries == 0 {
		c.maxRetries = defaultMaxRetries
	}
	if c.maxWait == 0 {
		c.maxWait = defaultMaxWait
	}
	if c.maxBody == 0 {
		c.maxBody = defaultMaxBody
	}
	return c
}

func (c *Client) Get(ctx context.Context, r Request) ([]byte, error) {
	if c.cacheEnabled(r) {
		if body, ok := c.cacheGet(r); ok {
			return body, nil
		}
	}
	for attempt := 0; ; attempt++ {
		if err := c.waitHost(ctx, r.URL); err != nil {
			return nil, err
		}
		body, wait, err := c.do(ctx, r)
		if err == nil {
			if c.cacheEnabled(r) {
				c.cachePut(r.URL, body)
			}
			return body, nil
		}
		if wait < 0 || attempt >= c.maxRetries {
			return nil, err
		}
		if wait == 0 {
			wait = backoff(attempt)
		}
		if serr := c.sleep(ctx, wait); serr != nil {
			return nil, serr
		}
	}
}

// do faz uma tentativa. wait < 0 significa "não repetir"; wait == 0 significa
// "repetir com backoff"; wait > 0 é o tempo pedido pelo servidor.
func (c *Client) do(ctx context.Context, r Request) ([]byte, time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.URL, nil)
	if err != nil {
		return nil, -1, err
	}
	for k, v := range r.Header {
		req.Header[k] = v
	}
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, -1, ctx.Err()
		}
		return nil, 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, c.maxBody+1))
	if err != nil {
		if ctx.Err() != nil {
			return nil, -1, ctx.Err()
		}
		return nil, 0, err
	}
	if int64(len(body)) > c.maxBody {
		return nil, -1, fmt.Errorf("%s: resposta maior que o limite de %d bytes", host(r.URL), c.maxBody)
	}

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return c.check(r, body)
	case resp.StatusCode == http.StatusTooManyRequests:
		wait := c.retryAfter(resp.Header.Get("Retry-After"))
		rl := &RateLimitError{RetryAfter: wait, URL: r.URL}
		if wait > c.maxWait {
			return nil, -1, rl
		}
		return nil, wait, rl
	case resp.StatusCode >= 500:
		return nil, 0, statusError(resp.StatusCode, r.URL, body)
	default:
		return nil, -1, statusError(resp.StatusCode, r.URL, body)
	}
}

func (c *Client) check(r Request, body []byte) ([]byte, time.Duration, error) {
	if r.Check == nil {
		return body, 0, nil
	}
	err := r.Check(body)
	if err == nil {
		return body, 0, nil
	}
	var ra *RetryAfterError
	if errors.As(err, &ra) && ra.After <= c.maxWait {
		return nil, ra.After, err
	}
	return nil, -1, err
}

func (c *Client) retryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := t.Sub(c.now()); d > 0 {
			return d
		}
	}
	return 0
}

func statusError(code int, url string, body []byte) *StatusError {
	if len(body) > maxErrorBody {
		body = body[:maxErrorBody]
	}
	return &StatusError{Code: code, URL: url, Body: string(body)}
}

func backoff(attempt int) time.Duration {
	d := baseBackoff << attempt
	jitter := 0.8 + 0.4*rand.Float64()
	return time.Duration(float64(d) * jitter)
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
