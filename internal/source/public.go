package source

import (
	"context"
	"fmt"
	"slices"
	"strings"

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
