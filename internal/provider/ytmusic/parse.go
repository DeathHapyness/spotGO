package ytmusic

import (
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/DeathHapyness/spotGO/internal/matching"
)

type kind int

const (
	kindSongs kind = iota
	kindVideos
	kindAll // busca sem filtro (ISRC)
)

const sourceName = "youtube-music"

var errFormat = errors.New("a resposta do YouTube Music mudou de formato")

type run struct {
	Text               string `json:"text"`
	NavigationEndpoint *struct {
		BrowseEndpoint *struct {
			BrowseID string `json:"browseId"`
		} `json:"browseEndpoint"`
		WatchEndpoint *struct {
			VideoID string `json:"videoId"`
		} `json:"watchEndpoint"`
	} `json:"navigationEndpoint"`
}

type text struct {
	Runs []run `json:"runs"`
}

type badge struct {
	MusicInlineBadgeRenderer struct {
		Icon struct {
			IconType string `json:"iconType"`
		} `json:"icon"`
	} `json:"musicInlineBadgeRenderer"`
}

type listItem struct {
	FlexColumns []struct {
		Renderer struct {
			Text text `json:"text"`
		} `json:"musicResponsiveListItemFlexColumnRenderer"`
	} `json:"flexColumns"`
	PlaylistItemData struct {
		VideoID string `json:"videoId"`
	} `json:"playlistItemData"`
	Badges []badge `json:"badges"`
}

type card struct {
	Title          text    `json:"title"`
	Subtitle       text    `json:"subtitle"`
	SubtitleBadges []badge `json:"subtitleBadges"`
}

type shelfItem struct {
	Item *listItem `json:"musicResponsiveListItemRenderer"`
}

type section struct {
	Shelf *struct {
		Contents []shelfItem `json:"contents"`
	} `json:"musicShelfRenderer"`
	ItemSection *struct {
		Contents []shelfItem `json:"contents"`
	} `json:"itemSectionRenderer"`
	Card *card `json:"musicCardShelfRenderer"`
}

type response struct {
	Contents *struct {
		Tabbed *struct {
			Tabs []struct {
				TabRenderer struct {
					Content struct {
						SectionList *struct {
							Contents []section `json:"contents"`
						} `json:"sectionListRenderer"`
					} `json:"content"`
				} `json:"tabRenderer"`
			} `json:"tabs"`
		} `json:"tabbedSearchResultsRenderer"`
	} `json:"contents"`
}

func parseSearch(body []byte, k kind) ([]matching.Result, error) {
	var resp response
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, errFormat
	}
	if resp.Contents == nil || resp.Contents.Tabbed == nil || len(resp.Contents.Tabbed.Tabs) == 0 {
		return nil, errFormat
	}
	sl := resp.Contents.Tabbed.Tabs[0].TabRenderer.Content.SectionList
	if sl == nil {
		return nil, errFormat
	}

	var out []matching.Result
	add := func(r matching.Result, ok bool) {
		if ok {
			out = append(out, r)
		}
	}
	for _, sec := range sl.Contents {
		if sec.Card != nil && k == kindAll {
			add(parseCard(sec.Card))
		}
		var items []shelfItem
		if sec.Shelf != nil {
			items = sec.Shelf.Contents
		} else if sec.ItemSection != nil && k == kindAll {
			items = sec.ItemSection.Contents
		}
		for _, it := range items {
			if it.Item != nil {
				add(parseItem(it.Item, k))
			}
		}
	}
	return out, nil
}

func parseItem(it *listItem, k kind) (matching.Result, bool) {
	id := it.PlaylistItemData.VideoID
	if id == "" || len(it.FlexColumns) < 2 {
		return matching.Result{}, false
	}
	titleRuns := it.FlexColumns[0].Renderer.Text.Runs
	if len(titleRuns) == 0 {
		return matching.Result{}, false
	}
	runs := it.FlexColumns[1].Renderer.Text.Runs

	verified := k == kindSongs
	if k == kindAll {
		var ok bool
		if verified, ok = resultType(runs); !ok {
			return matching.Result{}, false
		}
		runs = runs[min(2, len(runs)):]
	}

	r := newResult(id, titleRuns[0].Text, verified, explicit(it.Badges))
	fillRuns(&r, runs)
	if len(it.FlexColumns) > 2 && r.Views == 0 {
		if rs := it.FlexColumns[2].Renderer.Text.Runs; len(rs) > 0 {
			r.Views, _ = parseViews(rs[0].Text)
		}
	}
	return r, len(r.Artists) > 0
}

