package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"ai-usage-tracker/internal/apierr"
	"ai-usage-tracker/internal/collect"
	"ai-usage-tracker/internal/spool"
)

type Config struct {
	API          string
	Token        string
	SpoolPath    string
	ClaudeFile   string
	CodexDir     string
	CodexCommand string
	AgyCommand   string
	Now          time.Time
	Client       *http.Client
}

type Result struct {
	Collected int
	Accepted  int
	Duplicate int
	Rejected  int
	Pending   int
	Blocked   bool
	CodexLive string
	AgyLive   string
	Outcomes  []Outcome
}

// Outcome says what happened to one queued item in this send.
// Status is accepted, duplicate, rejected, pending or blocked.
type Outcome struct {
	Kind     string
	Provider string
	Status   string
	Code     string
}

func outcomeOf(item spool.Item, status string, code string) Outcome {
	var body struct {
		Provider string `json:"provider"`
	}
	_ = json.Unmarshal(item.Body, &body)
	return Outcome{Kind: item.Kind, Provider: body.Provider, Status: status, Code: code}
}

func outcomesFrom(items []spool.Item, from int, status string) []Outcome {
	var out []Outcome
	for i := from; i < len(items); i++ {
		if items[i].State == spool.StatePending {
			out = append(out, outcomeOf(items[i], status, ""))
		}
	}
	return out
}

func Run(ctx context.Context, cfg Config) (Result, error) {
	if cfg.Now.IsZero() {
		cfg.Now = time.Now().UTC()
	}
	if cfg.Client == nil {
		cfg.Client = &http.Client{Timeout: 10 * time.Second}
	}
	batch, err := collect.Read(collect.Request{
		ClaudeFile: cfg.ClaudeFile,
		CodexDir:   cfg.CodexDir,
		Now:        cfg.Now,
	})
	if err != nil {
		if !errors.Is(err, collect.ErrNoSource) || cfg.AgyCommand == "" {
			return Result{}, err
		}
		batch = collect.Batch{}
	}
	codexLive := ""
	if cfg.CodexDir != "" && cfg.CodexCommand != "" {
		live, liveErr := collect.ReadCodexLive(ctx, cfg.CodexCommand, cfg.Now)
		if liveErr == nil && live != nil {
			batch = collect.PreferCodex(batch, *live)
			codexLive = "read"
		} else {
			codexLive = "fallback"
		}
	}
	agyLive := ""
	if cfg.AgyCommand != "" {
		live, liveErr := collect.ReadAntigravity(ctx, cfg.AgyCommand, cfg.Now)
		if liveErr == nil && live != nil {
			batch.Samples = append(batch.Samples, *live)
			agyLive = "read"
		} else {
			agyLive = "miss"
		}
	}
	var incoming []spool.Item
	for _, sample := range batch.Samples {
		body, err := sample.Canonical()
		if err != nil {
			return Result{CodexLive: codexLive, AgyLive: agyLive}, err
		}
		incoming = append(incoming, spool.Item{Kind: spool.KindSample, State: spool.StatePending, Body: body})
	}
	for _, report := range batch.Reports {
		body, err := report.Canonical()
		if err != nil {
			return Result{CodexLive: codexLive, AgyLive: agyLive}, err
		}
		incoming = append(incoming, spool.Item{Kind: spool.KindReport, State: spool.StatePending, Body: body})
	}
	items, err := spool.Merge(cfg.SpoolPath, incoming)
	if err != nil {
		return Result{CodexLive: codexLive, AgyLive: agyLive}, err
	}
	result, err := send(ctx, cfg, items)
	result.CodexLive = codexLive
	result.AgyLive = agyLive
	return result, err
}

