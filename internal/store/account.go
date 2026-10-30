package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"ai-usage-tracker/internal/contract"
)

type Account struct {
	ID        string
	Provider  string
	Subject   string
	Contact   string
	CreatedAt time.Time
}

type PersonSession struct {
	AccountID string
	Provider  string
	ExpiresAt time.Time
}

type LoginAttempt struct {
	ID           string
	Provider     string
	Nonce        string
	CodeVerifier string
	ClientID     string
	IssuerURL    string
	TeamID       string
	KeyID        string
	ServicesID   string
	SecretRef    string
	RedirectURI  string
	ExpiresAt    time.Time
	Used         bool
}

func (s *Store) UpsertAccount(provider string, subject string, contact string, id string, now time.Time) (Account, error) {
	row := s.db.QueryRow(
		`INSERT INTO accounts(id, provider, subject, contact, created_at) VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(provider, subject) DO UPDATE SET
		   contact = CASE WHEN excluded.contact = '' THEN accounts.contact ELSE excluded.contact END
		 RETURNING id, provider, subject, contact, created_at`,
		id, provider, subject, contact, formatTime(now),
	)
	return scanAccount(row)
}

func (s *Store) FindAccount(provider string, subject string) (Account, error) {
	row := s.db.QueryRow(`SELECT id, provider, subject, contact, created_at FROM accounts WHERE provider = ? AND subject = ?`, provider, subject)
	return scanAccount(row)
}

