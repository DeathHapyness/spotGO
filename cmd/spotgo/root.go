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
Spotify, com capa e metadados. O áudio vem do YouTube Music; nada é baixado do Spotify.

Funciona sem conta e sem credenciais: a lista de faixas vem da página pública
de embed do Spotify e os metadados vêm da API pública do Deezer.
Quem tem Spotify Premium pode usar credenciais próprias (spotgo auth) como modo avançado.`,
		Version:       version,
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	pf := cmd.PersistentFlags()
	pf.StringVar(&opts.ClientID, "client-id", "", "Client ID da API do Spotify")
	pf.StringVar(&opts.ClientSecret, "client-secret", "", "Client Secret da API do Spotify")

	cmd.AddCommand(newAuthCmd(opts))
	cmd.AddCommand(newSaveCmd(func(p func(int, int)) resolver { return newPublicResolver(p) }))

	return cmd
}
