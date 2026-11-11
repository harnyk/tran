package transcriber

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/harnyk/tran/internal/config"
	"github.com/harnyk/tran/internal/meeting"
	"github.com/harnyk/tran/internal/stt"
)

// countingProvider records calls per file and can fail on a given file once.
type countingProvider struct {
	calls  map[string]int
	failOn string
}

func (p *countingProvider) Transcribe(ctx context.Context, audioPath, language string) (*stt.Result, error) {
	name := filepath.Base(audioPath)
	p.calls[name]++
	if name == p.failOn {
		p.failOn = ""
		return nil, errors.New("rate limited")
	}
	return &stt.Result{Segments: []stt.Segment{{Start: 0, End: 1, Text: name}}}, nil
}

func newMeetingWithChunks(t *testing.T, names ...string) *meeting.MeetingDir {
	t.Helper()
	m, err := meeting.Init(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(m.MixDir(), n), []byte("audio"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return m
}

func TestTranscribeMeetingResumesFromCache(t *testing.T) {
	m := newMeetingWithChunks(t, "record-001.mp3", "record-002.mp3", "record-003.mp3")
	p := &countingProvider{calls: map[string]int{}, failOn: "record-002.mp3"}
	tr := &Transcriber{cfg: &config.Config{STTProvider: "fake"}, provider: p}

	if err := tr.TranscribeMeeting(context.Background(), m, "en"); err == nil {
		t.Fatal("first run: expected failure on chunk 2")
	}
	if err := tr.TranscribeMeeting(context.Background(), m, "en"); err != nil {
		t.Fatal(err)
	}

	want := map[string]int{"record-001.mp3": 1, "record-002.mp3": 2, "record-003.mp3": 1}
	for name, n := range want {
		if p.calls[name] != n {
			t.Errorf("%s transcribed %d time(s), want %d", name, p.calls[name], n)
		}
	}
}

func TestChunkCacheInvalidation(t *testing.T) {
	m := newMeetingWithChunks(t, "record-001.mp3")
	p := &countingProvider{calls: map[string]int{}}
	tr := &Transcriber{cfg: &config.Config{STTProvider: "fake"}, provider: p}
	run := func(lang string) {
		t.Helper()
		if err := tr.TranscribeMeeting(context.Background(), m, lang); err != nil {
			t.Fatal(err)
		}
	}

	run("en")
	run("en") // cached
	run("pl") // language changed
	tr.cfg.STTProvider = "other"
	run("pl") // provider changed
	os.WriteFile(filepath.Join(m.MixDir(), "record-001.mp3"), []byte("re-mixed audio"), 0644)
	run("pl") // source changed

	if got := p.calls["record-001.mp3"]; got != 4 {
		t.Fatalf("transcribed %d time(s), want 4", got)
	}
}
