package agent

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
)

type File struct {
	API        string `json:"api"`
	TokenFile  string `json:"token_file"`
	Spool      string `json:"spool"`
	ClaudeFile string `json:"claude_file,omitempty"`
	CodexDir   string `json:"codex_dir,omitempty"`
	AutoUpdate bool   `json:"auto_update,omitempty"`
}

type Paths struct {
	Config string
	Token  string
	Spool  string
}

func (file File) Ready() bool {
	if file.API == "" || file.TokenFile == "" || file.Spool == "" {
		return false
	}
	return file.ClaudeFile != "" || file.CodexDir != ""
}

func DefaultPaths() (Paths, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return Paths{}, err
	}
	base := filepath.Join(dir, "ai-usage-tracker")
	return Paths{
		Config: filepath.Join(base, "agent.json"),
		Token:  filepath.Join(base, "device.token"),
		Spool:  filepath.Join(base, "spool.json"),
	}, nil
}

func Load(path string) (File, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return File{}, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var file File
	if err := dec.Decode(&file); err != nil {
		return File{}, err
	}
	return file, nil
}

func Save(path string, file File) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(path, raw, 0o600)
}
