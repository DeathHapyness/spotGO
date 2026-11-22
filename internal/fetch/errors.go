package fetch

import (
	"fmt"
	"net/url"
	"time"
)

type StatusError struct {
	Code int
	URL  string
	Body string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("%s respondeu HTTP %d", host(e.URL), e.Code)
}

type RateLimitError struct {
	RetryAfter time.Duration
	URL        string
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("limite de requisições em %s; tente de novo em %s", host(e.URL), e.RetryAfter.Round(time.Second))
}

func host(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	return u.Host
}

// RetryAfterError é devolvido por um Request.Check quando o corpo indica um
// limite de requisições (por exemplo, o Deezer responde HTTP 200 com erro).
type RetryAfterError struct {
	After time.Duration
	Err   error
}

func (e *RetryAfterError) Error() string {
	return fmt.Sprintf("%v; tente de novo em %s", e.Err, e.After.Round(time.Second))
}

func (e *RetryAfterError) Unwrap() error { return e.Err }
