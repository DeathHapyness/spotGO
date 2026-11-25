package output

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DeathHapyness/spotGO/internal/song"
)

var sample = song.Song{
	Title:       "Paranoid Android",
	Artists:     []string{"Radiohead"},
	Album:       "OK Computer",
	TrackNumber: 2,
	TrackCount:  12,
	DiscNumber:  1,
	Duration:    387 * time.Second,
	ReleaseDate: "1997-05-21",
	ISRC:        "GBAYE9700050",
	SpotifyID:   "6LgJvl0Xdtc73RJ1mmpotq",
}

func TestPath(t *testing.T) {
	feat := sample
	feat.Artists = []string{"A", "B"}
	feat.AlbumArtist = "Vários"

	tests := []struct {
		name string
		tmpl string
		s    song.Song
		v    Vars
		want string
	}{
		{"padrão", DefaultTemplate, sample, Vars{}, "Radiohead - Paranoid Android.mp3"},
		{"sem extensão no template", "{title}", sample, Vars{}, "Paranoid Android.mp3"},
		{"pastas", "{album-artist}/{year} - {album}/{track-number} {title}.{output-ext}", sample, Vars{},
			filepath.Join("Radiohead", "1997 - OK Computer", "02 Paranoid Android.mp3")},
		{"vários artistas", "{artists} | {album-artist} | {artist}", feat, Vars{}, "A, B - Vários - A.mp3"},
		{"playlist", "{list-name}/{list-position} - {title}", sample, Vars{ListName: "Rock: 90s", ListPosition: 7, ListLength: 120},
			filepath.Join("Rock- 90s", "007 - Paranoid Android.mp3")},
		{"barra invertida vira pasta", `{artist}\{title}`, sample, Vars{}, filepath.Join("Radiohead", "Paranoid Android.mp3")},
		{"absoluto", "/música/{artist}/{title}", sample, Vars{}, filepath.Join(string(filepath.Separator), "música", "Radiohead", "Paranoid Android.mp3")},
		{"campo vazio", "{album}/{title}", song.Song{Title: "x"}, Vars{}, filepath.Join("_", "x.mp3")},
		{"outros campos", "{isrc} {track-id} {duration} {disc-number} {tracks-count} {date}", sample, Vars{},
			"GBAYE9700050 6LgJvl0Xdtc73RJ1mmpotq 387 1 12 1997-05-21.mp3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl, err := Parse(tt.tmpl)
			if err != nil {
				t.Fatal(err)
			}
			v := tt.v
			v.Song, v.Ext = tt.s, "mp3"
			if got := tmpl.Path(v); got != tt.want {
				t.Errorf("Path = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPathTitleWithSlash(t *testing.T) {
	s := sample
	s.Title = "AC/DC: Live?"
	tmpl, _ := Parse(DefaultTemplate)
	got := tmpl.Path(Vars{Song: s, Ext: "flac"})
	if got != "Radiohead - AC-DC- Live.flac" {
		t.Errorf("got %q", got)
	}
}

func TestParseErrors(t *testing.T) {
	for _, tmpl := range []string{"", "  ", "{titel}", "{title"} {
		if _, err := Parse(tmpl); err == nil {
			t.Errorf("Parse(%q): esperava erro", tmpl)
		}
	}
	_, err := Parse("{titel} - {artist}")
	if err == nil || !strings.Contains(err.Error(), "{titel}") {
		t.Errorf("o erro deve citar a variável: %v", err)
	}
}

func TestSanitize(t *testing.T) {
	tests := map[string]string{
		`a<b>c:d"e/f\g|h?i*j`:  "abc-d'e-f-g-hij",
		"tab\there\nnl":        "tab here nl",
		"  muitos   espaços ":  "muitos espaços",
		"termina com ponto...": "termina com ponto",
		"CON":                  "CON_",
		"nul.txt":              "nul.txt_",
		"CONCERTO":             "CONCERTO",
		"":                     "_",
		"...":                  "_",
		"Beyoncé — Ação":       "Beyoncé — Ação",
	}
	for in, want := range tests {
		if got := Sanitize(in); got != want {
			t.Errorf("Sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLongNameKeepsExtensionAndUTF8(t *testing.T) {
	s := sample
	s.Title = strings.Repeat("ação ", 100)
	tmpl, _ := Parse("{title}")
	got := tmpl.Path(Vars{Song: s, Ext: "opus"})
	if len(got) > maxSegment {
		t.Errorf("nome com %d bytes", len(got))
	}
	if !strings.HasSuffix(got, ".opus") {
		t.Errorf("perdeu a extensão: %q", got)
	}
	if !strings.HasPrefix(got, "ação ação") || strings.ContainsRune(got, '�') {
		t.Errorf("cortou no meio de um caractere: %q", got[len(got)-20:])
	}
}
