package collect

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"ai-usage-tracker/internal/contract"
)

const codexLiveTimeout = 25 * time.Second

// ReadCodexLive asks the local Codex CLI for the current limits. The CLI owns
// the account credential. This process does not read auth.json and does not
// keep the raw response.
func ReadCodexLive(ctx context.Context, bin string, now time.Time) (*contract.Sample, error) {
	resolved, err := resolveCodex(bin)
	if err != nil {
		return nil, err
	}
	liveCtx, cancel := context.WithTimeout(ctx, codexLiveTimeout)
	defer cancel()
	cmd := exec.CommandContext(liveCtx, resolved, "app-server")
	cmd.Env = codexEnv()
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
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, errLiveCodex
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, errLiveCodex
	}
	if err := cmd.Start(); err != nil {
		return nil, errLiveCodex
	}
	defer func() {
		cancel()
		_ = stdin.Close()
		_ = cmd.Wait()
	}()
	writer := bufio.NewWriter(stdin)
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 1<<20)
	answered := 0
	if err := writeRPC(writer, rpcCall{
		Method: "initialize",
		ID:     1,
		Params: map[string]any{
			"clientInfo": map[string]string{
				"name":    "ai-usage-tracker",
				"title":   "Rastreador de uso",
				"version": "0.1.0",
			},
			"capabilities": map[string]bool{"experimentalApi": true},
		},
	}); err != nil {
		return nil, errLiveCodex
	}
	if _, err := readMatching(scanner, writer, 1, &answered); err != nil {
		return nil, errLiveCodex
	}
	if err := writeRPC(writer, rpcCall{Method: "initialized", Params: map[string]any{}}); err != nil {
		return nil, errLiveCodex
	}
	if err := writeRPC(writer, rpcCall{Method: "account/rateLimits/read", ID: 2}); err != nil {
		return nil, errLiveCodex
	}
	raw, err := readMatching(scanner, writer, 2, &answered)
	if err != nil {
		return nil, errLiveCodex
	}
	sample, err := sampleFromRateLimits(raw, now)
	if err != nil {
		return nil, errLiveCodex
	}
	return sample, nil
}

// PreferCodex keeps the other providers and drops the Codex file sample and
// any Codex report. A newer unavailable report would hide a usable sample.
func PreferCodex(batch Batch, sample contract.Sample) Batch {
	var next Batch
	for _, item := range batch.Samples {
		if item.Provider == contract.ProviderCodex {
			continue
		}
		next.Samples = append(next.Samples, item)
	}
	next.Samples = append(next.Samples, sample)
	for _, item := range batch.Reports {
		if item.Provider == contract.ProviderCodex {
			continue
		}
		next.Reports = append(next.Reports, item)
	}
	return next
}

var errLiveCodex = errors.New("leitura ao vivo do Codex falhou")

func resolveCodex(bin string) (string, error) {
	if bin == "" || strings.Contains(bin, "\x00") {
		return "", errLiveCodex
	}
	if strings.ContainsRune(bin, os.PathSeparator) {
		return bin, nil
	}
	found, err := exec.LookPath(bin)
	if err != nil {
		return "", errLiveCodex
	}
	return found, nil
}

func codexEnv() []string {
	allowed := []string{
		"HOME", "USER", "LOGNAME", "PATH", "LANG", "LC_ALL", "LC_CTYPE",
		"XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME", "XDG_RUNTIME_DIR",
		"SSL_CERT_FILE", "SSL_CERT_DIR", "CODEX_HOME", "TMPDIR",
	}
	var env []string
	for _, key := range allowed {
		value, ok := os.LookupEnv(key)
		if !ok || value == "" {
			continue
		}
		env = append(env, key+"="+value)
	}
	return env
}

type rpcCall struct {
	Method string `json:"method"`
	ID     int    `json:"id,omitempty"`
	Params any    `json:"params,omitempty"`
}

type rpcIncoming struct {
	ID     *int            `json:"id"`
	Method string          `json:"method"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code int `json:"code"`
	} `json:"error"`
}

func writeRPC(writer *bufio.Writer, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := writer.Write(raw); err != nil {
		return err
	}
	if err := writer.WriteByte('\n'); err != nil {
		return err
	}
	return writer.Flush()
}

func readMatching(scanner *bufio.Scanner, writer *bufio.Writer, want int, answered *int) (json.RawMessage, error) {
	seen := 0
	for scanner.Scan() {
		seen++
		if seen > 64 {
			return nil, errLiveCodex
		}
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var msg rpcIncoming
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}
		if msg.Method != "" && msg.ID != nil {
			if *answered >= 8 {
				return nil, errLiveCodex
			}
			*answered++
			reply := struct {
				ID    int `json:"id"`
				Error struct {
					Code    int    `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}{ID: *msg.ID}
			reply.Error.Code = -32601
			reply.Error.Message = "method not found"
			if err := writeRPC(writer, reply); err != nil {
				return nil, errLiveCodex
			}
			continue
		}
		if msg.ID != nil && *msg.ID == want {
			if msg.Error != nil {
				return nil, errLiveCodex
			}
			return append(json.RawMessage(nil), msg.Result...), nil
		}
	}
	if scanner.Err() != nil {
		return nil, errLiveCodex
	}
	return nil, errLiveCodex
}

type liveRateLimits struct {
	RateLimits *struct {
		PlanType  string      `json:"planType"`
		Primary   *liveWindow `json:"primary"`
		Secondary *liveWindow `json:"secondary"`
	} `json:"rateLimits"`
}

type liveWindow struct {
	UsedPercent        float64 `json:"usedPercent"`
	WindowDurationMins int64   `json:"windowDurationMins"`
	ResetsAt           *int64  `json:"resetsAt"`
}

func sampleFromRateLimits(raw json.RawMessage, now time.Time) (*contract.Sample, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, errLiveCodex
	}
	var parsed liveRateLimits
	if err := json.Unmarshal(raw, &parsed); err != nil || parsed.RateLimits == nil {
		return nil, errLiveCodex
	}
	limits := codexLimits{
		PlanType:  parsed.RateLimits.PlanType,
		Primary:   liveAsFile(parsed.RateLimits.Primary),
		Secondary: liveAsFile(parsed.RateLimits.Secondary),
	}
	windows, _ := codexWindows(limits, "")
	if len(windows) == 0 {
		return nil, errLiveCodex
	}
	observed := now.UTC().Truncate(time.Second)
	sample, report, err := finishClaude(contract.ProviderCodex, sanitizePlan(limits.PlanType), &observed, windows, false, observed)
	if err != nil || sample == nil || report != nil {
		return nil, errLiveCodex
	}
	return sample, nil
}

func liveAsFile(window *liveWindow) *codexWindow {
	if window == nil || window.ResetsAt == nil || *window.ResetsAt <= 0 {
		return nil
	}
	resets := *window.ResetsAt
	return &codexWindow{
		UsedPercent:   window.UsedPercent,
		WindowMinutes: window.WindowDurationMins,
		ResetsAt:      &resets,
	}
}
