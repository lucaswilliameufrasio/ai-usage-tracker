package store

import (
	"database/sql"
	"time"
)

type LoginProvider struct {
	Provider   string
	Enabled    bool
	ClientID   string
	IssuerURL  string
	TeamID     string
	KeyID      string
	ServicesID string
	SecretRef  string
	UpdatedAt  time.Time
}

func (s *Store) SecretBackend() (string, error) {
	var kind string
	err := s.db.QueryRow(`SELECT kind FROM secret_backend WHERE id = 1`).Scan(&kind)
	if err != nil {
		return "", err
	}
	return kind, nil
}

func (s *Store) SetSecretBackend(kind string) error {
	_, err := s.db.Exec(`UPDATE secret_backend SET kind = ? WHERE id = 1`, kind)
	return err
}

func (s *Store) ListLoginProviders() ([]LoginProvider, error) {
	rows, err := s.db.Query(`SELECT provider, enabled, client_id, issuer_url, team_id, key_id, services_id, secret_ref, updated_at FROM login_providers ORDER BY provider`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LoginProvider
	for rows.Next() {
		item, err := scanLogin(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) GuardLogin(next LoginProvider, removing bool) error {
	current, err := s.ListLoginProviders()
	if err != nil {
		return err
	}
	if lastLoginBroken(current, next, removing) {
		return ErrLastLogin
	}
	return nil
}

func (s *Store) SaveLoginProvider(next LoginProvider) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := listLoginTx(tx)
	if err != nil {
		return err
	}
	if lastLoginBroken(current, next, false) {
		return ErrLastLogin
	}
	_, err = tx.Exec(`
INSERT INTO login_providers(provider, enabled, client_id, issuer_url, team_id, key_id, services_id, secret_ref, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(provider) DO UPDATE SET
  enabled = excluded.enabled,
  client_id = excluded.client_id,
  issuer_url = excluded.issuer_url,
  team_id = excluded.team_id,
  key_id = excluded.key_id,
  services_id = excluded.services_id,
  secret_ref = excluded.secret_ref,
  updated_at = excluded.updated_at
`, next.Provider, boolInt(next.Enabled), next.ClientID, next.IssuerURL, next.TeamID, next.KeyID, next.ServicesID, next.SecretRef, formatTime(next.UpdatedAt))
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DeleteLoginProvider(provider string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := listLoginTx(tx)
	if err != nil {
		return err
	}
	var target LoginProvider
	found := false
	for _, item := range current {
		if item.Provider == provider {
			target = item
			found = true
		}
	}
	if !found {
		return ErrNotFound
	}
	target.Enabled = false
	if lastLoginBroken(current, target, true) {
		return ErrLastLogin
	}
	if _, err := tx.Exec(`DELETE FROM login_providers WHERE provider = ?`, provider); err != nil {
		return err
	}
	return tx.Commit()
}

func listLoginTx(tx *sql.Tx) ([]LoginProvider, error) {
	rows, err := tx.Query(`SELECT provider, enabled, client_id, issuer_url, team_id, key_id, services_id, secret_ref, updated_at FROM login_providers ORDER BY provider`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LoginProvider
	for rows.Next() {
		item, err := scanLogin(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

type loginScanner interface {
	Scan(dest ...any) error
}

func scanLogin(row loginScanner) (LoginProvider, error) {
	var item LoginProvider
	var enabled int
	var updated string
	err := row.Scan(&item.Provider, &enabled, &item.ClientID, &item.IssuerURL, &item.TeamID, &item.KeyID, &item.ServicesID, &item.SecretRef, &updated)
	if err != nil {
		return LoginProvider{}, err
	}
	item.Enabled = enabled == 1
	parsed, err := parseTime(updated)
	if err != nil {
		return LoginProvider{}, err
	}
	item.UpdatedAt = parsed
	return item, nil
}

func lastLoginBroken(current []LoginProvider, next LoginProvider, removing bool) bool {
	before := 0
	after := 0
	seen := false
	for _, item := range current {
		if item.Enabled {
			before++
		}
		if item.Provider == next.Provider {
			seen = true
			if next.Enabled && !removing {
				after++
			}
			continue
		}
		if item.Enabled {
			after++
		}
	}
	if !seen && next.Enabled && !removing {
		after++
	}
	return before > 0 && after == 0
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
