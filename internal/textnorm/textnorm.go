// Package textnorm normaliza títulos e nomes para comparar Spotify, Deezer e YouTube.
package textnorm

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Fold remove acentos, troca & por "and", põe em minúsculas e reduz toda
// pontuação a um espaço.
func Fold(s string) string {
	var b strings.Builder
	space := false
	for _, r := range norm.NFD.String(s) {
		switch {
		case unicode.Is(unicode.Mn, r):
			continue
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = false
			b.WriteRune(unicode.ToLower(r))
		case r == '&':
			if b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteString("and")
			space = true
		default:
			space = true
		}
	}
	return b.String()
}

var dropped = map[string]bool{"feat": true, "featuring": true, "ft": true, "with": true, "from": true}

// SplitTitle separa o título base das partes de versão: grupos entre
// parênteses/colchetes e o sufixo depois de " - ". Créditos (feat., with,
// from) são descartados.
func SplitTitle(s string) (base string, version []string) {
	rest, parts := groups(s)
	if i := strings.Index(rest, " - "); i >= 0 {
		parts = append(parts, rest[i+3:])
		rest = rest[:i]
	}
	for _, p := range parts {
		words := strings.Fields(Fold(p))
		if len(words) == 0 || dropped[words[0]] {
			continue
		}
		version = append(version, words...)
	}
	return Fold(rest), version
}

// groups tira de s os trechos (...) e [...] de primeiro nível e os devolve em ordem.
func groups(s string) (rest string, parts []string) {
	var out, cur strings.Builder
	depth := 0
	for _, r := range s {
		switch {
		case r == '(' || r == '[':
			if depth > 0 {
				cur.WriteRune(r)
			}
			depth++
		case (r == ')' || r == ']') && depth > 0:
			depth--
			if depth == 0 {
				parts = append(parts, cur.String())
				cur.Reset()
				out.WriteByte(' ')
			} else {
				cur.WriteRune(r)
			}
		case depth > 0:
			cur.WriteRune(r)
		default:
			out.WriteRune(r)
		}
	}
	if depth > 0 {
		out.WriteString(cur.String())
	}
	return out.String(), parts
}

var variantWords = map[string]string{
	"live": "live", "remix": "remix", "acoustic": "acoustic", "instrumental": "instrumental",
	"demo": "demo", "take": "take", "karaoke": "karaoke", "cover": "cover", "tribute": "cover",
}

// Variants diz quais tipos de gravação as partes de versão indicam.
// Remaster não conta: é a mesma gravação.
func Variants(version []string) map[string]bool {
	out := map[string]bool{}
	for _, w := range version {
		if v, ok := variantWords[w]; ok {
			out[v] = true
		}
	}
	joined := " " + strings.Join(version, " ") + " "
	if strings.Contains(joined, " originally performed ") || strings.Contains(joined, " in the style ") {
		out["cover"] = true
	}
	return out
}

// SameArtist compara dois nomes de artista ignorando acentos, pontuação e o "The" inicial.
func SameArtist(a, b string) bool {
	return stripThe(Fold(a)) == stripThe(Fold(b))
}

func stripThe(s string) string {
	return strings.TrimPrefix(s, "the ")
}
