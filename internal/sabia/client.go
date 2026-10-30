package sabia

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

type CallError struct {
	message string
}

func (e *CallError) Error() string {
	return e.message
}

func callError(message string) error {
	return &CallError{message: message}
}

type Change struct {
	ID                   string
	Operation            string
	ConfirmationRequired bool
}

type Plan struct {
	Changes []Change
}

type Applied struct {
	Applied   int
	Unchanged int
}

func PlanDeployment(ctx context.Context, client *http.Client, origin string, key string, manifest []byte) (Plan, error) {
	raw, err := post(ctx, client, origin, key, "/v1/template-deployments/plan", manifest)
	if err != nil {
		return Plan{}, err
	}
	var body struct {
		Changes []struct {
			ID                   string `json:"id"`
			Operation            string `json:"operation"`
			ConfirmationRequired bool   `json:"confirmation_required"`
		} `json:"changes"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return Plan{}, callError("resposta do Sabiá ilegível")
	}
	plan := Plan{Changes: make([]Change, 0, len(body.Changes))}
	for _, item := range body.Changes {
		plan.Changes = append(plan.Changes, Change{
			ID:                   item.ID,
			Operation:            operationLabel(item.Operation),
			ConfirmationRequired: item.ConfirmationRequired,
		})
	}
	return plan, nil
}

func ApplyDeployment(ctx context.Context, client *http.Client, origin string, key string, manifest []byte, confirmations []string) (Applied, error) {
	var decoded map[string]any
	if err := json.Unmarshal(manifest, &decoded); err != nil {
		return Applied{}, callError("não foi possível chamar o Sabiá")
	}
	if confirmations == nil {
		confirmations = []string{}
	}
	decoded["confirmations"] = confirmations
	payload, err := encodeBody(decoded)
	if err != nil {
		return Applied{}, callError("não foi possível chamar o Sabiá")
	}
	raw, err := post(ctx, client, origin, key, "/v1/template-deployments/apply", payload)
	if err != nil {
		return Applied{}, err
	}
	var body struct {
		Applied   int `json:"applied"`
		Unchanged int `json:"unchanged"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return Applied{}, callError("resposta do Sabiá ilegível")
	}
	return Applied{Applied: body.Applied, Unchanged: body.Unchanged}, nil
}

func post(ctx context.Context, client *http.Client, origin string, key string, path string, payload []byte) ([]byte, error) {
	if client == nil {
		client = http.DefaultClient
	}
	callCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(callCtx, http.MethodPost, strings.TrimRight(origin, "/")+path, bytes.NewReader(payload))
	if err != nil {
		return nil, callError("não foi possível chamar o Sabiá")
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ai-usage-tracker")
	res, err := client.Do(req)
	if err != nil {
		return nil, callError("o Sabiá não respondeu")
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return nil, callError("resposta do Sabiá ilegível")
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, statusError(res.StatusCode, raw, key)
	}
	return raw, nil
}

func statusError(status int, raw []byte, key string) error {
	switch status {
	case http.StatusUnauthorized:
		return callError("a chave de deploy do Sabiá foi recusada")
	case http.StatusRequestEntityTooLarge:
		return callError("o manifesto passou do limite do Sabiá")
	default:
		message := remoteMessage(raw, key)
		if message == "" {
			return callError("o Sabiá recusou o manifesto")
		}
		return callError("o Sabiá recusou o manifesto: " + message)
	}
}

func encodeBody(value any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(value); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func remoteMessage(raw []byte, key string) string {
	var body struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return ""
	}
	message := strings.TrimSpace(body.Message)
	if message == "" || strings.Contains(message, key) {
		return ""
	}
	if utf8.RuneCountInString(message) > 180 {
		message = string([]rune(message)[:180])
	}
	return message
}

func operationLabel(operation string) string {
	switch operation {
	case "create":
		return "criar"
	case "update":
		return "atualizar"
	case "move":
		return "mover"
	case "unchanged":
		return "sem mudança"
	default:
		return operation
	}
}
