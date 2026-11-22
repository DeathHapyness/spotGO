package song

import (
	"strings"
	"testing"
)

func TestParseURL(t *testing.T) {
	tests := []struct {
		in   string
		want Ref
	}{
		{"https://open.spotify.com/track/4HCOHRCSQelz2GPX8iTamO?si=abc", Ref{KindTrack, "4HCOHRCSQelz2GPX8iTamO"}},
		{"https://open.spotify.com/intl-pt/album/0ETFjACtuP2ADo6LFhL6HN", Ref{KindAlbum, "0ETFjACtuP2ADo6LFhL6HN"}},
		{"https://open.spotify.com/embed/playlist/37i9dQZF1DXcBWIGoYBM5M", Ref{KindPlaylist, "37i9dQZF1DXcBWIGoYBM5M"}},
		{"spotify:artist:3WrFJ7ztbogyGnTHbHJFl2", Ref{KindArtist, "3WrFJ7ztbogyGnTHbHJFl2"}},
		{"  https://open.spotify.com/track/4HCOHRCSQelz2GPX8iTamO \n", Ref{KindTrack, "4HCOHRCSQelz2GPX8iTamO"}},
	}
	for _, tt := range tests {
		got, err := ParseURL(tt.in)
		if err != nil {
			t.Errorf("ParseURL(%q): %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseURL(%q) = %+v, quero %+v", tt.in, got, tt.want)
		}
	}
}

func TestParseURLErrors(t *testing.T) {
	tests := []struct {
		in       string
		contains string
	}{
		{"https://open.spotify.com/episode/4HCOHRCSQelz2GPX8iTamO", "episode"},
		{"https://example.com/track/4HCOHRCSQelz2GPX8iTamO", ""},
		{"https://open.spotify.com/track/4HCOHRCSQelz2GPX8iTam", ""},
		{"spotify:track:4HCOHRCSQelz2GPX8iTa-O", ""},
		{"", ""},
	}
	for _, tt := range tests {
		_, err := ParseURL(tt.in)
		if err == nil {
			t.Errorf("ParseURL(%q): quero erro", tt.in)
			continue
		}
		if !strings.Contains(err.Error(), tt.contains) {
			t.Errorf("ParseURL(%q): erro %q deveria conter %q", tt.in, err, tt.contains)
		}
	}
}