func send(ctx context.Context, cfg Config, items []spool.Item) (Result, error) {
	result := Result{Collected: len(items)}
	if spool.HasBlocked(items) {
		result.Outcomes = outcomesFrom(items, 0, "blocked")
		changed := false
		for i := range items {
			if items[i].State == spool.StatePending {
				items[i].State = spool.StateBlocked
				changed = true
			}
		}
		if changed {
			if err := spool.Save(cfg.SpoolPath, items); err != nil {
				return Result{}, err
			}
		}
		result.Blocked = true
		result.Pending = countState(items, spool.StatePending)
		return result, nil
	}
	for i := range items {
		if items[i].State != spool.StatePending {
			continue
		}
		path := "/v1/samples"
		if items[i].Kind == spool.KindReport {
			path = "/v1/source-reports"
		}
		status, code, err := postJSON(ctx, cfg.Client, cfg.API, cfg.Token, path, items[i].Body)
		if err != nil {
			kept, saveErr := persist(cfg.SpoolPath, items)
			if saveErr != nil {
				return result, errors.Join(err, saveErr)
			}
			result.Pending = countState(kept, spool.StatePending)
			result.Outcomes = append(result.Outcomes, outcomesFrom(items, i, "pending")...)
			return result, err
		}
		if status == http.StatusUnauthorized && code == apierr.CodeDeviceRevoked {
			result.Outcomes = append(result.Outcomes, outcomesFrom(items, i, "blocked")...)
			for j := range items {
				if items[j].State == spool.StatePending {
					items[j].State = spool.StateBlocked
				}
			}
			if err := spool.Save(cfg.SpoolPath, items); err != nil {
				return result, err
			}
			result.Blocked = true
			return result, nil
		}
		if status == http.StatusOK {
			result.Outcomes = append(result.Outcomes, outcomeOf(items[i], "duplicate", ""))
			items[i].State = "confirmed"
			result.Duplicate++
			continue
		}
		if status == http.StatusCreated {
			result.Outcomes = append(result.Outcomes, outcomeOf(items[i], "accepted", ""))
			items[i].State = "confirmed"
			result.Accepted++
			continue
		}
		if status >= 400 && status < 500 {
			result.Outcomes = append(result.Outcomes, outcomeOf(items[i], "rejected", code))
			items[i].State = spool.StateRejected
			result.Rejected++
			continue
		}
		kept, err := persist(cfg.SpoolPath, items)
		if err != nil {
			return result, err
		}
		result.Pending = countState(kept, spool.StatePending)
		result.Outcomes = append(result.Outcomes, outcomesFrom(items, i, "pending")...)
		return result, fmt.Errorf("resposta %d", status)
	}
	kept, err := persist(cfg.SpoolPath, items)
	if err != nil {
		return result, err
	}
	result.Pending = countState(kept, spool.StatePending)
	result.Blocked = spool.HasBlocked(kept)
	return result, nil
}

func postJSON(ctx context.Context, client *http.Client, api string, token string, path string, body []byte) (int, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(api, "/")+path, bytes.NewReader(body))
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return resp.StatusCode, "", err
	}
	var payload struct {
		Code string `json:"error_code"`
	}
	_ = json.Unmarshal(raw, &payload)
	return resp.StatusCode, payload.Code, nil
}

func persist(path string, items []spool.Item) ([]spool.Item, error) {
	kept := make([]spool.Item, 0, len(items))
	for _, item := range items {
		if item.State == "confirmed" {
			continue
		}
		kept = append(kept, item)
	}
	if err := spool.Save(path, kept); err != nil {
		return nil, err
	}
	return kept, nil
}

func countState(items []spool.Item, state string) int {
	count := 0
	for _, item := range items {
		if item.State == state {
			count++
		}
	}
	return count
}

type Enrollment struct {
	DeviceID string
	Token    string
	Name     string
}

func Enroll(ctx context.Context, client *http.Client, api string, code string, name string) (Enrollment, error) {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	body, err := json.Marshal(map[string]string{"code": code, "name": name})
	if err != nil {
		return Enrollment{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(api, "/")+"/v1/devices", bytes.NewReader(body))
	if err != nil {
		return Enrollment{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return Enrollment{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Enrollment{}, err
	}
	if resp.StatusCode != http.StatusCreated {
		return Enrollment{}, fmt.Errorf("enrolamento recusado")
	}
	var payload struct {
		DeviceID string `json:"device_id"`
		Token    string `json:"token"`
		Name     string `json:"name"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return Enrollment{}, err
	}
	return Enrollment{DeviceID: payload.DeviceID, Token: payload.Token, Name: payload.Name}, nil
}

// Token states reported by CheckToken.
const (
	TokenAccepted = "accepted"
	TokenRefused  = "refused"
	TokenRevoked  = "revoked"
	TokenUnknown  = "unknown"
)

type TokenCheck struct {
	State  string
	Device string
}

// CheckToken asks the server whether it accepts the device token. It writes nothing.
// A server that predates GET /v1/device answers 404, which is reported as unknown.
func CheckToken(ctx context.Context, client *http.Client, api string, token string) (TokenCheck, error) {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(api, "/")+"/v1/device", nil)
	if err != nil {
		return TokenCheck{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		return TokenCheck{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return TokenCheck{}, err
	}
	switch resp.StatusCode {
	case http.StatusOK:
		var body struct {
			Name string `json:"name"`
		}
		_ = json.Unmarshal(raw, &body)
		return TokenCheck{State: TokenAccepted, Device: body.Name}, nil
	case http.StatusUnauthorized:
		var body struct {
			Code string `json:"error_code"`
		}
		_ = json.Unmarshal(raw, &body)
		if body.Code == apierr.CodeDeviceRevoked {
			return TokenCheck{State: TokenRevoked}, nil
		}
		return TokenCheck{State: TokenRefused}, nil
	}
	return TokenCheck{State: TokenUnknown}, nil
}