func parseCard(c *card) (matching.Result, bool) {
	if len(c.Title.Runs) == 0 {
		return matching.Result{}, false
	}
	t := c.Title.Runs[0]
	if t.NavigationEndpoint == nil || t.NavigationEndpoint.WatchEndpoint == nil || t.NavigationEndpoint.WatchEndpoint.VideoID == "" {
		return matching.Result{}, false
	}
	verified, ok := resultType(c.Subtitle.Runs)
	if !ok {
		return matching.Result{}, false
	}
	r := newResult(t.NavigationEndpoint.WatchEndpoint.VideoID, t.Text, verified, explicit(c.SubtitleBadges))
	fillRuns(&r, c.Subtitle.Runs[min(2, len(c.Subtitle.Runs)):])
	return r, len(r.Artists) > 0
}

// resultType lê o 1º run de uma busca sem filtro: true = "Song", false = "Video".
func resultType(runs []run) (verified, ok bool) {
	if len(runs) == 0 {
		return false, false
	}
	switch runs[0].Text {
	case "Song":
		return true, true
	case "Video":
		return false, true
	}
	return false, false
}

func newResult(id, name string, verified, explicit bool) matching.Result {
	host := "www"
	if verified {
		host = "music"
	}
	return matching.Result{
		Source:   sourceName,
		ID:       id,
		URL:      "https://" + host + ".youtube.com/watch?v=" + id,
		Name:     name,
		Verified: verified,
		Explicit: &explicit,
	}
}

func explicit(bs []badge) bool {
	for _, b := range bs {
		if b.MusicInlineBadgeRenderer.Icon.IconType == "MUSIC_EXPLICIT_BADGE" {
			return true
		}
	}
	return false
}

var (
	viewsLike    = regexp.MustCompile(`^\d[^ ]* [^ ]*$`)
	durationLike = regexp.MustCompile(`^(\d+:)*\d+:\d+$`)
	yearLike     = regexp.MustCompile(`^\d{4}$`)
	viewsRe      = regexp.MustCompile(`^(\d[\d,]*(?:\.\d+)?)([KMB]?) (?:plays?|views?)$`)
)

// fillRuns é o parse_song_runs do ytmusicapi: os runs ímpares são separadores
// (" • ", ", ", " & "). O run 0 nunca é lido como views: um artista sem link
// pode se chamar "50 Cent".
func fillRuns(r *matching.Result, runs []run) {
	for i := 0; i < len(runs); i += 2 {
		t := runs[i].Text
		if nav := runs[i].NavigationEndpoint; nav != nil {
			if nav.BrowseEndpoint != nil && strings.HasPrefix(nav.BrowseEndpoint.BrowseID, "MPRE") {
				r.Album = t
			} else {
				r.Artists = append(r.Artists, t)
			}
			continue
		}
		switch {
		case viewsLike.MatchString(t) && i > 0:
			r.Views, _ = parseViews(t)
		case durationLike.MatchString(t):
			r.Duration, _ = parseDuration(t)
		case yearLike.MatchString(t):
		default:
			r.Artists = append(r.Artists, t)
		}
	}
	if len(r.Artists) > 0 {
		r.Author = r.Artists[0]
	}
}

func parseViews(s string) (int64, bool) {
	m := viewsRe.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	n, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", ""), 64)
	if err != nil {
		return 0, false
	}
	switch m[2] {
	case "K":
		n *= 1e3
	case "M":
		n *= 1e6
	case "B":
		n *= 1e9
	}
	return int64(math.Round(n)), true
}

func parseDuration(s string) (time.Duration, bool) {
	if !durationLike.MatchString(s) {
		return 0, false
	}
	var secs int
	for _, p := range strings.Split(s, ":") {
		n, _ := strconv.Atoi(p)
		secs = secs*60 + n
	}
	return time.Duration(secs) * time.Second, true
}
