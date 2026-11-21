package config

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolve(t *testing.T) {
	fileCreds := Credentials{ClientID: "file-id", ClientSecret: "file-sec"}

	tests := []struct {
		name       string
		flags      Credentials
		env        map[string]string
		file       *Credentials
		want       Credentials
		wantSource Source
		wantErr    string
		wantNoCred bool
	}{
		{
			name:       "flags ganham de env e arquivo",
			flags:      Credentials{"flag-id", "flag-sec"},
			env:        map[string]string{EnvClientID: "env-id", EnvClientSecret: "env-sec"},
			file:       &fileCreds,
			want:       Credentials{"flag-id", "flag-sec"},
			wantSource: SourceFlags,
		},
		{
			name:       "env ganha do arquivo",
			env:        map[string]string{EnvClientID: "env-id", EnvClientSecret: "env-sec"},
			file:       &fileCreds,
			want:       Credentials{"env-id", "env-sec"},
			wantSource: SourceEnv,
		},
		{
			name:       "só arquivo",
			file:       &fileCreds,
			want:       fileCreds,
			wantSource: SourceFile,
		},
		{
			name:    "flag pela metade",
			flags:   Credentials{ClientID: "flag-id"},
			file:    &fileCreds,
			wantErr: "--client-secret",
		},
		{
			name:    "env pela metade",
			env:     map[string]string{EnvClientID: "env-id"},
			file:    &fileCreds,
			wantErr: EnvClientSecret,
		},
		{
			name:       "espaços e quebra de linha são aparados",
			flags:      Credentials{"  abc\n", "\tdef "},
			want:       Credentials{"abc", "def"},
			wantSource: SourceFlags,
		},
		{
			name:       "só espaços conta como vazio",
			flags:      Credentials{"   ", "  "},
			wantNoCred: true,
		},
		{
			name:       "nada configurado",
			wantNoCred: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if tt.file != nil {
				if err := Save(path, Config{Spotify: *tt.file}); err != nil {
					t.Fatal(err)
				}
			}
			getenv := func(k string) string { return tt.env[k] }

			got, src, err := Resolve(tt.flags, getenv, path)

			switch {
			case tt.wantNoCred:
				if !errors.Is(err, ErrNoCredentials) {
					t.Fatalf("quero ErrNoCredentials, veio %v", err)
				}
			case tt.wantErr != "":
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("quero erro contendo %q, veio %v", tt.wantErr, err)
				}
			default:
				if err != nil {
					t.Fatal(err)
				}
				if got != tt.want || src != tt.wantSource {
					t.Fatalf("Resolve = %+v, %q; quero %+v, %q", got, src, tt.want, tt.wantSource)
				}
			}
		})
	}
}
