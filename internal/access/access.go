package access

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"ai-usage-tracker/internal/store"
	"ai-usage-tracker/internal/vault"
)

const (
	GitHub     = "github"
	Gitea      = "gitea"
	Google     = "google"
	Apple      = "apple"
	BackendOwn = "own"
)

var Providers = []string{GitHub, Gitea, Google, Apple}

var (
	ErrUnknownProvider = errors.New("access: provedor")
	ErrIncomplete      = errors.New("access: incompleto")
)

type Field struct {
	Name    string
	Message string
}

type ValidationError struct {
	Fields []Field
}

func (e *ValidationError) Error() string {
	return "access: validação"
}

type Draft struct {
	Provider   string
	Enabled    bool
	ClientID   string
	IssuerURL  string
	TeamID     string
	KeyID      string
	ServicesID string
	Secret     *[]byte
	Now        time.Time
}

type View struct {
	Provider      string
	Enabled       bool
	Configured    bool
	ClientID      string
	IssuerURL     string
	TeamID        string
	KeyID         string
	ServicesID    string
	SecretPresent bool
	UpdatedAt     time.Time
}

func SecretRef(provider string) string {
	return "login." + provider
}

func Save(ctx context.Context, db *store.Store, box vault.Box, draft Draft) error {
	if !known(draft.Provider) {
		return ErrUnknownProvider
	}
	current, err := find(db, draft.Provider)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	secret := draft.Secret
	present := false
	if secret == nil {
		ok, err := box.Present(ctx, SecretRef(draft.Provider))
		if err != nil {
			return err
		}
		present = ok
	} else {
		present = len(*secret) > 0
	}
	record := store.LoginProvider{
		Provider:   draft.Provider,
		Enabled:    draft.Enabled,
		ClientID:   strings.TrimSpace(draft.ClientID),
		IssuerURL:  strings.TrimSpace(draft.IssuerURL),
		TeamID:     strings.TrimSpace(draft.TeamID),
		KeyID:      strings.TrimSpace(draft.KeyID),
		ServicesID: strings.TrimSpace(draft.ServicesID),
		SecretRef:  SecretRef(draft.Provider),
		UpdatedAt:  draft.Now.UTC(),
	}
	if record.Provider == Apple && record.ClientID == "" {
		record.ClientID = record.ServicesID
	}
	if err := validate(record, present, secret); err != nil {
		return err
	}
	if err := db.GuardLogin(record, false); err != nil {
		return err
	}
	if secret == nil && current.SecretRef != "" && current.SecretRef != record.SecretRef {
		return ErrIncomplete
	}
	if secret != nil {
		if err := box.Put(ctx, record.SecretRef, *secret); err != nil {
			return err
		}
	}
	return db.SaveLoginProvider(record)
}

func Remove(ctx context.Context, db *store.Store, box vault.Box, provider string) error {
	if !known(provider) {
		return ErrUnknownProvider
	}
	if err := db.DeleteLoginProvider(provider); err != nil {
		return err
	}
	// A linha sai antes do arquivo. Um ciphertext órfão é melhor do que uma referência sem segredo.
	return box.Delete(ctx, SecretRef(provider))
}

func Catalog(ctx context.Context, db *store.Store, box vault.Box) ([]View, error) {
	rows, err := db.ListLoginProviders()
	if err != nil {
		return nil, err
	}
	byName := map[string]store.LoginProvider{}
	for _, row := range rows {
		byName[row.Provider] = row
	}
	out := make([]View, 0, len(Providers))
	for _, name := range Providers {
		row, ok := byName[name]
		view := View{Provider: name}
		if ok {
			present, err := box.Present(ctx, row.SecretRef)
			if err != nil {
				return nil, err
			}
			view = View{
				Provider:      row.Provider,
				Enabled:       row.Enabled,
				ClientID:      row.ClientID,
				IssuerURL:     row.IssuerURL,
				TeamID:        row.TeamID,
				KeyID:         row.KeyID,
				ServicesID:    row.ServicesID,
				SecretPresent: present,
				UpdatedAt:     row.UpdatedAt,
				Configured:    configured(row, present),
			}
		}
		out = append(out, view)
	}
	return out, nil
}

