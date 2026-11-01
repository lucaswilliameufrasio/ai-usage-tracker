package collect

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"ai-usage-tracker/internal/contract"
)

const maxRolloutBytes = 32 << 20

var (
	planPattern = regexp.MustCompile(`^[a-z0-9_]{1,32}$`)
	// ErrNoSource is returned when neither a Claude file nor a Codex directory was given.
	// Antigravity is a separate command and does not count as one of these file sources.
	ErrNoSource = errors.New("nenhuma fonte informada")
)

type Request struct {
	ClaudeFile string
	CodexDir   string
	Now        time.Time
}

type Batch struct {
	Samples []contract.Sample
	Reports []contract.SourceReport
}

func Read(request Request) (Batch, error) {
	if request.ClaudeFile == "" && request.CodexDir == "" {
		return Batch{}, ErrNoSource
	}
	var batch Batch
	if request.ClaudeFile != "" {
		sample, report, err := readClaude(request.ClaudeFile, request.Now)
		if err != nil {
			return Batch{}, err
		}
		if sample != nil {
			batch.Samples = append(batch.Samples, *sample)
		}
		if report != nil {
			batch.Reports = append(batch.Reports, *report)
		}
	}
	if request.CodexDir != "" {
		sample, report, err := readCodex(request.CodexDir, request.Now)
		if err != nil {
			return Batch{}, err
		}
		if sample != nil {
			batch.Samples = append(batch.Samples, *sample)
		}
		if report != nil {
			batch.Reports = append(batch.Reports, *report)
		}
	}
	return batch, nil
}

