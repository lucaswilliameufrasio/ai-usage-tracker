package httpapi

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"ai-usage-tracker/internal/alert"
	"ai-usage-tracker/internal/apierr"
	"ai-usage-tracker/internal/mail"
	"ai-usage-tracker/internal/store"
	"ai-usage-tracker/internal/token"
	"ai-usage-tracker/internal/vault"
)

var (
	emailAddressPattern = regexp.MustCompile(`^[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}$`)
	namedFromPattern    = regexp.MustCompile(`^[^<>\r\n]{1,80} <[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}>$`)
	cloudflareAccount   = regexp.MustCompile(`^[A-Za-z0-9]{1,64}$`)
	sesRegionPattern    = regexp.MustCompile(`^[a-z]{2}-[a-z]+-\d$`)
)

func (s *Server) getEmail(w http.ResponseWriter, r *http.Request) {
	body, err := s.emailView(r.Context())
	if err != nil {
		s.writeEmail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, body)
}

func (s *Server) putEmail(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Provider        *string `json:"provider"`
		FromAddress     *string `json:"from_address"`
		ToAddress       *string `json:"to_address"`
		AccountRef      *string `json:"account_ref"`
		Active          *bool   `json:"active"`
		APIKey          *string `json:"api_key"`
		AccessKeyID     *string `json:"access_key_id"`
		SecretAccessKey *string `json:"secret_access_key"`
	}
	if err := s.decode(w, r, &body); err != nil {
		apierr.Write(w, err)
		return
	}
	provider := ""
	from := ""
	to := ""
	account := ""
	active := false
	if body.Provider != nil {
		provider = strings.TrimSpace(*body.Provider)
	}
	if body.FromAddress != nil {
		from = strings.TrimSpace(*body.FromAddress)
	}
	if body.ToAddress != nil {
		to = strings.TrimSpace(*body.ToAddress)
	}
	if body.AccountRef != nil {
		account = strings.TrimSpace(*body.AccountRef)
	}
	if body.Active != nil {
		active = *body.Active
	}
	apiKey := trimmed(body.APIKey)
	accessKey := trimmed(body.AccessKeyID)
	secretKey := trimmed(body.SecretAccessKey)
	fields := validateEmail(provider, from, to, account, body.Active != nil, apiKey, accessKey, secretKey)
	if len(fields) > 0 {
		apierr.Write(w, apierr.Invalid(fields))
		return
	}
	box, err := s.openVault()
	if err != nil {
		s.writeEmail(w, err)
		return
	}
	current, found, err := s.Store.EmailChannel()
	if err != nil {
		apierr.Write(w, err)
		return
	}
	existing, present, err := readEmailSecret(r.Context(), box)
	if err != nil {
		s.writeEmail(w, err)
		return
	}
	nextSecret, replaced, fields := mergeEmailSecret(provider, current.Provider, found, apiKey, accessKey, secretKey, existing, present)
	if active && !mail.Fits(provider, nextSecret) {
		field := "api_key"
		if provider == mail.ProviderSES {
			field = "secret_access_key"
		}
		fields = append(fields, apierr.FieldError{Field: field, Message: "Informe a chave."})
	}
	if len(fields) > 0 {
		apierr.Write(w, apierr.Invalid(fields))
		return
	}
	now := s.Now()
	if replaced {
		if err := s.Store.FailPendingEmail("EMAIL_REPLACED", now); err != nil {
			apierr.Write(w, err)
			return
		}
	} else if found && current.Active && !active {
		if err := s.Store.FailPendingEmail("EMAIL_INACTIVE", now); err != nil {
			apierr.Write(w, err)
			return
		}
	}
	if err := s.Store.SaveEmailChannel(store.EmailChannel{
		Provider:   provider,
		From:       from,
		To:         to,
		AccountRef: account,
		Active:     active,
	}, now); err != nil {
		apierr.Write(w, err)
		return
	}
	if apiKey != "" || accessKey != "" || secretKey != "" {
		raw, err := mail.EncodeSecret(nextSecret)
		if err != nil {
			apierr.Write(w, err)
			return
		}
		if err := box.Put(r.Context(), mail.SecretName, raw); err != nil {
			s.writeEmail(w, err)
			return
		}
	}
	view, err := s.emailView(r.Context())
	if err != nil {
		s.writeEmail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) deleteEmail(w http.ResponseWriter, r *http.Request) {
	box, err := s.openVault()
	if err != nil {
		s.writeEmail(w, err)
		return
	}
	now := s.Now()
	if err := s.Store.FailPendingEmail("EMAIL_REMOVED", now); err != nil {
		apierr.Write(w, err)
		return
	}
	if err := s.Store.DeleteEmailChannel(); err != nil {
		apierr.Write(w, err)
		return
	}
	if err := box.Delete(r.Context(), mail.SecretName); err != nil && !errors.Is(err, vault.ErrNotFound) {
		s.writeEmail(w, err)
		return
	}
	view, err := s.emailView(r.Context())
	if err != nil {
		s.writeEmail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func validateEmail(provider string, from string, to string, account string, activeSet bool, apiKey string, accessKey string, secretKey string) []apierr.FieldError {
	var fields []apierr.FieldError
	if !activeSet {
		fields = append(fields, apierr.FieldError{Field: "active", Message: "Informe se o e-mail fica ativo."})
	}
	switch provider {
	case mail.ProviderResend, mail.ProviderMillionSend, mail.ProviderCloudflare, mail.ProviderSES:
	default:
		fields = append(fields, apierr.FieldError{Field: "provider", Message: "Provedor de e-mail deve ser resend, millionsend, cloudflare ou ses."})
	}
	if from == "" {
		fields = append(fields, apierr.FieldError{Field: "from_address", Message: "Informe o remetente."})
	} else if !emailAddressPattern.MatchString(from) && !namedFromPattern.MatchString(from) {
		fields = append(fields, apierr.FieldError{Field: "from_address", Message: "O remetente precisa ser um e-mail."})
	}
	if to == "" {
		fields = append(fields, apierr.FieldError{Field: "to_address", Message: "Informe o destinatário."})
	} else if !emailAddressPattern.MatchString(to) {
		fields = append(fields, apierr.FieldError{Field: "to_address", Message: "O destinatário precisa ser um e-mail."})
	}
	switch provider {
	case mail.ProviderCloudflare:
		if account == "" {
			fields = append(fields, apierr.FieldError{Field: "account_ref", Message: "Informe a conta da Cloudflare."})
		} else if !cloudflareAccount.MatchString(account) {
			fields = append(fields, apierr.FieldError{Field: "account_ref", Message: "A conta da Cloudflare só pode ter letras e números."})
		}
		if accessKey != "" || secretKey != "" {
			fields = append(fields, apierr.FieldError{Field: "api_key", Message: "Este provedor usa a chave da API."})
		}
	case mail.ProviderSES:
		if account == "" {
			fields = append(fields, apierr.FieldError{Field: "account_ref", Message: "Informe a região do SES."})
		} else if !sesRegionPattern.MatchString(account) {
			fields = append(fields, apierr.FieldError{Field: "account_ref", Message: "A região do SES precisa ser como sa-east-1."})
		}
		if apiKey != "" {
			fields = append(fields, apierr.FieldError{Field: "api_key", Message: "O SES usa a chave de acesso e o segredo."})
		}
		if (accessKey == "") != (secretKey == "") {
			fields = append(fields, apierr.FieldError{Field: "secret_access_key", Message: "Informe a chave."})
		}
	default:
		if account != "" {
			fields = append(fields, apierr.FieldError{Field: "account_ref", Message: "Este provedor não usa conta."})
		}
		if accessKey != "" || secretKey != "" {
			fields = append(fields, apierr.FieldError{Field: "api_key", Message: "Este provedor usa a chave da API."})
		}
	}
	return fields
}

func mergeEmailSecret(provider string, currentProvider string, found bool, apiKey string, accessKey string, secretKey string, existing mail.Credentials, present bool) (mail.Credentials, bool, []apierr.FieldError) {
	replaced := found && currentProvider != "" && currentProvider != provider
	next := existing
	if !present || currentProvider != provider {
		next = mail.Credentials{}
	}
	wrote := false
	if provider == mail.ProviderSES {
		if accessKey != "" || secretKey != "" {
			next = mail.Credentials{AccessKeyID: accessKey, SecretAccessKey: secretKey}
			wrote = true
		}
	} else if apiKey != "" {
		next = mail.Credentials{APIKey: apiKey}
		wrote = true
	}
	if wrote && present && (next.APIKey != existing.APIKey || next.AccessKeyID != existing.AccessKeyID || next.SecretAccessKey != existing.SecretAccessKey) {
		replaced = true
	}
	return next, replaced, nil
}

func (s *Server) emailView(ctx context.Context) (map[string]any, error) {
	box, err := s.openVault()
	if err != nil {
		return nil, err
	}
	channel, found, err := s.Store.EmailChannel()
	if err != nil {
		return nil, err
	}
	present, err := box.Present(ctx, mail.SecretName)
	if err != nil {
		return nil, err
	}
	body := map[string]any{
		"active":         false,
		"secret_present": present,
	}
	if found {
		body["provider"] = channel.Provider
		body["from_address"] = channel.From
		body["to_address"] = channel.To
		body["active"] = channel.Active
		if channel.AccountRef != "" {
			body["account_ref"] = channel.AccountRef
		}
	}
	rows, err := s.Store.ListEmailDeliveries(1)
	if err != nil {
		return nil, err
	}
	if len(rows) == 1 {
		body["last_status"] = rows[0].Status
		if rows[0].Detail != "" {
			body["last_detail"] = rows[0].Detail
		}
	}
	return body, nil
}

func (s *Server) planEmail(intent alert.Intent, quiet bool) error {
	channel, found, err := s.Store.EmailChannel()
	if err != nil {
		return err
	}
	if !found || !channel.Active {
		return nil
	}
	if quiet {
		return s.insertEmail(intent, "suppressed", "QUIET_HOURS", intent.Key+"|email|quiet")
	}
	pending, err := s.Store.CountPendingEmail()
	if err != nil {
		return err
	}
	status := "pending"
	detail := ""
	if pending >= deliveryQueueCap {
		status = "failed"
		detail = "QUEUE_FULL"
	}
	return s.insertEmail(intent, status, detail, intent.Key+"|email")
}

func (s *Server) insertEmail(intent alert.Intent, status string, detail string, key string) error {
	id, err := token.New("eml_")
	if err != nil {
		return err
	}
	now := s.Now()
	_, err = s.Store.InsertEmailDelivery(store.EmailDelivery{
		ID:             id,
		IdempotencyKey: key,
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

func (s *Server) flushEmail() error {
	pending, err := s.Store.PendingEmail()
	if err != nil {
		return err
	}
	if len(pending) == 0 {
		return nil
	}
	client := s.Mail
	if client == nil {
		client = mail.Default()
	}
	channel, found, err := s.Store.EmailChannel()
	if err != nil {
		return err
	}
	now := s.Now()
	sent := 0
	for _, delivery := range pending {
		if now.Sub(delivery.CreatedAt) > deliveryMaxAge {
			if err := s.Store.UpdateEmailDelivery(delivery.ID, "failed", "DELIVERY_EXPIRED", delivery.Attempts, time.Time{}, now); err != nil {
				return err
			}
			continue
		}
		if !delivery.NextAttempt.IsZero() && delivery.NextAttempt.After(now) {
			continue
		}
		if !found || !channel.Active {
			detail := "EMAIL_INACTIVE"
			if !found {
				detail = "EMAIL_UNCONFIGURED"
			}
			if err := s.Store.UpdateEmailDelivery(delivery.ID, "failed", detail, delivery.Attempts, time.Time{}, now); err != nil {
				return err
			}
			continue
		}
		if sent >= deliveryFlushLimit {
			break
		}
		err := s.sendEmail(client, channel, delivery)
		attempts := delivery.Attempts + 1
		if errors.Is(err, errEmailUnconfigured) || errors.Is(err, mail.ErrPermanent) || (err != nil && attempts >= deliveryAttemptLimit) {
			detail := "SEND_FAILED"
			if errors.Is(err, errEmailUnconfigured) {
				detail = "EMAIL_UNCONFIGURED"
			}
			if err := s.Store.UpdateEmailDelivery(delivery.ID, "failed", detail, attempts, time.Time{}, now); err != nil {
				return err
			}
			if !errors.Is(err, errEmailUnconfigured) {
				sent++
			}
			continue
		}
		if err != nil {
			if err := s.Store.UpdateEmailDelivery(delivery.ID, "pending", "", attempts, now.Add(retryDelay(attempts)), now); err != nil {
				return err
			}
			sent++
			continue
		}
		if err := s.Store.UpdateEmailDelivery(delivery.ID, "sent", "", attempts, time.Time{}, now); err != nil {
			return err
		}
		sent++
	}
	return nil
}

var errEmailUnconfigured = errors.New("email unconfigured")

func (s *Server) sendEmail(client *mail.Client, channel store.EmailChannel, delivery store.EmailDelivery) error {
	box, err := s.openVault()
	if err != nil {
		return errEmailUnconfigured
	}
	creds, present, err := readEmailSecret(context.Background(), box)
	if err != nil || !present || !mail.Fits(channel.Provider, creds) {
		return errEmailUnconfigured
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return client.Send(ctx, channel.Provider, channel.AccountRef, creds, mail.Message{
		From:    channel.From,
		To:      channel.To,
		Subject: delivery.Title,
		Text:    delivery.Body,
	})
}

func readEmailSecret(ctx context.Context, box vault.Box) (mail.Credentials, bool, error) {
	raw, err := box.Get(ctx, mail.SecretName)
	if errors.Is(err, vault.ErrNotFound) {
		return mail.Credentials{}, false, nil
	}
	if err != nil {
		return mail.Credentials{}, false, err
	}
	creds, err := mail.ParseSecret(raw)
	if err != nil {
		return mail.Credentials{}, false, err
	}
	return creds, true, nil
}

func (s *Server) writeEmail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errVaultClosed):
		apierr.Write(w, apierr.New(http.StatusPreconditionFailed, apierr.CodeVaultUnavailable, "O cofre próprio não está aberto."))
	case errors.Is(err, vault.ErrSealed):
		apierr.Write(w, apierr.New(http.StatusInternalServerError, apierr.CodeVaultUnavailable, "O cofre próprio não pôde ser lido."))
	case errors.Is(err, vault.ErrTooLarge):
		apierr.Write(w, apierr.Invalid([]apierr.FieldError{{
			Field:   "api_key",
			Message: "A chave passa de 8 KiB.",
		}}))
	default:
		apierr.Write(w, err)
	}
}

func trimmed(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
