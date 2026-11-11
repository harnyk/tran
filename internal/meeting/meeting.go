package meeting

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var nonAlphanumRe = regexp.MustCompile(`[^a-zA-Z0-9]+`)

type MeetingDir struct {
	Path string
	Name string
	Time time.Time
}

// NewMeetingDir creates a new meeting directory under baseDir with source/, mix/, transcript/ subdirs.
// Name is slugified; if empty, defaults to "meeting".
func NewMeetingDir(baseDir, name string) (*MeetingDir, error) {
	if name == "" {
		name = "meeting"
	}
	slug := slugify(name)
	now := time.Now()
	dirName := fmt.Sprintf("%s--%s", now.Format("2006-01-02-15-04-05"), slug)
	absPath := filepath.Join(baseDir, dirName)
	for _, sub := range []string{"source", "mix", "transcript"} {
		if err := os.MkdirAll(filepath.Join(absPath, sub), 0755); err != nil {
			return nil, fmt.Errorf("create meeting dir: %w", err)
		}
	}
	return &MeetingDir{Path: absPath, Name: slug, Time: now}, nil
}

// Detect finds a meeting directory by looking for a source/ subdirectory in cwd.
func Detect(cwd string) (*MeetingDir, error) {
	src := filepath.Join(cwd, "source")
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return nil, fmt.Errorf("no source/ directory found in %s — run 'tran rec' first", cwd)
	}
	return &MeetingDir{Path: cwd}, nil
}

func (m *MeetingDir) SourceDir() string {
	return filepath.Join(m.Path, "source")
}

func (m *MeetingDir) MixDir() string {
	return filepath.Join(m.Path, "mix")
}

func (m *MeetingDir) TranscriptDir() string {
	return filepath.Join(m.Path, "transcript")
}

func (m *MeetingDir) RecordMP4Path() string {
	return filepath.Join(m.SourceDir(), "record.mp4")
}

func (m *MeetingDir) MicMP3Path() string {
	return filepath.Join(m.SourceDir(), "mic.mp3")
}

func (m *MeetingDir) SysMP3Path() string {
	return filepath.Join(m.SourceDir(), "sys.mp3")
}

// MixMP3Pattern returns the ffmpeg segment output pattern (1-indexed, zero-padded).
func (m *MeetingDir) MixMP3Pattern() string {
	return filepath.Join(m.MixDir(), "record-%03d.mp3")
}

func (m *MeetingDir) TranscriptPath() string {
	return filepath.Join(m.TranscriptDir(), "transcript.txt")
}

// ChunkCacheDir holds per-chunk transcription results used to resume interrupted runs.
func (m *MeetingDir) ChunkCacheDir() string {
	return filepath.Join(m.TranscriptDir(), "chunks")
}

func (m *MeetingDir) TranscriptMicPath() string {
	return filepath.Join(m.TranscriptDir(), "transcript_mic.yaml")
}

func (m *MeetingDir) TranscriptSysPath() string {
	return filepath.Join(m.TranscriptDir(), "transcript_sys.yaml")
}

// ListMixMP3s returns all mix/record-NNN.mp3 files sorted lexicographically.
func (m *MeetingDir) ListMixMP3s() ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(m.MixDir(), "record-*.mp3"))
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	return matches, nil
}

func slugify(name string) string {
	slug := nonAlphanumRe.ReplaceAllString(name, "-")
	slug = strings.Trim(slug, "-")
	return slug
}

// Init prepares an existing directory as a meeting directory by creating
// source/, mix/, and transcript/ inside it. Existing content is left untouched.
func Init(dir string) (*MeetingDir, error) {
	for _, sub := range []string{"source", "mix", "transcript"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0755); err != nil {
			return nil, fmt.Errorf("init meeting dir: %w", err)
		}
	}
	return &MeetingDir{Path: dir, Name: filepath.Base(dir)}, nil
}

// mediaExts lists file extensions accepted as a single audio/video source.
var mediaExts = map[string]bool{
	".mp4": true, ".mkv": true, ".mov": true, ".webm": true, ".avi": true,
	".mp3": true, ".m4a": true, ".wav": true, ".ogg": true, ".opus": true,
	".flac": true, ".aac": true,
}

// HasDualSources reports whether both source/mic.mp3 and source/sys.mp3 exist.
func (m *MeetingDir) HasDualSources() bool {
	return fileExists(m.MicMP3Path()) && fileExists(m.SysMP3Path())
}

// SingleSource returns the only audio/video file in source/.
// It fails if source/ contains no media files or more than one.
func (m *MeetingDir) SingleSource() (string, error) {
	entries, err := os.ReadDir(m.SourceDir())
	if err != nil {
		return "", fmt.Errorf("read source dir: %w", err)
	}
	var found []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || strings.HasPrefix(name, ".") {
			continue
		}
		if mediaExts[strings.ToLower(filepath.Ext(name))] {
			found = append(found, filepath.Join(m.SourceDir(), name))
		}
	}
	sort.Strings(found)
	switch len(found) {
	case 0:
		return "", fmt.Errorf("no audio/video file found in %s — put a recording there, or provide mic.mp3 + sys.mp3", m.SourceDir())
	case 1:
		return found[0], nil
	default:
		names := make([]string, len(found))
		for i, f := range found {
			names[i] = filepath.Base(f)
		}
		return "", fmt.Errorf("multiple audio/video files in %s (%s) — keep exactly one, or provide mic.mp3 + sys.mp3", m.SourceDir(), strings.Join(names, ", "))
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
