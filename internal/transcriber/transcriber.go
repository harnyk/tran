package transcriber

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
	"golang.org/x/text/language"

	"github.com/harnyk/tran/internal/config"
	"github.com/harnyk/tran/internal/converter"
	"github.com/harnyk/tran/internal/format"
	"github.com/harnyk/tran/internal/meeting"
	"github.com/harnyk/tran/internal/stt"
)

type Transcriber struct {
	cfg      *config.Config
	provider stt.Provider
}

func New(cfg *config.Config) (*Transcriber, error) {
	provider, err := stt.NewProvider(cfg)
	if err != nil {
		return nil, err
	}
	return &Transcriber{cfg: cfg, provider: provider}, nil
}

// TranscribeMeeting transcribes all MP3 chunks in a meeting dir and writes transcript.txt.
func (t *Transcriber) TranscribeMeeting(ctx context.Context, m *meeting.MeetingDir, lang string) error {
	apiLanguage, err := NormalizeLanguage(lang)
	if err != nil {
		return err
	}

	mp3s, err := m.ListMixMP3s()
	if err != nil {
		return err
	}
	if len(mp3s) == 0 {
		return fmt.Errorf("no MP3 files found — run 'tran mp3' first")
	}

	fmt.Printf("Transcribing %d chunk(s) with %s...\n", len(mp3s), t.cfg.STTProvider)
	started := time.Now()

	var sb strings.Builder
	fmt.Fprintf(&sb, "---\nmeeting: %s\n---\n\n", filepath.Base(m.Path))
	for i, path := range mp3s {
		chunkNum := i + 1
		result, err := t.transcribeChunk(ctx, path, apiLanguage, "", chunkNum, len(mp3s))
		if err != nil {
			return fmt.Errorf("chunk %03d: %w", chunkNum, err)
		}

		if i > 0 {
			sb.WriteString("\n---\n\n")
		}
		fmt.Fprintf(&sb, "## Chunk %03d\n\n", chunkNum)
		for _, seg := range result.Segments {
			fmt.Fprintf(&sb, "[%s] %s\n", format.Timestamp(seg.Start), strings.TrimSpace(seg.Text))
		}
	}

	fmt.Printf("Transcribed %d chunk(s) in %s\n", len(mp3s), formatElapsed(time.Since(started)))

	if err := os.MkdirAll(m.TranscriptDir(), 0755); err != nil {
		return err
	}
	return os.WriteFile(m.TranscriptPath(), []byte(sb.String()), 0644)
}

func NormalizeLanguage(lang string) (string, error) {
	if lang == "auto" {
		return "", nil
	}

	base, err := language.ParseBase(lang)
	if err != nil {
		return "", fmt.Errorf("invalid --lang %q: use a valid ISO 639 language code (2 or 3 letters) like \"en\", \"eng\", or \"pol\", or \"auto\"", lang)
	}

	return base.String(), nil
}

func (t *Transcriber) transcribeFile(ctx context.Context, audioPath string, language string) (*stt.Result, error) {
	result, err := t.provider.Transcribe(ctx, audioPath, language)
	if err != nil {
		return nil, err
	}
	if len(result.Segments) == 0 {
		return nil, fmt.Errorf("no segments in transcription response")
	}
	return result, nil
}

// transcribeChunk transcribes one chunk and logs its start, duration, and outcome.
// prefix (e.g. "mic") is prepended to log lines; empty means no prefix.
func (t *Transcriber) transcribeChunk(ctx context.Context, path, language, prefix string, n, total int) (*stt.Result, error) {
	label := fmt.Sprintf("[%d/%d] %s", n, total, filepath.Base(path))
	if prefix != "" {
		label = fmt.Sprintf("%-4s %s", prefix, label)
	}

	size := ""
	if info, err := os.Stat(path); err == nil {
		size = fmt.Sprintf(" (%.1f MB)", float64(info.Size())/1024/1024)
	}
	fmt.Printf("  %s%s started\n", label, size)

	started := time.Now()
	result, err := t.transcribeFile(ctx, path, language)
	elapsed := formatElapsed(time.Since(started))
	if err != nil {
		fmt.Printf("  %s failed after %s\n", label, elapsed)
		return nil, err
	}
	fmt.Printf("  %s done in %s (%d segments)\n", label, elapsed, len(result.Segments))
	return result, nil
}

