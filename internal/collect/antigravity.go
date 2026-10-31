package collect

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"ai-usage-tracker/internal/contract"
)

const antigravityTimeout = 45 * time.Second

var errLiveAntigravity = errors.New("leitura do Antigravity falhou")

// ReadAntigravity asks the local agy CLI for plan windows. The CLI owns the
// login. This process does not read the token and does not keep the raw reply.
func ReadAntigravity(ctx context.Context, bin string, now time.Time) (*contract.Sample, error) {
	resolved, err := resolveCodex(bin)
	if err != nil {
		return nil, errLiveAntigravity
	}
	liveCtx, cancel := context.WithTimeout(ctx, antigravityTimeout)
	defer cancel()
	cmd := exec.CommandContext(liveCtx, resolved, "--print-timeout", "20s", "--output-format", "json", "-p", "/usage")
	cmd.Env = antigravityEnv()
	cmd.Dir = os.TempDir()
	cmd.Stderr = io.Discard
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = time.Second
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	out, err := cmd.Output()
	if err != nil {
		return nil, errLiveAntigravity
	}
	if len(out) == 0 || len(out) > 1<<20 {
		return nil, errLiveAntigravity
	}
	sample, err := sampleFromAntigravity(out, now)
	if err != nil {
		return nil, errLiveAntigravity
	}
	return sample, nil
}

func antigravityEnv() []string {
	allowed := []string{
		"HOME", "USER", "LOGNAME", "PATH", "LANG", "LC_ALL", "LC_CTYPE",
		"XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME", "XDG_RUNTIME_DIR",
		"SSL_CERT_FILE", "SSL_CERT_DIR", "TMPDIR", "DBUS_SESSION_BUS_ADDRESS",
	}
	var env []string
	for _, key := range allowed {
		value, ok := os.LookupEnv(key)
		if !ok || value == "" {
			continue
		}
		env = append(env, key+"="+value)
	}
	env = append(env, "AGY_CLI_DISABLE_AUTO_UPDATE=true", "BROWSER=/bin/false")
	return env
}

type agyPayload struct {
	Status   string   `json:"status"`
	NumTurns *float64 `json:"num_turns"`
	Command  *struct {
		Name string `json:"name"`
		Data *struct {
			Groups []agyGroup `json:"groups"`
		} `json:"data"`
	} `json:"command"`
}

type agyGroup struct {
	Name    string      `json:"name"`
	Buckets []agyBucket `json:"buckets"`
}

type agyBucket struct {
	ID                string          `json:"id"`
	Window            string          `json:"window"`
	RemainingFraction *float64        `json:"remaining_fraction"`
	RemainingCamel    *float64        `json:"remainingFraction"`
	ResetTime         json.RawMessage `json:"reset_time"`
	ResetCamel        json.RawMessage `json:"resetTime"`
}

func sampleFromAntigravity(raw []byte, now time.Time) (*contract.Sample, error) {
	var parsed agyPayload
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, errLiveAntigravity
	}
	if parsed.Status != "" && parsed.Status != "SUCCESS" {
		return nil, errLiveAntigravity
	}
	if parsed.NumTurns != nil && *parsed.NumTurns > 0 {
		return nil, errLiveAntigravity
	}
	if parsed.Command == nil || parsed.Command.Name != "usage" || parsed.Command.Data == nil {
		return nil, errLiveAntigravity
	}
	windows := map[string]contract.Window{}
	for _, group := range parsed.Command.Data.Groups {
		for _, bucket := range group.Buckets {
			name, ok := antigravityWindowName(group.Name, bucket.ID, bucket.Window)
			if !ok {
				continue
			}
			remaining, ok := antigravityRemaining(bucket)
			if !ok {
				continue
			}
			resets, ok := antigravityReset(bucket)
			if !ok {
				continue
			}
			used := math.Round((1-remaining)*1000) / 10
			window := contract.Window{
				Name:     name,
				Used:     used,
				Limit:    100,
				Unit:     contract.UnitPercent,
				ResetsAt: resets,
			}
			current, exists := windows[name]
			if exists && current.Used >= window.Used {
				continue
			}
			windows[name] = window
		}
	}
	if len(windows) == 0 {
		return nil, errLiveAntigravity
	}
	ordered := make([]contract.Window, 0, len(windows))
	for _, window := range windows {
		ordered = append(ordered, window)
	}
	observed := now.UTC().Truncate(time.Second)
	sample, report, err := finishClaude(contract.ProviderAntigravity, "", &observed, ordered, false, observed)
	if err != nil || sample == nil || report != nil {
		return nil, errLiveAntigravity
	}
	return sample, nil
}

func antigravityRemaining(bucket agyBucket) (float64, bool) {
	value := bucket.RemainingFraction
	if value == nil {
		value = bucket.RemainingCamel
	}
	if value == nil {
		return 0, false
	}
	if *value < 0 || *value > 1 {
		return 0, false
	}
	return *value, true
}

func antigravityReset(bucket agyBucket) (time.Time, bool) {
	raw := bucket.ResetTime
	if len(bytes.TrimSpace(raw)) == 0 || string(raw) == "null" {
		raw = bucket.ResetCamel
	}
	if len(bytes.TrimSpace(raw)) == 0 || string(raw) == "null" {
		return time.Time{}, false
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		parsed, err := time.Parse(time.RFC3339, text)
		if err != nil {
			return time.Time{}, false
		}
		return parsed.UTC(), true
	}
	var unix float64
	if err := json.Unmarshal(raw, &unix); err != nil || unix <= 0 {
		return time.Time{}, false
	}
	return time.Unix(int64(unix), 0).UTC(), true
}

func antigravityWindowName(group string, id string, window string) (string, bool) {
	group = strings.ToLower(group)
	id = strings.ToLower(id)
	window = strings.ToLower(strings.TrimSpace(window))
	family := ""
	if strings.Contains(id, "argon") {
		family = "gemini_argon"
	} else if strings.HasPrefix(id, "3p") || strings.Contains(id, "claude") || strings.Contains(id, "gpt") || strings.Contains(group, "claude") || strings.Contains(group, "gpt") {
		family = "claude_gpt"
	} else if strings.HasPrefix(id, "gemini") || strings.Contains(group, "gemini") {
		family = "gemini"
	} else {
		return "", false
	}
	period := ""
	switch window {
	case "weekly", "7d", "seven_day":
		period = "seven_day"
	case "5h", "five_hour":
		period = "five_hour"
	default:
		if strings.Contains(id, "weekly") {
			period = "seven_day"
		} else if strings.Contains(id, "5h") || strings.Contains(id, "five_hour") || strings.Contains(id, "five-hour") {
			period = "five_hour"
		}
	}
	if period == "" {
		return "", false
	}
	name := family + "_" + period
	if len(name) > 32 {
		return "", false
	}
	return name, true
}
