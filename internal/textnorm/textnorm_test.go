package textnorm

import (
	"reflect"
	"testing"
)

func TestFold(t *testing.T) {
	cases := map[string]string{
		"Beyoncé":           "beyonce",
		"AC/DC":             "ac dc",
		"Simon & Garfunkel": "simon and garfunkel",
		"Don’t Stop":        "don t stop",
		"  Ünïcödé  ":       "unicode",
		"":                  "",
	}
	for in, want := range cases {
		if got := Fold(in); got != want {
			t.Errorf("Fold(%q) = %q, quero %q", in, got, want)
		}
	}
}

func TestSplitTitle(t *testing.T) {
	cases := []struct {
		in      string
		base    string
		version []string
	}{
		{"Come Together - Remastered 2009", "come together", []string{"remastered", "2009"}},
		{"Come Together (Remastered 2009)", "come together", []string{"remastered", "2009"}},
		{"Flower In A Hurricane (feat. Tame Impala, SZA) [From GTAVI: Original Score]", "flower in a hurricane", nil},
		{"Come Together (live)", "come together", []string{"live"}},
		{"Song [Radio Edit] (Acoustic)", "song", []string{"radio", "edit", "acoustic"}},
		{"Song (with Someone) - Live", "song", []string{"live"}},
		{"Plain", "plain", nil},
	}
	for _, c := range cases {
		base, version := SplitTitle(c.in)
		if base != c.base || !reflect.DeepEqual(version, c.version) {
			t.Errorf("SplitTitle(%q) = (%q, %q), quero (%q, %q)", c.in, base, version, c.base, c.version)
		}
	}
}

func TestVariants(t *testing.T) {
	cases := []struct {
		version []string
		want    map[string]bool
	}{
		{[]string{"live"}, map[string]bool{"live": true}},
		{[]string{"remastered", "2009"}, map[string]bool{}},
		{[]string{"originally", "performed", "by", "the", "beatles"}, map[string]bool{"cover": true}},
		{[]string{"in", "the", "style", "of", "x"}, map[string]bool{"cover": true}},
		{[]string{"tribute"}, map[string]bool{"cover": true}},
		{[]string{"take", "5"}, map[string]bool{"take": true}},
		{[]string{"2019", "mix"}, map[string]bool{}},
		{[]string{"remix"}, map[string]bool{"remix": true}},
		{nil, map[string]bool{}},
	}
	for _, c := range cases {
		if got := Variants(c.version); !reflect.DeepEqual(got, c.want) {
			t.Errorf("Variants(%q) = %v, quero %v", c.version, got, c.want)
		}
	}
}

func TestSameArtist(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"The Beatles", "Beatles", true},
		{"Beyoncé", "Beyonce", true},
		{"AC/DC", "AC DC", true},
		{"Justice", "Justin", false},
	}
	for _, c := range cases {
		if got := SameArtist(c.a, c.b); got != c.want {
			t.Errorf("SameArtist(%q, %q) = %v, quero %v", c.a, c.b, got, c.want)
		}
	}
}
