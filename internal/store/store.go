package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"ai-usage-tracker/internal/contract"

	_ "modernc.org/sqlite"
)

var (
	ErrNotFound          = errors.New("not found")
	ErrExists            = errors.New("exists")
	ErrConflict          = errors.New("conflict")
	ErrEnrollmentInvalid = errors.New("enrollment invalid")
	ErrEnrollmentExpired = errors.New("enrollment expired")
	ErrEnrollmentUsed    = errors.New("enrollment used")
	ErrRateLimited       = errors.New("rate limited")
	ErrLastLogin         = errors.New("last login")
	ErrLoginState        = errors.New("login state")
	ErrLoginExpired      = errors.New("login expired")
)

const observationCap = 240

const schemaVersion = 5

const deliveryColumns = `id, idempotency_key, subscription_id, kind, provider, title, body, status, detail, attempts, next_attempt_at, created_at, updated_at`

type Device struct {
	ID        string
	Name      string
	TokenHash string
	CreatedAt time.Time
	RevokedAt *time.Time
}

type Settings struct {
	StaleAfter     time.Duration
	AlertThreshold float64
	QuietStart     string
	QuietEnd       string
	Timezone       string
}

type PushSubscription struct {
	ID        string
	Endpoint  string
	P256dh    string
	Auth      string
	CreatedAt time.Time
	ExpiredAt *time.Time
	LastError string
}

type Delivery struct {
	ID             string
	IdempotencyKey string
	SubscriptionID string
	Kind           string
	Provider       string
	Title          string
	Body           string
	Status         string
	Detail         string
	Attempts       int
	NextAttempt    time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type SampleRecord struct {
	Sample     contract.Sample
	DeviceID   string
	ReceivedAt time.Time
}

type ReportRecord struct {
	Report     contract.SourceReport
	DeviceID   string
	ReceivedAt time.Time
}

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, fmt.Errorf("abrir sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	store := &Store{db: db}
	if err := store.useWAL(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) useWAL() error {
	var mode string
	if err := s.db.QueryRow(`PRAGMA journal_mode=WAL`).Scan(&mode); err != nil {
		return fmt.Errorf("abrir sqlite em wal: %w", err)
	}
	if !strings.EqualFold(mode, "wal") {
		return fmt.Errorf("abrir sqlite em wal: modo %s", mode)
	}
	return nil
}

func (s *Store) migrate() error {
	var version int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("ler versão: %w", err)
	}
	if version > schemaVersion {
		return fmt.Errorf("sqlite user_version %d não é suportada", version)
	}
	if version < 1 {
		if err := s.migrateV1(); err != nil {
			return err
		}
		version = 1
	}
	if version < 2 {
		if err := s.migrateV2(); err != nil {
			return err
		}
		version = 2
	}
	if version < 3 {
		if err := s.migrateV3(); err != nil {
			return err
		}
		version = 3
	}
	if version < 4 {
		if err := s.migrateV4(); err != nil {
			return err
		}
		version = 4
	}
	if version < 5 {
		if err := s.migrateV5(); err != nil {
			return err
		}
		version = 5
	}
	return nil
}

func (s *Store) migrateV1() error {
	schema := `
CREATE TABLE operator (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  token_hash TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE TABLE enrollment_codes (
  code_hash TEXT PRIMARY KEY,
  expires_at TEXT NOT NULL,
  used_at TEXT
);
CREATE TABLE devices (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  token_hash TEXT NOT NULL UNIQUE,
  created_at TEXT NOT NULL,
  revoked_at TEXT
);
CREATE TABLE samples (
  sample_id TEXT PRIMARY KEY,
  device_id TEXT NOT NULL REFERENCES devices(id),
  provider TEXT NOT NULL,
  collected_at TEXT NOT NULL,
  received_at TEXT NOT NULL,
  payload_json TEXT NOT NULL
);
CREATE TABLE source_reports (
  report_id TEXT PRIMARY KEY,
  device_id TEXT NOT NULL REFERENCES devices(id),
  provider TEXT NOT NULL,
  observed_at TEXT NOT NULL,
  received_at TEXT NOT NULL,
  payload_json TEXT NOT NULL
);
CREATE TABLE settings (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  stale_after_seconds INTEGER NOT NULL,
  alert_threshold_percent REAL NOT NULL,
  quiet_start TEXT NOT NULL,
  quiet_end TEXT NOT NULL,
  timezone TEXT NOT NULL
);
CREATE TABLE push_subscriptions (
  id TEXT PRIMARY KEY,
  endpoint TEXT NOT NULL UNIQUE,
  p256dh TEXT NOT NULL,
  auth_key TEXT NOT NULL,
  created_at TEXT NOT NULL,
  expired_at TEXT,
  last_error TEXT NOT NULL
);
CREATE TABLE push_deliveries (
  id TEXT PRIMARY KEY,
  idempotency_key TEXT NOT NULL UNIQUE,
  subscription_id TEXT NOT NULL,
  kind TEXT NOT NULL,
  provider TEXT NOT NULL,
  title TEXT NOT NULL,
  body TEXT NOT NULL,
  status TEXT NOT NULL,
  detail TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX samples_provider_collected ON samples(provider, collected_at DESC);
CREATE INDEX samples_received ON samples(received_at DESC, sample_id DESC);
CREATE INDEX reports_provider_observed ON source_reports(provider, observed_at DESC);
`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("migrar: %w", err)
	}
	if _, err := s.db.Exec(`PRAGMA user_version = 1`); err != nil {
		return fmt.Errorf("gravar versão: %w", err)
	}
	return nil
}