func (s *Store) InsertAttempt(attempt LoginAttempt, stateHash string) error {
	_, err := s.db.Exec(
		`INSERT INTO login_attempts(id, state_hash, provider, nonce, code_verifier, client_id, issuer_url, team_id, key_id, services_id, secret_ref, redirect_uri, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		attempt.ID,
		stateHash,
		attempt.Provider,
		attempt.Nonce,
		attempt.CodeVerifier,
		attempt.ClientID,
		attempt.IssuerURL,
		attempt.TeamID,
		attempt.KeyID,
		attempt.ServicesID,
		attempt.SecretRef,
		attempt.RedirectURI,
		formatTime(attempt.ExpiresAt),
	)
	return err
}

func (s *Store) TakeAttempt(stateHash string, now time.Time) (LoginAttempt, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return LoginAttempt{}, err
	}
	defer func() { _ = tx.Rollback() }()
	row := tx.QueryRow(
		`SELECT id, provider, nonce, code_verifier, client_id, issuer_url, team_id, key_id, services_id, secret_ref, redirect_uri, expires_at, used_at
		 FROM login_attempts WHERE state_hash = ?`,
		stateHash,
	)
	attempt, err := scanAttempt(row)
	if err != nil {
		return LoginAttempt{}, err
	}
	if attempt.Used {
		return attempt, ErrLoginState
	}
	if !attempt.ExpiresAt.After(now) {
		return attempt, ErrLoginExpired
	}
	result, err := tx.Exec(`UPDATE login_attempts SET used_at = ? WHERE state_hash = ? AND used_at IS NULL`, formatTime(now), stateHash)
	if err != nil {
		return LoginAttempt{}, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return LoginAttempt{}, err
	}
	if n != 1 {
		return LoginAttempt{}, ErrLoginState
	}
	if err := tx.Commit(); err != nil {
		return LoginAttempt{}, err
	}
	attempt.Used = true
	return attempt, nil
}

func (s *Store) CreateSession(tokenHash string, accountID string, now time.Time, expires time.Time) error {
	_, err := s.db.Exec(
		`INSERT INTO sessions(token_hash, account_id, created_at, expires_at) VALUES (?, ?, ?, ?)`,
		tokenHash, accountID, formatTime(now), formatTime(expires),
	)
	return err
}

func (s *Store) Session(tokenHash string, now time.Time) (PersonSession, error) {
	var session PersonSession
	var expiresText string
	var revoked sql.NullString
	err := s.db.QueryRow(
		`SELECT sessions.account_id, accounts.provider, sessions.expires_at, sessions.revoked_at
		 FROM sessions JOIN accounts ON accounts.id = sessions.account_id
		 WHERE sessions.token_hash = ?`,
		tokenHash,
	).Scan(&session.AccountID, &session.Provider, &expiresText, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return PersonSession{}, ErrNotFound
	}
	if err != nil {
		return PersonSession{}, err
	}
	session.ExpiresAt, err = parseTime(expiresText)
	if err != nil {
		return PersonSession{}, err
	}
	if revoked.Valid || !session.ExpiresAt.After(now) {
		return PersonSession{}, ErrNotFound
	}
	return session, nil
}

func (s *Store) RevokeSession(tokenHash string, now time.Time) error {
	_, err := s.db.Exec(`UPDATE sessions SET revoked_at = ? WHERE token_hash = ? AND revoked_at IS NULL`, formatTime(now), tokenHash)
	return err
}

func (s *Store) AccountsWithDevices() ([]Account, error) {
	rows, err := s.db.Query(
		`SELECT id, provider, subject, contact, created_at FROM accounts
		 WHERE id IN (SELECT account_id FROM devices WHERE account_id <> '')
		 ORDER BY created_at ASC, id ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var accounts []Account
	for rows.Next() {
		account, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, account)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if accounts == nil {
		accounts = []Account{}
	}
	return accounts, nil
}

func (s *Store) HasOrphanDevices() (bool, error) {
	var found int
	err := s.db.QueryRow(`SELECT 1 FROM devices WHERE account_id = '' LIMIT 1`).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) ListDevicesFor(accountID string) ([]Device, error) {
	rows, err := s.db.Query(`SELECT id, name, token_hash, created_at, revoked_at FROM devices WHERE account_id = ? ORDER BY created_at ASC, id ASC`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var devices []Device
	for rows.Next() {
		device, err := scanDevice(rows)
		if err != nil {
			return nil, err
		}
		devices = append(devices, device)
	}
	return devices, rows.Err()
}

func (s *Store) RevokeOwned(accountID string, id string, now time.Time) (Device, error) {
	row := s.db.QueryRow(`SELECT id, name, token_hash, created_at, revoked_at FROM devices WHERE id = ? AND account_id = ?`, id, accountID)
	device, err := scanDevice(row)
	if err != nil {
		return Device{}, err
	}
	if device.RevokedAt == nil {
		if _, err := s.db.Exec(`UPDATE devices SET revoked_at = ? WHERE id = ? AND account_id = ?`, formatTime(now), id, accountID); err != nil {
			return Device{}, err
		}
		revoked := now.UTC()
		device.RevokedAt = &revoked
	}
	return device, nil
}

func (s *Store) LatestSampleFor(accountID string, provider string) (SampleRecord, error) {
	row := s.db.QueryRow(
		`SELECT payload_json, device_id, received_at FROM samples
		 WHERE provider = ? AND device_id IN (SELECT id FROM devices WHERE account_id = ?)
		 ORDER BY collected_at DESC, sample_id DESC LIMIT 1`,
		provider, accountID,
	)
	return scanSampleRecord(row)
}

func (s *Store) LatestReportFor(accountID string, provider string) (ReportRecord, error) {
	row := s.db.QueryRow(
		`SELECT payload_json, device_id, received_at FROM source_reports
		 WHERE provider = ? AND device_id IN (SELECT id FROM devices WHERE account_id = ?)
		 ORDER BY observed_at DESC, report_id DESC LIMIT 1`,
		provider, accountID,
	)
	return scanReportRecord(row)
}

func (s *Store) RecentSamplesFor(accountID string, provider string, limit int) ([]contract.Sample, error) {
	rows, err := s.db.Query(
		`SELECT payload_json FROM samples
		 WHERE provider = ? AND device_id IN (SELECT id FROM devices WHERE account_id = ?)
		 ORDER BY collected_at DESC, sample_id DESC LIMIT ?`,
		provider, accountID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var samples []contract.Sample
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var sample contract.Sample
		if err := json.Unmarshal([]byte(payload), &sample); err != nil {
			return nil, err
		}
		samples = append(samples, sample)
	}
	return samples, rows.Err()
}

func (s *Store) ListSamplesFor(accountID string, provider string, cursorReceived time.Time, cursorID string, limit int) ([]SampleRecord, error) {
	query := `SELECT payload_json, device_id, received_at FROM samples WHERE device_id IN (SELECT id FROM devices WHERE account_id = ?) AND (? = '' OR provider = ?)`
	args := []any{accountID, provider, provider}
	if cursorID != "" {
		query += ` AND (received_at < ? OR (received_at = ? AND sample_id < ?))`
		text := formatTime(cursorReceived)
		args = append(args, text, text, cursorID)
	}
	query += ` ORDER BY received_at DESC, sample_id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []SampleRecord
	for rows.Next() {
		record, err := scanSampleRecord(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func scanAccount(row scanner) (Account, error) {
	var account Account
	var created string
	err := row.Scan(&account.ID, &account.Provider, &account.Subject, &account.Contact, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	if err != nil {
		return Account{}, err
	}
	account.CreatedAt, err = parseTime(created)
	if err != nil {
		return Account{}, err
	}
	return account, nil
}

func scanAttempt(row scanner) (LoginAttempt, error) {
	var attempt LoginAttempt
	var expires string
	var used sql.NullString
	err := row.Scan(
		&attempt.ID,
		&attempt.Provider,
		&attempt.Nonce,
		&attempt.CodeVerifier,
		&attempt.ClientID,
		&attempt.IssuerURL,
		&attempt.TeamID,
		&attempt.KeyID,
		&attempt.ServicesID,
		&attempt.SecretRef,
		&attempt.RedirectURI,
		&expires,
		&used,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return LoginAttempt{}, ErrLoginState
	}
	if err != nil {
		return LoginAttempt{}, err
	}
	attempt.ExpiresAt, err = parseTime(expires)
	if err != nil {
		return LoginAttempt{}, err
	}
	attempt.Used = used.Valid
	return attempt, nil
}

func scanSampleRecord(row scanner) (SampleRecord, error) {
	var payload string
	var record SampleRecord
	var received string
	err := row.Scan(&payload, &record.DeviceID, &received)
	if errors.Is(err, sql.ErrNoRows) {
		return SampleRecord{}, ErrNotFound
	}
	if err != nil {
		return SampleRecord{}, err
	}
	if err := json.Unmarshal([]byte(payload), &record.Sample); err != nil {
		return SampleRecord{}, err
	}
	record.ReceivedAt, err = parseTime(received)
	if err != nil {
		return SampleRecord{}, err
	}
	return record, nil
}

func scanReportRecord(row scanner) (ReportRecord, error) {
	var payload string
	var record ReportRecord
	var received string
	err := row.Scan(&payload, &record.DeviceID, &received)
	if errors.Is(err, sql.ErrNoRows) {
		return ReportRecord{}, ErrNotFound
	}
	if err != nil {
		return ReportRecord{}, err
	}
	if err := json.Unmarshal([]byte(payload), &record.Report); err != nil {
		return ReportRecord{}, err
	}
	record.ReceivedAt, err = parseTime(received)
	if err != nil {
		return ReportRecord{}, err
	}
	return record, nil
}