func Seed(ctx context.Context, db *store.Store, box vault.Box, env map[string]string, now time.Time) error {
	for _, spec := range seedSpecs(now) {
		values, err := spec.read(env)
		if err != nil {
			return err
		}
		if values == nil {
			continue
		}
		_, err = find(db, spec.provider)
		if err == nil {
			continue
		}
		if !errors.Is(err, store.ErrNotFound) {
			return err
		}
		if err := Save(ctx, db, box, spec.apply(values)); err != nil {
			return err
		}
	}
	return nil
}

type seedSpec struct {
	provider string
	fields   []string
	apply    func(map[string]string) Draft
}

func (s seedSpec) read(env map[string]string) (map[string]string, error) {
	values := map[string]string{}
	filled := 0
	for _, field := range s.fields {
		value := strings.TrimSpace(env[field])
		values[field] = value
		if value != "" {
			filled++
		}
	}
	if filled == 0 {
		return nil, nil
	}
	if filled != len(s.fields) {
		return nil, ErrIncomplete
	}
	return values, nil
}

func find(db *store.Store, provider string) (store.LoginProvider, error) {
	rows, err := db.ListLoginProviders()
	if err != nil {
		return store.LoginProvider{}, err
	}
	for _, row := range rows {
		if row.Provider == provider {
			return row, nil
		}
	}
	return store.LoginProvider{}, store.ErrNotFound
}

func configured(row store.LoginProvider, present bool) bool {
	check := row
	check.Enabled = true
	return present && validate(check, true, nil) == nil
}

func Known(provider string) bool {
	return known(provider)
}

func ReadEnv(getenv func(string) string) map[string]string {
	out := map[string]string{}
	for _, spec := range seedSpecs(time.Time{}) {
		for _, field := range spec.fields {
			out[field] = getenv(field)
		}
	}
	return out
}

func EnvTouched(env map[string]string) bool {
	for _, spec := range seedSpecs(time.Time{}) {
		for _, field := range spec.fields {
			if strings.TrimSpace(env[field]) != "" {
				return true
			}
		}
	}
	return false
}

func seedSpecs(now time.Time) []seedSpec {
	return []seedSpec{
		{
			provider: GitHub,
			fields:   []string{"TRACKER_LOGIN_GITHUB_CLIENT_ID", "TRACKER_LOGIN_GITHUB_CLIENT_SECRET"},
			apply: func(values map[string]string) Draft {
				secret := []byte(values["TRACKER_LOGIN_GITHUB_CLIENT_SECRET"])
				return Draft{
					Provider: GitHub,
					Enabled:  true,
					ClientID: values["TRACKER_LOGIN_GITHUB_CLIENT_ID"],
					Secret:   &secret,
					Now:      now,
				}
			},
		},
		{
			provider: Gitea,
			fields:   []string{"TRACKER_LOGIN_GITEA_BASE_URL", "TRACKER_LOGIN_GITEA_CLIENT_ID", "TRACKER_LOGIN_GITEA_CLIENT_SECRET"},
			apply: func(values map[string]string) Draft {
				secret := []byte(values["TRACKER_LOGIN_GITEA_CLIENT_SECRET"])
				return Draft{
					Provider:  Gitea,
					Enabled:   true,
					ClientID:  values["TRACKER_LOGIN_GITEA_CLIENT_ID"],
					IssuerURL: values["TRACKER_LOGIN_GITEA_BASE_URL"],
					Secret:    &secret,
					Now:       now,
				}
			},
		},
		{
			provider: Google,
			fields:   []string{"TRACKER_LOGIN_GOOGLE_CLIENT_ID", "TRACKER_LOGIN_GOOGLE_CLIENT_SECRET"},
			apply: func(values map[string]string) Draft {
				secret := []byte(values["TRACKER_LOGIN_GOOGLE_CLIENT_SECRET"])
				return Draft{
					Provider: Google,
					Enabled:  true,
					ClientID: values["TRACKER_LOGIN_GOOGLE_CLIENT_ID"],
					Secret:   &secret,
					Now:      now,
				}
			},
		},
		{
			provider: Apple,
			fields:   []string{"TRACKER_LOGIN_APPLE_TEAM_ID", "TRACKER_LOGIN_APPLE_KEY_ID", "TRACKER_LOGIN_APPLE_SERVICES_ID", "TRACKER_LOGIN_APPLE_PRIVATE_KEY"},
			apply: func(values map[string]string) Draft {
				secret := []byte(values["TRACKER_LOGIN_APPLE_PRIVATE_KEY"])
				return Draft{
					Provider:   Apple,
					Enabled:    true,
					ServicesID: values["TRACKER_LOGIN_APPLE_SERVICES_ID"],
					TeamID:     values["TRACKER_LOGIN_APPLE_TEAM_ID"],
					KeyID:      values["TRACKER_LOGIN_APPLE_KEY_ID"],
					Secret:     &secret,
					Now:        now,
				}
			},
		},
	}
}

