package deps

import (
	"errors"
	"strings"
	"testing"
)

const h64 = "e51e69e89489f199530f13d32a4342da3ddb2cf34842f364603a066db05afbf3"

func sumsOf(names ...string) map[string]string {
	m := map[string]string{}
	for _, n := range names {
		m[n] = h64
	}
	return m
}

func TestSourceForYTDLP(t *testing.T) {
	cases := []struct{ goos, goarch, want string }{
		{"linux", "amd64", "yt-dlp_linux"},
		{"linux", "arm64", "yt-dlp_linux_aarch64"},
		{"darwin", "amd64", "yt-dlp_macos"},
		{"darwin", "arm64", "yt-dlp_macos"},
		{"windows", "amd64", "yt-dlp.exe"},
		{"windows", "arm64", "yt-dlp_arm64.exe"},
	}
	all := sumsOf("yt-dlp_linux", "yt-dlp_linux_aarch64", "yt-dlp_macos", "yt-dlp.exe", "yt-dlp_arm64.exe", "yt-dlp")
	for _, c := range cases {
		src, err := SourceFor(YTDLP, c.goos, c.goarch)
		if err != nil {
			t.Fatalf("%s/%s: %v", c.goos, c.goarch, err)
		}
		if !strings.HasSuffix(src.SumsURL, "/SHA2-256SUMS") {
			t.Errorf("SumsURL = %q", src.SumsURL)
		}
		a, err := src.Pick(all)
		if err != nil || a.Name != c.want || a.Archive != Raw || !strings.HasSuffix(a.URL, "/"+c.want) {
			t.Errorf("%s/%s: %+v, %v", c.goos, c.goarch, a, err)
		}
	}
	if _, err := SourceFor(YTDLP, "freebsd", "amd64"); !errors.Is(err, ErrUnsupported) {
		t.Errorf("freebsd: %v", err)
	}
}

func TestSourceForDeno(t *testing.T) {
	src, err := SourceFor(Deno, "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(src.SumsURL, "/deno-x86_64-unknown-linux-gnu.zip.sha256sum") {
		t.Errorf("SumsURL = %q", src.SumsURL)
	}
	a, err := src.Pick(map[string]string{"qualquer-coisa": h64})
	if err != nil || a.Name != "deno-x86_64-unknown-linux-gnu.zip" || a.Archive != Zip || a.Member != "deno" {
		t.Errorf("asset = %+v, %v", a, err)
	}
	w, _ := SourceFor(Deno, "windows", "arm64")
	if a, err := w.Pick(map[string]string{"x": h64}); err != nil || a.Member != "deno.exe" || a.Name != "deno-aarch64-pc-windows-msvc.zip" {
		t.Errorf("windows = %+v, %v", a, err)
	}
	if _, err := SourceFor(Deno, "plan9", "386"); !errors.Is(err, ErrUnsupported) {
		t.Errorf("plan9: %v", err)
	}
}

func TestPickFFmpegHighestStable(t *testing.T) {
	sums := sumsOf(
		"ffmpeg-master-latest-linux64-gpl.tar.xz",
		"ffmpeg-n8.1-latest-linux64-gpl-8.1.tar.xz",
		"ffmpeg-n9.0-latest-linux64-gpl-9.0.tar.xz",
		"ffmpeg-n10.0-latest-linux64-gpl-10.0.tar.xz",
		"ffmpeg-n9.0-latest-linux64-lgpl-9.0.tar.xz",
		"ffmpeg-n11.0-latest-linux64-gpl-shared-11.0.tar.xz",
		"ffmpeg-n12.0-latest-linuxarm64-gpl-12.0.tar.xz",
		"ffmpeg-n9.0-latest-win64-gpl-9.0.zip",
	)
	src, err := SourceFor(FFmpeg, "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	a, err := src.Pick(sums)
	if err != nil || a.Name != "ffmpeg-n10.0-latest-linux64-gpl-10.0.tar.xz" || a.Archive != TarXZ || a.Member != "bin/ffmpeg" {
		t.Fatalf("linux = %+v, %v", a, err)
	}
	w, _ := SourceFor(FFmpeg, "windows", "amd64")
	if a, err := w.Pick(sums); err != nil || a.Name != "ffmpeg-n9.0-latest-win64-gpl-9.0.zip" || a.Archive != Zip || a.Member != "bin/ffmpeg.exe" {
		t.Fatalf("windows = %+v, %v", a, err)
	}
}

func TestPickFFmpegNoStable(t *testing.T) {
	src, _ := SourceFor(FFmpeg, "linux", "amd64")
	if _, err := src.Pick(sumsOf("ffmpeg-master-latest-linux64-gpl.tar.xz")); err == nil || !strings.Contains(err.Error(), "ffmpeg") {
		t.Fatalf("err = %v", err)
	}
	_, err := SourceFor(FFmpeg, "darwin", "arm64")
	if !errors.Is(err, ErrUnsupported) || !strings.Contains(err.Error(), "brew install ffmpeg") {
		t.Fatalf("darwin err = %v", err)
	}
}

func TestParseSums(t *testing.T) {
	data := h64 + "  yt-dlp\n\n" + strings.ToUpper(h64) + " *yt-dlp.exe\n"
	m, err := ParseSums([]byte(data))
	if err != nil || m["yt-dlp"] != h64 || m["yt-dlp.exe"] != h64 || len(m) != 2 {
		t.Fatalf("ParseSums = %v, %v", m, err)
	}
	for _, bad := range []string{"", "\n\n", "abc  yt-dlp\n", h64 + "\n", "zz" + h64[2:] + "  x\n"} {
		if _, err := ParseSums([]byte(bad)); err == nil {
			t.Errorf("ParseSums(%q) aceitou", bad)
		}
	}
}

func TestPickMissingAsset(t *testing.T) {
	src, _ := SourceFor(YTDLP, "linux", "amd64")
	if _, err := src.Pick(sumsOf("yt-dlp")); err == nil || !strings.Contains(err.Error(), "yt-dlp_linux") {
		t.Fatalf("err = %v", err)
	}
}

func TestBinaryName(t *testing.T) {
	if BinaryName(FFmpeg, "windows") != "ffmpeg.exe" || BinaryName(YTDLP, "linux") != "yt-dlp" {
		t.Fatal(BinaryName(FFmpeg, "windows"), BinaryName(YTDLP, "linux"))
	}
}

// O deno publica os checksums do Windows na saída do Get-FileHash do PowerShell.
func TestParseSumsPowerShell(t *testing.T) {
	data := "\r\nAlgorithm : SHA256\r\nHash      : " + strings.ToUpper(h64) + "\r\nPath      : C:\\a\\deno\\deno\\target\\release\\deno-x86_64-pc-windows-msvc.zip\r\n\r\n"
	m, err := ParseSums([]byte(data))
	if err != nil || len(m) != 1 || m["deno-x86_64-pc-windows-msvc.zip"] != h64 {
		t.Fatalf("ParseSums = %v, %v", m, err)
	}
	if _, err := ParseSums([]byte("Algorithm : SHA256\r\nHash      : abc\r\n")); err == nil {
		t.Fatal("hash curto aceito")
	}
}
