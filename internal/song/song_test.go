package song

import "testing"

func TestMainArtistAndURL(t *testing.T) {
	s := Song{Artists: []string{"A", "B"}, SpotifyID: "x"}
	if got := s.MainArtist(); got != "A" {
		t.Errorf("MainArtist = %q", got)
	}
	if got := s.SpotifyURL(); got != "https://open.spotify.com/track/x" {
		t.Errorf("SpotifyURL = %q", got)
	}

	var empty Song
	if empty.MainArtist() != "" || empty.SpotifyURL() != "" {
		t.Errorf("Song vazio deveria dar strings vazias")
	}
}
