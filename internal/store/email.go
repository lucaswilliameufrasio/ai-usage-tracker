package store

import (
	"database/sql"
	"errors"
	"time"
)

type EmailChannel struct {
	Provider   string
	From       string
	To         string
	AccountRef string
	Active     bool
	UpdatedAt  time.Time
}

type EmailDelivery struct {
	ID             string
	IdempotencyKey string
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

const emailDeliveryColumns = `id, idempotency_key, kind, provider, title, body, status, detail, attempts, next_attempt_at, created_at, updated_at`

func (s *Store) EmailChannel() (EmailChannel, bool, error) {
	var channel EmailChannel
	var active int
	var updated string
	err := s.db.QueryRow(`SELECT provider, from_address, to_address, account_ref, active, updated_at FROM email_channel WHERE id = 1`).Scan(
		&channel.Provider, &channel.From, &channel.To, &channel.AccountRef, &active, &updated,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return EmailChannel{}, false, nil
	}
	if err != nil {
		return EmailChannel{}, false, err
	}
	channel.Active = active != 0
	channel.UpdatedAt, err = parseTime(updated)
	if err != nil {
		return EmailChannel{}, false, err
	}
	return channel, true, nil
}

func (s *Store) SaveEmailChannel(channel EmailChannel, now time.Time) error {
	active := 0
	if channel.Active {
		active = 1
	}
	_, err := s.db.Exec(`
INSERT INTO email_channel(id, provider, from_address, to_address, account_ref, active, updated_at)
VALUES (1, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  provider = excluded.provider,
  from_address = excluded.from_address,
  to_address = excluded.to_address,
  account_ref = excluded.account_ref,
  active = excluded.active,
  updated_at = excluded.updated_at`,
		channel.Provider, channel.From, channel.To, channel.AccountRef, active, formatTime(now),
	)
	return err
}

func (s *Store) DeleteEmailChannel() error {
	_, err := s.db.Exec(`DELETE FROM email_channel WHERE id = 1`)
	return err
}

func (s *Store) InsertEmailDelivery(delivery EmailDelivery) (bool, error) {
	nextAttempt := ""
	if !delivery.NextAttempt.IsZero() {
		nextAttempt = formatTime(delivery.NextAttempt)
	}
	result, err := s.db.Exec(
		`INSERT INTO email_deliveries(id, idempotency_key, kind, provider, title, body, status, detail, attempts, next_attempt_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(idempotency_key) DO NOTHING`,
		delivery.ID, delivery.IdempotencyKey, delivery.Kind, delivery.Provider, delivery.Title, delivery.Body, delivery.Status, delivery.Detail, delivery.Attempts, nextAttempt, formatTime(delivery.CreatedAt), formatTime(delivery.UpdatedAt),
	)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n > 0, err
}

func (s *Store) ListEmailDeliveries(limit int) ([]EmailDelivery, error) {
	rows, err := s.db.Query(`SELECT `+emailDeliveryColumns+` FROM email_deliveries ORDER BY created_at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var deliveries []EmailDelivery
	for rows.Next() {
		delivery, err := scanEmailDelivery(rows)
		if err != nil {
			return nil, err
		}
		deliveries = append(deliveries, delivery)
	}
	return deliveries, rows.Err()
}

func (s *Store) PendingEmail() ([]EmailDelivery, error) {
	rows, err := s.db.Query(`SELECT ` + emailDeliveryColumns + ` FROM email_deliveries WHERE status = 'pending' ORDER BY created_at ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var deliveries []EmailDelivery
	for rows.Next() {
		delivery, err := scanEmailDelivery(rows)
		if err != nil {
			return nil, err
		}
		deliveries = append(deliveries, delivery)
	}
	return deliveries, rows.Err()
}

func (s *Store) UpdateEmailDelivery(id string, status string, detail string, attempts int, next time.Time, when time.Time) error {
	nextAttempt := ""
	if !next.IsZero() {
		nextAttempt = formatTime(next)
	}
	_, err := s.db.Exec(
		`UPDATE email_deliveries SET status = ?, detail = ?, attempts = ?, next_attempt_at = ?, updated_at = ? WHERE id = ?`,
		status, detail, attempts, nextAttempt, formatTime(when), id,
	)
	return err
}

func (s *Store) CountPendingEmail() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM email_deliveries WHERE status = 'pending'`).Scan(&count)
	return count, err
}

func (s *Store) FailPendingEmail(detail string, when time.Time) error {
	_, err := s.db.Exec(
		`UPDATE email_deliveries SET status = 'failed', detail = ?, next_attempt_at = '', updated_at = ? WHERE status = 'pending'`,
		detail, formatTime(when),
	)
	return err
}

func scanEmailDelivery(row scanner) (EmailDelivery, error) {
	var delivery EmailDelivery
	var created string
	var updated string
	var next string
	err := row.Scan(&delivery.ID, &delivery.IdempotencyKey, &delivery.Kind, &delivery.Provider, &delivery.Title, &delivery.Body, &delivery.Status, &delivery.Detail, &delivery.Attempts, &next, &created, &updated)
	if err != nil {
		return EmailDelivery{}, err
	}
	delivery.CreatedAt, err = parseTime(created)
	if err != nil {
		return EmailDelivery{}, err
	}
	delivery.UpdatedAt, err = parseTime(updated)
	if err != nil {
		return EmailDelivery{}, err
	}
	if next != "" {
		delivery.NextAttempt, err = parseTime(next)
		if err != nil {
			return EmailDelivery{}, err
		}
	}
	return delivery, nil
}
