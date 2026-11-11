package transcriber

import (
	"fmt"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"

	"github.com/harnyk/tran/internal/stt"
)

// chunkCache is the on-disk record of one transcribed chunk, stored in
// transcript/chunks/ so an interrupted run can resume without redoing work.
type chunkCache struct {
	Provider   string          `yaml:"provider"`
	Language   string          `yaml:"language"`
	SourceSize int64           `yaml:"source_size"`
	Segments   []segmentRecord `yaml:"segments"`
}

// cacheKey identifies the inputs a cached chunk was produced from.
type cacheKey struct {
	provider   string
	language   string
	sourceSize int64
}

// loadChunkCache returns the cached result for a chunk, or nil if there is no
// usable cache entry (missing, unreadable, or produced from different inputs).
func loadChunkCache(path string, key cacheKey) *stt.Result {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var c chunkCache
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil
	}
	if c.Provider != key.provider || c.Language != key.language || c.SourceSize != key.sourceSize || len(c.Segments) == 0 {
		return nil
	}
	res := &stt.Result{Segments: make([]stt.Segment, len(c.Segments))}
	for i, s := range c.Segments {
		res.Segments[i] = stt.Segment{Start: s.Start, End: s.End, Text: s.Text}
	}
	return res
}

// saveChunkCache writes a chunk result atomically (temp file + rename).
func saveChunkCache(path string, key cacheKey, res *stt.Result) error {
	c := chunkCache{Provider: key.provider, Language: key.language, SourceSize: key.sourceSize}
	for _, s := range res.Segments {
		c.Segments = append(c.Segments, segmentRecord{Start: s.Start, End: s.End, Text: s.Text})
	}
	data, err := yaml.Marshal(&c)
	if err != nil {
		return fmt.Errorf("marshal chunk cache: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
