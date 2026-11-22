package deezer

import (
	"context"
	"errors"
	"maps"
	"slices"
	"time"

	"github.com/DeathHapyness/spotGO/internal/song"
	"github.com/DeathHapyness/spotGO/internal/textnorm"
)

// EnrichTrack completa uma faixa com os dados do Deezer: nomes do Spotify,
// números do Deezer. Sem candidato aceitável, devolve s intacta e ok=false.
func (c *Client) EnrichTrack(ctx context.Context, s song.Song) (song.Song, bool, error) {
	base, _ := textnorm.SplitTitle(s.Title)
	cands, err := c.searchTracks(ctx, s.MainArtist()+" "+base)
	if err != nil {
		return s, false, notFoundIsNil(err)
	}
	pick, ok := pickTrack(s, s.Album, cands)
	if !ok {
		return s, false, nil
	}
	t, err := c.track(ctx, pick.ID)
	if err != nil {
		return s, false, notFoundIsNil(err)
	}

	out := s
	out.ISRC = t.ISRC
	if s.Album != "" {
		known, _ := textnorm.SplitTitle(s.Album)
		if got, _ := textnorm.SplitTitle(t.Album.Title); got != known {
			// Mesma gravação, outro álbum: só o ISRC vale; números e capa seriam do álbum errado.
			return out, true, nil
		}
	}
	if out.Album == "" {
		out.Album = t.Album.Title
	}
	if out.AlbumArtist == "" {
		out.AlbumArtist = t.Artist.Name
	}
	if out.ReleaseDate == "" {
		out.ReleaseDate = firstNonEmpty(t.ReleaseDate, t.Album.ReleaseDate)
	}
	if out.TrackCount == 0 {
		out.TrackCount = t.Album.NbTracks
	}
	out.TrackNumber = t.TrackPosition
	out.DiscNumber = t.DiskNumber
	if t.Album.CoverXL != "" {
		out.CoverURL = t.Album.CoverXL
	}
	return out, true, nil
}

// EnrichAlbum completa as faixas de um álbum com uma busca e duas leituras,
// sem uma chamada por faixa. Devolve uma cópia; sem álbum, as songs originais.
func (c *Client) EnrichAlbum(ctx context.Context, name, artist string, songs []song.Song) ([]song.Song, error) {
	base, _ := textnorm.SplitTitle(name)
	main := artist
	if a := splitArtists(artist); len(a) > 0 {
		main = a[0]
	}
	cands, err := c.searchAlbums(ctx, main+" "+base)
	if err != nil {
		return songs, notFoundIsNil(err)
	}
	pick, ok := pickAlbum(name, artist, len(songs), cands)
	if !ok {
		return songs, nil
	}
	al, err := c.album(ctx, pick.ID)
	if err != nil {
		return songs, notFoundIsNil(err)
	}
	tracks, err := c.albumTracks(ctx, pick.ID)
	if err != nil {
		return songs, notFoundIsNil(err)
	}

	out := slices.Clone(songs)
	used := make([]bool, len(tracks))
	for i := range out {
		j := matchAlbumTrack(out[i], i, tracks, used)
		if j < 0 {
			continue
		}
		used[j] = true
		t := tracks[j]
		out[i].ISRC = t.ISRC
		out[i].TrackNumber = t.TrackPosition
		out[i].DiscNumber = t.DiskNumber
		out[i].TrackCount = al.NbTracks
		if out[i].ReleaseDate == "" {
			out[i].ReleaseDate = al.ReleaseDate
		}
		if out[i].AlbumArtist == "" {
			out[i].AlbumArtist = al.Artist.Name
		}
		if al.CoverXL != "" {
			out[i].CoverURL = al.CoverXL
		}
	}
	return out, nil
}

// matchAlbumTrack tenta primeiro a mesma posição e depois a primeira faixa
// livre com o mesmo título base e duração compatível.
func matchAlbumTrack(s song.Song, i int, tracks []apiTrack, used []bool) int {
	base, ver := textnorm.SplitTitle(s.Title)
	variants := textnorm.Variants(ver)
	fits := func(j int) bool {
		if used[j] {
			return false
		}
		tBase, tVer := textnorm.SplitTitle(tracks[j].Title)
		if tBase != base || !maps.Equal(textnorm.Variants(tVer), variants) {
			return false
		}
		d := (s.Duration - time.Duration(tracks[j].Duration)*time.Second).Abs()
		return s.Duration == 0 || d <= maxDelta
	}
	if i < len(tracks) && fits(i) {
		return i
	}
	for j := range tracks {
		if fits(j) {
			return j
		}
	}
	return -1
}

type AlbumRef struct {
	ID          int64
	Title       string
	RecordType  string
	ReleaseDate string
}

// Discography lista os lançamentos do artista com mais fãs entre os de mesmo
// nome. types vazio = só álbuns.
func (c *Client) Discography(ctx context.Context, artist string, types []string) ([]AlbumRef, error) {
	if len(types) == 0 {
		types = []string{"album"}
	}
	cands, err := c.searchArtists(ctx, artist)
	if err != nil {
		return nil, err
	}
	var best *apiArtist
	for i := range cands {
		if textnorm.SameArtist(artist, cands[i].Name) && (best == nil || cands[i].NbFan > best.NbFan) {
			best = &cands[i]
		}
	}
	if best == nil {
		return nil, ErrNotFound
	}
	albums, err := c.artistAlbums(ctx, best.ID)
	if err != nil {
		return nil, err
	}
	var out []AlbumRef
	for _, a := range albums {
		if slices.Contains(types, a.RecordType) {
			out = append(out, AlbumRef{ID: a.ID, Title: a.Title, RecordType: a.RecordType, ReleaseDate: a.ReleaseDate})
		}
	}
	return out, nil
}

// AlbumSongs monta as faixas de um álbum só com dados do Deezer.
func (c *Client) AlbumSongs(ctx context.Context, id int64) ([]song.Song, error) {
	al, err := c.album(ctx, id)
	if err != nil {
		return nil, err
	}
	tracks, err := c.albumTracks(ctx, id)
	if err != nil {
		return nil, err
	}
	out := make([]song.Song, 0, len(tracks))
	for _, t := range tracks {
		out = append(out, song.Song{
			Title:       t.Title,
			Artists:     []string{firstNonEmpty(t.Artist.Name, al.Artist.Name)},
			Album:       al.Title,
			AlbumArtist: al.Artist.Name,
			TrackNumber: t.TrackPosition,
			TrackCount:  al.NbTracks,
			DiscNumber:  t.DiskNumber,
			Duration:    time.Duration(t.Duration) * time.Second,
			ReleaseDate: al.ReleaseDate,
			ISRC:        t.ISRC,
			CoverURL:    al.CoverXL,
			Explicit:    t.Explicit,
		})
	}
	return out, nil
}

func notFoundIsNil(err error) error {
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
