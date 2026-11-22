package matching

import (
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/DeathHapyness/spotGO/internal/song"
)

// Palavras que juntam nomes de artistas ("velmar-und-misdemeanor",
// "wasted-feat-alice-gray"); o YT Music às vezes responde em alemão ("mit").
var artistJoinWords = []string{"feat", "ft", "featuring", "with", "mit", "x", "and", "und", "vs"}

var songFeatures = regexp.MustCompile(`(?i)\s(?:[(\[]\s*(?:feat|ft|featuring|with)\b|(?:feat|ft|featuring)\b)`)

func slugs(ss []string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = slug(s)
	}
	return out
}

// mainArtistMatch é o calc_main_artist_match.
func mainArtistMatch(s song.Song, r Result) float64 {
	if len(r.Artists) == 0 || len(s.Artists) == 0 {
		return 0
	}
	// O based_sort do Python ordena song_artists no lugar; o resto da função
	// usa essa lista já em ordem alfabética.
	songArtists := slugs(s.Artists)
	_, sortedResult := basedSort(songArtists, slugs(r.Artists))
	sort.Strings(songArtists)

	mainSong, mainResult := slug(s.Artists[0]), sortedResult[0]

	// Um só artista no resultado: o nome dele pode trazer os outros
	// ("velmar-und-misdemeanor"); os outros são avaliados em otherArtistsMatch.
	if len(s.Artists) > 1 && len(r.Artists) == 1 {
		longest := slices.Clone(songArtists)
		sort.SliceStable(longest, func(i, j int) bool { return len(longest[i]) > len(longest[j]) })
		leftover := "-" + mainResult + "-"
		for _, a := range longest {
			if a != "" {
				leftover = strings.Replace(leftover, "-"+a+"-", "-", 1)
			}
		}
		clean := true
		for _, w := range strings.Split(leftover, "-") {
			if w != "" && !slices.Contains(artistJoinWords, w) {
				clean = false
				break
			}
		}
		if containsWords(mainSong, mainResult) && clean {
			return 100
		}
		if spelling := ratio(noDash(mainSong), noDash(mainResult)); spelling >= 85 {
			return spelling
		}
		return 0
	}

	match := ratio(mainSong, mainResult)
	if match < 50 && len(songArtists) > 1 {
		for _, sa := range songArtists[:2] {
			for _, ra := range sortedResult[:min(2, len(sortedResult))] {
				match = max(match, ratio(sa, ra))
			}
		}
	}
	return match
}

// otherArtistsMatch é o calc_artists_match: quantos dos outros artistas aparecem.
func otherArtistsMatch(s song.Song, r Result) float64 {
	if len(s.Artists) <= 1 || len(r.Artists) == 0 {
		return 0
	}

	if len(r.Artists) == 1 {
		resultArtist, resultName := slug(r.Artists[0]), slug(r.Name)
		songName := s.Title
		if loc := songFeatures.FindStringIndex(songName); loc != nil {
			songName = songName[:loc[0]]
		}
		songName = slug(songName)

		var titleArtists string
		if containsWords(songName, resultName) {
			titleArtists = strings.Replace("-"+resultName+"-", "-"+songName+"-", "-", 1)
		} else {
			words := strings.Split(resultName, "-")
			j := slices.IndexFunc(words, func(w string) bool { return slices.Contains(artistJoinWords, w) })
			if j < 0 {
				j = len(words)
			}
			titleArtists = strings.Join(words[min(j+1, len(words)):], "-")
		}

		// Tira o artista principal, para "khalid" não ser achado dentro de "dj-khalid".
		mainArtist := slug(s.Artists[0])
		otherArtists := strings.Replace("-"+resultArtist+"-", "-"+mainArtist+"-", "-", 1)
		otherTitleArtists := strings.Replace("-"+titleArtists+"-", "-"+mainArtist+"-", "-", 1)

		found := 0
		for _, a := range slugs(s.Artists[1:]) {
			var ok bool
			if containsWords(mainArtist, a) {
				ok = containsWords(a, resultArtist) || containsWords(a, titleArtists)
			} else {
				ok = containsWords(a, otherArtists) || containsWords(a, otherTitleArtists)
			}
			if ok {
				found++
			}
		}
		return float64(found) / float64(len(s.Artists)-1) * 100
	}

	l1, l2 := basedSort(slugs(s.Artists), slugs(r.Artists))
	l1, l2 = l1[1:], l2[1:]
	total := 0.0
	for i := 0; i < max(len(l1), len(l2)); i++ {
		if i < len(l1) && i < len(l2) {
			total += ratio(l1[i], l2[i])
		}
	}
	return total / float64(len(l1))
}

