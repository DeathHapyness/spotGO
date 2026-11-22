// Package song define o modelo de faixa compartilhado por todos os estágios.
package song

import "time"

type Song struct {
	Title       string
	Artists     []string
	Album       string
	AlbumArtist string
	TrackNumber int
	TrackCount  int
	DiscNumber  int
	Duration    time.Duration
	ReleaseDate string
	ISRC        string
	CoverURL    string
	Explicit    bool
	SpotifyID   string
}

func (s Song) MainArtist() string {
	if len(s.Artists) == 0 {
		return ""
	}
	return s.Artists[0]
}

func (s Song) SpotifyURL() string {
	if s.SpotifyID == "" {
		return ""
	}
	return "https://open.spotify.com/track/" + s.SpotifyID
}