func (s *Store) migrateV2() error {
	schema := `
CREATE TABLE secret_backend (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  kind TEXT NOT NULL
);
CREATE TABLE login_providers (
  provider TEXT PRIMARY KEY,
  enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
  client_id TEXT NOT NULL,
  issuer_url TEXT NOT NULL,
  team_id TEXT NOT NULL,
  key_id TEXT NOT NULL,
  services_id TEXT NOT NULL,
  secret_ref TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
INSERT INTO secret_backend(id, kind) VALUES (1, 'own');
`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("migrar cofre: %w", err)
	}
	if _, err := s.db.Exec(`PRAGMA user_version = 2`); err != nil {
		return fmt.Errorf("gravar versão: %w", err)
	}
	return nil
}

func (s *Store) migrateV3() error {
	schema := `
CREATE TABLE accounts (
  id TEXT PRIMARY KEY,
  provider TEXT NOT NULL,
  subject TEXT NOT NULL,
  contact TEXT NOT NULL,
  created_at TEXT NOT NULL,
  UNIQUE(provider, subject)
);
CREATE TABLE login_attempts (
  id TEXT PRIMARY KEY,
  state_hash TEXT NOT NULL UNIQUE,
  provider TEXT NOT NULL,
  nonce TEXT NOT NULL,
  code_verifier TEXT NOT NULL,
  client_id TEXT NOT NULL,
  issuer_url TEXT NOT NULL,
  team_id TEXT NOT NULL,
  key_id TEXT NOT NULL,
  services_id TEXT NOT NULL,
  secret_ref TEXT NOT NULL,
  redirect_uri TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  used_at TEXT
);
CREATE TABLE sessions (
  token_hash TEXT PRIMARY KEY,
  account_id TEXT NOT NULL REFERENCES accounts(id),
  created_at TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  revoked_at TEXT
);
ALTER TABLE enrollment_codes ADD COLUMN account_id TEXT NOT NULL DEFAULT '';
ALTER TABLE devices ADD COLUMN account_id TEXT NOT NULL DEFAULT '';
`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("migrar contas: %w", err)
	}
	if _, err := s.db.Exec(`PRAGMA user_version = 3`); err != nil {
		return fmt.Errorf("gravar versão: %w", err)
	}
	return nil
}

