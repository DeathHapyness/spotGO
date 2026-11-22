// Package embed lê a página pública de embed do Spotify (open.spotify.com/embed).
package embed

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/DeathHapyness/spotGO/internal/song"
)

var ErrFormatChanged = errors.New("o formato da página de embed do Spotify mudou; atualize o spotgo")

const PlaylistLimit = 100

type Collection struct {
	Kind      song.Kind
	ID        string
	Name      string
	Artist    string
	CoverURL  string
	Songs     []song.Song
	Truncated bool
	Skipped   int
}

var (
	scriptOpen  = []byte(`<script id="__NEXT_DATA__" type="application/json">`)
	scriptClose = []byte(`</script>`)
)

// O embed separa artistas com vírgula + NBSP; nomes como "Tyler, The Creator"
// usam vírgula + espaço comum e não podem ser quebrados.
const artistSep = ", "

type page struct {
	Props struct {
		PageProps struct {
			State struct {
				Data struct {
					Entity *entity `json:"entity"`
				} `json:"data"`
			} `json:"state"`
		} `json:"pageProps"`
	} `json:"props"`
}

type entity struct {
	URI      string `json:"uri"`
	Name     string `json:"name"`
	Subtitle string `json:"subtitle"`
	Duration int64  `json:"duration"`
	Explicit bool   `json:"isExplicit"`
	Artists  []struct {
		Name string `json:"name"`
	} `json:"artists"`
	ReleaseDate *struct {
		ISOString string `json:"isoString"`
	} `json:"releaseDate"`
	VisualIdentity struct {
		Image []image `json:"image"`
	} `json:"visualIdentity"`
	TrackList []trackItem `json:"trackList"`
}

type image struct {
	URL      string `json:"url"`
	MaxWidth int    `json:"maxWidth"`
}

type trackItem struct {
	URI        string `json:"uri"`
	Title      string `json:"title"`
	Subtitle   string `json:"subtitle"`
	Duration   int64  `json:"duration"`
	Explicit   bool   `json:"isExplicit"`
	IsPlayable *bool  `json:"isPlayable"`
	EntityType string `json:"entityType"`
}

func parseEntity(raw []byte) (*entity, error) {
	start := bytes.Index(raw, scriptOpen)
	if start < 0 {
		return nil, fmt.Errorf("%w (sem __NEXT_DATA__)", ErrFormatChanged)
	}
	rest := raw[start+len(scriptOpen):]
	end := bytes.Index(rest, scriptClose)
	if end < 0 {
		return nil, fmt.Errorf("%w (__NEXT_DATA__ sem fechamento)", ErrFormatChanged)
	}

	var p page
	if err := json.Unmarshal(rest[:end], &p); err != nil {
		return nil, fmt.Errorf("%w (%v)", ErrFormatChanged, err)
	}
	e := p.Props.PageProps.State.Data.Entity
	if e == nil || e.URI == "" {
		return nil, fmt.Errorf("%w (sem entity)", ErrFormatChanged)
	}
	return e, nil
}

func parseTrack(raw []byte) (song.Song, error) {
	e, err := parseEntity(raw)
	if err != nil {
		return song.Song{}, err
	}
	s := song.Song{
		Title:     e.Name,
		Duration:  time.Duration(e.Duration) * time.Millisecond,
		Explicit:  e.Explicit,
		CoverURL:  bestImage(e.VisualIdentity.Image),
		SpotifyID: idFromURI(e.URI),
	}
	for _, a := range e.Artists {
		s.Artists = append(s.Artists, a.Name)
	}
	if e.ReleaseDate != nil && len(e.ReleaseDate.ISOString) >= 10 {
		s.ReleaseDate = e.ReleaseDate.ISOString[:10]
	}
	return s, nil
}

func parseCollection(raw []byte, kind song.Kind) (Collection, error) {
	e, err := parseEntity(raw)
	if err != nil {
		return Collection{}, err
	}
	c := Collection{
		Kind:     kind,
		ID:       idFromURI(e.URI),
		Name:     e.Name,
		Artist:   e.Subtitle,
		CoverURL: bestImage(e.VisualIdentity.Image),
	}

	for _, t := range e.TrackList {
		if !isTrack(t) {
			c.Skipped++
			continue
		}
		c.Songs = append(c.Songs, song.Song{
			Title:     t.Title,
			Artists:   splitArtists(t.Subtitle),
			Duration:  time.Duration(t.Duration) * time.Millisecond,
			Explicit:  t.Explicit,
			SpotifyID: idFromURI(t.URI),
		})
	}

	switch kind {
	case song.KindAlbum:
		for i := range c.Songs {
			s := &c.Songs[i]
			s.Album, s.AlbumArtist, s.CoverURL = c.Name, c.Artist, c.CoverURL
			s.TrackNumber, s.TrackCount = i+1, len(c.Songs)
		}
	case song.KindPlaylist:
		c.Truncated = len(e.TrackList) >= PlaylistLimit
	}
	return c, nil
}

func isTrack(t trackItem) bool {
	if t.EntityType != "" && t.EntityType != "track" {
		return false
	}
	if !strings.HasPrefix(t.URI, "spotify:track:") {
		return false
	}
	return t.IsPlayable == nil || *t.IsPlayable
}

func splitArtists(s string) []string {
	var out []string
	for _, part := range strings.Split(s, artistSep) {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func bestImage(imgs []image) string {
	best := image{}
	for _, img := range imgs {
		if img.URL != "" && img.MaxWidth >= best.MaxWidth {
			best = img
		}
	}
	return best.URL
}

func idFromURI(uri string) string {
	if i := strings.LastIndexByte(uri, ':'); i >= 0 {
		return uri[i+1:]
	}
	return uri
}
