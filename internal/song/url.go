package song

import (
	"fmt"
	"net/url"
	"strings"
)

type Kind string

const (
	KindTrack    Kind = "track"
	KindAlbum    Kind = "album"
	KindPlaylist Kind = "playlist"
	KindArtist   Kind = "artist"
	// KindSearch: o ID é o texto da busca.
	KindSearch Kind = "search"
)

type Ref struct {
	Kind Kind
	ID   string
}

func ParseURL(raw string) (Ref, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Ref{}, fmt.Errorf("link vazio")
	}

	var kind, id string
	if strings.HasPrefix(raw, "spotify:") {
		parts := strings.Split(raw, ":")
		if len(parts) != 3 {
			return Ref{}, fmt.Errorf("URI do Spotify inválida: %q", raw)
		}
		kind, id = parts[1], parts[2]
	} else {
		u, err := url.Parse(raw)
		if err != nil {
			return Ref{}, fmt.Errorf("link inválido %q: %w", raw, err)
		}
		if u.Host != "open.spotify.com" {
			return Ref{}, fmt.Errorf("não é um link do Spotify: %q", raw)
		}
		segs := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(segs) > 0 && strings.HasPrefix(segs[0], "intl-") {
			segs = segs[1:]
		}
		if len(segs) > 0 && segs[0] == "embed" {
			segs = segs[1:]
		}
		if len(segs) != 2 {
			return Ref{}, fmt.Errorf("link do Spotify sem tipo/ID: %q", raw)
		}
		kind, id = segs[0], segs[1]
	}

	k := Kind(kind)
	switch k {
	case KindTrack, KindAlbum, KindPlaylist, KindArtist:
	default:
		return Ref{}, fmt.Errorf("tipo %q não suportado (só track, album, playlist e artist)", kind)
	}
	if !isBase62ID(id) {
		return Ref{}, fmt.Errorf("ID do Spotify inválido: %q", id)
	}
	return Ref{Kind: k, ID: id}, nil
}

func isBase62ID(s string) bool {
	if len(s) != 22 {
		return false
	}
	for _, r := range s {
		if !('0' <= r && r <= '9' || 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z') {
			return false
		}
	}
	return true
}

// ParseInput aceita um link ou URI do Spotify; qualquer outro texto vira busca.
func ParseInput(raw string) (Ref, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Ref{}, fmt.Errorf("link vazio")
	}
	if strings.HasPrefix(raw, "spotify:") || strings.Contains(raw, "://") || strings.HasPrefix(raw, "open.spotify.com/") {
		if strings.HasPrefix(raw, "open.spotify.com/") {
			raw = "https://" + raw
		}
		return ParseURL(raw)
	}
	return Ref{Kind: KindSearch, ID: raw}, nil
}
