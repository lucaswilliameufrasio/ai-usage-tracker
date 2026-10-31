package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"ai-usage-tracker/internal/access"
	"ai-usage-tracker/internal/apierr"
	"ai-usage-tracker/internal/mail"
	"ai-usage-tracker/internal/sabia"
	"ai-usage-tracker/internal/store"
	"ai-usage-tracker/internal/vault"
)

var errVaultClosed = errors.New("vault closed")

func (s *Server) getSecretBackend(w http.ResponseWriter, _ *http.Request) {
	s.writeBackend(w)
}

func (s *Server) putSecretBackend(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Kind string `json:"kind"`
	}
	if err := s.decode(w, r, &body); err != nil {
		apierr.Write(w, err)
		return
	}
	kind := strings.TrimSpace(body.Kind)
	if !access.KnownBackend(kind) {
		apierr.Write(w, apierr.Invalid([]apierr.FieldError{{
			Field:   "kind",
			Message: "Cofre desconhecido.",
		}}))
		return
	}
	if s.boxFor(kind) == nil {
		apierr.Write(w, apierr.Invalid([]apierr.FieldError{{
			Field:   "kind",
			Message: "Este cofre não está aberto neste processo.",
		}}))
		return
	}
	current, err := s.Store.SecretBackend()
	if err != nil {
		apierr.Write(w, err)
		return
	}
	if current != kind {
		currentBox := s.boxFor(current)
		if currentBox == nil {
			apierr.Write(w, apierr.New(http.StatusConflict, apierr.CodeVaultSwitch, "O cofre atual não está aberto, então a troca não dá para conferir."))
			return
		}
		occupied, occupyErr := backendOccupied(r.Context(), currentBox)
		if occupyErr != nil {
			s.writeAccess(w, occupyErr)
			return
		}
		if occupied {
			apierr.Write(w, apierr.New(http.StatusConflict, apierr.CodeVaultSwitch, "Já existe segredo neste cofre. A troca é uma migração e esta versão não copia sozinha."))
			return
		}
		if err := s.Store.SetSecretBackend(kind); err != nil {
			apierr.Write(w, err)
			return
		}
	}
	s.writeBackend(w)
}

func (s *Server) listLoginProviders(w http.ResponseWriter, r *http.Request) {
	box, err := s.openVault()
	if err != nil {
		s.writeAccess(w, err)
		return
	}
	views, err := access.Catalog(r.Context(), s.Store, box)
	if err != nil {
		s.writeAccess(w, err)
		return
	}
	data := make([]map[string]any, 0, len(views))
	for _, view := range views {
		data = append(data, loginJSON(view))
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": data})
}

func (s *Server) putLoginProvider(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	if !access.Known(provider) {
		apierr.Write(w, apierr.New(http.StatusNotFound, apierr.CodeNotFound, "Provedor de login desconhecido."))
		return
	}
	var body struct {
		Enabled    *bool   `json:"enabled"`
		ClientID   string  `json:"client_id"`
		IssuerURL  string  `json:"issuer_url"`
		TeamID     string  `json:"team_id"`
		KeyID      string  `json:"key_id"`
		ServicesID string  `json:"services_id"`
		Secret     *string `json:"secret"`
	}
	if err := s.decode(w, r, &body); err != nil {
		apierr.Write(w, err)
		return
	}
	if body.Enabled == nil {
		apierr.Write(w, apierr.Invalid([]apierr.FieldError{{
			Field:   "enabled",
			Message: "Informe se o login fica ativo.",
		}}))
		return
	}
	box, err := s.openVault()
	if err != nil {
		s.writeAccess(w, err)
		return
	}
	draft := access.Draft{
		Provider:   provider,
		Enabled:    *body.Enabled,
		ClientID:   body.ClientID,
		IssuerURL:  body.IssuerURL,
		TeamID:     body.TeamID,
		KeyID:      body.KeyID,
		ServicesID: body.ServicesID,
		Now:        s.Now(),
	}
	if body.Secret != nil {
		secret := []byte(*body.Secret)
		draft.Secret = &secret
	}
	if err := access.Save(r.Context(), s.Store, box, draft); err != nil {
		s.writeAccess(w, err)
		return
	}
	s.writeOneLogin(w, r, provider)
}

func (s *Server) deleteLoginProvider(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	if !access.Known(provider) {
		apierr.Write(w, apierr.New(http.StatusNotFound, apierr.CodeNotFound, "Provedor de login desconhecido."))
		return
	}
	box, err := s.openVault()
	if err != nil {
		s.writeAccess(w, err)
		return
	}
	if err := access.Remove(r.Context(), s.Store, box, provider); err != nil {
		s.writeAccess(w, err)
		return
	}
	s.writeOneLogin(w, r, provider)
}

func (s *Server) writeOneLogin(w http.ResponseWriter, r *http.Request, provider string) {
	box, err := s.openVault()
	if err != nil {
		s.writeAccess(w, err)
		return
	}
	views, err := access.Catalog(r.Context(), s.Store, box)
	if err != nil {
		s.writeAccess(w, err)
		return
	}
	for _, view := range views {
		if view.Provider == provider {
			writeJSON(w, http.StatusOK, loginJSON(view))
			return
		}
	}
	apierr.Write(w, apierr.New(http.StatusNotFound, apierr.CodeNotFound, "Provedor de login desconhecido."))
}

func (s *Server) writeBackend(w http.ResponseWriter) {
	kind, err := s.Store.SecretBackend()
	if err != nil {
		apierr.Write(w, err)
		return
	}
	box := s.boxFor(kind)
	ready := false
	if box != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, err = box.Present(ctx, access.SecretRef(access.GitHub))
		ready = err == nil
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"kind":  kind,
		"ready": ready,
	})
}

