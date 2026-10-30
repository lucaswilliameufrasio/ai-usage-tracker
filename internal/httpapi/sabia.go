package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"ai-usage-tracker/internal/apierr"
	"ai-usage-tracker/internal/sabia"
	"ai-usage-tracker/internal/store"
	"ai-usage-tracker/internal/vault"
)

var (
	applicationIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	confirmationPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,79}$`)
)

func (s *Server) getSabia(w http.ResponseWriter, r *http.Request) {
	body, err := s.sabiaView(r)
	if err != nil {
		s.writeSabia(w, err)
		return
	}
	writeJSON(w, http.StatusOK, body)
}

func (s *Server) putSabia(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Origin        *string `json:"origin"`
		ApplicationID *string `json:"application_id"`
		DeployKey     *string `json:"deploy_key"`
		Template      *struct {
			Name    string `json:"name"`
			Subject string `json:"subject"`
			HTML    string `json:"html"`
			Text    string `json:"text"`
		} `json:"template"`
	}
	if err := s.decode(w, r, &body); err != nil {
		apierr.Write(w, err)
		return
	}
	link, err := s.Store.SabiaLink()
	if err != nil {
		apierr.Write(w, err)
		return
	}
	origin := link.Origin
	applicationID := link.ApplicationID
	var fields []apierr.FieldError
	if body.Origin != nil {
		raw := strings.TrimSpace(*body.Origin)
		if raw == "" {
			origin = ""
		} else {
			normalized, ok := sabia.NormalizeOrigin(raw)
			if !ok {
				fields = append(fields, apierr.FieldError{Field: "origin", Message: "Informe um endereço http ou https do Sabiá, sem usuário, consulta ou fragmento."})
			} else {
				origin = normalized
			}
		}
	}
	if body.ApplicationID != nil {
		raw := strings.TrimSpace(*body.ApplicationID)
		if raw == "" {
			applicationID = ""
		} else if !applicationIDPattern.MatchString(raw) {
			fields = append(fields, apierr.FieldError{Field: "application_id", Message: "O id da aplicação precisa ser um UUID."})
		} else {
			applicationID = raw
		}
	}
	var template *sabia.Template
	if body.Template != nil {
		next := sabia.Template{
			Name:    strings.TrimSpace(body.Template.Name),
			Subject: strings.TrimSpace(body.Template.Subject),
			HTML:    strings.TrimSpace(body.Template.HTML),
			Text:    strings.TrimSpace(body.Template.Text),
		}
		if !sabia.ValidTemplate(next) {
			fields = append(fields, apierr.FieldError{Field: "template", Message: "O template precisa de nome, assunto, HTML e texto, dentro do limite."})
		} else {
			template = &next
		}
	}
	key := ""
	if body.DeployKey != nil {
		key = strings.TrimSpace(*body.DeployKey)
		if len(key) > 8<<10 {
			fields = append(fields, apierr.FieldError{Field: "deploy_key", Message: "A chave passa de 8 KiB."})
		}
	}
	if len(fields) > 0 {
		apierr.Write(w, apierr.Invalid(fields))
		return
	}
	var box vault.Box
	if key != "" {
		box, err = s.openVault()
		if err != nil {
			s.writeSabia(w, err)
			return
		}
	}
	now := s.Now()
	if template != nil {
		if err := s.Store.SaveSabiaTemplate(store.SabiaTemplate{
			Name:    template.Name,
			Subject: template.Subject,
			HTML:    template.HTML,
			Text:    template.Text,
		}, now); err != nil {
			apierr.Write(w, err)
			return
		}
	}
	if body.Origin != nil || body.ApplicationID != nil {
		if err := s.Store.SaveSabiaLink(origin, applicationID, now); err != nil {
			apierr.Write(w, err)
			return
		}
	}
	if key != "" {
		if err := box.Put(r.Context(), sabia.DeploySecret, []byte(key)); err != nil {
			s.writeSabia(w, err)
			return
		}
	}
	view, err := s.sabiaView(r)
	if err != nil {
		s.writeSabia(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) postSabiaPlan(w http.ResponseWriter, r *http.Request) {
	manifest, origin, key, err := s.sabiaCall(r)
	if err != nil {
		s.writeSabia(w, err)
		return
	}
	plan, err := sabia.PlanDeployment(r.Context(), s.SabiaHTTP, origin, key, manifest)
	if err != nil {
		s.writeSabia(w, err)
		return
	}
	changes := make([]map[string]any, 0, len(plan.Changes))
	for _, change := range plan.Changes {
		changes = append(changes, map[string]any{
			"id":                    change.ID,
			"operation":             change.Operation,
			"confirmation_required": change.ConfirmationRequired,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"changes": changes})
}

func (s *Server) postSabiaApply(w http.ResponseWriter, r *http.Request) {
	raw, err := readBody(r)
	if err != nil {
		apierr.Write(w, err)
		return
	}
	var confirmations []string
	if len(bytes.TrimSpace(raw)) > 0 {
		var body struct {
			Confirmations []string `json:"confirmations"`
		}
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&body); err != nil {
			apierr.Write(w, apierr.New(http.StatusBadRequest, apierr.CodeMalformed, "JSON malformado."))
			return
		}
		if len(body.Confirmations) > 8 {
			apierr.Write(w, apierr.Invalid([]apierr.FieldError{{
				Field:   "confirmations",
				Message: "Há confirmações demais.",
			}}))
			return
		}
		for _, item := range body.Confirmations {
			if !confirmationPattern.MatchString(item) {
				apierr.Write(w, apierr.Invalid([]apierr.FieldError{{
					Field:   "confirmations",
					Message: "Confirmação inválida.",
				}}))
				return
			}
		}
		confirmations = body.Confirmations
	}
	manifest, origin, key, err := s.sabiaCall(r)
	if err != nil {
		s.writeSabia(w, err)
		return
	}
	applied, err := sabia.ApplyDeployment(r.Context(), s.SabiaHTTP, origin, key, manifest, confirmations)
	if err != nil {
		s.writeSabia(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"applied":   applied.Applied,
		"unchanged": applied.Unchanged,
	})
}

func (s *Server) sabiaCall(r *http.Request) ([]byte, string, string, error) {
	link, err := s.Store.SabiaLink()
	if err != nil {
		return nil, "", "", err
	}
	if link.Origin == "" {
		return nil, "", "", apierr.Invalid([]apierr.FieldError{{
			Field:   "origin",
			Message: "Informe o endereço do Sabiá.",
		}})
	}
	box, err := s.openVault()
	if err != nil {
		return nil, "", "", err
	}
	secret, err := box.Get(r.Context(), sabia.DeploySecret)
	if errors.Is(err, vault.ErrNotFound) {
		return nil, "", "", apierr.New(http.StatusPreconditionFailed, apierr.CodeVaultUnavailable, "A chave de deploy do Sabiá não está no cofre.")
	}
	if err != nil {
		return nil, "", "", err
	}
	stored, found, err := s.Store.SabiaTemplate()
	if err != nil {
		return nil, "", "", err
	}
	template := sabia.DefaultTemplate()
	if found {
		template = sabia.Template{Name: stored.Name, Subject: stored.Subject, HTML: stored.HTML, Text: stored.Text}
	}
	manifest, err := sabia.Manifest(sabia.Link{Origin: link.Origin, ApplicationID: link.ApplicationID}, template)
	if err != nil {
		return nil, "", "", err
	}
	return manifest, link.Origin, string(secret), nil
}

func (s *Server) sabiaView(r *http.Request) (map[string]any, error) {
	link, err := s.Store.SabiaLink()
	if err != nil {
		return nil, err
	}
	stored, found, err := s.Store.SabiaTemplate()
	if err != nil {
		return nil, err
	}
	template := sabia.DefaultTemplate()
	if found {
		template = sabia.Template{Name: stored.Name, Subject: stored.Subject, HTML: stored.HTML, Text: stored.Text}
	}
	present := false
	if box, err := s.openVault(); err == nil {
		ok, err := box.Present(r.Context(), sabia.DeploySecret)
		if err == nil {
			present = ok
		}
	}
	body := map[string]any{
		"deploy_key_present": present,
		"template": map[string]string{
			"name":    template.Name,
			"subject": template.Subject,
			"html":    template.HTML,
			"text":    template.Text,
		},
	}
	if link.Origin != "" {
		body["origin"] = link.Origin
	}
	if link.ApplicationID != "" {
		body["application_id"] = link.ApplicationID
	}
	return body, nil
}

func (s *Server) writeSabia(w http.ResponseWriter, err error) {
	var call *sabia.CallError
	if errors.As(err, &call) {
		apierr.Write(w, apierr.New(http.StatusBadGateway, apierr.CodeUpstream, call.Error()))
		return
	}
	s.writeAccess(w, err)
}
