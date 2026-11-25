// Package provider define o que todo provedor de áudio faz (achar o melhor
// resultado para uma faixa) e tenta vários em ordem.
package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/DeathHapyness/spotGO/internal/matching"
	"github.com/DeathHapyness/spotGO/internal/song"
)

// ErrNotFound: o provedor respondeu, mas nada serve. Os provedores embrulham
// este erro (errors.Is funciona com todos).
var ErrNotFound = errors.New("não encontrada")

type Provider interface {
	Find(ctx context.Context, s song.Song) (matching.Scored, error)
}

type Named struct {
	Name string
	Provider
}

// Chain tenta os provedores em ordem e fica com o primeiro que achar.
// Um erro de rede num provedor não impede os seguintes de tentar.
type Chain []Named

func (c Chain) Find(ctx context.Context, s song.Song) (matching.Scored, error) {
	var errs []string
	for _, p := range c {
		found, err := p.Find(ctx, s)
		if err == nil {
			return found, nil
		}
		if ctx.Err() != nil {
			return matching.Scored{}, ctx.Err()
		}
		if !errors.Is(err, ErrNotFound) {
			errs = append(errs, fmt.Sprintf("%s: %v", p.Name, err))
		}
	}
	if len(errs) > 0 {
		return matching.Scored{}, errors.New(strings.Join(errs, "; "))
	}
	return matching.Scored{}, fmt.Errorf("%w em %s", ErrNotFound, c.names())
}

func (c Chain) names() string {
	n := make([]string, len(c))
	for i, p := range c {
		n[i] = p.Name
	}
	return strings.Join(n, ", ")
}

// Pick dá a nota de cada resultado e escolhe o melhor.
func Pick(s song.Song, results []matching.Result, source string) (matching.Scored, error) {
	if best, ok := matching.Best(matching.Order(s, results)); ok {
		return best, nil
	}
	return matching.Scored{}, fmt.Errorf("%w no %s", ErrNotFound, source)
}

// SimpleQuery é "Artista Título", sem o sufixo depois de " - " ("Remastered
// 2009", "Live"). As buscas do SoundCloud e do Bandcamp não devolvem nada com
// esses sufixos; o matching ainda confere o título inteiro.
func SimpleQuery(s song.Song) string {
	title, _, _ := strings.Cut(s.Title, " - ")
	return strings.TrimSpace(s.MainArtist() + " " + title)
}
