package matching

import (
	"slices"
	"sort"
	"strings"

	"github.com/DeathHapyness/spotGO/internal/textnorm"
)

var apostrophes = strings.NewReplacer("'", "", "’", "")

// slug imita o python-slugify: apóstrofos somem, o resto que não é letra ou
// dígito vira "-". Diferença: "&" vira "and" (o slugify usa "-").
func slug(s string) string {
	return strings.ReplaceAll(textnorm.Fold(apostrophes.Replace(s)), " ", "-")
}

func noDash(s string) string { return strings.ReplaceAll(s, "-", "") }

// ratio é o fuzz.ratio do rapidfuzz: 100·2·LCS/(len(a)+len(b)), por runes.
func ratio(a, b string) float64 {
	return ratioSeq([]rune(a), []rune(b))
}

// ratioTokens compara sequências de palavras (o ratio de tuplas do spotDL).
func ratioTokens(a, b []string) float64 { return ratioSeq(a, b) }

func ratioSeq[T comparable](a, b []T) float64 {
	if len(a)+len(b) == 0 {
		return 100
	}
	return 100 * 2 * float64(lcs(a, b)) / float64(len(a)+len(b))
}

func lcs[T comparable](a, b []T) int {
	prev, cur := make([]int, len(b)+1), make([]int, len(b)+1)
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			switch {
			case a[i-1] == b[j-1]:
				cur[j] = prev[j-1] + 1
			case prev[j] >= cur[j-1]:
				cur[j] = prev[j]
			default:
				cur[j] = cur[j-1]
			}
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

func containsWords(words, s string) bool {
	if words == "" {
		return false
	}
	return strings.Contains("-"+s+"-", "-"+words+"-")
}

// basedSort é o based_sort do spotDL: os itens de strs que estão em basedOn
// vêm primeiro, em ordem decrescente da posição em basedOn ordenado; o resto
// vai para o fim em ordem alfabética. Devolve basedOn ordenado e invertido.
func basedSort(strs, basedOn []string) ([]string, []string) {
	s, b := slices.Clone(strs), slices.Clone(basedOn)
	sort.Strings(s)
	sort.Strings(b)
	index := make(map[string]int, len(b))
	for i, v := range b {
		index[v] = i
	}
	key := func(v string) int {
		if i, ok := index[v]; ok {
			return i
		}
		return -1
	}
	sort.SliceStable(s, func(i, j int) bool { return key(s[i]) > key(s[j]) })
	slices.Reverse(b)
	return s, b
}

func fillString(strs []string, main, check string) string {
	final, test, simple := main, noDash(main), noDash(check)
	for _, s := range strs {
		sl := noDash(slug(s))
		if strings.Contains(simple, sl) && !strings.Contains(test, sl) {
			final += "-" + sl
			test += sl
		}
	}
	return final
}

func createCleanString(words []string, s string, sorted bool) string {
	s = noDash(slug(s))
	var out []string
	for _, w := range words {
		w = noDash(slug(w))
		if strings.Contains(s, w) {
			continue
		}
		out = append(out, w)
	}
	if sorted {
		sort.Strings(out)
	}
	return strings.Join(out, "-")
}

func songTitle(name string, artists []string) string {
	if len(artists) == 0 {
		return name
	}
	return strings.Join(artists, ", ") + " - " + name
}
