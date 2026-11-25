// Package youtube busca no YouTube comum pela API innertube. É o reserva do
// YouTube Music: acha uploads que não estão no catálogo de músicas.
package youtube

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/DeathHapyness/spotGO/internal/fetch"
	"github.com/DeathHapyness/spotGO/internal/matching"
	"github.com/DeathHapyness/spotGO/internal/provider"
	"github.com/DeathHapyness/spotGO/internal/song"
)

const (
	defaultBase = "https://www.youtube.com"
	searchPath  = "/youtubei/v1/search?prettyPrint=false"
	// Filtro "só vídeos".
	paramsVideos = "EgIQAQ=="
	ttlSearch    = 24 * time.Hour
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
	Params string `json:"params"`
}

type runs struct {
	Runs       []struct{ Text string } `json:"runs"`
	SimpleText string                  `json:"simpleText"`
}

func (r runs) text() string {
	if r.SimpleText != "" {
		return r.SimpleText
	}
	var b strings.Builder
	for _, x := range r.Runs {
		b.WriteString(x.Text)
	}
	return b.String()
}

type video struct {
	VideoID       string `json:"videoId"`
	Title         runs   `json:"title"`
	OwnerText     runs   `json:"ownerText"`
	LengthText    runs   `json:"lengthText"`
	ViewCountText runs   `json:"viewCountText"`
	OwnerBadges   []struct {
		MetadataBadgeRenderer struct {
			Style string `json:"style"`
		} `json:"metadataBadgeRenderer"`
	} `json:"ownerBadges"`
}

type response struct {
	Contents struct {
		TwoColumnSearchResultsRenderer struct {
			PrimaryContents struct {
				SectionListRenderer struct {
					Contents []struct {
						ItemSectionRenderer *struct {
							Contents []struct {
								VideoRenderer *video `json:"videoRenderer"`
							} `json:"contents"`
						} `json:"itemSectionRenderer"`
					} `json:"contents"`
				} `json:"sectionListRenderer"`
			} `json:"primaryContents"`
		} `json:"twoColumnSearchResultsRenderer"`
	} `json:"contents"`
}

func parse(body []byte) ([]matching.Result, error) {
	var r response
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("resposta do YouTube ilegível: %w", err)
	}
	secs := r.Contents.TwoColumnSearchResultsRenderer.PrimaryContents.SectionListRenderer.Contents
	if len(secs) == 0 {
		return nil, fmt.Errorf("resposta do YouTube sem resultados de busca (o formato mudou?)")
	}
	var out []matching.Result
	for _, s := range secs {
		if s.ItemSectionRenderer == nil {
			continue
		}
		for _, it := range s.ItemSectionRenderer.Contents {
			v := it.VideoRenderer
			// Sem duração é transmissão ao vivo ou estreia.
			if v == nil || v.VideoID == "" || v.LengthText.text() == "" {
				continue
			}
			d, ok := parseDuration(v.LengthText.text())
			if !ok {
				continue
			}
			out = append(out, matching.Result{
				Source:   "youtube",
				ID:       v.VideoID,
				URL:      "https://www.youtube.com/watch?v=" + v.VideoID,
				Name:     v.Title.text(),
				Author:   v.OwnerText.text(),
				Duration: d,
				Views:    parseViews(v.ViewCountText.text()),
			})
		}
	}
	return out, nil
}

func (c *Client) Search(ctx context.Context, query string) ([]matching.Result, error) {
	var b searchBody
	b.Context.Client.ClientName = "WEB"
	b.Context.Client.ClientVersion = "2." + c.now().UTC().Format("20060102") + ".00.00"
	b.Context.Client.HL = "en"
	b.Query, b.Params = query, paramsVideos
	payload, err := json.Marshal(b)
	if err != nil {
		return nil, err
	}
	b.Context.Client.ClientVersion = ""
	key, _ := json.Marshal(b)
	body, err := c.f.Get(ctx, fetch.Request{
		Method: http.MethodPost, URL: c.base + searchPath, Body: payload, CacheKey: key,
		Header:   http.Header{"Content-Type": {"application/json"}},
		CacheTTL: ttlSearch,
		Check: func(body []byte) error {
			_, err := parse(body)
			return err
		},
	})
	if err != nil {
		return nil, fmt.Errorf("busca no YouTube %q: %w", query, err)
	}
	return parse(body)
}

func (c *Client) Find(ctx context.Context, s song.Song) (matching.Scored, error) {
	rs, err := c.Search(ctx, matching.Query(s))
	if err != nil {
		return matching.Scored{}, err
	}
	return provider.Pick(s, rs, "YouTube")
}

func parseDuration(s string) (time.Duration, bool) {
	var secs int
	for _, p := range strings.Split(s, ":") {
		n, err := strconv.Atoi(p)
		if err != nil {
			return 0, false
		}
		secs = secs*60 + n
	}
	return time.Duration(secs) * time.Second, secs > 0
}

// parseViews lê "166,174,637 views"; 0 é desconhecido.
func parseViews(s string) int64 {
	first, _, _ := strings.Cut(s, " ")
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, first)
	n, _ := strconv.ParseInt(digits, 10, 64)
	return n
}
