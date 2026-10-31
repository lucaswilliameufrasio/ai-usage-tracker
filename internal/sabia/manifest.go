package sabia

import (
	"bytes"
	"encoding/json"
	"net/url"
	"strings"
	"unicode/utf8"
)

const (
	TemplateID   = "usage-alert-v1"
	TemplateKey  = "usage.alert"
	DeploySecret = "sabia.deploy"
	SendSecret   = "sabia.send"
	maxField     = 4000
)

type Template struct {
	Name    string
	Subject string
	HTML    string
	Text    string
}

type Link struct {
	Origin        string
	ApplicationID string
}

func DefaultTemplate() Template {
	return Template{
		Name:    "Alerta de uso",
		Subject: "Uso de {{provider}} em {{used}}%",
		HTML:    "<p>{{provider}}: {{used}}% na janela {{window}}. Reinicia em {{resets_at}}.</p>",
		Text:    "{{provider}}: {{used}}% na janela {{window}}. Reinicia em {{resets_at}}.",
	}
}

func NormalizeOrigin(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" || parsed.User != nil {
		return "", false
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", false
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed.String(), true
}

func ValidTemplate(template Template) bool {
	if strings.TrimSpace(template.Name) == "" || strings.TrimSpace(template.Subject) == "" {
		return false
	}
	if strings.TrimSpace(template.HTML) == "" || strings.TrimSpace(template.Text) == "" {
		return false
	}
	if utf8.RuneCountInString(template.Name) > 80 {
		return false
	}
	if utf8.RuneCountInString(template.Subject) > maxField || utf8.RuneCountInString(template.HTML) > maxField || utf8.RuneCountInString(template.Text) > maxField {
		return false
	}
	return true
}

func Manifest(link Link, template Template) ([]byte, error) {
	var application any
	if strings.TrimSpace(link.ApplicationID) == "" {
		application = nil
	} else {
		application = strings.TrimSpace(link.ApplicationID)
	}
	body := map[string]any{
		"version": 1,
		"templates": []map[string]any{
			{
				"id":               TemplateID,
				"application_id":   application,
				"template_key":     TemplateKey,
				"name":             strings.TrimSpace(template.Name),
				"channel":          "email",
				"locale":           "pt-BR",
				"editor_mode":      "html",
				"subject_template": template.Subject,
				"html_source":      template.HTML,
				"text_source":      template.Text,
				"variables": []map[string]any{
					{"key": "provider", "label": "Provedor", "type": "string", "required": true, "example": "Claude"},
					{"key": "window", "label": "Janela", "type": "string", "required": true, "example": "5 horas"},
					{"key": "used", "label": "Percentual", "type": "string", "required": true, "example": "80"},
					{"key": "resets_at", "label": "Reinício", "type": "string", "required": true, "example": "30/10/2026 18:00"},
				},
				"preview_data": map[string]string{
					"provider":  "Claude",
					"window":    "5 horas",
					"used":      "80",
					"resets_at": "30/10/2026 18:00",
				},
			},
		},
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(body); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
