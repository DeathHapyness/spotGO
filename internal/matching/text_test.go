package matching

import (
	"math"
	"slices"
	"testing"
)

func near(a, b float64) bool { return math.Abs(a-b) < 0.01 }

// Valores do slugify do spotDL (python-slugify + unidecode, com ! @ $ permitidos).
// Única diferença deliberada: apóstrofos somem ("dont"); no Python, ' vira "-" ("don-t").
func TestSlug(t *testing.T) {
	cases := map[string]string{
		"LUCO & Alice Gray":         "luco-alice-gray",
		"Rock & Roll":               "rock-roll",
		"Don't Stop":                "dont-stop",
		"Don’t Stop":                "dont-stop",
		"A$AP Rocky":                "a$ap-rocky",
		"P!nk":                      "p!nk",
		"1,000 Years":               "1000-years",
		"a, b":                      "a-b",
		"🔥":                         "",
		"Wasted (feat. Alice Gray)": "wasted-feat-alice-gray",
		"Beyoncé":                   "beyonce",
		"LUCØ":                      "luco",
		"Łona":                      "lona",
		"Straße":                    "strasse",
		"Ænima":                     "aenima",
		"Кино":                      "kino",
		"  --Señorita--  ":          "senorita",
	}
	for in, want := range cases {
		if got := slug(in); got != want {
			t.Errorf("slug(%q) = %q, quero %q", in, got, want)
		}
	}
}

func TestRatio(t *testing.T) {
	cases := []struct {
		a, b string
		want float64
	}{
		{"abc", "abc", 100},
		{"", "", 100},
		{"abc", "", 0},
		{"", "abc", 0},
		{"abcd", "abce", 75},
		{"drake", "drake-bell", 66.6667},
		{"luco", "luca", 75},
		{"luço", "luço", 100},
	}
	for _, c := range cases {
		if got := ratio(c.a, c.b); !near(got, c.want) {
			t.Errorf("ratio(%q, %q) = %v, quero %v", c.a, c.b, got, c.want)
		}
	}
}

func TestRatioTokens(t *testing.T) {
	if got := ratioTokens([]string{"a", "b"}, []string{"a", "b"}); got != 100 {
		t.Errorf("iguais = %v", got)
	}
	got := ratioTokens([]string{"charlie", "moncler", "fukaj"}, []string{"fukaj", "charlie", "moncler"})
	if !near(got, 66.6667) {
		t.Errorf("reordenado = %v, quero 66.67", got)
	}
	if got := ratioTokens(nil, nil); got != 100 {
		t.Errorf("vazios = %v", got)
	}
}

func TestContainsWords(t *testing.T) {
	cases := []struct {
		words, s string
		want     bool
	}{
		{"queen", "queens-of-the-stone-age", false},
		{"drake", "drake-bell", true},
		{"stone-age", "queens-of-the-stone-age", true},
		{"", "x", false},
	}
	for _, c := range cases {
		if got := containsWords(c.words, c.s); got != c.want {
			t.Errorf("containsWords(%q, %q) = %v", c.words, c.s, got)
		}
	}
}

func TestBasedSort(t *testing.T) {
	strs, based := []string{"b", "a", "c"}, []string{"c", "a"}
	gotS, gotB := basedSort(strs, based)
	if !slices.Equal(gotS, []string{"c", "a", "b"}) || !slices.Equal(gotB, []string{"c", "a"}) {
		t.Fatalf("basedSort = %v, %v", gotS, gotB)
	}
	if !slices.Equal(strs, []string{"b", "a", "c"}) || !slices.Equal(based, []string{"c", "a"}) {
		t.Fatalf("alterou as entradas: %v, %v", strs, based)
	}
	gotS, _ = basedSort([]string{"z", "y", "x"}, nil)
	if !slices.Equal(gotS, []string{"x", "y", "z"}) {
		t.Fatalf("sem base = %v, quero ordem alfabética", gotS)
	}
}

func TestFillString(t *testing.T) {
	got := fillString([]string{"Alice Gray"}, "wasted", "luco-wasted-alice-gray")
	if got != "wasted-alicegray" {
		t.Fatalf("fillString = %q", got)
	}
	if again := fillString([]string{"Alice Gray"}, got, "luco-wasted-alice-gray"); again != got {
		t.Fatalf("duplicou: %q", again)
	}
	if got := fillString([]string{"Bob"}, "wasted", "luco-wasted"); got != "wasted" {
		t.Fatalf("adicionou artista ausente: %q", got)
	}
}

func TestCreateCleanString(t *testing.T) {
	if got := createCleanString([]string{"Luco", "Alice Gray", "Wasted"}, "wasted", true); got != "alicegray-luco" {
		t.Fatalf("createCleanString = %q", got)
	}
	if got := createCleanString([]string{"b", "a"}, "", false); got != "b-a" {
		t.Fatalf("sem ordenar = %q", got)
	}
}

func TestSongTitle(t *testing.T) {
	if got := songTitle("Down", []string{"Jay Sean", "Lil Wayne"}); got != "Jay Sean, Lil Wayne - Down" {
		t.Errorf("songTitle = %q", got)
	}
	if got := songTitle("X", nil); got != "X" {
		t.Errorf("songTitle sem artistas = %q", got)
	}
}
