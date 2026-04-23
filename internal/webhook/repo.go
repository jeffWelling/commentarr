package webhook

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// Subscriber is one configured webhook endpoint.
type Subscriber struct {
	ID        string
	Name      string
	URL       string
	Events    []Event
	BasicUser string
	BasicPass string
	Headers   map[string]string
	Enabled   bool
}

// Repo persists subscribers + delivery records.
type Repo struct {
	db *sql.DB
}

// NewRepo returns a Repo.
func NewRepo(d *sql.DB) *Repo { return &Repo{db: d} }

// SaveSubscriber upserts a subscriber.
func (r *Repo) SaveSubscriber(ctx context.Context, s Subscriber) error {
	events, err := json.Marshal(s.Events)
	if err != nil {
		return err
	}
	headers, err := json.Marshal(s.Headers)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO webhooks (id, name, url, events_json, basic_user, basic_pass, headers_json, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
		  name = excluded.name,
		  url = excluded.url,
		  events_json = excluded.events_json,
		  basic_user = excluded.basic_user,
		  basic_pass = excluded.basic_pass,
		  headers_json = excluded.headers_json,
		  enabled = excluded.enabled,
		  updated_at = CURRENT_TIMESTAMP`,
		s.ID, s.Name, s.URL, string(events),
		nullIfEmpty(s.BasicUser), nullIfEmpty(s.BasicPass),
		string(headers), s.Enabled)
	if err != nil {
		return fmt.Errorf("save subscriber %s: %w", s.ID, err)
	}
	return nil
}

// SubscribersFor returns enabled subscribers interested in event.
func (r *Repo) SubscribersFor(ctx context.Context, event Event) ([]Subscriber, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, url, events_json, COALESCE(basic_user,''), COALESCE(basic_pass,''), headers_json, enabled
		FROM webhooks WHERE enabled = TRUE`)
	if err != nil {
		return nil, fmt.Errorf("query subscribers: %w", err)
	}
	defer rows.Close()

	var out []Subscriber
	for rows.Next() {
		var s Subscriber
		var eventsJSON, headersJSON string
		if err := rows.Scan(&s.ID, &s.Name, &s.URL, &eventsJSON,
			&s.BasicUser, &s.BasicPass, &headersJSON, &s.Enabled); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(eventsJSON), &s.Events)
		_ = json.Unmarshal([]byte(headersJSON), &s.Headers)
		for _, e := range s.Events {
			if e == event {
				out = append(out, s)
				break
			}
		}
	}
	return out, rows.Err()
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