func readClaude(path string, now time.Time) (*contract.Sample, *contract.SourceReport, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		report := unavailable(contract.ProviderClaude, contract.ReasonSourceUnreadable, now)
		return nil, &report, nil
	}
	var envelope struct {
		Cached json.RawMessage `json:"cachedUsageUtilization"`
		Limits json.RawMessage `json:"rate_limits"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		report := unavailable(contract.ProviderClaude, contract.ReasonSourceUnreadable, now)
		return nil, &report, nil
	}
	if len(envelope.Cached) > 0 && string(envelope.Cached) != "null" {
		return claudeFromCache(envelope.Cached, now)
	}
	if len(envelope.Limits) > 0 && string(envelope.Limits) != "null" {
		return claudeFromStatusline(envelope.Limits, now)
	}
	report := unavailable(contract.ProviderClaude, contract.ReasonLimitsAbsent, now)
	return nil, &report, nil
}

type cachedWindow struct {
	Utilization *float64 `json:"utilization"`
	ResetsAt    *string  `json:"resets_at"`
}

func claudeFromCache(raw json.RawMessage, now time.Time) (*contract.Sample, *contract.SourceReport, error) {
	var cached struct {
		FetchedAtMs int64 `json:"fetchedAtMs"`
		Utilization struct {
			FiveHour *cachedWindow `json:"five_hour"`
			SevenDay *cachedWindow `json:"seven_day"`
		} `json:"utilization"`
	}
	if err := json.Unmarshal(raw, &cached); err != nil {
		report := unavailable(contract.ProviderClaude, contract.ReasonSourceUnreadable, now)
		return nil, &report, nil
	}
	var observed *time.Time
	if cached.FetchedAtMs > 0 {
		parsed := unixFlexible(cached.FetchedAtMs)
		observed = &parsed
	}
	windows, sawUsedWithoutReset := pairWindows(map[string]*cachedWindow{
		"five_hour": cached.Utilization.FiveHour,
		"seven_day": cached.Utilization.SevenDay,
	})
	return finishClaude(contract.ProviderClaude, "", observed, windows, sawUsedWithoutReset, now)
}

func claudeFromStatusline(raw json.RawMessage, now time.Time) (*contract.Sample, *contract.SourceReport, error) {
	var limits struct {
		FiveHour *struct {
			UsedPercentage *float64 `json:"used_percentage"`
			ResetsAt       *float64 `json:"resets_at"`
		} `json:"five_hour"`
		SevenDay *struct {
			UsedPercentage *float64 `json:"used_percentage"`
			ResetsAt       *float64 `json:"resets_at"`
		} `json:"seven_day"`
	}
	if err := json.Unmarshal(raw, &limits); err != nil {
		report := unavailable(contract.ProviderClaude, contract.ReasonSourceUnreadable, now)
		return nil, &report, nil
	}
	mapped := map[string]*cachedWindow{}
	if limits.FiveHour != nil {
		mapped["five_hour"] = statusToCached(limits.FiveHour.UsedPercentage, limits.FiveHour.ResetsAt)
	}
	if limits.SevenDay != nil {
		mapped["seven_day"] = statusToCached(limits.SevenDay.UsedPercentage, limits.SevenDay.ResetsAt)
	}
	windows, sawUsedWithoutReset := pairWindows(mapped)
	return finishClaude(contract.ProviderClaude, "", nil, windows, sawUsedWithoutReset, now)
}

func statusToCached(used *float64, resets *float64) *cachedWindow {
	window := &cachedWindow{Utilization: used}
	if resets != nil {
		text := fmt.Sprintf("%d", int64(*resets))
		window.ResetsAt = &text
	}
	return window
}

func pairWindows(items map[string]*cachedWindow) ([]contract.Window, bool) {
	names := make([]string, 0, len(items))
	for name := range items {
		names = append(names, name)
	}
	sort.Strings(names)
	var windows []contract.Window
	sawUsedWithoutReset := false
	for _, name := range names {
		item := items[name]
		if item == nil || item.Utilization == nil {
			continue
		}
		if item.ResetsAt == nil || strings.TrimSpace(*item.ResetsAt) == "" {
			sawUsedWithoutReset = true
			continue
		}
		resets, err := contract.ParseTime(strings.TrimSpace(*item.ResetsAt))
		if err != nil {
			sawUsedWithoutReset = true
			continue
		}
		windows = append(windows, contract.Window{
			Name:     name,
			Used:     *item.Utilization,
			Limit:    100,
			Unit:     contract.UnitPercent,
			ResetsAt: resets.UTC(),
		})
	}
	return windows, sawUsedWithoutReset
}

func finishClaude(provider string, plan string, observed *time.Time, windows []contract.Window, sawUsedWithoutReset bool, now time.Time) (*contract.Sample, *contract.SourceReport, error) {
	if len(windows) == 0 {
		reason := contract.ReasonLimitsAbsent
		if sawUsedWithoutReset {
			reason = contract.ReasonResetUnknown
		}
		when := now
		if observed != nil {
			when = *observed
		}
		report := unavailable(provider, reason, when)
		return nil, &report, nil
	}
	collected := now.UTC()
	if observed != nil {
		collected = observed.UTC()
	}
	sample := contract.Sample{
		Provider:         provider,
		CollectedAt:      collected,
		SourceObservedAt: observed,
		Plan:             plan,
		Windows:          windows,
	}
	sample.SampleID = contract.FingerprintSample(provider, plan, observed, windows)
	return &sample, nil, nil
}

func readCodex(dir string, now time.Time) (*contract.Sample, *contract.SourceReport, error) {
	paths, err := rolloutFiles(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			report := unavailable(contract.ProviderCodex, contract.ReasonSourceUnreadable, now)
			return nil, &report, nil
		}
		report := unavailable(contract.ProviderCodex, contract.ReasonSourceUnreadable, now)
		return nil, &report, nil
	}
	if len(paths) == 0 {
		report := unavailable(contract.ProviderCodex, contract.ReasonLimitsAbsent, now)
		return nil, &report, nil
	}
	var best *codexCandidate
	readable := 0
	sawNullOnly := false
	sawResetUnknown := false
	for order, path := range paths {
		info, err := os.Stat(path)
		if err != nil || info.Size() > maxRolloutBytes {
			continue
		}
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		snapshot, status := lastCodexSnapshot(file)
		_ = file.Close()
		if status == "error" {
			continue
		}
		readable++
		if status == "null" {
			sawNullOnly = true
			continue
		}
		if status == "reset" {
			sawResetUnknown = true
			continue
		}
		if status != "ok" {
			continue
		}
		item := codexCandidate{order: order, plan: snapshot.plan, windows: snapshot.windows}
		if snapshot.hasTime {
			item.hasTime = true
			item.at = snapshot.at
		}
		if best == nil || betterCandidate(item, *best) {
			copyItem := item
			best = &copyItem
		}
	}
	if readable == 0 {
		report := unavailable(contract.ProviderCodex, contract.ReasonSourceUnreadable, now)
		return nil, &report, nil
	}
	if best == nil {
		reason := contract.ReasonLimitsAbsent
		if sawResetUnknown {
			reason = contract.ReasonResetUnknown
		} else if sawNullOnly {
			reason = contract.ReasonLimitsAbsent
		}
		report := unavailable(contract.ProviderCodex, reason, now)
		return nil, &report, nil
	}
	var observed *time.Time
	if best.hasTime {
		at := best.at.UTC()
		observed = &at
	}
	return finishClaude(contract.ProviderCodex, best.plan, observed, best.windows, false, now)
}

type codexSnapshot struct {
	at      time.Time
	hasTime bool
	plan    string
	windows []contract.Window
}

func lastCodexSnapshot(reader io.Reader) (codexSnapshot, string) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 2<<20)
	var last *codexSnapshot
	sawNull := false
	sawReset := false
	for scanner.Scan() {
		line := scanner.Bytes()
		if !strings.Contains(string(line), "rate_limits") {
			continue
		}
		var parsed struct {
			Timestamp string `json:"timestamp"`
			Payload   struct {
				RateLimits *codexLimits `json:"rate_limits"`
			} `json:"payload"`
			RateLimits *codexLimits `json:"rate_limits"`
		}
		if err := json.Unmarshal(line, &parsed); err != nil {
			continue
		}
		limits := parsed.Payload.RateLimits
		if limits == nil {
			limits = parsed.RateLimits
		}
		if limits == nil {
			sawNull = true
			continue
		}
		windows, unknown := codexWindows(*limits, parsed.Timestamp)
		if len(windows) == 0 {
			if unknown {
				sawReset = true
			}
			continue
		}
		snap := codexSnapshot{plan: sanitizePlan(limits.PlanType), windows: windows}
		if parsed.Timestamp != "" {
			if at, err := contract.ParseTime(parsed.Timestamp); err == nil {
				snap.hasTime = true
				snap.at = at
			}
		}
		last = &snap
	}
	if last != nil {
		return *last, "ok"
	}
	if scanner.Err() != nil {
		return codexSnapshot{}, "error"
	}
	if sawReset {
		return codexSnapshot{}, "reset"
	}
	if sawNull {
		return codexSnapshot{}, "null"
	}
	return codexSnapshot{}, "absent"
}

type codexLimits struct {
	PlanType  string       `json:"plan_type"`
	Primary   *codexWindow `json:"primary"`
	Secondary *codexWindow `json:"secondary"`
}

type codexWindow struct {
	UsedPercent     float64 `json:"used_percent"`
	WindowMinutes   int64   `json:"window_minutes"`
	ResetsAt        *int64  `json:"resets_at"`
	ResetsInSeconds *int64  `json:"resets_in_seconds"`
}

func codexWindows(limits codexLimits, eventTimestamp string) ([]contract.Window, bool) {
	var event time.Time
	var hasEvent bool
	if eventTimestamp != "" {
		if parsed, err := contract.ParseTime(eventTimestamp); err == nil {
			event = parsed
			hasEvent = true
		}
	}
	unknown := false
	var windows []contract.Window
	usedNames := map[string]struct{}{}
	for _, item := range []struct {
		name   string
		window *codexWindow
	}{
		{name: "primary", window: limits.Primary},
		{name: "secondary", window: limits.Secondary},
	} {
		if item.window == nil {
			continue
		}
		resets, ok := codexReset(*item.window, event, hasEvent)
		if !ok {
			if item.window.UsedPercent != 0 || item.window.WindowMinutes != 0 {
				unknown = true
			}
			continue
		}
		name := windowName(item.window.WindowMinutes)
		if name == "" {
			unknown = true
			continue
		}
		if _, exists := usedNames[name]; exists {
			name = name + "_" + item.name
		}
		usedNames[name] = struct{}{}
		windows = append(windows, contract.Window{
			Name:     name,
			Used:     item.window.UsedPercent,
			Limit:    100,
			Unit:     contract.UnitPercent,
			ResetsAt: resets.UTC(),
		})
	}
	return windows, unknown
}

func codexReset(window codexWindow, event time.Time, hasEvent bool) (time.Time, bool) {
	if window.ResetsAt != nil && *window.ResetsAt > 0 {
		return time.Unix(*window.ResetsAt, 0).UTC(), true
	}
	if window.ResetsInSeconds != nil && hasEvent {
		return event.Add(time.Duration(*window.ResetsInSeconds) * time.Second).UTC(), true
	}
	return time.Time{}, false
}

func windowName(minutes int64) string {
	if minutes >= 240 && minutes <= 360 {
		return "five_hour"
	}
	if minutes >= 9000 && minutes <= 11000 {
		return "seven_day"
	}
	if minutes <= 0 || minutes > 100000 {
		return ""
	}
	return fmt.Sprintf("minutes_%d", minutes)
}

func rolloutFiles(dir string) ([]string, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("rollouts directory unreadable")
	}
	type fileInfo struct {
		path string
		mod  time.Time
	}
	var files []fileInfo
	err = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if entry != nil && entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "rollout") || !strings.HasSuffix(name, ".jsonl") {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		files = append(files, fileInfo{path: path, mod: info.ModTime()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].mod.After(files[j].mod)
	})
	if len(files) > 20 {
		files = files[:20]
	}
	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, file.path)
	}
	return paths, nil
}

type codexCandidate struct {
	at      time.Time
	hasTime bool
	plan    string
	windows []contract.Window
	order   int
}

func betterCandidate(next codexCandidate, current codexCandidate) bool {
	if next.hasTime != current.hasTime {
		return next.hasTime
	}
	if next.hasTime && current.hasTime && !next.at.Equal(current.at) {
		return next.at.After(current.at)
	}
	return next.order < current.order
}

func sanitizePlan(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if planPattern.MatchString(value) {
		return value
	}
	return ""
}

func unavailable(provider string, reason string, when time.Time) contract.SourceReport {
	return contract.SourceReport{
		ReportID:   contract.FingerprintReport(provider, reason),
		Provider:   provider,
		ObservedAt: when.UTC(),
		Status:     contract.StatusUnavailable,
		ReasonCode: reason,
	}
}

func unixFlexible(value int64) time.Time {
	if value > 1_000_000_000_000 {
		return time.UnixMilli(value).UTC()
	}
	return time.Unix(value, 0).UTC()
}
