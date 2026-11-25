// Package bandcamp busca faixas na API pública de busca do Bandcamp.
package bandcamp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/DeathHapyness/spotGO/internal/fetch"
	"github.com/DeathHapyness/spotGO/internal/matching"
	"github.com/DeathHapyness/spotGO/internal/provider"
	"github.com/DeathHapyness/spotGO/internal/song"
)

const (
	defaultBase = "https://bandcamp.com"
	searchPath  = "/api/bcsearch_public_api/1/autocomplete_elastic"
	ttlSearch   = 7 * 24 * time.Hour
)

type Client struct {
	f    *fetch.Client
	base string
}

func New(f *fetch.Client) *Client { return &Client{f: f, base: defaultBase} }

type item struct {
	Type      string `json:"type"`
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	BandName  string `json:"band_name"`
	AlbumName string `json:"album_name"`
	URL       string `json:"item_url_path"`
}

func parse(body []byte) ([]item, error) {
	var r struct {
		Auto *struct {
			Results []item `json:"results"`
		} `json:"auto"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("resposta do Bandcamp ilegível: %w", err)
	}
	if r.Auto == nil {
		return nil, fmt.Errorf("resposta do Bandcamp sem resultados (o formato mudou?)")
	}
	return r.Auto.Results, nil
}

func (c *Client) Search(ctx context.Context, query string) ([]item, error) {
	payload, err := json.Marshal(map[string]any{
		"search_text": query, "search_filter": "t", "full_page": false, "fan_id": nil,
	})
	if err != nil {
		return nil, err
	}
	body, err := c.f.Get(ctx, fetch.Request{
		Method: http.MethodPost, URL: c.base + searchPath, Body: payload,
		Header:   http.Header{"Content-Type": {"application/json"}},
		CacheTTL: ttlSearch,
		Check: func(b []byte) error {
			_, err := parse(b)
			return err
		},
	})
	if err != nil {
		return nil, fmt.Errorf("busca no Bandcamp %q: %w", query, err)
	}
	return parse(body)
}

func (c *Client) Find(ctx context.Context, s song.Song) (matching.Scored, error) {
	items, err := c.Search(ctx, provider.SimpleQuery(s))
	if err != nil {
		return matching.Scored{}, err
	}
	var rs []matching.Result
	for _, it := range items {
		if it.Type != "t" || it.URL == "" {
			continue
		}
		rs = append(rs, matching.Result{
			Source: "bandcamp",
			ID:     fmt.Sprint(it.ID),
			URL:    it.URL,
			Name:   it.Name,
			// A página é do próprio artista ou selo: o nome da banda é confiável.
			Artists:  []string{it.BandName},
			Author:   it.BandName,
			Album:    it.AlbumName,
			Verified: true,
			// A busca não traz a duração; sem ela o matching descartaria tudo.
			Duration: s.Duration,
		})
	}
	return provider.Pick(s, rs, "Bandcamp")
}
