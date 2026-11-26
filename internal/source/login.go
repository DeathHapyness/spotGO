package source

import (
	"context"
	"errors"
	"strings"

	"github.com/DeathHapyness/spotGO/internal/song"
	"github.com/DeathHapyness/spotGO/internal/spotify"
)

type Playlists interface {
	Playlist(ctx context.Context, id string) (spotify.Playlist, error)
}

type Resolver interface {
	Resolve(ctx context.Context, ref song.Ref) (Result, error)
}

// LoggedIn lê pela API as playlists da conta logada (todas as faixas, com os
// metadados do Spotify) e deixa o resto com Next (o modo público).
type LoggedIn struct {
	Next Resolver
	User Playlists
	// Meta completa o ISRC quando a API não o devolve.
	Meta Meta
	// Notice recebe avisos, como "a playlist não é sua; usando o embed".
	Notice func(msg string)
}

func (l *LoggedIn) Resolve(ctx context.Context, ref song.Ref) (Result, error) {
	if ref.Kind != song.KindPlaylist || l.User == nil {
		return l.Next.Resolve(ctx, ref)
	}
	pl, err := l.User.Playlist(ctx, ref.ID)
	switch {
	case errors.Is(err, spotify.ErrNotOwner):
		l.notice("a playlist não é sua nem colaborativa; a API não lista as faixas dela, então vai pelo embed (até 100 faixas)")
		return l.Next.Resolve(ctx, ref)
	case err != nil:
		return Result{}, err
	}
	r := Result{Ref: ref, Name: pl.Name, Artist: pl.Owner, Songs: pl.Songs}
	for i := range r.Songs {
		s := &r.Songs[i]
		if s.ISRC != "" || l.Meta == nil {
			continue
		}
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		found, ok, err := l.Meta.EnrichTrack(ctx, *s)
		switch {
		case err != nil && ctx.Err() != nil:
			return Result{}, ctx.Err()
		case err != nil:
			r.Failed++
		case ok:
			s.ISRC = found.ISRC
		}
	}
	r.Unmatched = countUnmatched(r.Songs)
	return r, nil
}

func (l *LoggedIn) notice(msg string) {
	if l.Notice != nil {
		l.Notice(strings.TrimSpace(msg))
	}
}
