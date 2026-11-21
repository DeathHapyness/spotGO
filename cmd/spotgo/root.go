package main

import "github.com/spf13/cobra"

// globalOptions guarda as flags que valem para todos os subcomandos.
type globalOptions struct {
	ClientID     string
	ClientSecret string
}

func newRootCmd() *cobra.Command {
	opts := &globalOptions{}

	cmd := &cobra.Command{
		Use:   "spotgo",
		Short: "Baixa músicas do Spotify buscando o áudio no YouTube Music",
		Long: `spotgo baixa faixas, álbuns, playlists e artistas a partir de links do
Spotify. Os metadados vêm da API do Spotify e o áudio vem do YouTube Music.

Na primeira vez, o spotgo pede suas credenciais da API do Spotify
(crie um app grátis em https://developer.spotify.com/dashboard).`,
		Version: version,
		// Erros já são impressos pelo main; sem isso o cobra imprime duas vezes
		// e ainda mostra o texto de uso a cada erro.
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	pf := cmd.PersistentFlags()
	pf.StringVar(&opts.ClientID, "client-id", "", "Client ID da API do Spotify")
	pf.StringVar(&opts.ClientSecret, "client-secret", "", "Client Secret da API do Spotify")

	return cmd
}
