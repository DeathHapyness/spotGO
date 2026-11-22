package embed

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/DeathHapyness/spotGO/internal/fetch"
	"github.com/DeathHapyness/spotGO/internal/song"
)

var ErrNotFound = errors.New("não encontrado no Spotify")

const defaultBase = "https://open.spotify.com/embed"

var cacheTTL = map[song.Kind]time.Duration{
	song.KindTrack:    30 * 24 * time.Hour,
	song.KindAlbum:    30 * 24 * time.Hour,
	song.KindArtist:   24 * time.Hour,
	song.KindPlaylist: 10 * time.Minute,
}

type Client struct {
	f    *fetch.Client
	base string
}

func New(f *fetch.Client) *Client {
	return &Client{f: f, base: defaultBase}
}

func (c *Client) Track(ctx context.Context, id string) (song.Song, error) {
	page, err := c.page(ctx, song.KindTrack, id)
	if err != nil {
		return song.Song{}, err
	}
	s, err := parseTrack(page)
	if err != nil {
		return song.Song{}, fmt.Errorf("track %s: %w", id, err)
	}
	return s, nil
}

func (c *Client) Album(ctx context.Context, id string) (Collection, error) {
	return c.collection(ctx, song.KindAlbum, id)
}

func (c *Client) Playlist(ctx context.Context, id string) (Collection, error) {
	return c.collection(ctx, song.KindPlaylist, id)
}

func (c *Client) Artist(ctx context.Context, id string) (Collection, error) {
	return c.collection(ctx, song.KindArtist, id)
}

func (c *Client) collection(ctx context.Context, kind song.Kind, id string) (Collection, error) {
	page, err := c.page(ctx, kind, id)
	if err != nil {
		return Collection{}, err
	}
	col, err := parseCollection(page, kind)
	if err != nil {
		return Collection{}, fmt.Errorf("%s %s: %w", kind, id, err)
	}
	return col, nil
}

func (c *Client) page(ctx context.Context, kind song.Kind, id string) ([]byte, error) {
	body, err := c.f.Get(ctx, fetch.Request{
		URL:      fmt.Sprintf("%s/%s/%s", c.base, kind, id),
		CacheTTL: cacheTTL[kind],
	})
	var se *fetch.StatusError
	if errors.As(err, &se) && se.Code == http.StatusNotFound {
		return nil, fmt.Errorf("%w: https://open.spotify.com/%s/%s", ErrNotFound, kind, id)
	}
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", kind, id, err)
	}
	return body, nil
}
