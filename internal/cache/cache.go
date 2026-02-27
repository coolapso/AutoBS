package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cfcolaco/autobs/pkg/models"
)

const fileName = ".autobs_cache.json"

// Entry holds the cached LLM summary and source commits for a single ticket.
type Entry struct {
	Body    string          `json:"body"`
	Commits []models.Commit `json:"commits"`
}

// File is the top-level cache structure persisted to disk.
type File struct {
	Date        string           `json:"date"`         // YYYY-MM-DD of the targeted commit date
	GeneratedAt time.Time        `json:"generated_at"` // UTC time the dry-run was produced
	Summaries   map[string]Entry `json:"summaries"`    // ticketID -> Entry
}

func filePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}
	return filepath.Join(home, fileName), nil
}

// Load reads the cache file from disk. Returns (nil, nil) if the file does not exist.
func Load() (*File, error) {
	p, err := filePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading cache: %w", err)
	}
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parsing cache: %w", err)
	}
	return &f, nil
}

// Save writes summaries to the cache file tagged with the given target date.
func Save(date string, summaries map[string]Entry) error {
	p, err := filePath()
	if err != nil {
		return err
	}
	f := File{
		Date:        date,
		GeneratedAt: time.Now().UTC(),
		Summaries:   summaries,
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding cache: %w", err)
	}
	if err := os.WriteFile(p, data, 0600); err != nil {
		return fmt.Errorf("writing cache: %w", err)
	}
	return nil
}

// Delete removes the cache file. It is a no-op if the file does not exist.
func Delete() error {
	p, err := filePath()
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("deleting cache: %w", err)
	}
	return nil
}
