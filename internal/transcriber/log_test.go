package transcriber

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/harnyk/tran/internal/config"
	"github.com/harnyk/tran/internal/meeting"
	"github.com/harnyk/tran/internal/stt"
)

type fakeProvider struct{}

func (fakeProvider) Transcribe(ctx context.Context, audioPath, language string) (*stt.Result, error) {
	return &stt.Result{Segments: []stt.Segment{{Start: 0, End: 1, Text: "hello " + filepath.Base(audioPath)}}}, nil
}

func TestTranscribeMeetingWithFakeProvider(t *testing.T) {
	m, err := meeting.Init(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"record-001.mp3", "record-002.mp3"} {
		if err := os.WriteFile(filepath.Join(m.MixDir(), name), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	tr := &Transcriber{cfg: &config.Config{STTProvider: "fake"}, provider: fakeProvider{}}
	if err := tr.TranscribeMeeting(context.Background(), m, "en"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(m.TranscriptPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "hello record-002.mp3") {
		t.Fatalf("unexpected transcript:\n%s", data)
	}
}

func TestFormatElapsed(t *testing.T) {
	tests := map[time.Duration]string{
		41*time.Second + 349*time.Millisecond:                "41.3s",
		2*time.Minute + 4*time.Second + 600*time.Millisecond: "2m5s",
	}
	for in, want := range tests {
		if got := formatElapsed(in); got != want {
			t.Errorf("formatElapsed(%v) = %q, want %q", in, got, want)
		}
	}
}
