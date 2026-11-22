// Package deezer completa os metadados das faixas usando a API pública do Deezer.
package deezer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/DeathHapyness/spotGO/internal/fetch"
)

var ErrNotFound = errors.New("não encontrado no Deezer")

const (
	defaultBase = "https://api.deezer.com"
	pageSize    = 100
	quotaWait   = 5 * time.Second

	ttlSearch = 24 * time.Hour
	ttlItem   = 30 * 24 * time.Hour
)

type Client struct {
	f    *fetch.Client
	base string
}

func New(f *fetch.Client) *Client {
	return &Client{f: f, base: defaultBase}
}

type apiArtist struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	NbFan int    `json:"nb_fan"`
}

type apiAlbum struct {
	ID          int64     `json:"id"`
	Title       string    `json:"title"`
	ReleaseDate string    `json:"release_date"`
	RecordType  string    `json:"record_type"`
	CoverXL     string    `json:"cover_xl"`
	NbTracks    int       `json:"nb_tracks"`
	Artist      apiArtist `json:"artist"`
}

type apiTrack struct {
	ID            int64     `json:"id"`
	Title         string    `json:"title"`
	TitleShort    string    `json:"title_short"`
	TitleVersion  string    `json:"title_version"`
	ISRC          string    `json:"isrc"`
	ReleaseDate   string    `json:"release_date"`
	Duration      int       `json:"duration"`
	TrackPosition int       `json:"track_position"`
	DiskNumber    int       `json:"disk_number"`
	Explicit      bool      `json:"explicit_lyrics"`
	Artist        apiArtist `json:"artist"`
	Album         apiAlbum  `json:"album"`
}

type page[T any] struct {
	Data  []T `json:"data"`
	Total int `json:"total"`
}

// checkBody trata os erros que o Deezer devolve com HTTP 200.
func checkBody(body []byte) error {
	var e struct {
		Error *struct {
			Type    string `json:"type"`
			Message string `json:"message"`
			Code    int    `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &e) != nil || e.Error == nil {
		return nil
	}
	switch e.Error.Code {
	case 4:
		return &fetch.RetryAfterError{After: quotaWait, Err: errors.New("cota do Deezer excedida")}
	case 800:
		return ErrNotFound
	default:
		return fmt.Errorf("deezer: %s (código %d)", e.Error.Message, e.Error.Code)
	}
}

func getJSON[T any](ctx context.Context, c *Client, path string, q url.Values, ttl time.Duration) (T, error) {
	var v T
	u := c.base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	body, err := c.f.Get(ctx, fetch.Request{URL: u, CacheTTL: ttl, Check: checkBody})
	if err != nil {
		return v, err
	}
	if err := json.Unmarshal(body, &v); err != nil {
		return v, fmt.Errorf("deezer: resposta inesperada de %s: %w", path, err)
	}
	return v, nil
}

func search[T any](ctx context.Context, c *Client, path, q string, limit int) ([]T, error) {
	p, err := getJSON[page[T]](ctx, c, path, url.Values{"q": {q}, "limit": {strconv.Itoa(limit)}}, ttlSearch)
	return p.Data, err
}

// paginate segue index/limit até total, sem usar o campo "next" (URL absoluta).
func paginate[T any](ctx context.Context, c *Client, path string, ttl time.Duration) ([]T, error) {
	var all []T
	for index := 0; ; index += pageSize {
		q := url.Values{"index": {strconv.Itoa(index)}, "limit": {strconv.Itoa(pageSize)}}
		p, err := getJSON[page[T]](ctx, c, path, q, ttl)
		if err != nil {
			return nil, err
		}
		all = append(all, p.Data...)
		if len(p.Data) == 0 || index+pageSize >= p.Total {
			return all, nil
		}
	}
}

func (c *Client) searchTracks(ctx context.Context, q string) ([]apiTrack, error) {
	return search[apiTrack](ctx, c, "/search", q, 10)
}

func (c *Client) searchAlbums(ctx context.Context, q string) ([]apiAlbum, error) {
	return search[apiAlbum](ctx, c, "/search/album", q, 10)
}

func (c *Client) searchArtists(ctx context.Context, q string) ([]apiArtist, error) {
	return search[apiArtist](ctx, c, "/search/artist", q, 5)
}

func (c *Client) track(ctx context.Context, id int64) (apiTrack, error) {
	return getJSON[apiTrack](ctx, c, "/track/"+strconv.FormatInt(id, 10), nil, ttlItem)
}

func (c *Client) album(ctx context.Context, id int64) (apiAlbum, error) {
	return getJSON[apiAlbum](ctx, c, "/album/"+strconv.FormatInt(id, 10), nil, ttlItem)
}

func (c *Client) albumTracks(ctx context.Context, id int64) ([]apiTrack, error) {
	return paginate[apiTrack](ctx, c, "/album/"+strconv.FormatInt(id, 10)+"/tracks", ttlItem)
}

func (c *Client) artistAlbums(ctx context.Context, id int64) ([]apiAlbum, error) {
	return paginate[apiAlbum](ctx, c, "/artist/"+strconv.FormatInt(id, 10)+"/albums", ttlSearch)
}
