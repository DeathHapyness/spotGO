package ffmpeg

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/DeathHapyness/spotGO/internal/song"
)

var sample = song.Song{
	Title:       "Tí=tle; #1",
	Artists:     []string{"A", "B"},
	Album:       "Alb",
	TrackNumber: 3,
	TrackCount:  10,
	DiscNumber:  1,
	ReleaseDate: "2020-05-01",
	ISRC:        "USABC1234567",
	SpotifyID:   "abc",
}

func TestParseFormat(t *testing.T) {
	if f, err := ParseFormat("FLAC"); err != nil || f != FLAC {
		t.Errorf("ParseFormat(FLAC) = %q, %v", f, err)
	}
	if _, err := ParseFormat("aiff"); err == nil || !strings.Contains(err.Error(), "mp3") {
		t.Errorf("esperava erro listando os formatos: %v", err)
	}
}

func TestMetadata(t *testing.T) {
	got := string(Metadata(Job{Song: sample, Format: MP3}))
	want := ";FFMETADATA1\n" +
		`title=Tí\=tle\; \#1` + "\n" +
		"artist=A, B\nalbum=Alb\nalbum_artist=A\ntrack=3/10\ndisc=1\ndate=2020-05-01\n" +
		"ISRC=USABC1234567\ncomment=https://open.spotify.com/track/abc\n"
	if got != want {
		t.Errorf("Metadata =\n%s\nwant\n%s", got, want)
	}
	if strings.Contains(got, "METADATA_BLOCK_PICTURE") {
		t.Error("mp3 não leva a capa nas tags")
	}
}

func TestMetadataLyrics(t *testing.T) {
	m := string(Metadata(Job{Song: song.Song{Title: "x"}, Format: MP3, Lyrics: "[00:01.00]um\ndois=três"}))
	if !strings.Contains(m, "lyrics=[00:01.00]um\\\ndois\\=três\n") {
		t.Errorf("letra mal escapada:\n%s", m)
	}
}

func TestMetadataPictureBlock(t *testing.T) {
	img := append([]byte("\xff\xd8\xff\xe0"), bytes.Repeat([]byte{1}, 100)...)
	m := string(Metadata(Job{Song: song.Song{Title: "x"}, Format: Opus, Cover: img}))
	_, v, ok := strings.Cut(m, "METADATA_BLOCK_PICTURE=")
	if !ok {
		t.Fatal("opus deve levar a capa nas tags")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(strings.TrimSpace(v), `\=`, "="))
	if err != nil {
		t.Fatal(err)
	}
	if raw[3] != 3 || !bytes.Contains(raw, []byte("image/jpeg")) || !bytes.HasSuffix(raw, img) {
		t.Errorf("bloco PICTURE inválido: %x", raw[:40])
	}
}

func TestArgsCopyOrEncode(t *testing.T) {
	tests := []struct {
		name  string
		job   Job
		codec string
		extra []string
	}{
		{"opus copiado", Job{Format: Opus, InputCodec: "opus"}, "copy", nil},
		{"aac copiado", Job{Format: M4A, InputCodec: "aac"}, "copy", []string{"+faststart"}},
		{"opus para m4a", Job{Format: M4A, InputCodec: "opus"}, "aac", []string{"192k"}},
		{"bitrate força reencodar", Job{Format: Opus, InputCodec: "opus", Bitrate: "96k"}, "libopus", []string{"96k"}},
		{"mp3 V0", Job{Format: MP3}, "libmp3lame", []string{"-q:a", "-id3v2_version"}},
		{"flac ignora bitrate", Job{Format: FLAC, Bitrate: "320k"}, "flac", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := args(tt.job, specs[tt.job.Format], "meta", "", "out")
			i := slices.Index(a, "-c:a")
			if i < 0 || a[i+1] != tt.codec {
				t.Fatalf("codec: %v", a)
			}
			for _, e := range tt.extra {
				if !slices.Contains(a, e) {
					t.Errorf("faltou %q em %v", e, a)
				}
			}
			if tt.job.Format == FLAC && slices.Contains(a, "320k") {
				t.Errorf("flac não usa bitrate: %v", a)
			}
			if a[len(a)-1] != "out" || a[len(a)-3] != "-f" {
				t.Errorf("saída: %v", a)
			}
		})
	}
}

func TestArgsCover(t *testing.T) {
	a := strings.Join(args(Job{Format: MP3}, specs[MP3], "meta", "cover.jpg", "out"), " ")
	if !strings.Contains(a, "-i cover.jpg") || !strings.Contains(a, "-map 2:0") || !strings.Contains(a, "attached_pic") {
		t.Errorf("capa: %s", a)
	}
}

