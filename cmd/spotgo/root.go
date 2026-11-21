package main

import "github.com/spf13/cobra"

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
		Version:       version,
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	pf := cmd.PersistentFlags()
	pf.StringVar(&opts.ClientID, "client-id", "", "Client ID da API do Spotify")
	pf.StringVar(&opts.ClientSecret, "client-secret", "", "Client Secret da API do Spotify")

	cmd.AddCommand(newAuthCmd(opts))

	return cmd
}
