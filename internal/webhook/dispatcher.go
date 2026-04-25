package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/jeffWelling/commentarr/internal/metrics"
)

// DispatcherConfig configures retry behaviour.
type DispatcherConfig struct {
	MaxAttempts  int
	RetryBackoff time.Duration
	Timeout      time.Duration
}

// Dispatcher fans events out to configured subscribers synchronously.
// Synchronous matches the *arr baseline — operators expect the call
// that triggered the event to wait for a 200 from the receiver.
// Async queueing belongs behind a separate config flag if it ever lands.
type Dispatcher struct {
	repo *Repo
	cfg  DispatcherConfig
	hc   *http.Client
}

// NewDispatcher returns a Dispatcher.
func NewDispatcher(repo *Repo, cfg DispatcherConfig) *Dispatcher {
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 3
	}
	if cfg.RetryBackoff <= 0 {
		cfg.RetryBackoff = 30 * time.Second
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	return &Dispatcher{repo: repo, cfg: cfg, hc: &http.Client{Timeout: cfg.Timeout}}
}

// Dispatch sends event to every enabled subscriber subscribed to event.
func (d *Dispatcher) Dispatch(ctx context.Context, event Event, payload map[string]interface{}) error {
	subs, err := d.repo.SubscribersFor(ctx, event)
	if err != nil {
		return err
	}
	if len(subs) == 0 {
		return nil
	}

	envelope := Envelope{
		EventType: event,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Version:   "1",
		Payload:   payload,
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}

	for _, sub := range subs {
		d.deliver(ctx, sub, event, body)
	}
	return nil
}

func (d *Dispatcher) deliver(ctx context.Context, sub Subscriber, event Event, body []byte) {
	start := time.Now()
	backoff := d.cfg.RetryBackoff

	for attempt := 1; attempt <= d.cfg.MaxAttempts; attempt++ {
		err := d.postOnce(ctx, sub, body)
		if err == nil {
			metrics.WebhookDeliveriesTotal.WithLabelValues(string(event), "success").Inc()
			metrics.WebhookDeliveryDurationSeconds.WithLabelValues(string(event)).Observe(time.Since(start).Seconds())
			return
		}
		if attempt == d.cfg.MaxAttempts {
			metrics.WebhookDeliveriesTotal.WithLabelValues(string(event), "failure").Inc()
			return
		}
		metrics.WebhookDeliveriesTotal.WithLabelValues(string(event), "retry").Inc()
		select {
		case <-time.After(backoff):
			backoff *= 2
		case <-ctx.Done():
			return
		}
	}
}

func (d *Dispatcher) postOnce(ctx context.Context, sub Subscriber, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sub.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if sub.BasicUser != "" {
		req.SetBasicAuth(sub.BasicUser, sub.BasicPass)
	}
	for k, v := range sub.Headers {
		req.Header.Set(k, v)
	}
	resp, err := d.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook %s: %d", sub.Name, resp.StatusCode)
	}
	return nil
}