func (s *Server) openVault() (vault.Box, error) {
	box := s.activeVault()
	if box == nil {
		return nil, errVaultClosed
	}
	return box, nil
}

func (s *Server) activeVault() vault.Box {
	if s.Store == nil {
		return s.Vault
	}
	kind, err := s.Store.SecretBackend()
	if err != nil || !access.KnownBackend(kind) {
		return nil
	}
	return s.boxFor(kind)
}

func (s *Server) boxFor(kind string) vault.Box {
	if s.Vaults != nil {
		return s.Vaults[kind]
	}
	if kind == access.BackendOwn {
		return s.Vault
	}
	return nil
}

func backendOccupied(ctx context.Context, box vault.Box) (bool, error) {
	names := []string{
		access.SecretRef(access.GitHub),
		access.SecretRef(access.Gitea),
		access.SecretRef(access.Google),
		access.SecretRef(access.Apple),
		mail.SecretName,
		sabia.DeploySecret,
		sabia.SendSecret,
	}
	for _, name := range names {
		present, err := box.Present(ctx, name)
		if err != nil {
			return false, err
		}
		if present {
			return true, nil
		}
	}
	return false, nil
}

func (s *Server) writeAccess(w http.ResponseWriter, err error) {
	var invalid *access.ValidationError
	if errors.As(err, &invalid) {
		fields := make([]apierr.FieldError, 0, len(invalid.Fields))
		for _, field := range invalid.Fields {
			fields = append(fields, apierr.FieldError{Field: field.Name, Message: field.Message})
		}
		apierr.Write(w, apierr.Invalid(fields))
		return
	}
	switch {
	case errors.Is(err, errVaultClosed):
		apierr.Write(w, apierr.New(http.StatusPreconditionFailed, apierr.CodeVaultUnavailable, "O cofre próprio não está aberto."))
	case errors.Is(err, vault.ErrUnavailable):
		apierr.Write(w, apierr.New(http.StatusPreconditionFailed, apierr.CodeVaultUnavailable, "O cofre não respondeu."))
	case errors.Is(err, vault.ErrSealed):
		apierr.Write(w, apierr.New(http.StatusInternalServerError, apierr.CodeVaultUnavailable, "O cofre próprio não pôde ser lido."))
	case errors.Is(err, store.ErrLastLogin):
		apierr.Write(w, apierr.New(http.StatusConflict, apierr.CodeLastLogin, "Pelo menos um login precisa permanecer ativo."))
	case errors.Is(err, store.ErrNotFound):
		apierr.Write(w, apierr.New(http.StatusNotFound, apierr.CodeNotFound, "Provedor de login não encontrado."))
	case errors.Is(err, access.ErrUnknownProvider):
		apierr.Write(w, apierr.New(http.StatusNotFound, apierr.CodeNotFound, "Provedor de login desconhecido."))
	case errors.Is(err, access.ErrIncomplete):
		apierr.Write(w, apierr.Invalid([]apierr.FieldError{{
			Field:   "login",
			Message: "A configuração do login está incompleta.",
		}}))
	case errors.Is(err, vault.ErrTooLarge):
		apierr.Write(w, apierr.Invalid([]apierr.FieldError{{
			Field:   "secret",
			Message: "O segredo passa de 8 KiB.",
		}}))
	default:
		apierr.Write(w, err)
	}
}

func loginJSON(view access.View) map[string]any {
	body := map[string]any{
		"provider":       view.Provider,
		"enabled":        view.Enabled,
		"configured":     view.Configured,
		"secret_present": view.SecretPresent,
	}
	if view.ClientID != "" {
		body["client_id"] = view.ClientID
	}
	if view.IssuerURL != "" {
		body["issuer_url"] = view.IssuerURL
	}
	if view.TeamID != "" {
		body["team_id"] = view.TeamID
	}
	if view.KeyID != "" {
		body["key_id"] = view.KeyID
	}
	if view.ServicesID != "" {
		body["services_id"] = view.ServicesID
	}
	if !view.UpdatedAt.IsZero() {
		body["updated_at"] = view.UpdatedAt.UTC()
	}
	return body
}
