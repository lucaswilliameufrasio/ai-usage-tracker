package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigRoundTripOmitsTheToken(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.json")
	file := File{
		API:        "http://127.0.0.1:8787",
		TokenFile:  filepath.Join(dir, "device.token"),
		Spool:      filepath.Join(dir, "spool.json"),
		ClaudeFile: filepath.Join(dir, ".claude.json"),
		CodexDir:   filepath.Join(dir, "sessions"),
	}
	if err := Save(path, file); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("modo %o", info.Mode().Perm())
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded != file || !loaded.Ready() {
		t.Fatalf("%+v", loaded)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "segredo") || strings.Contains(string(raw), "auto_update") {
		t.Fatal("configuração guardou segredo ou opção desligada")
	}
	file.AutoUpdate = true
	if err := Save(path, file); err != nil {
		t.Fatal(err)
	}
	loaded, err = Load(path)
	if err != nil || !loaded.AutoUpdate {
		t.Fatal("auto_update")
	}
	if err := os.WriteFile(path, []byte("{\"api\":\"http://127.0.0.1:8787\",\"token\":\"segredo\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("aceitou campo desconhecido")
	}
}
