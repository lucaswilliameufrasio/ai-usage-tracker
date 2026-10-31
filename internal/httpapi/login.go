package httpapi

import (
	"errors"
	"net/http"
	"time"

	"ai-usage-tracker/internal/access"
	"ai-usage-tracker/internal/apierr"
	"ai-usage-tracker/internal/login"
	"ai-usage-tracker/internal/store"
	"ai-usage-tracker/internal/token"
	"ai-usage-tracker/internal/vault"
)

const sessionCookie = "tracker_session"

func (s *Server) flow() *login.Flow {
	if s.Login != nil {
		return s.Login
	}
	return &login.Flow{
		Store: s.Store,
		Vault: s.activeVault(),
		Ends:  login.Production(),
		Now:   s.Now,
	}
}

func (s *Server) publicLogins(w http.ResponseWriter, r *http.Request) {
	data := make([]map[string]string, 0)
	box := s.activeVault()
	if box == nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": data})
		return
	}
	views, err := access.Catalog(r.Context(), s.Store, box)
	if err != nil {
		if errors.Is(err, vault.ErrSealed) {
			apierr.Write(w, apierr.New(http.StatusInternalServerError, apierr.CodeVaultUnavailable, "O cofre próprio não pôde ser lido."))
			return
		}
		apierr.Write(w, err)
		return
	}
	for _, view := range views {
		if !view.Enabled || !view.Configured || !view.SecretPresent {
			continue
		}
		data = append(data, map[string]string{"provider": view.Provider})
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": data})
}

func (s *Server) loginStart(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	location, err := s.flow().Begin(r.Context(), provider, callbackURL(r, provider))
	if err != nil {
		redirectLogin(w, r, "/?login=falha")
		return
	}
	redirectLogin(w, r, location)
}

func (s *Server) loginCallback(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	provider := r.PathValue("provider")
	if query.Get("error") != "" || query.Get("code") == "" || query.Get("state") == "" {
		redirectLogin(w, r, "/?login=falha")
		return
	}
	raw, err := s.flow().Finish(r.Context(), provider, query.Get("code"), query.Get("state"), callbackURL(r, provider))
	if err != nil {
		redirectLogin(w, r, "/?login=falha")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    raw,
		Path:     "/",
		Expires:  s.Now().Add(14 * 24 * time.Hour),
		MaxAge:   14 * 24 * 3600,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil,
	})
	redirectLogin(w, r, "/")
}

func (s *Server) currentSession(w http.ResponseWriter, r *http.Request) {
	session, ok := s.personSession(r)
	if !ok {
		apierr.Write(w, apierr.New(http.StatusUnauthorized, apierr.CodeUnauthorized, "Autenticação necessária."))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"provider": session.Provider})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if raw := sessionValue(r); raw != "" {
		_ = s.Store.RevokeSession(token.Hash(raw), s.Now())
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil,
	})
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) person(next func(http.ResponseWriter, *http.Request, store.PersonSession)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session, ok := s.personSession(r)
		if !ok {
			apierr.Write(w, apierr.New(http.StatusUnauthorized, apierr.CodeUnauthorized, "Autenticação necessária."))
			return
		}
		next(w, r, session)
	}
}

func (s *Server) accountEnrollment(w http.ResponseWriter, _ *http.Request, session store.PersonSession) {
	s.writeEnrollment(w, session.AccountID)
}

func (s *Server) accountDevices(w http.ResponseWriter, _ *http.Request, session store.PersonSession) {
	devices, err := s.Store.ListDevicesFor(session.AccountID)
	if err != nil {
		apierr.Write(w, err)
		return
	}
	writeDevices(w, devices)
}

func (s *Server) accountRevoke(w http.ResponseWriter, r *http.Request, session store.PersonSession) {
	device, err := s.Store.RevokeOwned(session.AccountID, r.PathValue("id"), s.Now())
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			apierr.Write(w, apierr.New(http.StatusNotFound, apierr.CodeDeviceNotFound, "Dispositivo não encontrado."))
			return
		}
		apierr.Write(w, err)
		return
	}
	body := map[string]any{
		"id":         device.ID,
		"name":       device.Name,
		"created_at": device.CreatedAt,
	}
	if device.RevokedAt != nil {
		body["revoked_at"] = device.RevokedAt.UTC()
	}
	writeJSON(w, http.StatusOK, body)
}

func (s *Server) accountOverview(w http.ResponseWriter, _ *http.Request, session store.PersonSession) {
	s.writePersonOverview(w, session.AccountID)
}

func (s *Server) accountSamples(w http.ResponseWriter, r *http.Request, session store.PersonSession) {
	s.writeSamples(w, r, session.AccountID)
}

func (s *Server) listedSamples(accountID string, provider string, cursor time.Time, cursorID string, limit int) ([]store.SampleRecord, error) {
	if accountID == "" {
		return s.Store.ListSamples(provider, cursor, cursorID, limit)
	}
	return s.Store.ListSamplesFor(accountID, provider, cursor, cursorID, limit)
}

func (s *Server) personSession(r *http.Request) (store.PersonSession, bool) {
	raw := sessionValue(r)
	if raw == "" {
		return store.PersonSession{}, false
	}
	session, err := s.Store.Session(token.Hash(raw), s.Now())
	if err != nil {
		return store.PersonSession{}, false
	}
	return session, true
}

func sessionValue(r *http.Request) string {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func callbackURL(r *http.Request, provider string) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host + "/v1/login/" + provider + "/callback"
}

func redirectLogin(w http.ResponseWriter, r *http.Request, target string) {
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, target, http.StatusFound)
}