func known(provider string) bool {
	for _, name := range Providers {
		if name == provider {
			return true
		}
	}
	return false
}

func validate(record store.LoginProvider, present bool, secret *[]byte) error {
	fields := []Field{}
	if secret != nil && !acceptableSecret(record.Provider, *secret) {
		fields = append(fields, Field{Name: "secret", Message: secretProblem(record.Provider, *secret)})
	}
	if record.Enabled {
		if !present && secret == nil {
			fields = append(fields, Field{Name: "secret", Message: "Guarde um segredo neste login ou mantenha o que já está no cofre."})
		}
		switch record.Provider {
		case GitHub, Google:
			if record.ClientID == "" {
				fields = append(fields, Field{Name: "client_id", Message: "Informe o client id."})
			}
		case Gitea:
			if record.ClientID == "" {
				fields = append(fields, Field{Name: "client_id", Message: "Informe o client id."})
			}
			if !httpURL(record.IssuerURL) {
				fields = append(fields, Field{Name: "issuer_url", Message: "Informe a URL base http ou https, sem usuário na URL."})
			}
		case Apple:
			if record.TeamID == "" {
				fields = append(fields, Field{Name: "team_id", Message: "Informe o Team ID."})
			}
			if record.KeyID == "" {
				fields = append(fields, Field{Name: "key_id", Message: "Informe o Key ID."})
			}
			if record.ServicesID == "" {
				fields = append(fields, Field{Name: "services_id", Message: "Informe o Services ID."})
			}
		default:
			return ErrUnknownProvider
		}
	}
	if len(fields) == 0 {
		return nil
	}
	return &ValidationError{Fields: fields}
}

func secretProblem(provider string, secret []byte) string {
	if len(secret) == 0 {
		return "Um segredo vazio não substitui o guardado. Deixe o campo de fora para manter."
	}
	if len(secret) > 8<<10 {
		return "O segredo passa de 8 KiB."
	}
	if provider == Apple {
		return "Envie a chave privada .p8, com o bloco PRIVATE KEY."
	}
	return "O segredo não serve para este login."
}

func acceptableSecret(provider string, secret []byte) bool {
	if len(secret) == 0 || len(secret) > 8<<10 {
		return false
	}
	if provider != Apple {
		return true
	}
	text := string(secret)
	return strings.Contains(text, "BEGIN PRIVATE KEY") || strings.Contains(text, "BEGIN EC PRIVATE KEY")
}

func httpURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if parsed.Host == "" || parsed.User != nil {
		return false
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return false
	}
	return true
}
