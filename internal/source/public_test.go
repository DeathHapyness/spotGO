package source

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
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
	discoTypes func(types []string) []deezer.AlbumRef
	discoErr   error
	ignoreCtx  bool // simula respostas vindas do cache, que não olham o ctx
	albumSongs map[int64][]song.Song
	albumErr   map[int64]error
}

func (f *fakeMeta) EnrichTrack(ctx context.Context, s song.Song) (song.Song, bool, error) {
	f.enriched = append(f.enriched, s.Title)
	if f.onEnrich != nil {
		f.onEnrich(ctx, len(f.enriched))
	}
	if err := ctx.Err(); err != nil && !f.ignoreCtx {
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

func (f *fakeMeta) Discography(_ context.Context, _ string, types []string) ([]deezer.AlbumRef, error) {
	if f.discoTypes != nil {
		return f.discoTypes(types), f.discoErr
	}
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

func albumEmbed(songs []song.Song) *fakeEmbed {
	return &fakeEmbed{cols: map[song.Kind]embed.Collection{song.KindAlbum: {Name: "Disco", Artist: "Artista", Songs: songs}}}
}

func withISRC(songs []song.Song, titles ...string) []song.Song {
	out := slices.Clone(songs)
	for i := range out {
		if slices.Contains(titles, out[i].Title) {
			out[i].ISRC = "ALB-" + out[i].Title
		}
	}
	return out
}

func TestAlbumFallbackOnlyMissing(t *testing.T) {
	m := &fakeMeta{album: func(s []song.Song) ([]song.Song, error) { return withISRC(s, "A", "C"), nil }}
	p := NewPublic(albumEmbed(titles("A", "B", "C")), m)
	var progress []string
	p.Progress = func(done, total int) { progress = append(progress, fmt.Sprintf("%d/%d", done, total)) }

	r, err := p.Resolve(context.Background(), song.Ref{Kind: song.KindAlbum, ID: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(m.enriched, []string{"B"}) {
		t.Fatalf("EnrichTrack chamado com %v, quero só [B]", m.enriched)
	}
	got := []string{r.Songs[0].ISRC, r.Songs[1].ISRC, r.Songs[2].ISRC}
	if !slices.Equal(got, []string{"ALB-A", "ISRC-B", "ALB-C"}) {
		t.Fatalf("ISRCs = %v", got)
	}
	if r.Unmatched != 0 || r.Name != "Disco" || r.Artist != "Artista" {
		t.Fatalf("Result = %+v", r)
	}
	if strings.Join(progress, ",") != "1/1" {
		t.Fatalf("progress = %v, quero [1/1]", progress)
	}
}

func TestAlbumNothingMatched(t *testing.T) {
	m := &fakeMeta{}
	r, err := NewPublic(albumEmbed(titles("A", "B", "C")), m).Resolve(context.Background(), song.Ref{Kind: song.KindAlbum, ID: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(m.enriched, []string{"A", "B", "C"}) {
		t.Fatalf("EnrichTrack chamado com %v", m.enriched)
	}
	if r.Unmatched != 0 {
		t.Fatalf("Unmatched = %d", r.Unmatched)
	}
}

func TestAlbumEnrichError(t *testing.T) {
	m := &fakeMeta{album: func(s []song.Song) ([]song.Song, error) { return s, errors.New("conexão recusada") }}
	r, err := NewPublic(albumEmbed(titles("A", "B", "C")), m).Resolve(context.Background(), song.Ref{Kind: song.KindAlbum, ID: "a"})
	if err != nil {
		t.Fatalf("err = %v, quero nil (fallback por faixa)", err)
	}
	if !slices.Equal(m.enriched, []string{"A", "B", "C"}) || r.Unmatched != 0 {
		t.Fatalf("enriched = %v, Unmatched = %d", m.enriched, r.Unmatched)
	}
}

func TestAlbumCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &fakeMeta{album: func(s []song.Song) ([]song.Song, error) { cancel(); return s, context.Canceled }}
	_, err := NewPublic(albumEmbed(titles("A", "B")), m).Resolve(ctx, song.Ref{Kind: song.KindAlbum, ID: "a"})
	if !errors.Is(err, context.Canceled) || len(m.enriched) != 0 {
		t.Fatalf("err = %v, enriched = %v", err, m.enriched)
	}
}

func artistSetup(m *fakeMeta) *Public {
	e := &fakeEmbed{cols: map[song.Kind]embed.Collection{song.KindArtist: {Name: "The Beatles", Songs: titles("top1")}}}
	return NewPublic(e, m)
}

func TestArtistDedup(t *testing.T) {
	m := &fakeMeta{
		disco: []deezer.AlbumRef{{ID: 1, Title: "Disco"}, {ID: 2, Title: "Disco (Deluxe)"}},
		albumSongs: map[int64][]song.Song{
			1: {{Title: "x", ISRC: "X"}, {Title: "y", ISRC: "Y"}},
			2: {{Title: "x", ISRC: "X"}, {Title: "y", ISRC: "Y"}, {Title: "z", ISRC: "Z"}, {Title: "sem"}, {Title: "sem"}},
		},
	}
	p := artistSetup(m)
	var progress []string
	p.Progress = func(done, total int) { progress = append(progress, fmt.Sprintf("%d/%d", done, total)) }

	r, err := p.Resolve(context.Background(), song.Ref{Kind: song.KindArtist, ID: "ar"})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, s := range r.Songs {
		got = append(got, s.Title)
	}
	if !slices.Equal(got, []string{"x", "y", "z", "sem", "sem"}) {
		t.Fatalf("faixas = %v", got)
	}
	if r.Name != "The Beatles" || r.Artist != "The Beatles" || r.Unmatched != 2 {
		t.Fatalf("Result = %+v", r)
	}
	if strings.Join(progress, ",") != "1/2,2/2" {
		t.Fatalf("progress = %v", progress)
	}
	if len(m.enriched) != 0 {
		t.Fatalf("artista não deve chamar EnrichTrack: %v", m.enriched)
	}
}

func TestArtistNotFound(t *testing.T) {
	m := &fakeMeta{discoErr: deezer.ErrNotFound}
	_, err := artistSetup(m).Resolve(context.Background(), song.Ref{Kind: song.KindArtist, ID: "ar"})
	if !errors.Is(err, deezer.ErrNotFound) || !strings.Contains(err.Error(), "The Beatles") {
		t.Fatalf("err = %v", err)
	}
}

func TestArtistAlbumError(t *testing.T) {
	m := &fakeMeta{
		disco:      []deezer.AlbumRef{{ID: 1}, {ID: 2}},
		albumSongs: map[int64][]song.Song{2: {{Title: "z", ISRC: "Z"}}},
		albumErr:   map[int64]error{1: errors.New("conexão recusada")},
	}
	r, err := artistSetup(m).Resolve(context.Background(), song.Ref{Kind: song.KindArtist, ID: "ar"})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Songs) != 1 || r.Songs[0].ISRC != "Z" || r.Failed != 1 {
		t.Fatalf("Result = %+v", r)
	}
}

func TestArtistAlbumCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	m := &fakeMeta{disco: []deezer.AlbumRef{{ID: 1}}, albumErr: map[int64]error{1: context.Canceled}}
	if _, err := artistSetup(m).Resolve(ctx, song.Ref{Kind: song.KindArtist, ID: "ar"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}

func TestArtistDiscographyNetworkError(t *testing.T) {
	m := &fakeMeta{discoErr: errors.New("conexão recusada")}
	_, err := artistSetup(m).Resolve(context.Background(), song.Ref{Kind: song.KindArtist, ID: "ar"})
	if err == nil || strings.Contains(err.Error(), "não encontrado") || !strings.Contains(err.Error(), "conexão recusada") {
		t.Fatalf("err = %v", err)
	}
}

func TestPlaylistCanceledCached(t *testing.T) {
	e := &fakeEmbed{cols: map[song.Kind]embed.Collection{song.KindPlaylist: {Songs: titles("A", "B", "C")}}}
	ctx, cancel := context.WithCancel(context.Background())
	m := &fakeMeta{ignoreCtx: true, onEnrich: func(_ context.Context, n int) {
		if n == 1 {
			cancel()
		}
	}}
	_, err := NewPublic(e, m).Resolve(ctx, song.Ref{Kind: song.KindPlaylist, ID: "p"})
	if !errors.Is(err, context.Canceled) || len(m.enriched) != 1 {
		t.Fatalf("err = %v, enriched = %v; quero Canceled depois de 1 faixa", err, m.enriched)
	}
}

func TestArtistOnlySingles(t *testing.T) {
	var asked [][]string
	m := &fakeMeta{
		discoTypes: func(types []string) []deezer.AlbumRef {
			asked = append(asked, types)
			if slices.Contains(types, "single") {
				return []deezer.AlbumRef{{ID: 3}}
			}
			return nil
		},
		albumSongs: map[int64][]song.Song{3: {{Title: "s", ISRC: "S"}}},
	}
	r, err := artistSetup(m).Resolve(context.Background(), song.Ref{Kind: song.KindArtist, ID: "ar"})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Songs) != 1 || r.Songs[0].ISRC != "S" {
		t.Fatalf("Songs = %+v (tipos pedidos: %v)", r.Songs, asked)
	}
}

func TestArtistNoReleases(t *testing.T) {
	m := &fakeMeta{discoTypes: func([]string) []deezer.AlbumRef { return nil }}
	_, err := artistSetup(m).Resolve(context.Background(), song.Ref{Kind: song.KindArtist, ID: "ar"})
	if err == nil || !strings.Contains(err.Error(), "The Beatles") {
		t.Fatalf("err = %v, quero erro citando o artista", err)
	}
}
