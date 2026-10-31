package store

import (
	"database/sql"
	"errors"
	"time"
)

type SabiaLink struct {
	Origin        string
	ApplicationID string
	Recipient     string
	UpdatedAt     time.Time
}

type SabiaDelivery struct {
	ID             string
	IdempotencyKey string
	Kind           string
	Provider       string
	Title          string
	Body           string
	WindowName     string
	UsedLabel      string
	ResetsLabel    string
	Status         string
	Detail         string
	Attempts       int
	NextAttempt    time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type SabiaTemplate struct {
	Name      string
	Subject   string
	HTML      string
	Text      string
	UpdatedAt time.Time
}

func (s *Store) SabiaLink() (SabiaLink, error) {
	var link SabiaLink
	var updated string
	err := s.db.QueryRow(`SELECT origin, application_id, recipient, updated_at FROM sabia_link WHERE id = 1`).Scan(&link.Origin, &link.ApplicationID, &link.Recipient, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return SabiaLink{}, nil
	}
	if err != nil {
		return SabiaLink{}, err
	}
	when, err := time.Parse(time.RFC3339, updated)
	if err != nil {
		return SabiaLink{}, err
	}
	link.UpdatedAt = when
	return link, nil
}

func (s *Store) SaveSabiaLink(origin string, applicationID string, recipient string, now time.Time) error {
	_, err := s.db.Exec(`
INSERT INTO sabia_link(id, origin, application_id, recipient, updated_at) VALUES (1, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET origin = excluded.origin, application_id = excluded.application_id, recipient = excluded.recipient, updated_at = excluded.updated_at
`, origin, applicationID, recipient, now.UTC().Format(time.RFC3339))
	return err
}

func (s *Store) SabiaTemplate() (SabiaTemplate, bool, error) {
	var template SabiaTemplate
	var updated string
	err := s.db.QueryRow(`SELECT name, subject, html, text, updated_at FROM sabia_template WHERE id = 1`).Scan(&template.Name, &template.Subject, &template.HTML, &template.Text, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return SabiaTemplate{}, false, nil
	}
	if err != nil {
		return SabiaTemplate{}, false, err
	}
	when, err := time.Parse(time.RFC3339, updated)
	if err != nil {
		return SabiaTemplate{}, false, err
	}
	template.UpdatedAt = when
	return template, true, nil
}

func (s *Store) SaveSabiaTemplate(template SabiaTemplate, now time.Time) error {
	_, err := s.db.Exec(`
INSERT INTO sabia_template(id, name, subject, html, text, updated_at) VALUES (1, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET name = excluded.name, subject = excluded.subject, html = excluded.html, text = excluded.text, updated_at = excluded.updated_at
`, template.Name, template.Subject, template.HTML, template.Text, now.UTC().Format(time.RFC3339))
	return err
}

const sabiaDeliveryColumns = `id, idempotency_key, kind, provider, title, body, window_name, used_label, resets_label, status, detail, attempts, next_attempt_at, created_at, updated_at`

func (s *Store) InsertSabiaDelivery(delivery SabiaDelivery) (bool, error) {
	nextAttempt := ""
	if !delivery.NextAttempt.IsZero() {
		nextAttempt = formatTime(delivery.NextAttempt)
	}
	result, err := s.db.Exec(
		`INSERT INTO sabia_deliveries(id, idempotency_key, kind, provider, title, body, window_name, used_label, resets_label, status, detail, attempts, next_attempt_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(idempotency_key) DO NOTHING`,
		delivery.ID, delivery.IdempotencyKey, delivery.Kind, delivery.Provider, delivery.Title, delivery.Body, delivery.WindowName, delivery.UsedLabel, delivery.ResetsLabel, delivery.Status, delivery.Detail, delivery.Attempts, nextAttempt, formatTime(delivery.CreatedAt), formatTime(delivery.UpdatedAt),
	)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n > 0, err
}

func (s *Store) ListSabiaDeliveries(limit int) ([]SabiaDelivery, error) {
	rows, err := s.db.Query(`SELECT `+sabiaDeliveryColumns+` FROM sabia_deliveries ORDER BY created_at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var deliveries []SabiaDelivery
	for rows.Next() {
		delivery, err := scanSabiaDelivery(rows)
		if err != nil {
			return nil, err
		}
		deliveries = append(deliveries, delivery)
	}
	return deliveries, rows.Err()
}

func (s *Store) PendingSabia() ([]SabiaDelivery, error) {
	rows, err := s.db.Query(`SELECT ` + sabiaDeliveryColumns + ` FROM sabia_deliveries WHERE status = 'pending' ORDER BY created_at ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var deliveries []SabiaDelivery
	for rows.Next() {
		delivery, err := scanSabiaDelivery(rows)
		if err != nil {
			return nil, err
		}
		deliveries = append(deliveries, delivery)
	}
	return deliveries, rows.Err()
}

func (s *Store) UpdateSabiaDelivery(id string, status string, detail string, attempts int, next time.Time, when time.Time) error {
	nextAttempt := ""
	if !next.IsZero() {
		nextAttempt = formatTime(next)
	}
	_, err := s.db.Exec(
		`UPDATE sabia_deliveries SET status = ?, detail = ?, attempts = ?, next_attempt_at = ?, updated_at = ? WHERE id = ?`,
		status, detail, attempts, nextAttempt, formatTime(when), id,
	)
	return err
}

func (s *Store) CountPendingSabia() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM sabia_deliveries WHERE status = 'pending'`).Scan(&count)
	return count, err
}

func (s *Store) FailPendingSabia(detail string, when time.Time) error {
	_, err := s.db.Exec(
		`UPDATE sabia_deliveries SET status = 'failed', detail = ?, next_attempt_at = '', updated_at = ? WHERE status = 'pending'`,
		detail, formatTime(when),
	)
	return err
}

func scanSabiaDelivery(row scanner) (SabiaDelivery, error) {
	var delivery SabiaDelivery
	var created string
	var updated string
	var next string
	err := row.Scan(&delivery.ID, &delivery.IdempotencyKey, &delivery.Kind, &delivery.Provider, &delivery.Title, &delivery.Body, &delivery.WindowName, &delivery.UsedLabel, &delivery.ResetsLabel, &delivery.Status, &delivery.Detail, &delivery.Attempts, &next, &created, &updated)
	if err != nil {
		return SabiaDelivery{}, err
	}
	delivery.CreatedAt, err = parseTime(created)
	if err != nil {
		return SabiaDelivery{}, err
	}
	delivery.UpdatedAt, err = parseTime(updated)
	if err != nil {
		return SabiaDelivery{}, err
	}
	if next != "" {
		delivery.NextAttempt, err = parseTime(next)
		if err != nil {
			return SabiaDelivery{}, err
		}
	}
	return delivery, nil
}
