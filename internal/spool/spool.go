package spool

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const MaxItems = 100

const (
	KindSample    = "sample"
	KindReport    = "report"
	StatePending  = "pending"
	StateBlocked  = "blocked_revoked"
	StateRejected = "rejected"
)

var ErrFull = errors.New("spool full")

type Item struct {
	Kind  string          `json:"kind"`
	State string          `json:"state"`
	Body  json.RawMessage `json:"body"`
}

func Load(path string) ([]Item, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, nil
	}
	var items []Item
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func Save(path string, items []Item) error {
	if items == nil {
		items = []Item{}
	}
	raw, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, "spool-*.json")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	_, writeErr := temp.Write(raw)
	closeErr := temp.Close()
	if writeErr != nil {
		_ = os.Remove(tempName)
		return writeErr
	}
	if closeErr != nil {
		_ = os.Remove(tempName)
		return closeErr
	}
	if err := os.Chmod(tempName, 0o600); err != nil {
		_ = os.Remove(tempName)
		return err
	}
	return os.Rename(tempName, path)
}

func Merge(path string, incoming []Item) ([]Item, error) {
	items, err := Load(path)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	for _, item := range items {
		seen[Identity(item)] = struct{}{}
	}
	for _, item := range incoming {
		id := Identity(item)
		if _, ok := seen[id]; ok {
			continue
		}
		if len(items) >= MaxItems {
			return nil, ErrFull
		}
		items = append(items, item)
		seen[id] = struct{}{}
	}
	if err := Save(path, items); err != nil {
		return nil, err
	}
	return items, nil
}

func Identity(item Item) string {
	var probe struct {
		SampleID string `json:"sample_id"`
		ReportID string `json:"report_id"`
	}
	if err := json.Unmarshal(item.Body, &probe); err != nil {
		return ""
	}
	if probe.SampleID != "" {
		return probe.SampleID
	}
	return probe.ReportID
}

func HasBlocked(items []Item) bool {
	for _, item := range items {
		if item.State == StateBlocked {
			return true
		}
	}
	return false
}
