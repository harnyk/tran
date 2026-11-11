package converter

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/harnyk/tran/internal/meeting"
)

func TestBuildInputArgs(t *testing.T) {
	t.Run("dual sources are mixed", func(t *testing.T) {
		m, _ := meeting.Init(t.TempDir())
		for _, p := range []string{m.MicMP3Path(), m.SysMP3Path(), m.RecordMP4Path()} {
			os.WriteFile(p, nil, 0644)
		}
		args, err := buildInputArgs(m, Options{MicVolumeDB: 3})
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(args, "-filter_complex") || !slices.Contains(args, m.SysMP3Path()) {
			t.Fatalf("expected mix args, got %v", args)
		}
	})

	t.Run("single source uses its audio track", func(t *testing.T) {
		m, _ := meeting.Init(t.TempDir())
		src := filepath.Join(m.SourceDir(), "call.mkv")
		os.WriteFile(src, nil, 0644)
		args, err := buildInputArgs(m, Options{})
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"-i", src, "-map", "0:a:0", "-vn"}
		if !slices.Equal(args, want) {
			t.Fatalf("got %v, want %v", args, want)
		}
	})

	t.Run("no source fails", func(t *testing.T) {
		m, _ := meeting.Init(t.TempDir())
		if _, err := buildInputArgs(m, Options{}); err == nil {
			t.Fatal("expected error")
		}
	})
}
