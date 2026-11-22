package source

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/DeathHapyness/spotGO/internal/deezer"
	"github.com/DeathHapyness/spotGO/internal/song"
)

// Public é o modo sem credenciais: lista de faixas pelo embed, metadados pelo Deezer.
type Public struct {
	// Progress, se não for nil, é chamado depois de cada consulta ao Deezer.
	Progress func(done, total int)

	embed Embed
	meta  Meta
}

func NewPublic(e Embed, m Meta) *Public {
	return &Public{embed: e, meta: m}
}

func (p *Public) Resolve(ctx context.Context, ref song.Ref) (Result, error) {
	r := Result{Ref: ref}
	var err error
	switch ref.Kind {
	case song.KindTrack:
		err = p.track(ctx, &r)
	case song.KindPlaylist:
		err = p.playlist(ctx, &r)
	case song.KindAlbum:
		err = p.album(ctx, &r)
	case song.KindArtist:
		err = p.artist(ctx, &r)
	default:
		return Result{}, fmt.Errorf("%s: ainda não suportado", ref.Kind)
	}
	if err != nil {
		return Result{}, err
	}
	r.Unmatched = countUnmatched(r.Songs)
	return r, nil
}

func (p *Public) track(ctx context.Context, r *Result) error {
	s, err := p.embed.Track(ctx, r.Ref.ID)
	if err != nil {
		return err
	}
	r.Name, r.Artist, r.Songs = s.Title, strings.Join(s.Artists, ", "), []song.Song{s}
	return p.enrichEach(ctx, r, []int{0})
}

func (p *Public) playlist(ctx context.Context, r *Result) error {
	col, err := p.embed.Playlist(ctx, r.Ref.ID)
	if err != nil {
		return err
	}
	r.Name, r.Artist, r.Songs, r.Truncated = col.Name, col.Artist, slices.Clone(col.Songs), col.Truncated
	idx := make([]int, len(r.Songs))
	for i := range idx {
		idx[i] = i
	}
	return p.enrichEach(ctx, r, idx)
}

// album tenta o álbum inteiro de uma vez e consulta faixa por faixa só as que não casaram.
func (p *Public) album(ctx context.Context, r *Result) error {
	col, err := p.embed.Album(ctx, r.Ref.ID)
	if err != nil {
		return err
	}
	r.Name, r.Artist = col.Name, col.Artist
	r.Songs, err = p.meta.EnrichAlbum(ctx, col.Name, col.Artist, slices.Clone(col.Songs))
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		r.Songs = slices.Clone(col.Songs)
	}
	var missing []int
	for i, s := range r.Songs {
		if s.ISRC == "" {
			missing = append(missing, i)
		}
	}
	return p.enrichEach(ctx, r, missing)
}

// artist monta a discografia pelo Deezer; o embed só dá o nome e o top 10.
func (p *Public) artist(ctx context.Context, r *Result) error {
	col, err := p.embed.Artist(ctx, r.Ref.ID)
	if err != nil {
		return err
	}
	r.Name, r.Artist = col.Name, col.Name
	albums, err := p.meta.Discography(ctx, col.Name, nil)
	switch {
	case errors.Is(err, deezer.ErrNotFound):
		return fmt.Errorf("artista %q não encontrado no Deezer: %w", col.Name, err)
	case err != nil:
		return fmt.Errorf("discografia de %q: %w", col.Name, err)
	}
	seen := map[string]bool{}
	for n, a := range albums {
		songs, err := p.meta.AlbumSongs(ctx, a.ID)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			r.Failed++
		}
		for _, s := range songs {
			if s.ISRC != "" {
				if seen[s.ISRC] {
					continue
				}
				seen[s.ISRC] = true
			}
			r.Songs = append(r.Songs, s)
		}
		if p.Progress != nil {
			p.Progress(n+1, len(albums))
		}
	}
	return nil
}

// enrichEach consulta o Deezer para as faixas r.Songs[idx...], uma por vez.
// Erro de rede mantém a faixa como veio; cancelamento interrompe tudo.
func (p *Public) enrichEach(ctx context.Context, r *Result, idx []int) error {
	for n, i := range idx {
		s, _, err := p.meta.EnrichTrack(ctx, r.Songs[i])
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			r.Failed++
		} else {
			r.Songs[i] = s
		}
		if p.Progress != nil {
			p.Progress(n+1, len(idx))
		}
	}
	return nil
}
