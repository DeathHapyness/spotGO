package source

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/DeathHapyness/spotGO/internal/deezer"
	"github.com/DeathHapyness/spotGO/internal/embed"
	"github.com/DeathHapyness/spotGO/internal/song"
)

type fakeEmbed struct {
	track song.Song
	cols  map[song.Kind]embed.Collection
	err   error
}

func (f *fakeEmbed) Track(context.Context, string) (song.Song, error) { return f.track, f.err }
func (f *fakeEmbed) Album(_ context.Context, _ string) (embed.Collection, error) {
	return f.cols[song.KindAlbum], f.err
}
func (f *fakeEmbed) Playlist(_ context.Context, _ string) (embed.Collection, error) {
	return f.cols[song.KindPlaylist], f.err
}
func (f *fakeEmbed) Artist(_ context.Context, _ string) (embed.Collection, error) {
	return f.cols[song.KindArtist], f.err
}

// fakeMeta: títulos com "?" não casam; com "!" dão erro de rede.
type fakeMeta struct {
	enriched   []string
	onEnrich   func(ctx context.Context, n int)
	album      func([]song.Song) ([]song.Song, error)
	disco      []deezer.AlbumRef
	discoErr   error
	albumSongs map[int64][]song.Song
	albumErr   map[int64]error
}

func (f *fakeMeta) EnrichTrack(ctx context.Context, s song.Song) (song.Song, bool, error) {
	f.enriched = append(f.enriched, s.Title)
	if f.onEnrich != nil {
		f.onEnrich(ctx, len(f.enriched))
	}
	if err := ctx.Err(); err != nil {
		return s, false, err
	}
	switch {
	case strings.HasPrefix(s.Title, "?"):
		return s, false, nil
	case strings.HasPrefix(s.Title, "!"):
		return s, false, errors.New("conexão recusada")
	}
	s.ISRC = "ISRC-" + s.Title
	return s, true, nil
}

func (f *fakeMeta) EnrichAlbum(_ context.Context, _, _ string, songs []song.Song) ([]song.Song, error) {
	if f.album == nil {
		return songs, nil
	}
	return f.album(songs)
}

func (f *fakeMeta) Discography(context.Context, string, []string) ([]deezer.AlbumRef, error) {
	return f.disco, f.discoErr
}

func (f *fakeMeta) AlbumSongs(_ context.Context, id int64) ([]song.Song, error) {
	return f.albumSongs[id], f.albumErr[id]
}

func titles(n ...string) []song.Song {
	out := make([]song.Song, len(n))
	for i, t := range n {
		out[i] = song.Song{Title: t, Artists: []string{"Artista"}, SpotifyID: fmt.Sprint("id", i)}
	}
	return out
}

func TestTrack(t *testing.T) {
	e := &fakeEmbed{track: song.Song{Title: "A", Artists: []string{"X", "Y"}}}
	r, err := NewPublic(e, &fakeMeta{}).Resolve(context.Background(), song.Ref{Kind: song.KindTrack, ID: "t"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Name != "A" || r.Artist != "X, Y" || len(r.Songs) != 1 || r.Songs[0].ISRC != "ISRC-A" || r.Unmatched != 0 {
		t.Fatalf("Result = %+v", r)
	}
	if r.Ref.Kind != song.KindTrack || r.Ref.ID != "t" {
		t.Fatalf("Ref = %+v", r.Ref)
	}
}

func TestPlaylistMixed(t *testing.T) {
	src := titles("A", "?B", "!C", "D")
	e := &fakeEmbed{cols: map[song.Kind]embed.Collection{song.KindPlaylist: {Name: "Mix", Songs: src}}}
	p := NewPublic(e, &fakeMeta{})
	var progress []string
	p.Progress = func(done, total int) { progress = append(progress, fmt.Sprintf("%d/%d", done, total)) }

	r, err := p.Resolve(context.Background(), song.Ref{Kind: song.KindPlaylist, ID: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Songs) != 4 || r.Name != "Mix" {
		t.Fatalf("Result = %+v", r)
	}
	for i, want := range []string{"A", "?B", "!C", "D"} {
		if r.Songs[i].Title != want {
			t.Errorf("[%d] = %q, quero %q", i, r.Songs[i].Title, want)
		}
	}
	for _, i := range []int{1, 2} {
		if !reflect.DeepEqual(r.Songs[i], src[i]) {
			t.Errorf("[%d] alterada: %+v", i, r.Songs[i])
		}
	}
	if r.Unmatched != 2 || r.Failed != 1 {
		t.Fatalf("Unmatched=%d Failed=%d, quero 2 e 1", r.Unmatched, r.Failed)
	}
	if strings.Join(progress, ",") != "1/4,2/4,3/4,4/4" {
		t.Fatalf("progress = %v", progress)
	}
}

func TestPlaylistTruncated(t *testing.T) {
	e := &fakeEmbed{cols: map[song.Kind]embed.Collection{song.KindPlaylist: {Songs: titles("A"), Truncated: true}}}
	r, err := NewPublic(e, &fakeMeta{}).Resolve(context.Background(), song.Ref{Kind: song.KindPlaylist, ID: "p"})
	if err != nil || !r.Truncated {
		t.Fatalf("Truncated = %v, err = %v", r.Truncated, err)
	}
}

func TestPlaylistCanceled(t *testing.T) {
	e := &fakeEmbed{cols: map[song.Kind]embed.Collection{song.KindPlaylist: {Songs: titles("A", "B", "C", "D")}}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := &fakeMeta{onEnrich: func(_ context.Context, n int) {
		if n == 2 {
			cancel()
		}
	}}

	_, err := NewPublic(e, m).Resolve(ctx, song.Ref{Kind: song.KindPlaylist, ID: "p"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, quero context.Canceled", err)
	}
	if len(m.enriched) > 2 {
		t.Fatalf("continuou depois do cancelamento: %v", m.enriched)
	}
}

func TestEmbedError(t *testing.T) {
	e := &fakeEmbed{err: fmt.Errorf("%w: x", embed.ErrNotFound)}
	for _, k := range []song.Kind{song.KindTrack, song.KindPlaylist} {
		if _, err := NewPublic(e, &fakeMeta{}).Resolve(context.Background(), song.Ref{Kind: k, ID: "x"}); !errors.Is(err, embed.ErrNotFound) {
			t.Errorf("%s: err = %v, quero embed.ErrNotFound", k, err)
		}
	}
}
