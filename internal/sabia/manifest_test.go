package sabia

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestManifestOmitsApplicationWhenEmpty(t *testing.T) {
	raw, err := Manifest(Link{}, DefaultTemplate())
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	templates, _ := body["templates"].([]any)
	if len(templates) != 1 {
		t.Fatal("template")
	}
	item, _ := templates[0].(map[string]any)
	if _, ok := item["application_id"]; !ok {
		t.Fatal("application_id ausente")
	}
	if item["application_id"] != nil {
		t.Fatal("application_id deveria ser null")
	}
	if item["template_key"] != TemplateKey || item["id"] != TemplateID {
		t.Fatal("identidade")
	}
	withID, err := Manifest(Link{ApplicationID: "11111111-1111-1111-1111-111111111111"}, DefaultTemplate())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(withID), "11111111-1111-1111-1111-111111111111") {
		t.Fatal("id da aplicação")
	}
}

func TestTemplateRejectsOversizedHTML(t *testing.T) {
	template := DefaultTemplate()
	template.HTML = strings.Repeat("a", 4001)
	if ValidTemplate(template) {
		t.Fatal("html longo")
	}
	origin, ok := NormalizeOrigin("https://sabia.exemplo/app/")
	if !ok || origin != "https://sabia.exemplo/app" {
		t.Fatalf("origem %q", origin)
	}
	if _, ok := NormalizeOrigin("https://user:pass@sabia.exemplo"); ok {
		t.Fatal("origem com usuário")
	}
}
