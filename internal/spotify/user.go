package spotify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/DeathHapyness/spotGO/internal/fetch"
	"github.com/DeathHapyness/spotGO/internal/song"
)

const APIBase = "https://api.spotify.com/v1"

var (
	// ErrNotOwner: a API só lista as faixas de playlists que são suas ou em que você colabora.
	ErrNotOwner = errors.New("a playlist não é sua")
	// ErrLoggedOut: o refresh token foi revogado ou é de outro app.
	ErrLoggedOut = errors.New("o login do Spotify expirou; rode spotgo login de novo")
	// ErrNotAllowed: em modo de desenvolvimento, só as contas cadastradas no app podem usá-lo.
	ErrNotAllowed = errors.New("esta conta não está cadastrada no seu app do Spotify; adicione-a em User Management no dashboard")
)

// User é o cliente da Web API com o login do usuário (refresh token).
type User struct {
	Fetch    *fetch.Client
	HTTP     *http.Client
	APIBase  string
	TokenURL string
	ClientID string
	// OnRefresh grava o refresh token novo quando o Spotify troca o antigo.
	OnRefresh func(refresh string) error

	mu      sync.Mutex
	refresh string
	token   Token
}

func NewUser(f *fetch.Client, clientID, refresh string) *User {
	return &User{Fetch: f, HTTP: &http.Client{Timeout: 15 * time.Second}, APIBase: APIBase, TokenURL: TokenURL, ClientID: clientID, refresh: refresh}
}

func (u *User) accessToken(ctx context.Context, force bool) (string, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if !force && u.token.AccessToken != "" && time.Until(u.token.ExpiresAt) > time.Minute {
		return u.token.AccessToken, nil
	}
	tok, next, err := Refresh(ctx, u.HTTP, u.TokenURL, u.ClientID, u.refresh)
	if errors.Is(err, ErrInvalidCredentials) {
		return "", fmt.Errorf("%w (%v)", ErrLoggedOut, err)
	}
	if err != nil {
		return "", err
	}
	u.token = tok
	if next != "" && next != u.refresh {
		u.refresh = next
		if u.OnRefresh != nil {
			if err := u.OnRefresh(next); err != nil {
				return "", fmt.Errorf("gravando o login renovado: %w", err)
			}
		}
	}
	return tok.AccessToken, nil
}

// get chama a API; um 401 renova o token e tenta mais uma vez.
func (u *User) get(ctx context.Context, rawURL string, v any) error {
	for attempt := 0; ; attempt++ {
		tok, err := u.accessToken(ctx, attempt > 0)
		if err != nil {
			return err
		}
		body, err := u.Fetch.Get(ctx, fetch.Request{URL: rawURL, Header: http.Header{"Authorization": {"Bearer " + tok}}})
		var se *fetch.StatusError
		if errors.As(err, &se) && se.Code == http.StatusUnauthorized && attempt == 0 {
			continue
		}
		if err != nil {
			return err
		}
		if err := json.Unmarshal(body, v); err != nil {
			return fmt.Errorf("resposta inesperada da API do Spotify: %w", err)
		}
		return nil
	}
}

type Playlist struct {
	Name  string
	Owner string
	Songs []song.Song
	// Skipped conta episódios e arquivos locais, que não são faixas do catálogo.
	Skipped int
}

type apiImage struct {
	URL   string `json:"url"`
	Width int    `json:"width"`
}

type apiTrack struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Name       string `json:"name"`
	DurationMS int64  `json:"duration_ms"`
	Explicit   bool   `json:"explicit"`
	TrackNum   int    `json:"track_number"`
	DiscNum    int    `json:"disc_number"`
	IsLocal    bool   `json:"is_local"`
	Artists    []struct {
		Name string `json:"name"`
	} `json:"artists"`
	Album struct {
		Name        string `json:"name"`
		ReleaseDate string `json:"release_date"`
		TotalTracks int    `json:"total_tracks"`
		Artists     []struct {
			Name string `json:"name"`
		} `json:"artists"`
		Images []apiImage `json:"images"`
	} `json:"album"`
	ExternalIDs struct {
		ISRC string `json:"isrc"`
	} `json:"external_ids"`
}

type itemsPage struct {
	Items []struct {
		// Desde 2026 o campo se chama "item"; "track" é o nome antigo.
		Item  *apiTrack `json:"item"`
		Track *apiTrack `json:"track"`
	} `json:"items"`
	Next string `json:"next"`
}

// Playlist lê todas as faixas de uma playlist sua (sem o limite de 100 do embed).
func (u *User) Playlist(ctx context.Context, id string) (Playlist, error) {
	base := strings.TrimRight(u.APIBase, "/") + "/playlists/" + url.PathEscape(id)
	var p Playlist
	next := base + "/items?limit=50&additional_types=track"
	for next != "" {
		var page itemsPage
		if err := u.get(ctx, next, &page); err != nil {
			var se *fetch.StatusError
			if errors.As(err, &se) && se.Code == http.StatusForbidden && strings.Contains(strings.ToLower(se.Body), "registered") {
				return Playlist{}, ErrNotAllowed
			}
			if errors.As(err, &se) && (se.Code == http.StatusForbidden || se.Code == http.StatusNotFound) {
				return Playlist{}, ErrNotOwner
			}
			return Playlist{}, err
		}
		for _, it := range page.Items {
			t := it.Item
			if t == nil {
				t = it.Track
			}
			if t == nil || t.ID == "" || t.IsLocal || (t.Type != "" && t.Type != "track") {
				p.Skipped++
				continue
			}
			p.Songs = append(p.Songs, t.song())
		}
		next = page.Next
	}

	var meta struct {
		Name  string `json:"name"`
		Owner struct {
			DisplayName string `json:"display_name"`
		} `json:"owner"`
	}
	if err := u.get(ctx, base+"?fields=name,owner(display_name)", &meta); err != nil {
		return Playlist{}, err
	}
	p.Name, p.Owner = meta.Name, meta.Owner.DisplayName
	return p, nil
}

func (t *apiTrack) song() song.Song {
	s := song.Song{
		Title:       t.Name,
		Album:       t.Album.Name,
		TrackNumber: t.TrackNum,
		TrackCount:  t.Album.TotalTracks,
		DiscNumber:  t.DiscNum,
		Duration:    time.Duration(t.DurationMS) * time.Millisecond,
		ReleaseDate: t.Album.ReleaseDate,
		ISRC:        t.ExternalIDs.ISRC,
		Explicit:    t.Explicit,
		SpotifyID:   t.ID,
	}
	for _, a := range t.Artists {
		s.Artists = append(s.Artists, a.Name)
	}
	if len(t.Album.Artists) > 0 {
		s.AlbumArtist = t.Album.Artists[0].Name
	}
	best := 0
	for _, im := range t.Album.Images {
		if im.Width >= best || s.CoverURL == "" {
			best, s.CoverURL = im.Width, im.URL
		}
	}
	return s
}
