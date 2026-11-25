// Package lyrics busca letras no LRCLIB (lrclib.net), uma base pública e sem
// chave que tem letras sincronizadas (LRC).
package lyrics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/DeathHapyness/spotGO/internal/fetch"
	"github.com/DeathHapyness/spotGO/internal/song"
)

var ErrNotFound = errors.New("letra não encontrada")

const (
	defaultBase = "https://lrclib.net"
	ttl         = 30 * 24 * time.Hour
	// O /api/get do LRCLIB aceita ±2 s; na busca, aceitamos um pouco mais.
	maxDrift = 3 * time.Second
)

type Lyrics struct {
	Synced       string
	Plain        string
	Instrumental bool
}

// Text é o que vai na tag: a sincronizada, se houver (os players que leem LRC
// na tag mostram em sincronia, os outros mostram o texto com os tempos).
func (l Lyrics) Text() string {
	if l.Synced != "" {
		return l.Synced
	}
	return l.Plain
}

type Client struct {
	f    *fetch.Client
	base string
}

func New(f *fetch.Client) *Client { return &Client{f: f, base: defaultBase} }

type record struct {
	TrackName    string  `json:"trackName"`
	ArtistName   string  `json:"artistName"`
	AlbumName    string  `json:"albumName"`
	Duration     float64 `json:"duration"`
	Instrumental bool    `json:"instrumental"`
	PlainLyrics  string  `json:"plainLyrics"`
	SyncedLyrics string  `json:"syncedLyrics"`
}

func (r record) lyrics() Lyrics {
	return Lyrics{Synced: strings.TrimSpace(r.SyncedLyrics), Plain: strings.TrimSpace(r.PlainLyrics), Instrumental: r.Instrumental}
}

func (r record) empty() bool {
	return !r.Instrumental && strings.TrimSpace(r.SyncedLyrics) == "" && strings.TrimSpace(r.PlainLyrics) == ""
}

// Get tenta o registro exato (artista, título, álbum, duração) e, se não
// houver, a busca, ficando com o resultado de duração mais próxima.
func (c *Client) Get(ctx context.Context, s song.Song) (Lyrics, error) {
	if s.Title == "" || s.MainArtist() == "" {
		return Lyrics{}, ErrNotFound
	}
	if s.Duration > 0 {
		q := url.Values{
			"artist_name": {s.MainArtist()},
			"track_name":  {s.Title},
			"duration":    {strconv.Itoa(int(math.Round(s.Duration.Seconds())))},
		}
		if s.Album != "" {
			q.Set("album_name", s.Album)
		}
		var r record
		found, err := c.getJSON(ctx, "/api/get?"+q.Encode(), &r)
		if err != nil {
			return Lyrics{}, err
		}
		if found && !r.empty() {
			return r.lyrics(), nil
		}
	}

	title, _, _ := strings.Cut(s.Title, " - ")
	q := url.Values{"artist_name": {s.MainArtist()}, "track_name": {title}}
	var rs []record
	if _, err := c.getJSON(ctx, "/api/search?"+q.Encode(), &rs); err != nil {
		return Lyrics{}, err
	}
	best, bestDrift := -1, time.Duration(math.MaxInt64)
	for i, r := range rs {
		if r.empty() {
			continue
		}
		drift := time.Duration(math.Abs(r.Duration*float64(time.Second) - float64(s.Duration)))
		if s.Duration > 0 && drift > maxDrift {
			continue
		}
		// Empate: a que tem a versão sincronizada.
		if drift < bestDrift || (drift == bestDrift && rs[i].SyncedLyrics != "" && rs[best].SyncedLyrics == "") {
			best, bestDrift = i, drift
		}
	}
	if best < 0 {
		return Lyrics{}, ErrNotFound
	}
	return rs[best].lyrics(), nil
}

// getJSON devolve found=false para 404 (o LRCLIB responde 404 quando não tem).
func (c *Client) getJSON(ctx context.Context, path string, v any) (bool, error) {
	body, err := c.f.Get(ctx, fetch.Request{URL: c.base + path, CacheTTL: ttl})
	var se *fetch.StatusError
	if errors.As(err, &se) && se.Code == 404 {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("LRCLIB: %w", err)
	}
	if err := json.Unmarshal(body, v); err != nil {
		return false, fmt.Errorf("resposta do LRCLIB ilegível: %w", err)
	}
	return true, nil
}
