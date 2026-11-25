// Package output monta o caminho do arquivo final a partir de um template
// (no estilo do spotDL: "{artists} - {title}.{output-ext}").
package output

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/DeathHapyness/spotGO/internal/song"
)

const DefaultTemplate = "{artists} - {title}.{output-ext}"

// maxSegment é o limite de bytes de um nome de arquivo no ext4, NTFS e APFS.
const maxSegment = 255

type Vars struct {
	Song         song.Song
	Ext          string
	ListName     string
	ListPosition int
	ListLength   int
}

var fields = map[string]func(v Vars) string{
	"title":        func(v Vars) string { return v.Song.Title },
	"artists":      func(v Vars) string { return strings.Join(v.Song.Artists, ", ") },
	"artist":       func(v Vars) string { return v.Song.MainArtist() },
	"album":        func(v Vars) string { return v.Song.Album },
	"album-artist": func(v Vars) string { return albumArtist(v.Song) },
	"track-number": func(v Vars) string { return pad(v.Song.TrackNumber) },
	"tracks-count": func(v Vars) string { return num(v.Song.TrackCount) },
	"disc-number":  func(v Vars) string { return num(v.Song.DiscNumber) },
	"year":         func(v Vars) string { return year(v.Song.ReleaseDate) },
	"date":         func(v Vars) string { return v.Song.ReleaseDate },
	"isrc":         func(v Vars) string { return v.Song.ISRC },
	"track-id":     func(v Vars) string { return v.Song.SpotifyID },
	"duration":     func(v Vars) string { return num(int(v.Song.Duration.Seconds())) },
	"list-name":    func(v Vars) string { return v.ListName },
	"list-position": func(v Vars) string {
		return padTo(v.ListPosition, len(strconv.Itoa(v.ListLength)))
	},
	"list-length": func(v Vars) string { return num(v.ListLength) },
	"output-ext":  func(v Vars) string { return v.Ext },
}

type part struct {
	lit   string
	field func(Vars) string
}

type Template struct {
	segments [][]part
}

// Parse valida o template. Uma variável desconhecida é erro, para o usuário
// não descobrir o erro de digitação só depois de baixar a playlist inteira.
func Parse(tmpl string) (*Template, error) {
	if strings.TrimSpace(tmpl) == "" {
		return nil, fmt.Errorf("template de saída vazio")
	}
	if !strings.Contains(tmpl, "{output-ext}") {
		tmpl += ".{output-ext}"
	}
	tmpl = strings.ReplaceAll(tmpl, `\`, "/")
	t := &Template{}
	for _, seg := range strings.Split(tmpl, "/") {
		if seg == "" {
			continue
		}
		parts, err := parseSegment(seg)
		if err != nil {
			return nil, err
		}
		t.segments = append(t.segments, parts)
	}
	if len(t.segments) == 0 {
		return nil, fmt.Errorf("template de saída sem nome de arquivo: %q", tmpl)
	}
	return t, nil
}

func parseSegment(seg string) ([]part, error) {
	var parts []part
	for seg != "" {
		i := strings.IndexByte(seg, '{')
		if i < 0 {
			parts = append(parts, part{lit: seg})
			break
		}
		if i > 0 {
			parts = append(parts, part{lit: seg[:i]})
		}
		j := strings.IndexByte(seg[i:], '}')
		if j < 0 {
			return nil, fmt.Errorf("template de saída: falta fechar a chave em %q", seg[i:])
		}
		name := seg[i+1 : i+j]
		f, ok := fields[name]
		if !ok {
			return nil, fmt.Errorf("template de saída: variável desconhecida {%s}", name)
		}
		parts = append(parts, part{field: f})
		seg = seg[i+j+1:]
	}
	return parts, nil
}

// Path devolve o caminho relativo, com cada pasta e o nome já sanitizados.
func (t *Template) Path(v Vars) string {
	segs := make([]string, len(t.segments))
	last := len(t.segments) - 1
	for i, parts := range t.segments {
		var b strings.Builder
		for _, p := range parts {
			if p.field != nil {
				b.WriteString(p.field(v))
			} else {
				b.WriteString(p.lit)
			}
		}
		if i == last {
			segs[i] = sanitizeFile(b.String(), v.Ext)
		} else {
			segs[i] = Sanitize(b.String())
		}
	}
	return filepath.Join(segs...)
}

var replacer = strings.NewReplacer(
	"/", "-", `\`, "-", ":", "-", "|", "-",
	"<", "", ">", "", `"`, "'", "?", "", "*", "",
)

// Sanitize transforma s num nome válido no Linux, macOS e Windows.
func Sanitize(s string) string {
	s = replacer.Replace(s)
	s = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return ' '
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	// O Windows não aceita nomes terminados em ponto ou espaço.
	s = strings.TrimRight(s, ". ")
	s = truncate(s, maxSegment)
	s = strings.TrimRight(s, ". ")
	if s == "" {
		return "_"
	}
	if reserved(s) {
		s += "_"
	}
	return s
}

// sanitizeFile corta o nome sem perder a extensão.
func sanitizeFile(s, ext string) string {
	suffix := ""
	if ext != "" && strings.HasSuffix(s, "."+ext) {
		suffix = "." + ext
		s = strings.TrimSuffix(s, suffix)
	}
	s = replacer.Replace(s)
	base := Sanitize(truncate(s, maxSegment-len(suffix)))
	return base + suffix
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}

func reserved(s string) bool {
	base, _, _ := strings.Cut(s, ".")
	switch strings.ToUpper(strings.TrimSpace(base)) {
	case "CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return true
	}
	return false
}

func albumArtist(s song.Song) string {
	if s.AlbumArtist != "" {
		return s.AlbumArtist
	}
	return s.MainArtist()
}

func num(n int) string {
	if n <= 0 {
		return ""
	}
	return strconv.Itoa(n)
}

func pad(n int) string { return padTo(n, 2) }

func padTo(n, width int) string {
	if n <= 0 {
		return ""
	}
	return fmt.Sprintf("%0*d", width, n)
}

func year(date string) string {
	if len(date) >= 4 {
		return date[:4]
	}
	return ""
}
