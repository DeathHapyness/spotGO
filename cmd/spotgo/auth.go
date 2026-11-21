package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/DeathHapyness/spotGO/internal/config"
)

func newAuthCmd(opts *globalOptions) *cobra.Command {
	var status, clear bool

	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Configura as credenciais da API do Spotify",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := configPath()
			if err != nil {
				return err
			}
			switch {
			case status:
				return authStatus(cmd, opts, path)
			case clear:
				if err := config.Delete(path); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Credenciais apagadas (%s).\n", path)
				return nil
			default:
				return authSetup(cmd, path)
			}
		},
	}

	cmd.Flags().BoolVar(&status, "status", false, "mostra de onde vêm as credenciais (sem exibir o secret)")
	cmd.Flags().BoolVar(&clear, "clear", false, "apaga as credenciais salvas")
	cmd.MarkFlagsMutuallyExclusive("status", "clear")
	return cmd
}

func authSetup(cmd *cobra.Command, path string) error {
	if _, ok := stdinTerminal(cmd); !ok {
		return fmt.Errorf("spotgo auth precisa de um terminal interativo; fora dele use %s e %s",
			config.EnvClientID, config.EnvClientSecret)
	}

	creds, err := config.RunWizard(cmd.Context(), newPrompter(cmd), validateCredentials, isInvalidCredentials)
	if err != nil {
		return err
	}
	if err := config.Save(path, config.Config{Spotify: creds}); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Credenciais salvas em %s\n", path)
	return nil
}

func authStatus(cmd *cobra.Command, opts *globalOptions, path string) error {
	out := cmd.OutOrStdout()
	flags := config.Credentials{ClientID: opts.ClientID, ClientSecret: opts.ClientSecret}

	creds, src, err := config.Resolve(flags, os.Getenv, path)
	if errors.Is(err, config.ErrNoCredentials) {
		fmt.Fprintln(out, err)
		return nil
	}
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "Fonte:     %s\n", src)
	fmt.Fprintf(out, "Client ID: %s\n", config.Mask(creds.ClientID))
	fmt.Fprintf(out, "Arquivo:   %s\n", path)
	return nil
}
