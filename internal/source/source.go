// Package source resolve um link do Spotify em faixas com metadados completos.
package source

import (
	"context"

	"github.com/DeathHapyness/spotGO/internal/deezer"
	"github.com/DeathHapyness/spotGO/internal/embed"
	"github.com/DeathHapyness/spotGO/internal/song"
)

type Embed interface {
	Track(ctx context.Context, id string) (song.Song, error)
	Album(ctx context.Context, id string) (embed.Collection, error)
	Playlist(ctx context.Context, id string) (embed.Collection, error)
	Artist(ctx context.Context, id string) (embed.Collection, error)
}

type Meta interface {
	EnrichTrack(ctx context.Context, s song.Song) (song.Song, bool, error)
	EnrichAlbum(ctx context.Context, name, artist string, songs []song.Song) ([]song.Song, error)
	Discography(ctx context.Context, artist string, types []string) ([]deezer.AlbumRef, error)
	AlbumSongs(ctx context.Context, id int64) ([]song.Song, error)
}

var (
	_ Embed = (*embed.Client)(nil)
	_ Meta  = (*deezer.Client)(nil)
)

type Result struct {
	Ref    song.Ref
	Name   string
	Artist string
	Songs  []song.Song
	// Unmatched conta as faixas sem ISRC no fim (sem dados do Deezer), incluindo as de Failed.
	Unmatched int
	// Failed conta as consultas ao Deezer que deram erro de rede: faixas, ou
	// álbuns da discografia no caso de um artista.
	Failed    int
	Truncated bool
}

func countUnmatched(songs []song.Song) int {
	n := 0
	for _, s := range songs {
		if s.ISRC == "" {
			n++
		}
	}
	return n
}
