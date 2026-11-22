package matching

import (
	"math"
	"sort"
	"strings"

	"github.com/DeathHapyness/spotGO/internal/song"
)

var forbiddenWords = []string{"bassboosted", "remix", "remastered", "remaster", "reverb", "bassboost", "live",
	"acoustic", "8daudio", "concert", "acapella", "slowed", "instrumental", "cover", "karaoke"}

type Scored struct {
	Result Result
	Score  float64
}

// nameMatch é o calc_name_match.
func nameMatch(s song.Song, r Result) float64 {
	ms1, ms2 := matchStrings(s, r)
	l1, l2 := basedSort(strings.Split(slug(r.Name), "-"), strings.Split(slug(s.Title), "-"))
	match := ratio(strings.Join(l1, "-"), strings.Join(l2, "-"))
	if match <= 75 {
		match = max(match, ratio(ms1, ms2))
	}
	return match
}

func timeMatch(s song.Song, r Result) float64 {
	return 100 * math.Exp(-0.1*math.Abs((s.Duration-r.Duration).Seconds()))
}

func albumMatch(s song.Song, r Result) float64 {
	if r.Album == "" || s.Album == "" {
		return 0
	}
	return ratio(slug(s.Album), slug(r.Album))
}

func hasCommonWord(s song.Song, r Result) bool {
	check := noDash(slug(r.Name))
	for _, w := range strings.Split(slug(s.Title), "-") {
		if w != "" && strings.Contains(check, w) {
			return true
		}
	}
	return false
}

func forbiddenCount(s song.Song, r Result) int {
	songName, check := noDash(slug(s.Title)), noDash(slug(r.Name))
	n := 0
	for _, w := range forbiddenWords {
		if strings.Contains(check, w) && !strings.Contains(songName, w) {
			n++
		}
	}
	return n
}

// Order é o order_results do spotDL: devolve só os resultados aprovados,
// na ordem de entrada, cada um com a nota de 0 a 100.
func Order(s song.Song, results []Result) []Scored {
	var out []Scored
	for _, r := range results {
		if !hasCommonWord(s, r) {
			continue
		}
		artists := artistsMatch(s, r)
		name := nameMatch(s, r) - 15*float64(forbiddenCount(s, r))
		album := albumMatch(s, r)
		tm := timeMatch(s, r)

		if name <= 60 || artists < 70 {
			continue
		}
		avg := (artists + name) / 2
		if r.Verified && !r.ISRCSearch && r.Album != "" && album <= 80 {
			avg = (avg + album) / 2
		}
		if tm < 25 || (tm < 50 && avg < 75) {
			continue
		}
		if (!r.ISRCSearch && avg <= 85) || tm < 0 {
			avg = (avg + tm) / 2
			if r.Explicit != nil && *r.Explicit != s.Explicit {
				avg -= 5
			}
		}
		out = append(out, Scored{Result: r, Score: min(avg, 100)})
	}
	return out
}

// Best é o get_best_result do spotDL: entre os que estão a até 8 pontos do
// melhor, desempata pelas visualizações (até +15 pontos).
func Best(scored []Scored) (Scored, bool) {
	if len(scored) == 0 {
		return Scored{}, false
	}
	sorted := append([]Scored(nil), scored...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Score > sorted[j].Score })
	top := sorted[:1]
	for _, sc := range sorted[1:] {
		if sorted[0].Score-sc.Score <= 8 {
			top = append(top, sc)
		}
	}
	if len(top) == 1 || (top[0].Score > 80 && top[0].Result.ISRCSearch) {
		return top[0], true
	}

	hi, lo := top[0].Result.Views, top[0].Result.Views
	for _, sc := range top[1:] {
		hi, lo = max(hi, sc.Result.Views), min(lo, sc.Result.Views)
	}
	if hi == 0 || hi == lo {
		return top[0], true
	}
	best := Scored{Score: math.Inf(-1)}
	for _, sc := range top {
		weighted := sc
		weighted.Score = min(sc.Score+float64(sc.Result.Views-lo)/float64(hi-lo)*15, 100)
		if weighted.Score > best.Score {
			best = weighted
		}
	}
	return best, true
}