// formatElapsed rounds a duration for human-readable logs (e.g. 41.3s, 2m4s).
func formatElapsed(d time.Duration) string {
	if d < time.Minute {
		return d.Round(100 * time.Millisecond).String()
	}
	return d.Round(time.Second).String()
}

// channelTranscript is the YAML schema for intermediate per-channel transcript files.
type channelTranscript struct {
	Meeting string        `yaml:"meeting"`
	Channel string        `yaml:"channel"`
	Chunks  []chunkRecord `yaml:"chunks"`
}

type chunkRecord struct {
	Index    int             `yaml:"index"`
	Offset   float64         `yaml:"offset"`
	Segments []segmentRecord `yaml:"segments"`
}

type segmentRecord struct {
	Start float64 `yaml:"start"`
	End   float64 `yaml:"end"`
	Text  string  `yaml:"text"`
}

// segmentToTempDir splits audioPath into SegmentTime-second chunks in a new temp directory.
// The caller must defer os.RemoveAll(tempDir) to clean up.
func (t *Transcriber) segmentToTempDir(ctx context.Context, audioPath string) (chunks []string, tempDir string, err error) {
	tempDir, err = os.MkdirTemp("", "tran-seg-*")
	if err != nil {
		return nil, "", err
	}

	pattern := filepath.Join(tempDir, "chunk-%03d.mp3")
	cmd := exec.CommandContext(ctx, t.cfg.FFmpegBin,
		"-i", audioPath,
		"-f", "segment",
		"-segment_time", fmt.Sprintf("%d", converter.SegmentTime),
		"-segment_start_number", "1",
		"-y",
		pattern,
	)
	var ffmpegOutput bytes.Buffer
	cmd.Stdout = &ffmpegOutput
	cmd.Stderr = &ffmpegOutput
	if err := cmd.Run(); err != nil {
		os.RemoveAll(tempDir)
		return nil, "", fmt.Errorf("segment audio: %w\n%s", err, ffmpegOutput.String())
	}

	matches, err := filepath.Glob(filepath.Join(tempDir, "chunk-*.mp3"))
	if err != nil {
		os.RemoveAll(tempDir)
		return nil, "", err
	}
	sort.Strings(matches)
	return matches, tempDir, nil
}

