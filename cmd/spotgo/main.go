// Command spotgo baixa músicas a partir de links do Spotify, buscando o
// áudio no YouTube Music.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
)

// version é sobrescrita no build de release:
//
//	go build -ldflags "-X main.version=v0.1.0" ./cmd/spotgo
var version = "dev"

func main() {
	// Ctrl+C cancela o contexto; todo código que recebe ctx deve parar.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := newRootCmd().ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
}
