// Package ytmusic busca faixas no YouTube Music pela API innertube, sem
// yt-dlp, e escolhe o melhor vídeo com internal/matching. O fluxo de Find é um
// porte de AudioProvider.search do spotDL.
package ytmusic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/DeathHapyness/spotGO/internal/fetch"
	"github.com/DeathHapyness/spotGO/internal/matching"
	"github.com/DeathHapyness/spotGO/internal/song"
)

var ErrNotFound = errors.New("não encontrada no YouTube Music")

const (
	defaultBase = "https://music.youtube.com"
	searchPath  = "/youtubei/v1/search?prettyPrint=false"
	ttlSearch   = 24 * time.Hour

	// params do ytmusicapi para filter=songs/videos com ignore_spelling.
	paramsSongs  = "EgWKAQIIAUICCAFqDBAOEAoQAxAEEAkQBQ=="
	paramsVideos = "EgWKAQIQAUICCAFqDBAOEAoQAxAEEAkQBQ=="

	// Com nota acima disso, o spotDL para de procurar.
	goodScore = 80

	formatRetry = time.Second
)

type Client struct {
	f    *fetch.Client
	base string
	now  func() time.Time
}

func New(f *fetch.Client) *Client {
	return &Client{f: f, base: defaultBase, now: time.Now}
}

type searchBody struct {
	Context struct {
		Client struct {
			ClientName    string `json:"clientName"`
			ClientVersion string `json:"clientVersion"`
			HL            string `json:"hl"`
		} `json:"client"`
	} `json:"context"`
	Query  string `json:"query"`
	Params string `json:"params,omitempty"`
}

func (c *Client) Search(ctx context.Context, query string, k kind) ([]matching.Result, error) {
	var b searchBody
	b.Context.Client.ClientName = "WEB_REMIX"
	b.Context.Client.ClientVersion = "1." + c.now().UTC().Format("20060102") + ".01.00"
	b.Context.Client.HL = "en"
	b.Query = query
	switch k {
	case kindSongs:
		b.Params = paramsSongs
	case kindVideos:
		b.Params = paramsVideos
	}
	payload, err := json.Marshal(b)
	if err != nil {
		return nil, err
	}
	// O clientVersion muda todo dia; sem ele na chave, o cache dura as 24 h
	// inteiras e a busca do dia seguinte sobrescreve o mesmo arquivo.
	b.Context.Client.ClientVersion = ""
	key, err := json.Marshal(b)
	if err != nil {
		return nil, err
	}

	body, err := c.f.Get(ctx, fetch.Request{
		Method:   http.MethodPost,
		URL:      c.base + searchPath,
		Body:     payload,
		CacheKey: key,
		Header: http.Header{
			"Content-Type": {"application/json"},
			"Origin":       {"https://music.youtube.com"},
		},
		CacheTTL: ttlSearch,
		// O YouTube Music varia o formato de vez em quando; uma nova tentativa costuma resolver.
		Check: func(body []byte) error {
			if _, err := parseSearch(body, k); err != nil {
				return &fetch.RetryAfterError{After: formatRetry, Err: err}
			}
			return nil
		},
	})
	if err != nil {
		return nil, fmt.Errorf("busca no YouTube Music %q: %w", query, err)
	}
	rs, err := parseSearch(body, k)
	if err != nil {
		return nil, err
	}
	if k == kindAll {
		for i := range rs {
			rs[i].ISRCSearch = true
		}
	}
	return rs, nil
}

// Find procura por ISRC, depois em "songs" e por último em "videos", parando
// assim que tiver um resultado confiável.
func (c *Client) Find(ctx context.Context, s song.Song) (matching.Scored, error) {
	isrcURLs := map[string]bool{}
	if s.ISRC != "" {
		rs, err := c.Search(ctx, s.ISRC, kindAll)
		if err != nil {
			return matching.Scored{}, err
		}
		// Na busca por ISRC, a faixa certa às vezes vem sem duração e o matching a
		// descartaria; o ISRC já identifica a gravação, então usamos a do Spotify.
		for i := range rs {
			if rs[i].Verified && rs[i].Duration == 0 {
				rs[i].Duration = s.Duration
			}
		}
		if len(rs) == 1 && rs[0].Verified {
			return matching.Scored{Result: rs[0], Score: 100}, nil
		}
		var top matching.Scored
		for _, sc := range matching.Order(s, rs) {
			if sc.Score > top.Score {
				top = sc
			}
			// Só os aprovados pelo matching: a busca por ISRC às vezes devolve
			// músicas populares sem relação com a faixa.
			isrcURLs[sc.Result.URL] = true
		}
		if top.Score > goodScore {
			return top, nil
		}
	}

	var all []matching.Scored
	for _, k := range []kind{kindSongs, kindVideos} {
		rs, err := c.Search(ctx, matching.Query(s), k)
		if err != nil {
			return matching.Scored{}, err
		}
		for _, r := range rs {
			if isrcURLs[r.URL] {
				return matching.Scored{Result: r, Score: 100}, nil
			}
		}
		scored := matching.Order(s, rs)
		if best, ok := matching.Best(scored); ok && best.Score >= goodScore && best.Result.Verified {
			return best, nil
		}
		all = append(all, scored...)
	}
	if best, ok := matching.Best(all); ok {
		return best, nil
	}
	return matching.Scored{}, ErrNotFound
}
