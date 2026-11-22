// Package matching dá uma nota a cada resultado de busca de áudio e escolhe o
// melhor. É um porte de spotdl/utils/matching.py (spotDL 58ced9af).
package matching

import "time"

type Result struct {
	Source   string
	URL      string
	ID       string
	Name     string
	Artists  []string
	Author   string
	Album    string
	Duration time.Duration
	// Verified: resultado do tipo "song" no YouTube Music.
	Verified   bool
	ISRCSearch bool
	// Explicit nil = desconhecido.
	Explicit *bool
	Views    int64
}
