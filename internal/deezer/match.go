package deezer

import (
	"maps"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/DeathHapyness/spotGO/internal/song"
	"github.com/DeathHapyness/spotGO/internal/textnorm"
)

const maxDelta = 5 * time.Second

// pickTrack escolhe a mesma gravação: rejeita duração, título, artista ou
// variante (ao vivo, take, cover...) diferentes; entre as restantes, prefere a
// edição do álbum conhecido e evita coletâneas. Empate fica com o primeiro.
func pickTrack(src song.Song, knownAlbum string, cands []apiTrack) (apiTrack, bool) {
	srcBase, srcVer := textnorm.SplitTitle(src.Title)
	srcVariants := textnorm.Variants(srcVer)
	knownBase := ""
	if knownAlbum != "" {
		knownBase, _ = textnorm.SplitTitle(knownAlbum)
	}

	var best apiTrack
	bestScore, found := math.Inf(-1), false
	for _, c := range cands {
		delta := 0.0
		if src.Duration > 0 {
			d := (src.Duration - time.Duration(c.Duration)*time.Second).Abs()
			if d > maxDelta {
				continue
			}
			delta = d.Seconds()
		}
		base, ver := textnorm.SplitTitle(c.Title)
		if base != srcBase || !anySameArtist(src.Artists, c.Artist.Name) ||
			!maps.Equal(textnorm.Variants(ver), srcVariants) {
			continue
		}

		score := 100 - 5*delta
		if knownBase != "" {
			if albumBase, _ := textnorm.SplitTitle(c.Album.Title); albumBase == knownBase {
				score += 30
			}
		}
		if slices.Equal(ver, srcVer) {
			score += 15
		}
		if isCompilation(c.Album.Title) {
			score -= 20
		}
		if score > bestScore {
			best, bestScore, found = c, score, true
		}
	}
	return best, found
}

// pickAlbum escolhe a edição certa do álbum: mesmo título base e artista,
// preferindo o mesmo número de faixas e as mesmas palavras de versão.
func pickAlbum(name, artist string, nTracks int, cands []apiAlbum) (apiAlbum, bool) {
	base, ver := textnorm.SplitTitle(name)
	variants := textnorm.Variants(ver)
	artists := splitArtists(artist)

	var best apiAlbum
	bestScore, found := math.MinInt, false
	for _, c := range cands {
		cBase, cVer := textnorm.SplitTitle(c.Title)
		if cBase != base || !anySameArtist(artists, c.Artist.Name) ||
			!maps.Equal(textnorm.Variants(cVer), variants) {
			continue
		}
		score := 0
		diff := c.NbTracks - nTracks
		if diff == 0 {
			score += 30
		}
		if slices.Equal(cVer, ver) {
			score += 15
		}
		score -= min(10*max(diff, -diff), 30)
		if score > bestScore {
			best, bestScore, found = c, score, true
		}
	}
	return best, found
}

func anySameArtist(artists []string, name string) bool {
	for _, a := range artists {
		if textnorm.SameArtist(a, name) {
			return true
		}
	}
	return false
}

// Sem "gold", "hits" nem títulos só com números: "Fool's Gold", "21" e "1989"
// são álbuns normais. A busca do Deezer não traz o record_type para decidir.
var compilationWords = []string{"greatest hits", "best of", "the very best", "collection", "anthology", "essential", "essentials"}

func isCompilation(albumTitle string) bool {
	padded := " " + textnorm.Fold(albumTitle) + " "
	for _, w := range compilationWords {
		if strings.Contains(padded, " "+w+" ") {
			return true
		}
	}
	return false
}

// splitArtists separa o "A, B" que o embed usa para álbuns com vários artistas.
func splitArtists(s string) []string {
	parts := strings.Split(s, ", ")
	out := parts[:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
