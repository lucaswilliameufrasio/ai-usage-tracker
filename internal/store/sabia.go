package store

import (
	"database/sql"
	"errors"
	"time"
)

type SabiaLink struct {
	Origin        string
	ApplicationID string
	UpdatedAt     time.Time
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
	err := s.db.QueryRow(`SELECT origin, application_id, updated_at FROM sabia_link WHERE id = 1`).Scan(&link.Origin, &link.ApplicationID, &updated)
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

func (s *Store) SaveSabiaLink(origin string, applicationID string, now time.Time) error {
	_, err := s.db.Exec(`
INSERT INTO sabia_link(id, origin, application_id, updated_at) VALUES (1, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET origin = excluded.origin, application_id = excluded.application_id, updated_at = excluded.updated_at
`, origin, applicationID, now.UTC().Format(time.RFC3339))
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
