package source

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/DeathHapyness/spotGO/internal/song"
	"github.com/DeathHapyness/spotGO/internal/spotify"
)

type fakeUser struct {
	pl  spotify.Playlist
	err error
}

func (f fakeUser) Playlist(context.Context, string) (spotify.Playlist, error) { return f.pl, f.err }

type fakeNext struct{ refs []song.Ref }

func (f *fakeNext) Resolve(_ context.Context, ref song.Ref) (Result, error) {
	f.refs = append(f.refs, ref)
	return Result{Ref: ref, Name: "do embed"}, nil
}

func TestLoggedInPlaylist(t *testing.T) {
	next, m := &fakeNext{}, &fakeMeta{}
	l := &LoggedIn{Next: next, Meta: m, User: fakeUser{pl: spotify.Playlist{Name: "Minha", Owner: "Eu", Songs: []song.Song{
		{Title: "Com", ISRC: "I1"}, {Title: "Sem", Artists: []string{"A"}},
	}}}}
	r, err := l.Resolve(context.Background(), song.Ref{Kind: song.KindPlaylist, ID: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Name != "Minha" || r.Artist != "Eu" || len(r.Songs) != 2 || len(next.refs) != 0 {
		t.Fatalf("Result = %+v", r)
	}
	if !slices.Equal(m.enriched, []string{"Sem"}) {
		t.Errorf("o Deezer só completa quem não tem ISRC: %v", m.enriched)
	}

	// Faixa, álbum etc. continuam pelo modo público.
	if _, err := l.Resolve(context.Background(), song.Ref{Kind: song.KindAlbum, ID: "a"}); err != nil || len(next.refs) != 1 {
		t.Errorf("álbum: %v %v", err, next.refs)
	}
}

func TestLoggedInFallbacks(t *testing.T) {
	next := &fakeNext{}
	var notes []string
	l := &LoggedIn{Next: next, User: fakeUser{err: spotify.ErrNotOwner}, Notice: func(m string) { notes = append(notes, m) }}
	r, err := l.Resolve(context.Background(), song.Ref{Kind: song.KindPlaylist, ID: "p"})
	if err != nil || r.Name != "do embed" || len(notes) != 1 {
		t.Errorf("playlist de outra pessoa vai pelo embed: %+v %v %v", r, err, notes)
	}
	l.User = fakeUser{err: spotify.ErrLoggedOut}
	if _, err := l.Resolve(context.Background(), song.Ref{Kind: song.KindPlaylist, ID: "p"}); !errors.Is(err, spotify.ErrLoggedOut) {
		t.Errorf("login expirado tem que aparecer: %v", err)
	}
}