func (s *Store) migrateV4() error {
	if _, err := s.db.Exec(`ALTER TABLE push_deliveries ADD COLUMN attempts INTEGER NOT NULL DEFAULT 0`); err != nil {
		return fmt.Errorf("migrar fila: %w", err)
	}
	if _, err := s.db.Exec(`ALTER TABLE push_deliveries ADD COLUMN next_attempt_at TEXT NOT NULL DEFAULT ''`); err != nil {
		return fmt.Errorf("migrar fila: %w", err)
	}
	if _, err := s.db.Exec(`PRAGMA user_version = 4`); err != nil {
		return fmt.Errorf("gravar versão: %w", err)
	}
	return nil
}

func (s *Store) migrateV5() error {
	schema := `
CREATE TABLE sabia_link (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  origin TEXT NOT NULL,
  application_id TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE sabia_template (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  name TEXT NOT NULL,
  subject TEXT NOT NULL,
  html TEXT NOT NULL,
  text TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("migrar sabiá: %w", err)
	}
	if _, err := s.db.Exec(`PRAGMA user_version = 5`); err != nil {
		return fmt.Errorf("gravar versão: %w", err)
	}
	return nil
}

func (s *Store) Setup(tokenHash string, now time.Time) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var count int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM operator`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return ErrExists
	}
	if _, err := tx.Exec(`INSERT INTO operator(id, token_hash, created_at) VALUES (1, ?, ?)`, tokenHash, formatTime(now)); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO settings(id, stale_after_seconds, alert_threshold_percent, quiet_start, quiet_end, timezone) VALUES (1, 1800, 80, '', '', 'UTC')`); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) OperatorHash() (string, error) {
	var hash string
	err := s.db.QueryRow(`SELECT token_hash FROM operator WHERE id = 1`).Scan(&hash)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return hash, err
}

func (s *Store) CreateEnrollment(codeHash string, accountID string, expires time.Time) error {
	_, err := s.db.Exec(`INSERT INTO enrollment_codes(code_hash, expires_at, account_id) VALUES (?, ?, ?)`, codeHash, formatTime(expires), accountID)
	return err
}

func (s *Store) Enroll(codeHash string, deviceID string, name string, tokenHash string, now time.Time) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var expiresText string
	var used sql.NullString
	var accountID string
	err = tx.QueryRow(`SELECT expires_at, used_at, account_id FROM enrollment_codes WHERE code_hash = ?`, codeHash).Scan(&expiresText, &used, &accountID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrEnrollmentInvalid
	}
	if err != nil {
		return err
	}
	if used.Valid {
		return ErrEnrollmentUsed
	}
	expires, err := parseTime(expiresText)
	if err != nil {
		return err
	}
	if !expires.After(now) {
		return ErrEnrollmentExpired
	}
	if _, err := tx.Exec(`UPDATE enrollment_codes SET used_at = ? WHERE code_hash = ?`, formatTime(now), codeHash); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO devices(id, name, token_hash, created_at, account_id) VALUES (?, ?, ?, ?, ?)`, deviceID, name, tokenHash, formatTime(now), accountID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DeviceByTokenHash(hash string) (Device, error) {
	row := s.db.QueryRow(`SELECT id, name, token_hash, created_at, revoked_at FROM devices WHERE token_hash = ?`, hash)
	return scanDevice(row)
}