func TestConvertFailureLeavesNothing(t *testing.T) {
	dir := t.TempDir()
	c := &Converter{Run: func(ctx context.Context, path string, args []string) ([]byte, error) {
		out := args[len(args)-1]
		os.WriteFile(out, []byte("meio"), 0o644)
		return []byte("linha 1\nInvalid data found\n"), errors.New("saiu com código 1")
	}}
	err := c.Convert(context.Background(), Job{Input: "in.webm", Output: filepath.Join(dir, "x.mp3"), Format: MP3, Song: sample})
	if err == nil || !strings.Contains(err.Error(), "Invalid data found") {
		t.Fatalf("o erro deve trazer o stderr: %v", err)
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 0 {
		t.Errorf("sobrou arquivo: %v", ents)
	}
}

func TestConvertCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := &Converter{Run: func(ctx context.Context, path string, args []string) ([]byte, error) {
		return nil, errors.New("signal: killed")
	}}
	err := c.Convert(ctx, Job{Input: "in", Output: filepath.Join(t.TempDir(), "x.mp3"), Format: MP3})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v", err)
	}
}

// Os testes abaixo rodam o ffmpeg de verdade, se estiver instalado.

func needFFmpeg(t *testing.T) {
	t.Helper()
	for _, b := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(b); err != nil {
			t.Skipf("%s não está no PATH", b)
		}
	}
}

func gen(t *testing.T, dir string) (audio string, cover []byte) {
	t.Helper()
	audio = filepath.Join(dir, "in.webm")
	jpg := filepath.Join(dir, "cover.jpg")
	for _, a := range [][]string{
		{"-f", "lavfi", "-i", "sine=d=1", "-c:a", "libopus", audio},
		{"-f", "lavfi", "-i", "color=red:s=32x32", "-frames:v", "1", jpg},
	} {
		if out, err := exec.Command("ffmpeg", append([]string{"-v", "error", "-y"}, a...)...).CombinedOutput(); err != nil {
			t.Fatalf("gerando fixture: %v\n%s", err, out)
		}
	}
	cover, err := os.ReadFile(jpg)
	if err != nil {
		t.Fatal(err)
	}
	return audio, cover
}

type probe struct {
	Streams []struct {
		CodecName   string            `json:"codec_name"`
		CodecType   string            `json:"codec_type"`
		Tags        map[string]string `json:"tags"`
		Disposition map[string]int    `json:"disposition"`
	} `json:"streams"`
	Format struct {
		Tags map[string]string `json:"tags"`
	} `json:"format"`
}

func ffprobe(t *testing.T, path string) probe {
	t.Helper()
	out, err := exec.Command("ffprobe", "-v", "error", "-show_streams", "-show_format", "-of", "json", path).Output()
	if err != nil {
		t.Fatalf("ffprobe %s: %v", path, err)
	}
	var p probe
	if err := json.Unmarshal(out, &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestConvertReal(t *testing.T) {
	needFFmpeg(t)
	dir := t.TempDir()
	audio, cover := gen(t, dir)
	s := sample
	s.Duration = time.Second

	for _, f := range Formats {
		t.Run(string(f), func(t *testing.T) {
			out := filepath.Join(dir, "sub", "x."+string(f))
			job := Job{Input: audio, InputCodec: "opus", Output: out, Format: f, Song: s, Cover: cover, Lyrics: "[00:01.00]um\ndois"}
			if err := (&Converter{}).Convert(context.Background(), job); err != nil {
				t.Fatal(err)
			}
			p := ffprobe(t, out)
			tags := map[string]string{}
			for k, v := range p.Format.Tags {
				tags[strings.ToLower(k)] = v
			}
			for _, st := range p.Streams {
				if st.CodecType == "audio" {
					for k, v := range st.Tags {
						tags[strings.ToLower(k)] = v
					}
				}
			}
			if tags["title"] != s.Title || tags["artist"] != "A, B" {
				t.Errorf("tags: %v", tags)
			}
			if _, ok := tags["duration"]; ok && f == Opus {
				t.Errorf("tags da entrada vazaram para a saída: %v", tags)
			}
			if f != WAV && tags["lyrics"] != "[00:01.00]um\ndois" {
				t.Errorf("letra: %q", tags["lyrics"])
			}
			if f != WAV && f != M4A && tags["isrc"] != s.ISRC {
				t.Errorf("ISRC: %v", tags)
			}
			hasCover := false
			for _, st := range p.Streams {
				if st.Disposition["attached_pic"] == 1 || st.CodecType == "video" {
					hasCover = true
				}
			}
			if f != WAV && !hasCover {
				t.Errorf("sem capa: %+v", p.Streams)
			}
			if st, err := os.Stat(out); err != nil || st.Mode().Perm() != 0o644 {
				t.Errorf("permissão: %v %v", st.Mode(), err)
			}
			if ents, _ := os.ReadDir(filepath.Dir(out)); len(ents) != 1 {
				t.Errorf("sobrou temporário: %v", ents)
			}
			os.Remove(out)
		})
	}
}

func TestConvertRealBadInput(t *testing.T) {
	needFFmpeg(t)
	dir := t.TempDir()
	in := filepath.Join(dir, "lixo.webm")
	os.WriteFile(in, []byte("não é áudio"), 0o644)
	out := filepath.Join(dir, "out", "x.mp3")
	err := (&Converter{}).Convert(context.Background(), Job{Input: in, Output: out, Format: MP3})
	if err == nil || !strings.Contains(err.Error(), "lixo.webm") {
		t.Fatalf("err = %v", err)
	}
	if ents, _ := os.ReadDir(filepath.Dir(out)); len(ents) != 0 {
		t.Errorf("sobrou arquivo: %v", ents)
	}
}