// artistsFixup1: resultados não verificados (uploads), procura os artistas no canal e no título.
func artistsFixup1(s song.Song, r Result, score float64) float64 {
	if r.Verified || score > 50 {
		return score
	}
	joined := ""
	if len(r.Artists) > 0 {
		joined = slug(strings.Join(r.Artists, ", "))
	}
	score = max(score, ratio(slug(s.MainArtist()), joined))

	if score <= 70 {
		name := noDash(slug(r.Name))
		found := 0
		for _, a := range s.Artists {
			if strings.Contains(name, noDash(slug(a))) {
				found++
			}
		}
		score = max(score, float64(found)/float64(len(s.Artists))*100)
	}

	if score <= 70 {
		var t1, t2 []string
		for _, a := range s.Artists {
			t1 = append(t1, strings.Split(slug(a), "-")...)
		}
		for _, a := range r.Artists {
			t2 = append(t2, strings.Split(slug(a), "-")...)
		}
		score = max(score, ratioTokens(t1, t2))
	}
	return score
}

// artistsFixup2: resultados verificados com nota baixa.
func artistsFixup2(s song.Song, r Result, score float64) float64 {
	if score > 70 || !r.Verified {
		return score
	}
	songName, resultName := slug(s.Title), slug(r.Name)

	div := 1.0
	if len(s.Artists) > 1 {
		div = 2
	}
	skip := 0
	if score/div > 50 {
		skip = 1
	}

	_, ms2 := matchStrings(s, r)
	ms2 = noDash(ms2)
	for _, a := range s.Artists[min(skip, len(s.Artists)):] {
		if strings.Contains(ms2, noDash(slug(a))) {
			score += 5
		}
	}

	if score <= 70 {
		resultArtists := r.Artists
		if len(resultArtists) == 0 {
			resultArtists = []string{r.Author}
		}
		l1 := createCleanString(s.Artists, songName, true)
		l2 := createCleanString(resultArtists, resultName, true)
		score = max(score, ratio(l1, l2))
	}
	return score
}

// artistsFixup3: resultado com um só artista para uma song com vários.
func artistsFixup3(s song.Song, r Result, score float64) float64 {
	if score > 70 || len(r.Artists) != 1 || len(s.Artists) <= 1 {
		return score
	}
	fix := ratio(slug(r.Name), slug(songTitle(s.Title, []string{s.MainArtist()})))
	if fix >= 80 {
		score = (score + fix) / 2
	}
	return min(score, 100)
}

func artistsMatch(s song.Song, r Result) float64 {
	if len(s.Artists) == 0 {
		return 0
	}
	score := mainArtistMatch(s, r) + otherArtistsMatch(s, r)
	if len(s.Artists) > 1 {
		score /= 2
	}
	score = artistsFixup1(s, r, score)
	score = artistsFixup2(s, r, score)
	return artistsFixup3(s, r, score)
}

// matchStrings é o create_match_strings: título do resultado × título da song,
// completando os artistas que faltam de cada lado e ordenando as palavras.
func matchStrings(s song.Song, r Result) (string, string) {
	t1 := slug(r.Name)
	t2 := slug(songTitle(s.Title, s.Artists))
	if r.Verified {
		t2 = slug(s.Title)
	}
	t1 = fillString(s.Artists, t1, t2)
	t2 = fillString(s.Artists, t2, t1)
	l1, l2 := basedSort(strings.Split(t1, "-"), strings.Split(t2, "-"))
	return strings.Join(l1, "-"), strings.Join(l2, "-")
}