// TranscribeChannel transcribes a single audio file (e.g. source/mic.mp3) and writes
// a YAML intermediate transcript to outputPath.
// Offset is computed as float64(chunkIndex) * SegmentTime — no dependency on Whisper's
// reported duration, which can be unreliable for silent or very short chunks.
func (t *Transcriber) TranscribeChannel(ctx context.Context, audioPath, lang, outputPath, channel, meetingName string) error {
	fmt.Printf("  %-4s splitting %s into chunks...\n", channel, filepath.Base(audioPath))
	segStarted := time.Now()
	chunks, tempDir, err := t.segmentToTempDir(ctx, audioPath)
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)
	fmt.Printf("  %-4s %d chunk(s) ready in %s\n", channel, len(chunks), formatElapsed(time.Since(segStarted)))

	if len(chunks) == 0 {
		return fmt.Errorf("no segments produced from %s", audioPath)
	}

	ct := channelTranscript{
		Meeting: meetingName,
		Channel: channel,
	}

	for i, chunk := range chunks {
		chunkNum := i + 1
		offset := float64(i) * converter.SegmentTime
		result, err := t.transcribeChunk(ctx, chunk, lang, channel, chunkNum, len(chunks))
		if err != nil {
			return fmt.Errorf("chunk %03d: %w", chunkNum, err)
		}

		rec := chunkRecord{Index: chunkNum, Offset: offset}
		for _, seg := range result.Segments {
			rec.Segments = append(rec.Segments, segmentRecord{
				Start: seg.Start,
				End:   seg.End,
				Text:  strings.TrimSpace(seg.Text),
			})
		}
		ct.Chunks = append(ct.Chunks, rec)
	}

	data, err := yaml.Marshal(&ct)
	if err != nil {
		return fmt.Errorf("marshal yaml: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(outputPath, data, 0644)
}

type mergeSegment struct {
	absSeconds float64
	speaker    string
	text       string
}

// readChannelTranscript reads a YAML intermediate file and returns merge-ready segments.
func readChannelTranscript(path, speaker string) ([]mergeSegment, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var ct channelTranscript
	if err := yaml.Unmarshal(data, &ct); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	var segs []mergeSegment
	for _, chunk := range ct.Chunks {
		for _, seg := range chunk.Segments {
			segs = append(segs, mergeSegment{
				absSeconds: chunk.Offset + seg.Start,
				speaker:    speaker,
				text:       seg.Text,
			})
		}
	}
	return segs, nil
}

// MergeChannelTranscripts combines intermediate per-channel YAML transcripts into a single
// transcript.txt with Us/Them speaker labels sorted by timestamp.
func (t *Transcriber) MergeChannelTranscripts(micPath, sysPath, outputPath, meetingName string) error {
	micSegs, err := readChannelTranscript(micPath, "Us")
	if err != nil {
		return fmt.Errorf("read mic transcript: %w", err)
	}
	sysSegs, err := readChannelTranscript(sysPath, "Them")
	if err != nil {
		return fmt.Errorf("read sys transcript: %w", err)
	}

	all := append(micSegs, sysSegs...)
	sort.SliceStable(all, func(i, j int) bool {
		return all[i].absSeconds < all[j].absSeconds
	})

	var sb strings.Builder
	fmt.Fprintf(&sb, "---\nmeeting: %s\n---\n\n", meetingName)
	for _, seg := range all {
		fmt.Fprintf(&sb, "[%s] %s: %s\n", format.Timestamp(seg.absSeconds), seg.speaker, seg.text)
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(outputPath, []byte(sb.String()), 0644)
}

// TranscribeMeetingDualChannel transcribes mic and sys channels separately, writes
// intermediate transcript_mic.yaml and transcript_sys.yaml, then merges into transcript.txt.
// If an intermediate file already exists it is reused (skips re-transcription).
func (t *Transcriber) TranscribeMeetingDualChannel(ctx context.Context, m *meeting.MeetingDir, lang string) error {
	apiLanguage, err := NormalizeLanguage(lang)
	if err != nil {
		return err
	}

	meetingName := filepath.Base(m.Path)
	started := time.Now()

	if !m.HasDualSources() {
		return fmt.Errorf("dual-channel mode needs source/mic.mp3 and source/sys.mp3; a single recording cannot be split into Us/Them — run without --dual-channel")
	}

	fmt.Printf("Transcribing mic and sys channels with %s...\n", t.cfg.STTProvider)

	if _, err := os.Stat(m.TranscriptMicPath()); err == nil {
		fmt.Printf("  mic  reusing existing %s\n", filepath.Base(m.TranscriptMicPath()))
	} else if os.IsNotExist(err) {
		if err := t.TranscribeChannel(ctx, m.MicMP3Path(), apiLanguage, m.TranscriptMicPath(), "mic", meetingName); err != nil {
			return fmt.Errorf("mic channel: %w", err)
		}
	}

	if _, err := os.Stat(m.TranscriptSysPath()); err == nil {
		fmt.Printf("  sys  reusing existing %s\n", filepath.Base(m.TranscriptSysPath()))
	} else if os.IsNotExist(err) {
		if err := t.TranscribeChannel(ctx, m.SysMP3Path(), apiLanguage, m.TranscriptSysPath(), "sys", meetingName); err != nil {
			return fmt.Errorf("sys channel: %w", err)
		}
	}

	fmt.Println("  merging channels")
	if err := t.MergeChannelTranscripts(m.TranscriptMicPath(), m.TranscriptSysPath(), m.TranscriptPath(), meetingName); err != nil {
		return err
	}
	fmt.Printf("Transcribed both channels in %s\n", formatElapsed(time.Since(started)))
	return nil
}
