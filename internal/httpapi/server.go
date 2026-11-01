package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"ai-usage-tracker/internal/alert"
	"ai-usage-tracker/internal/apierr"
	"ai-usage-tracker/internal/contract"
	"ai-usage-tracker/internal/freshness"
	"ai-usage-tracker/internal/login"
	"ai-usage-tracker/internal/mail"
	"ai-usage-tracker/internal/push"
	"ai-usage-tracker/internal/store"
	"ai-usage-tracker/internal/token"
	"ai-usage-tracker/internal/vault"
)

const maxBody = 16 << 10

const (
	deliveryQueueCap     = 32
	deliveryFlushLimit   = 8
	deliveryAttemptLimit = 5
	deliveryMaxAge       = 24 * time.Hour
)

type Server struct {
	Store          *store.Store
	Now            func() time.Time
	Sender         push.Sender
	AllowOpenSetup bool
	SetupToken     string
	WebDir         string
	Logger         *slog.Logger
	Vault          vault.Box
	Vaults         map[string]vault.Box
	Login          *login.Flow
	SabiaHTTP      *http.Client
	Mail           *mail.Client
}

func (s *Server) Handler() http.Handler {
	if s.Now == nil {
		s.Now = func() time.Time { return time.Now().UTC() }
	}
	if s.Logger == nil {
		s.Logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	if s.Mail == nil {
		s.Mail = mail.Default()
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("POST /v1/setup", s.setup)
	mux.HandleFunc("POST /v1/enrollment-codes", s.operator(s.createEnrollment))
	mux.HandleFunc("POST /v1/devices", s.enroll)
	mux.HandleFunc("GET /v1/devices", s.operator(s.listDevices))
	mux.HandleFunc("POST /v1/devices/{id}/revoke", s.operator(s.revokeDevice))
	mux.HandleFunc("GET /v1/device", s.device(s.currentDevice))
	mux.HandleFunc("POST /v1/samples", s.device(s.ingestSample))
	mux.HandleFunc("POST /v1/source-reports", s.device(s.ingestReport))
	mux.HandleFunc("GET /v1/samples", s.operator(s.listSamples))
	mux.HandleFunc("GET /v1/overview", s.operator(s.overview))
	mux.HandleFunc("GET /v1/settings", s.operator(s.getSettings))
	mux.HandleFunc("PUT /v1/settings", s.operator(s.putSettings))
	mux.HandleFunc("GET /v1/push/public-key", s.operator(s.pushKey))
	mux.HandleFunc("POST /v1/push/subscriptions", s.operator(s.createSubscription))
	mux.HandleFunc("GET /v1/push/subscriptions", s.operator(s.listSubscriptions))
	mux.HandleFunc("DELETE /v1/push/subscriptions/{id}", s.operator(s.deleteSubscription))
	mux.HandleFunc("GET /v1/push/deliveries", s.operator(s.listDeliveries))
	mux.HandleFunc("GET /v1/admin/secret-backend", s.operator(s.getSecretBackend))
	mux.HandleFunc("PUT /v1/admin/secret-backend", s.operator(s.putSecretBackend))
	mux.HandleFunc("GET /v1/admin/login-providers", s.operator(s.listLoginProviders))
	mux.HandleFunc("PUT /v1/admin/login-providers/{provider}", s.operator(s.putLoginProvider))
	mux.HandleFunc("DELETE /v1/admin/login-providers/{provider}", s.operator(s.deleteLoginProvider))
	mux.HandleFunc("GET /v1/admin/sabia", s.operator(s.getSabia))
	mux.HandleFunc("PUT /v1/admin/sabia", s.operator(s.putSabia))
	mux.HandleFunc("POST /v1/admin/sabia/plan", s.operator(s.postSabiaPlan))
	mux.HandleFunc("POST /v1/admin/sabia/apply", s.operator(s.postSabiaApply))
	mux.HandleFunc("GET /v1/admin/email", s.operator(s.getEmail))
	mux.HandleFunc("PUT /v1/admin/email", s.operator(s.putEmail))
	mux.HandleFunc("DELETE /v1/admin/email", s.operator(s.deleteEmail))
	mux.HandleFunc("GET /v1/login/providers", s.publicLogins)
	mux.HandleFunc("GET /v1/login/{provider}/start", s.loginStart)
	mux.HandleFunc("GET /v1/login/{provider}/callback", s.loginCallback)
	mux.HandleFunc("GET /v1/session", s.currentSession)
	mux.HandleFunc("POST /v1/session/logout", s.logout)
	mux.HandleFunc("POST /v1/account/enrollment-codes", s.person(s.accountEnrollment))
	mux.HandleFunc("GET /v1/account/devices", s.person(s.accountDevices))
	mux.HandleFunc("POST /v1/account/devices/{id}/revoke", s.person(s.accountRevoke))
	mux.HandleFunc("GET /v1/account/overview", s.person(s.accountOverview))
	mux.HandleFunc("GET /v1/account/samples", s.person(s.accountSamples))
	mux.HandleFunc("/", s.ui)
	return s.recover(s.log(mux))
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) setup(w http.ResponseWriter, r *http.Request) {
	if s.SetupToken != "" {
		if !fixedEqual(r.Header.Get("X-Setup-Token"), s.SetupToken) {
			apierr.Write(w, apierr.New(http.StatusForbidden, apierr.CodeSetupRequired, "O setup desta instalação exige o token de configuração."))
			return
		}
	} else if !s.AllowOpenSetup {
		apierr.Write(w, apierr.New(http.StatusForbidden, apierr.CodeSetupRequired, "O setup desta instalação exige o token de configuração."))
		return
	}
	raw, err := token.New("op_")
	if err != nil {
		apierr.Write(w, err)
		return
	}
	if err := s.Store.Setup(token.Hash(raw), s.Now()); err != nil {
		if errors.Is(err, store.ErrExists) {
			apierr.Write(w, apierr.New(http.StatusConflict, apierr.CodeAlreadySetup, "Esta instalação já tem operador."))
			return
		}
		apierr.Write(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"operator_token": raw})
}

func (s *Server) createEnrollment(w http.ResponseWriter, _ *http.Request) {
	s.writeEnrollment(w, "")
}

func (s *Server) writeEnrollment(w http.ResponseWriter, accountID string) {
	raw, err := token.New("enr_")
	if err != nil {
		apierr.Write(w, err)
		return
	}
	expires := s.Now().Add(15 * time.Minute)
	if err := s.Store.CreateEnrollment(token.Hash(raw), accountID, expires); err != nil {
		apierr.Write(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"code":       raw,
		"expires_at": expires.UTC(),
	})
}

func (s *Server) enroll(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code string `json:"code"`
		Name string `json:"name"`
	}
	if err := s.decode(w, r, &body); err != nil {
		apierr.Write(w, err)
		return
	}
	var fields []apierr.FieldError
	if len(body.Code) < 8 || len(body.Code) > 120 {
		fields = append(fields, apierr.FieldError{Field: "code", Message: "Código inválido."})
	}
	if !validDeviceName(body.Name) {
		fields = append(fields, apierr.FieldError{Field: "name", Message: "Nome do dispositivo inválido."})
	}
	if len(fields) > 0 {
		apierr.Write(w, apierr.Invalid(fields))
		return
	}
	deviceToken, err := token.New("dev_")
	if err != nil {
		apierr.Write(w, err)
		return
	}
	deviceID, err := token.New("dvc_")
	if err != nil {
		apierr.Write(w, err)
		return
	}
	err = s.Store.Enroll(token.Hash(body.Code), deviceID, strings.TrimSpace(body.Name), token.Hash(deviceToken), s.Now())
	if err != nil {
		switch {
		case errors.Is(err, store.ErrEnrollmentUsed):
			apierr.Write(w, apierr.New(http.StatusUnauthorized, apierr.CodeEnrollmentUsed, "Este código já foi usado."))
		case errors.Is(err, store.ErrEnrollmentExpired):
			apierr.Write(w, apierr.New(http.StatusUnauthorized, apierr.CodeEnrollmentExpired, "Este código expirou."))
		case errors.Is(err, store.ErrEnrollmentInvalid):
			apierr.Write(w, apierr.New(http.StatusUnauthorized, apierr.CodeEnrollmentInvalid, "Código de enrolamento inválido."))
		default:
			apierr.Write(w, err)
		}
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{
		"device_id": deviceID,
		"token":     deviceToken,
		"name":      strings.TrimSpace(body.Name),
	})
}

func (s *Server) listDevices(w http.ResponseWriter, _ *http.Request) {
	devices, err := s.Store.ListDevices()
	if err != nil {
		apierr.Write(w, err)
		return
	}
	writeDevices(w, devices)
}

// currentDevice answers who the device token belongs to. It writes nothing, so the agent
// and its doctor can ask whether the server still accepts the token.
func (s *Server) currentDevice(w http.ResponseWriter, _ *http.Request, device store.Device) {
	writeJSON(w, http.StatusOK, map[string]any{
		"id":         device.ID,
		"name":       device.Name,
		"created_at": device.CreatedAt,
	})
}

func writeDevices(w http.ResponseWriter, devices []store.Device) {
	data := make([]map[string]any, 0, len(devices))
	for _, device := range devices {
		item := map[string]any{
			"id":         device.ID,
			"name":       device.Name,
			"created_at": device.CreatedAt,
		}
		if device.RevokedAt != nil {
			item["revoked_at"] = device.RevokedAt.UTC()
		}
		data = append(data, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": data})
}

func (s *Server) revokeDevice(w http.ResponseWriter, r *http.Request) {
	device, err := s.Store.Revoke(r.PathValue("id"), s.Now())
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

func (s *Server) ingestSample(w http.ResponseWriter, r *http.Request, device store.Device) {
	raw, err := readBody(r)
	if err != nil {
		apierr.Write(w, err)
		return
	}
	sample, err := contract.DecodeSample(raw, s.Now())
	if err != nil {
		s.writeContract(w, err)
		return
	}
	created, err := s.Store.InsertSample(device.ID, sample, s.Now())
	if err != nil {
		s.writeStore(w, err, true)
		return
	}
	if created {
		if err := s.planAlerts(sample); err != nil {
			s.Logger.Error("alert", "provider", sample.Provider, "sample_id", sample.SampleID)
		}
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, sample)
}

func (s *Server) ingestReport(w http.ResponseWriter, r *http.Request, device store.Device) {
	raw, err := readBody(r)
	if err != nil {
		apierr.Write(w, err)
		return
	}
	report, err := contract.DecodeReport(raw, s.Now())
	if err != nil {
		s.writeContract(w, err)
		return
	}
	if err := s.Store.UpsertReport(device.ID, report, s.Now()); err != nil {
		s.writeStore(w, err, false)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (s *Server) listSamples(w http.ResponseWriter, r *http.Request) {
	s.writeSamples(w, r, "")
}

func (s *Server) writeSamples(w http.ResponseWriter, r *http.Request, accountID string) {
	provider := r.URL.Query().Get("provider")
	if provider != "" && provider != contract.ProviderClaude && provider != contract.ProviderCodex && provider != contract.ProviderAntigravity {
		apierr.Write(w, apierr.Invalid([]apierr.FieldError{{Field: "provider", Message: "Provedor deve ser claude, codex ou antigravity."}}))
		return
	}
	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := parseLimit(raw)
		if err != nil {
			apierr.Write(w, apierr.Invalid([]apierr.FieldError{{Field: "limit", Message: "Limite inválido."}}))
			return
		}
		limit = parsed
	}
	var cursorTime time.Time
	cursorID := ""
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		decoded, err := decodeCursor(raw)
		if err != nil {
			apierr.Write(w, apierr.Invalid([]apierr.FieldError{{Field: "cursor", Message: "Cursor inválido."}}))
			return
		}
		cursorTime = decoded.received
		cursorID = decoded.id
	}
	records, err := s.listedSamples(accountID, provider, cursorTime, cursorID, limit+1)
	if err != nil {
		apierr.Write(w, err)
		return
	}
	hasMore := len(records) > limit
	if hasMore {
		records = records[:limit]
	}
	data := make([]contract.Sample, 0, len(records))
	for _, record := range records {
		data = append(data, record.Sample)
	}
	body := map[string]any{
		"data":     data,
		"has_more": hasMore,
	}
	if hasMore && len(records) > 0 {
		last := records[len(records)-1]
		body["next_cursor"] = encodeCursor(last.ReceivedAt, last.Sample.SampleID)
	}
	writeJSON(w, http.StatusOK, body)
}

func (s *Server) overview(w http.ResponseWriter, _ *http.Request) {
	s.writeOperatorOverview(w)
}

func (s *Server) writeOperatorOverview(w http.ResponseWriter) {
	settings, err := s.Store.Settings()
	if err != nil {
		apierr.Write(w, err)
		return
	}
	now := s.Now()
	accounts, err := s.Store.AccountsWithDevices()
	if err != nil {
		apierr.Write(w, err)
		return
	}
	groups := make([]map[string]any, 0)
	for _, account := range accounts {
		cards, err := s.providerCards(account.ID, now, settings.StaleAfter)
		if err != nil {
			apierr.Write(w, err)
			return
		}
		groups = append(groups, map[string]any{
			"id":        account.ID,
			"label":     accountLabel(account),
			"login":     account.Provider,
			"providers": cards,
		})
	}
	orphan, err := s.Store.HasOrphanDevices()
	if err != nil {
		apierr.Write(w, err)
		return
	}
	if orphan {
		cards, err := s.providerCards("", now, settings.StaleAfter)
		if err != nil {
			apierr.Write(w, err)
			return
		}
		groups = append(groups, map[string]any{
			"id":        "",
			"label":     "Sem conta",
			"login":     "",
			"providers": cards,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"generated_at":        now.UTC(),
		"stale_after_seconds": int(settings.StaleAfter / time.Second),
		"accounts":            groups,
	})
}

func (s *Server) writePersonOverview(w http.ResponseWriter, accountID string) {
	settings, err := s.Store.Settings()
	if err != nil {
		apierr.Write(w, err)
		return
	}
	now := s.Now()
	providers, err := s.providerCards(accountID, now, settings.StaleAfter)
	if err != nil {
		apierr.Write(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"generated_at":        now.UTC(),
		"stale_after_seconds": int(settings.StaleAfter / time.Second),
		"providers":           providers,
	})
}

func (s *Server) providerCards(accountID string, now time.Time, stale time.Duration) ([]map[string]any, error) {
	providers := make([]map[string]any, 0, 3)
	for _, provider := range []string{contract.ProviderClaude, contract.ProviderCodex, contract.ProviderAntigravity} {
		var sample *contract.Sample
		record, err := s.Store.LatestSampleFor(accountID, provider)
		if err == nil {
			copied := record.Sample
			sample = &copied
		} else if !errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
		var report *contract.SourceReport
		reportRecord, err := s.Store.LatestReportFor(accountID, provider)
		if err == nil {
			copied := reportRecord.Report
			report = &copied
		} else if !errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
		if provider == contract.ProviderAntigravity && sample == nil && report == nil {
			continue
		}
		history, err := s.Store.RecentSamplesFor(accountID, provider, 8)
		if err != nil {
			return nil, err
		}
		card := freshness.Build(provider, sample, report, history, now, stale)
		item := map[string]any{
			"provider":  card.Provider,
			"freshness": card.Freshness,
		}
		if card.ReasonCode != "" {
			item["reason_code"] = card.ReasonCode
		}
		if card.ObservedAt != nil {
			item["observed_at"] = card.ObservedAt.UTC()
		}
		if card.LastSample != nil {
			item["last_sample"] = card.LastSample
		}
		if card.Projection != nil {
			item["projection"] = card.Projection
		}
		providers = append(providers, item)
	}
	return providers, nil
}

func accountLabel(account store.Account) string {
	contact := strings.TrimSpace(account.Contact)
	if contact != "" {
		return contact
	}
	switch account.Provider {
	case "github":
		return "GitHub"
	case "gitea":
		return "Gitea"
	case "google":
		return "Google"
	case "apple":
		return "Apple"
	default:
		return "Conta"
	}
}

func (s *Server) getSettings(w http.ResponseWriter, _ *http.Request) {
	settings, err := s.Store.Settings()
	if err != nil {
		apierr.Write(w, err)
		return
	}
	writeJSON(w, http.StatusOK, settingsJSON(settings))
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		StaleAfterSeconds int     `json:"stale_after_seconds"`
		AlertThreshold    float64 `json:"alert_threshold_percent"`
		Timezone          string  `json:"timezone"`
		QuietHours        *struct {
			Start string `json:"start"`
			End   string `json:"end"`
		} `json:"quiet_hours"`
	}
	if err := s.decode(w, r, &body); err != nil {
		apierr.Write(w, err)
		return
	}
	var fields []apierr.FieldError
	if body.StaleAfterSeconds < 60 || body.StaleAfterSeconds > 7*24*3600 {
		fields = append(fields, apierr.FieldError{Field: "stale_after_seconds", Message: "Use entre 60 segundos e 7 dias."})
	}
	if body.AlertThreshold < 1 || body.AlertThreshold > 100 {
		fields = append(fields, apierr.FieldError{Field: "alert_threshold_percent", Message: "O limiar fica entre 1 e 100."})
	}
	if _, err := time.LoadLocation(body.Timezone); err != nil || body.Timezone == "" {
		fields = append(fields, apierr.FieldError{Field: "timezone", Message: "Fuso horário inválido."})
	}
	settings := store.Settings{
		StaleAfter:     time.Duration(body.StaleAfterSeconds) * time.Second,
		AlertThreshold: body.AlertThreshold,
		Timezone:       body.Timezone,
	}
	if body.QuietHours != nil {
		if _, err := alert.ClockMinutes(body.QuietHours.Start); err != nil {
			fields = append(fields, apierr.FieldError{Field: "quiet_hours.start", Message: "Use HH:MM."})
		}
		if _, err := alert.ClockMinutes(body.QuietHours.End); err != nil {
			fields = append(fields, apierr.FieldError{Field: "quiet_hours.end", Message: "Use HH:MM."})
		}
		if body.QuietHours.Start == body.QuietHours.End {
			fields = append(fields, apierr.FieldError{Field: "quiet_hours", Message: "Início e fim não podem ser iguais."})
		}
		settings.QuietStart = body.QuietHours.Start
		settings.QuietEnd = body.QuietHours.End
	}
	if len(fields) > 0 {
		apierr.Write(w, apierr.Invalid(fields))
		return
	}
	if err := s.Store.SaveSettings(settings); err != nil {
		apierr.Write(w, err)
		return
	}
	writeJSON(w, http.StatusOK, settingsJSON(settings))
}

func (s *Server) pushKey(w http.ResponseWriter, _ *http.Request) {
	if s.Sender == nil || !s.Sender.Configured() {
		apierr.Write(w, apierr.New(http.StatusNotFound, apierr.CodePushUnconfigured, "Web Push não está configurado nesta instalação."))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"public_key": s.Sender.PublicKey()})
}

func (s *Server) createSubscription(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Endpoint string `json:"endpoint"`
		P256dh   string `json:"p256dh"`
		Auth     string `json:"auth"`
	}
	if err := s.decode(w, r, &body); err != nil {
		apierr.Write(w, err)
		return
	}
	if err := validateSubscription(body.Endpoint, body.P256dh, body.Auth); err != nil {
		apierr.Write(w, err)
		return
	}
	id, err := token.New("sub_")
	if err != nil {
		apierr.Write(w, err)
		return
	}
	sub := store.PushSubscription{
		ID:        id,
		Endpoint:  body.Endpoint,
		P256dh:    body.P256dh,
		Auth:      body.Auth,
		CreatedAt: s.Now(),
	}
	storedID, err := s.Store.UpsertSubscription(sub)
	if err != nil {
		apierr.Write(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": storedID})
}

func (s *Server) listSubscriptions(w http.ResponseWriter, _ *http.Request) {
	subs, err := s.Store.ListSubscriptions()
	if err != nil {
		apierr.Write(w, err)
		return
	}
	data := make([]map[string]any, 0, len(subs))
	for _, sub := range subs {
		item := map[string]any{
			"id":         sub.ID,
			"endpoint":   redactEndpoint(sub.Endpoint),
			"created_at": sub.CreatedAt,
			"status":     "active",
		}
		if sub.ExpiredAt != nil {
			item["status"] = "expired"
			item["expired_at"] = sub.ExpiredAt.UTC()
		}
		if sub.LastError != "" {
			item["last_error"] = sub.LastError
		}
		data = append(data, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": data})
}

func (s *Server) deleteSubscription(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.DeleteSubscription(r.PathValue("id")); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			apierr.Write(w, apierr.New(http.StatusNotFound, apierr.CodeNotFound, "Inscrição não encontrada."))
			return
		}
		apierr.Write(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": r.PathValue("id")})
}

func (s *Server) listDeliveries(w http.ResponseWriter, _ *http.Request) {
	deliveries, err := s.Store.ListDeliveries(50)
	if err != nil {
		apierr.Write(w, err)
		return
	}
	data := make([]map[string]any, 0, len(deliveries))
	for _, delivery := range deliveries {
		item := map[string]any{
			"id":         delivery.ID,
			"kind":       delivery.Kind,
			"provider":   delivery.Provider,
			"title":      delivery.Title,
			"body":       delivery.Body,
			"status":     delivery.Status,
			"created_at": delivery.CreatedAt,
		}
		if delivery.Detail != "" {
			item["detail"] = delivery.Detail
		}
		data = append(data, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": data})
}

func (s *Server) planAlerts(sample contract.Sample) error {
	settings, err := s.Store.Settings()
	if err != nil {
		return err
	}
	now := s.Now()
	intents := alert.FromSample(sample, settings.AlertThreshold, now)
	quiet, err := alert.Quiet(now, settings.QuietStart, settings.QuietEnd, settings.Timezone)
	if err != nil {
		return err
	}
	subs, err := s.Store.ListSubscriptions()
	if err != nil {
		return err
	}
	var active []store.PushSubscription
	for _, sub := range subs {
		if sub.ExpiredAt == nil {
			active = append(active, sub)
		}
	}
	sabiaReady, err := s.sabiaSendReady()
	if err != nil {
		return err
	}
	for _, intent := range intents {
		if err := s.planPush(intent, quiet, active); err != nil {
			return err
		}
		if err := s.planEmail(intent, quiet); err != nil {
			return err
		}
		if err := s.planSabia(intent, quiet, settings.Timezone, sabiaReady); err != nil {
			return err
		}
	}
	return s.flush()
}

func (s *Server) planPush(intent alert.Intent, quiet bool, active []store.PushSubscription) error {
	if quiet {
		return s.insertDelivery(intent, "", "suppressed", "QUIET_HOURS", intent.Key+"|quiet")
	}
	if len(active) == 0 || s.Sender == nil || !s.Sender.Configured() {
		status := "no_subscription"
		detail := ""
		suffix := "|none"
		if s.Sender == nil || !s.Sender.Configured() {
			status = "unconfigured"
			detail = "PUSH_UNCONFIGURED"
			suffix = "|unconfigured"
		}
		return s.insertDelivery(intent, "", status, detail, intent.Key+suffix)
	}
	for _, sub := range active {
		status := "pending"
		detail := ""
		pending, err := s.Store.CountPendingDeliveries()
		if err != nil {
			return err
		}
		if pending >= deliveryQueueCap {
			status = "failed"
			detail = "QUEUE_FULL"
		}
		if err := s.insertDelivery(intent, sub.ID, status, detail, intent.Key+"|"+sub.ID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) insertDelivery(intent alert.Intent, subscriptionID string, status string, detail string, key string) error {
	id, err := token.New("dly_")
	if err != nil {
		return err
	}
	now := s.Now()
	_, err = s.Store.InsertDelivery(store.Delivery{
		ID:             id,
		IdempotencyKey: key,
		SubscriptionID: subscriptionID,
		Kind:           intent.Kind,
		Provider:       intent.Provider,
		Title:          intent.Title,
		Body:           intent.Body,
		Status:         status,
		Detail:         detail,
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	return err
}

func (s *Server) FlushDue() error {
	if s.Now == nil {
		s.Now = func() time.Time { return time.Now().UTC() }
	}
	return s.flush()
}

func (s *Server) flush() error {
	if err := s.flushPush(); err != nil {
		return err
	}
	if err := s.flushEmail(); err != nil {
		return err
	}
	return s.flushSabia()
}

func (s *Server) flushPush() error {
	if s.Sender == nil || !s.Sender.Configured() {
		return nil
	}
	pending, err := s.Store.PendingDeliveries()
	if err != nil {
		return err
	}
	subs, err := s.Store.ListSubscriptions()
	if err != nil {
		return err
	}
	byID := map[string]store.PushSubscription{}
	for _, sub := range subs {
		byID[sub.ID] = sub
	}
	now := s.Now()
	sent := 0
	dead := map[string]bool{}
	for _, delivery := range pending {
		if dead[delivery.SubscriptionID] {
			continue
		}
		if now.Sub(delivery.CreatedAt) > deliveryMaxAge {
			if err := s.Store.UpdateDelivery(delivery.ID, "failed", "DELIVERY_EXPIRED", delivery.Attempts, time.Time{}, now); err != nil {
				return err
			}
			continue
		}
		if !delivery.NextAttempt.IsZero() && delivery.NextAttempt.After(now) {
			continue
		}
		if sent >= deliveryFlushLimit {
			break
		}
		sub, ok := byID[delivery.SubscriptionID]
		if !ok || sub.ExpiredAt != nil {
			if err := s.Store.UpdateDelivery(delivery.ID, "subscription_expired", "SUBSCRIPTION_EXPIRED", delivery.Attempts, time.Time{}, now); err != nil {
				return err
			}
			continue
		}
		payload, err := json.Marshal(map[string]string{
			"title":    delivery.Title,
			"body":     delivery.Body,
			"provider": delivery.Provider,
			"kind":     delivery.Kind,
		})
		if err != nil {
			return err
		}
		err = s.Sender.Send(context.Background(), push.Subscription{
			Endpoint: sub.Endpoint,
			P256dh:   sub.P256dh,
			Auth:     sub.Auth,
		}, payload)
		attempts := delivery.Attempts + 1
		if errors.Is(err, push.ErrGone) {
			if err := s.Store.ExpireSubscription(sub.ID, now, "SUBSCRIPTION_EXPIRED"); err != nil {
				return err
			}
			if err := s.Store.ExpirePendingForSubscription(sub.ID, now); err != nil {
				return err
			}
			dead[sub.ID] = true
			continue
		}
		if errors.Is(err, push.ErrRejected) || (err != nil && attempts >= deliveryAttemptLimit) {
			if err := s.Store.UpdateDelivery(delivery.ID, "failed", "SEND_FAILED", attempts, time.Time{}, now); err != nil {
				return err
			}
			sent++
			continue
		}
		if err != nil {
			if err := s.Store.UpdateDelivery(delivery.ID, "pending", "", attempts, now.Add(retryDelay(attempts)), now); err != nil {
				return err
			}
			sent++
			continue
		}
		if err := s.Store.UpdateDelivery(delivery.ID, "sent", "", attempts, time.Time{}, now); err != nil {
			return err
		}
		sent++
	}
	return nil
}

func retryDelay(attempts int) time.Duration {
	shift := attempts - 1
	if shift < 0 {
		shift = 0
	}
	if shift > 3 {
		shift = 3
	}
	return time.Minute << shift
}

func (s *Server) ui(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/healthz" || strings.HasPrefix(r.URL.Path, "/v1/") {
		http.NotFound(w, r)
		return
	}
	if s.WebDir == "" || r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	rel := filepath.Clean("/" + r.URL.Path)
	if rel == "/" {
		rel = "/index.html"
	}
	root, err := filepath.Abs(s.WebDir)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	full := filepath.Join(root, filepath.FromSlash(rel))
	abs, err := filepath.Abs(full)
	if err != nil || (abs != root && !strings.HasPrefix(abs, root+string(os.PathSeparator))) {
		http.NotFound(w, r)
		return
	}
	info, err := os.Stat(abs)
	if err == nil && !info.IsDir() {
		http.ServeFile(w, r, abs)
		return
	}
	if filepath.Ext(rel) != "" {
		http.NotFound(w, r)
		return
	}
	index := filepath.Join(root, "index.html")
	info, err = os.Stat(index)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, index)
}

func (s *Server) operator(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw := bearer(r)
		if raw == "" {
			apierr.Write(w, apierr.New(http.StatusUnauthorized, apierr.CodeUnauthorized, "Autenticação necessária."))
			return
		}
		if _, err := s.Store.DeviceByTokenHash(token.Hash(raw)); err == nil {
			apierr.Write(w, apierr.New(http.StatusForbidden, apierr.CodePermissionDenied, "O token de dispositivo não acessa esta operação."))
			return
		}
		hash, err := s.Store.OperatorHash()
		if err != nil || !token.Match(raw, hash) {
			apierr.Write(w, apierr.New(http.StatusUnauthorized, apierr.CodeUnauthorized, "Autenticação necessária."))
			return
		}
		next(w, r)
	}
}

func (s *Server) device(next func(http.ResponseWriter, *http.Request, store.Device)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw := bearer(r)
		if raw == "" {
			apierr.Write(w, apierr.New(http.StatusUnauthorized, apierr.CodeUnauthorized, "Autenticação necessária."))
			return
		}
		device, err := s.Store.DeviceByTokenHash(token.Hash(raw))
		if err != nil {
			apierr.Write(w, apierr.New(http.StatusUnauthorized, apierr.CodeUnauthorized, "Autenticação necessária."))
			return
		}
		if device.RevokedAt != nil {
			apierr.Write(w, apierr.New(http.StatusUnauthorized, apierr.CodeDeviceRevoked, "Este dispositivo foi revogado."))
			return
		}
		next(w, r, device)
	}
}

func (s *Server) decode(w http.ResponseWriter, r *http.Request, dest any) error {
	raw, err := readBody(r)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dest); err != nil {
		return apierr.New(http.StatusBadRequest, apierr.CodeMalformed, "JSON malformado.")
	}
	return nil
}

func (s *Server) writeContract(w http.ResponseWriter, err error) {
	var validation *contract.ValidationError
	if errors.As(err, &validation) {
		fields := make([]apierr.FieldError, 0, len(validation.Fields))
		for _, field := range validation.Fields {
			fields = append(fields, apierr.FieldError{Field: field.Field, Message: field.Message})
		}
		apierr.Write(w, apierr.Invalid(fields))
		return
	}
	if errors.Is(err, contract.ErrMalformed) {
		apierr.Write(w, apierr.New(http.StatusBadRequest, apierr.CodeMalformed, "JSON malformado."))
		return
	}
	apierr.Write(w, err)
}

func (s *Server) writeStore(w http.ResponseWriter, err error, sample bool) {
	switch {
	case errors.Is(err, store.ErrConflict):
		if sample {
			apierr.Write(w, apierr.New(http.StatusConflict, apierr.CodeConflict, "O identificador já existe com outro conteúdo."))
			return
		}
		apierr.Write(w, apierr.New(http.StatusConflict, apierr.CodeReportConflict, "O relatório já existe com outro motivo."))
	case errors.Is(err, store.ErrRateLimited):
		apierr.Write(w, apierr.New(http.StatusTooManyRequests, apierr.CodeRateLimited, "Limite de ingestão desta hora foi atingido."))
	default:
		apierr.Write(w, err)
	}
}

func (s *Server) log(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		s.Logger.Info("http", "method", r.Method, "path", r.URL.Path, "status", recorder.status, "ms", time.Since(started).Milliseconds())
	})
}

func (s *Server) recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				s.Logger.Error("panic")
				apierr.Write(w, apierr.New(http.StatusInternalServerError, apierr.CodeUnexpected, "Erro interno."))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func readBody(r *http.Request) ([]byte, error) {
	r.Body = http.MaxBytesReader(nil, r.Body, maxBody)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return nil, apierr.New(http.StatusRequestEntityTooLarge, apierr.CodePayloadTooLarge, "Corpo maior que o limite.")
		}
		return nil, apierr.New(http.StatusBadRequest, apierr.CodeMalformed, "Não foi possível ler o corpo.")
	}
	return body, nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(value)
}

func settingsJSON(settings store.Settings) map[string]any {
	body := map[string]any{
		"stale_after_seconds":     int(settings.StaleAfter / time.Second),
		"alert_threshold_percent": settings.AlertThreshold,
		"timezone":                settings.Timezone,
	}
	if settings.QuietStart != "" && settings.QuietEnd != "" {
		body["quiet_hours"] = map[string]string{
			"start": settings.QuietStart,
			"end":   settings.QuietEnd,
		}
	}
	return body
}

func bearer(r *http.Request) string {
	header := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, prefix))
}

func fixedEqual(got string, want string) bool {
	if len(got) != len(want) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func validDeviceName(name string) bool {
	name = strings.TrimSpace(name)
	if len(name) < 1 || len(name) > 64 {
		return false
	}
	for _, char := range name {
		if unicode.IsLetter(char) || unicode.IsNumber(char) || char == ' ' || char == '-' || char == '_' {
			continue
		}
		return false
	}
	return true
}

func validateSubscription(endpoint string, p256dh string, auth string) error {
	var fields []apierr.FieldError
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		fields = append(fields, apierr.FieldError{Field: "endpoint", Message: "Endpoint inválido."})
	} else if parsed.Scheme == "http" && parsed.Hostname() != "127.0.0.1" && parsed.Hostname() != "localhost" {
		fields = append(fields, apierr.FieldError{Field: "endpoint", Message: "HTTP só é aceito em localhost."})
	}
	if len(p256dh) < 8 || len(p256dh) > 256 {
		fields = append(fields, apierr.FieldError{Field: "p256dh", Message: "Chave p256dh inválida."})
	}
	if len(auth) < 8 || len(auth) > 256 {
		fields = append(fields, apierr.FieldError{Field: "auth", Message: "Chave auth inválida."})
	}
	if len(fields) > 0 {
		return apierr.Invalid(fields)
	}
	return nil
}

func redactEndpoint(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

type cursor struct {
	received time.Time
	id       string
}

func encodeCursor(received time.Time, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(received.UTC().Format(time.RFC3339Nano) + "\n" + id))
}

func decodeCursor(value string) (cursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return cursor{}, err
	}
	parts := strings.SplitN(string(raw), "\n", 2)
	if len(parts) != 2 || parts[1] == "" {
		return cursor{}, errors.New("cursor")
	}
	received, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return cursor{}, err
	}
	return cursor{received: received.UTC(), id: parts[1]}, nil
}

func parseLimit(value string) (int, error) {
	limit := 0
	for _, char := range value {
		if char < '0' || char > '9' {
			return 0, errors.New("limit")
		}
		limit = limit*10 + int(char-'0')
	}
	if limit < 1 || limit > 100 {
		return 0, errors.New("limit")
	}
	return limit, nil
}
