package meeting

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, nil, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestInitIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	if _, err := Init(dir); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(dir, "source", "call.mp4")
	touch(t, keep)
	if _, err := Init(dir); err != nil {
		t.Fatal(err)
	}
	for _, sub := range []string{"source", "mix", "transcript"} {
		if info, err := os.Stat(filepath.Join(dir, sub)); err != nil || !info.IsDir() {
			t.Fatalf("%s/ not created", sub)
		}
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("existing file removed: %v", err)
	}
	if _, err := Detect(dir); err != nil {
		t.Fatalf("Detect after Init: %v", err)
	}
}

func TestSingleSource(t *testing.T) {
	tests := []struct {
		name    string
		files   []string
		want    string
		wantErr string
	}{
		{name: "empty", wantErr: "no audio/video file"},
		{name: "one video", files: []string{"Zoom Call.MP4"}, want: "Zoom Call.MP4"},
		{name: "non-media ignored", files: []string{"notes.txt", ".DS_Store", "rec.m4a"}, want: "rec.m4a"},
		{name: "ambiguous", files: []string{"a.mp4", "b.wav"}, wantErr: "a.mp4, b.wav"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := Init(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			for _, f := range tt.files {
				touch(t, filepath.Join(m.SourceDir(), f))
			}
			got, err := m.SingleSource()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if filepath.Base(got) != tt.want {
				t.Fatalf("got %q, want %q", filepath.Base(got), tt.want)
			}
		})
	}
}

func TestHasDualSources(t *testing.T) {
	m, err := Init(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	touch(t, m.MicMP3Path())
	if m.HasDualSources() {
		t.Fatal("only mic.mp3 present, want false")
	}
	touch(t, m.SysMP3Path())
	if !m.HasDualSources() {
		t.Fatal("mic.mp3 + sys.mp3 present, want true")
	}
}