func (s *Store) ListDevices() ([]Device, error) {
	rows, err := s.db.Query(`SELECT id, name, token_hash, created_at, revoked_at FROM devices ORDER BY created_at ASC, id ASC`)
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

func (s *Store) Revoke(id string, now time.Time) (Device, error) {
	device, err := s.getDevice(id)
	if err != nil {
		return Device{}, err
	}
	if device.RevokedAt == nil {
		if _, err := s.db.Exec(`UPDATE devices SET revoked_at = ? WHERE id = ?`, formatTime(now), id); err != nil {
			return Device{}, err
		}
		revoked := now.UTC()
		device.RevokedAt = &revoked
	}
	return device, nil
}

func (s *Store) InsertSample(deviceID string, sample contract.Sample, receivedAt time.Time) (bool, error) {
	canonical, err := sample.Canonical()
	if err != nil {
		return false, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	var existing string
	err = tx.QueryRow(`SELECT payload_json FROM samples WHERE sample_id = ?`, sample.SampleID).Scan(&existing)
	if err == nil {
		var stored contract.Sample
		if unmarshalErr := json.Unmarshal([]byte(existing), &stored); unmarshalErr != nil {
			return false, unmarshalErr
		}
		if contract.SameSample(stored, sample) {
			return false, tx.Commit()
		}
		return false, ErrConflict
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if err := enforceCap(tx, deviceID, receivedAt); err != nil {
		return false, err
	}
	_, err = tx.Exec(
		`INSERT INTO samples(sample_id, device_id, provider, collected_at, received_at, payload_json) VALUES (?, ?, ?, ?, ?, ?)`,
		sample.SampleID, deviceID, sample.Provider, formatTime(sample.CollectedAt), formatTime(receivedAt), string(canonical),
	)
	if err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func (s *Store) UpsertReport(deviceID string, report contract.SourceReport, receivedAt time.Time) error {
	canonical, err := report.Canonical()
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var existing string
	err = tx.QueryRow(`SELECT payload_json FROM source_reports WHERE report_id = ?`, report.ReportID).Scan(&existing)
	if err == nil {
		var stored contract.SourceReport
		if unmarshalErr := json.Unmarshal([]byte(existing), &stored); unmarshalErr != nil {
			return unmarshalErr
		}
		if !contract.SameReport(stored, report) {
			return ErrConflict
		}
		if !report.ObservedAt.After(stored.ObservedAt) {
			return tx.Commit()
		}
		_, err = tx.Exec(
			`UPDATE source_reports SET observed_at = ?, received_at = ?, payload_json = ?, device_id = ? WHERE report_id = ?`,
			formatTime(report.ObservedAt), formatTime(receivedAt), string(canonical), deviceID, report.ReportID,
		)
		if err != nil {
			return err
		}
		return tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err := enforceCap(tx, deviceID, receivedAt); err != nil {
		return err
	}
	_, err = tx.Exec(
		`INSERT INTO source_reports(report_id, device_id, provider, observed_at, received_at, payload_json) VALUES (?, ?, ?, ?, ?, ?)`,
		report.ReportID, deviceID, report.Provider, formatTime(report.ObservedAt), formatTime(receivedAt), string(canonical),
	)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ListSamples(provider string, cursorReceived time.Time, cursorID string, limit int) ([]SampleRecord, error) {
	query := `SELECT payload_json, device_id, received_at FROM samples WHERE (? = '' OR provider = ?)`
	args := []any{provider, provider}
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
		var payload string
		var record SampleRecord
		var received string
		if err := rows.Scan(&payload, &record.DeviceID, &received); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(payload), &record.Sample); err != nil {
			return nil, err
		}
		record.ReceivedAt, err = parseTime(received)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *Store) LatestSample(provider string) (SampleRecord, error) {
	row := s.db.QueryRow(`SELECT payload_json, device_id, received_at FROM samples WHERE provider = ? ORDER BY collected_at DESC, sample_id DESC LIMIT 1`, provider)
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
	return record, err
}

func (s *Store) LatestReport(provider string) (ReportRecord, error) {
	row := s.db.QueryRow(`SELECT payload_json, device_id, received_at FROM source_reports WHERE provider = ? ORDER BY observed_at DESC, report_id DESC LIMIT 1`, provider)
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
	return record, err
}

func (s *Store) RecentSamples(provider string, limit int) ([]contract.Sample, error) {
	rows, err := s.db.Query(`SELECT payload_json FROM samples WHERE provider = ? ORDER BY collected_at DESC, sample_id DESC LIMIT ?`, provider, limit)
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

func (s *Store) StoredPayloads() ([]string, error) {
	rows, err := s.db.Query(`SELECT payload_json FROM samples UNION ALL SELECT payload_json FROM source_reports`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var payloads []string
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		payloads = append(payloads, payload)
	}
	return payloads, rows.Err()
}

func (s *Store) CountSamples() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM samples`).Scan(&count)
	return count, err
}

func (s *Store) Settings() (Settings, error) {
	var seconds int
	var settings Settings
	err := s.db.QueryRow(`SELECT stale_after_seconds, alert_threshold_percent, quiet_start, quiet_end, timezone FROM settings WHERE id = 1`).Scan(
		&seconds, &settings.AlertThreshold, &settings.QuietStart, &settings.QuietEnd, &settings.Timezone,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Settings{}, ErrNotFound
	}
	settings.StaleAfter = time.Duration(seconds) * time.Second
	return settings, err
}

func (s *Store) SaveSettings(settings Settings) error {
	seconds := int(settings.StaleAfter / time.Second)
	result, err := s.db.Exec(
		`UPDATE settings SET stale_after_seconds = ?, alert_threshold_percent = ?, quiet_start = ?, quiet_end = ?, timezone = ? WHERE id = 1`,
		seconds, settings.AlertThreshold, settings.QuietStart, settings.QuietEnd, settings.Timezone,
	)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) UpsertSubscription(sub PushSubscription) (string, error) {
	var existing string
	err := s.db.QueryRow(`SELECT id FROM push_subscriptions WHERE endpoint = ?`, sub.Endpoint).Scan(&existing)
	if err == nil {
		_, err = s.db.Exec(`UPDATE push_subscriptions SET p256dh = ?, auth_key = ?, expired_at = NULL, last_error = '' WHERE id = ?`, sub.P256dh, sub.Auth, existing)
		return existing, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	_, err = s.db.Exec(
		`INSERT INTO push_subscriptions(id, endpoint, p256dh, auth_key, created_at, last_error) VALUES (?, ?, ?, ?, ?, '')`,
		sub.ID, sub.Endpoint, sub.P256dh, sub.Auth, formatTime(sub.CreatedAt),
	)
	if err != nil {
		return "", err
	}
	return sub.ID, nil
}

func (s *Store) ListSubscriptions() ([]PushSubscription, error) {
	rows, err := s.db.Query(`SELECT id, endpoint, p256dh, auth_key, created_at, expired_at, last_error FROM push_subscriptions ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var subs []PushSubscription
	for rows.Next() {
		var sub PushSubscription
		var created string
		var expired sql.NullString
		if err := rows.Scan(&sub.ID, &sub.Endpoint, &sub.P256dh, &sub.Auth, &created, &expired, &sub.LastError); err != nil {
			return nil, err
		}
		sub.CreatedAt, err = parseTime(created)
		if err != nil {
			return nil, err
		}
		if expired.Valid {
			parsed, err := parseTime(expired.String)
			if err != nil {
				return nil, err
			}
			sub.ExpiredAt = &parsed
		}
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}

func (s *Store) DeleteSubscription(id string) error {
	result, err := s.db.Exec(`DELETE FROM push_subscriptions WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ExpireSubscription(id string, when time.Time, reason string) error {
	_, err := s.db.Exec(`UPDATE push_subscriptions SET expired_at = ?, last_error = ? WHERE id = ?`, formatTime(when), reason, id)
	return err
}

func (s *Store) InsertDelivery(delivery Delivery) (bool, error) {
	nextAttempt := ""
	if !delivery.NextAttempt.IsZero() {
		nextAttempt = formatTime(delivery.NextAttempt)
	}
	result, err := s.db.Exec(
		`INSERT INTO push_deliveries(id, idempotency_key, subscription_id, kind, provider, title, body, status, detail, attempts, next_attempt_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(idempotency_key) DO NOTHING`,
		delivery.ID, delivery.IdempotencyKey, delivery.SubscriptionID, delivery.Kind, delivery.Provider, delivery.Title, delivery.Body, delivery.Status, delivery.Detail, delivery.Attempts, nextAttempt, formatTime(delivery.CreatedAt), formatTime(delivery.UpdatedAt),
	)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n > 0, err
}

func (s *Store) ListDeliveries(limit int) ([]Delivery, error) {
	rows, err := s.db.Query(`SELECT `+deliveryColumns+` FROM push_deliveries ORDER BY created_at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var deliveries []Delivery
	for rows.Next() {
		delivery, err := scanDelivery(rows)
		if err != nil {
			return nil, err
		}
		deliveries = append(deliveries, delivery)
	}
	return deliveries, rows.Err()
}

func (s *Store) PendingDeliveries() ([]Delivery, error) {
	rows, err := s.db.Query(`SELECT ` + deliveryColumns + ` FROM push_deliveries WHERE status = 'pending' ORDER BY created_at ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var deliveries []Delivery
	for rows.Next() {
		delivery, err := scanDelivery(rows)
		if err != nil {
			return nil, err
		}
		deliveries = append(deliveries, delivery)
	}
	return deliveries, rows.Err()
}

func (s *Store) UpdateDelivery(id string, status string, detail string, attempts int, next time.Time, when time.Time) error {
	nextAttempt := ""
	if !next.IsZero() {
		nextAttempt = formatTime(next)
	}
	_, err := s.db.Exec(
		`UPDATE push_deliveries SET status = ?, detail = ?, attempts = ?, next_attempt_at = ?, updated_at = ? WHERE id = ?`,
		status, detail, attempts, nextAttempt, formatTime(when), id,
	)
	return err
}

func (s *Store) CountPendingDeliveries() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM push_deliveries WHERE status = 'pending'`).Scan(&count)
	return count, err
}

func (s *Store) ExpirePendingForSubscription(subscriptionID string, when time.Time) error {
	_, err := s.db.Exec(
		`UPDATE push_deliveries SET status = 'subscription_expired', detail = 'SUBSCRIPTION_EXPIRED', next_attempt_at = '', updated_at = ? WHERE subscription_id = ? AND status = 'pending'`,
		formatTime(when), subscriptionID,
	)
	return err
}

func (s *Store) getDevice(id string) (Device, error) {
	row := s.db.QueryRow(`SELECT id, name, token_hash, created_at, revoked_at FROM devices WHERE id = ?`, id)
	return scanDevice(row)
}

type scanner interface {
	Scan(dest ...any) error
}

func scanDevice(row scanner) (Device, error) {
	var device Device
	var created string
	var revoked sql.NullString
	err := row.Scan(&device.ID, &device.Name, &device.TokenHash, &created, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return Device{}, ErrNotFound
	}
	if err != nil {
		return Device{}, err
	}
	device.CreatedAt, err = parseTime(created)
	if err != nil {
		return Device{}, err
	}
	if revoked.Valid {
		parsed, err := parseTime(revoked.String)
		if err != nil {
			return Device{}, err
		}
		device.RevokedAt = &parsed
	}
	return device, nil
}

func scanDelivery(row scanner) (Delivery, error) {
	var delivery Delivery
	var created string
	var updated string
	var next string
	err := row.Scan(&delivery.ID, &delivery.IdempotencyKey, &delivery.SubscriptionID, &delivery.Kind, &delivery.Provider, &delivery.Title, &delivery.Body, &delivery.Status, &delivery.Detail, &delivery.Attempts, &next, &created, &updated)
	if err != nil {
		return Delivery{}, err
	}
	delivery.CreatedAt, err = parseTime(created)
	if err != nil {
		return Delivery{}, err
	}
	delivery.UpdatedAt, err = parseTime(updated)
	if err != nil {
		return Delivery{}, err
	}
	if next != "" {
		delivery.NextAttempt, err = parseTime(next)
		if err != nil {
			return Delivery{}, err
		}
	}
	return delivery, nil
}

func enforceCap(tx *sql.Tx, deviceID string, now time.Time) error {
	since := formatTime(now.Add(-time.Hour))
	var count int
	err := tx.QueryRow(
		`SELECT
		  (SELECT COUNT(*) FROM samples WHERE device_id = ? AND received_at >= ?)
		  + (SELECT COUNT(*) FROM source_reports WHERE device_id = ? AND received_at >= ?)`,
		deviceID, since, deviceID, since,
	).Scan(&count)
	if err != nil {
		return err
	}
	if count >= observationCap {
		return ErrRateLimited
	}
	return nil
}

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func parseTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.UTC(), nil
}

func NormalizeQuiet(value string) string {
	return strings.TrimSpace(value)
}
