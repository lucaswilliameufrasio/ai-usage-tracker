package contract

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var ErrMalformed = errors.New("malformed")

const (
	ProviderClaude         = "claude"
	ProviderCodex          = "codex"
	ProviderAntigravity    = "antigravity"
	UnitPercent            = "percent"
	StatusUnavailable      = "unavailable"
	ReasonLimitsAbsent     = "LIMITS_ABSENT"
	ReasonResetUnknown     = "RESET_UNKNOWN"
	ReasonSourceUnreadable = "SOURCE_UNREADABLE"
)

var (
	idPattern   = regexp.MustCompile(`^[A-Za-z0-9_-]{8,80}$`)
	namePattern = regexp.MustCompile(`^[a-z0-9_]{1,32}$`)
	planPattern = regexp.MustCompile(`^[a-z0-9_]{1,32}$`)
)

var forbiddenKeys = map[string]struct{}{
	"prompt": {}, "prompts": {}, "message": {}, "messages": {}, "content": {},
	"transcript": {}, "transcript_path": {}, "cwd": {}, "path": {}, "file_path": {},
	"filepath": {}, "token": {}, "access_token": {}, "refresh_token": {}, "id_token": {},
	"cookie": {}, "cookies": {}, "authorization": {}, "credential": {}, "credentials": {},
	"api_key": {}, "apikey": {}, "secret": {}, "password": {}, "account_uuid": {},
	"accountuuid": {}, "oauth": {}, "oauthaccount": {}, "auth_json": {},
}

type FieldError struct {
	Field   string
	Message string
}

type ValidationError struct {
	Fields []FieldError
}

func (e *ValidationError) Error() string {
	return "validação"
}

type Window struct {
	Name     string    `json:"name"`
	Used     float64   `json:"used"`
	Limit    float64   `json:"limit"`
	Unit     string    `json:"unit"`
	ResetsAt time.Time `json:"resets_at"`
}

type Sample struct {
	SampleID         string     `json:"sample_id"`
	Provider         string     `json:"provider"`
	CollectedAt      time.Time  `json:"collected_at"`
	SourceObservedAt *time.Time `json:"source_observed_at,omitempty"`
	Plan             string     `json:"plan,omitempty"`
	Windows          []Window   `json:"windows"`
}

type SourceReport struct {
	ReportID   string    `json:"report_id"`
	Provider   string    `json:"provider"`
	ObservedAt time.Time `json:"observed_at"`
	Status     string    `json:"status"`
	ReasonCode string    `json:"reason_code"`
}

func DecodeSample(raw []byte, now time.Time) (Sample, error) {
	if err := rejectKeys(raw, map[string]struct{}{
		"sample_id": {}, "provider": {}, "collected_at": {},
		"source_observed_at": {}, "plan": {}, "windows": {},
	}, map[string]map[string]struct{}{
		"windows": {"name": {}, "used": {}, "limit": {}, "unit": {}, "resets_at": {}},
	}); err != nil {
		return Sample{}, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var sample Sample
	if err := dec.Decode(&sample); err != nil {
		return Sample{}, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	if err := rejectTrailing(dec); err != nil {
		return Sample{}, err
	}
	sample = normalizeSample(sample)
	if err := validateSample(sample, now); err != nil {
		return Sample{}, err
	}
	return sample, nil
}

func DecodeReport(raw []byte, now time.Time) (SourceReport, error) {
	if err := rejectKeys(raw, map[string]struct{}{
		"report_id": {}, "provider": {}, "observed_at": {}, "status": {}, "reason_code": {},
	}, nil); err != nil {
		return SourceReport{}, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var report SourceReport
	if err := dec.Decode(&report); err != nil {
		return SourceReport{}, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	if err := rejectTrailing(dec); err != nil {
		return SourceReport{}, err
	}
	report.Provider = strings.TrimSpace(report.Provider)
	report.Status = strings.TrimSpace(report.Status)
	report.ReasonCode = strings.TrimSpace(report.ReasonCode)
	report.ObservedAt = report.ObservedAt.UTC()
	var fields []FieldError
	if !idPattern.MatchString(report.ReportID) {
		fields = append(fields, FieldError{Field: "report_id", Message: "Identificador inválido."})
	}
	if !validProvider(report.Provider) {
		fields = append(fields, FieldError{Field: "provider", Message: "Provedor deve ser claude, codex ou antigravity."})
	}
	if report.ObservedAt.IsZero() || report.ObservedAt.After(now.Add(5*time.Minute)) {
		fields = append(fields, FieldError{Field: "observed_at", Message: "Instante ausente ou no futuro."})
	}
	if report.Status != StatusUnavailable {
		fields = append(fields, FieldError{Field: "status", Message: "Nesta etapa o relatório só aceita unavailable."})
	}
	switch report.ReasonCode {
	case ReasonLimitsAbsent, ReasonResetUnknown, ReasonSourceUnreadable:
	default:
		fields = append(fields, FieldError{Field: "reason_code", Message: "Motivo desconhecido."})
	}
	if len(fields) > 0 {
		return SourceReport{}, &ValidationError{Fields: fields}
	}
	return report, nil
}

func SameSample(left Sample, right Sample) bool {
	if left.Provider != right.Provider || left.Plan != right.Plan || len(left.Windows) != len(right.Windows) {
		return false
	}
	if !sameOptionalTime(left.SourceObservedAt, right.SourceObservedAt) {
		return false
	}
	lw := append([]Window(nil), left.Windows...)
	rw := append([]Window(nil), right.Windows...)
	sort.Slice(lw, func(i, j int) bool { return lw[i].Name < lw[j].Name })
	sort.Slice(rw, func(i, j int) bool { return rw[i].Name < rw[j].Name })
	for i := range lw {
		if lw[i].Name != rw[i].Name || lw[i].Unit != rw[i].Unit || lw[i].Limit != rw[i].Limit || lw[i].Used != rw[i].Used {
			return false
		}
		if !lw[i].ResetsAt.UTC().Equal(rw[i].ResetsAt.UTC()) {
			return false
		}
	}
	return true
}

func SameReport(left SourceReport, right SourceReport) bool {
	return left.Provider == right.Provider && left.Status == right.Status && left.ReasonCode == right.ReasonCode
}

func (s Sample) Canonical() ([]byte, error) {
	s = normalizeSample(s)
	return json.Marshal(s)
}

func (r SourceReport) Canonical() ([]byte, error) {
	r.ObservedAt = r.ObservedAt.UTC()
	return json.Marshal(r)
}

func FingerprintSample(provider string, plan string, observed *time.Time, windows []Window) string {
	copied := append([]Window(nil), windows...)
	sort.Slice(copied, func(i, j int) bool { return copied[i].Name < copied[j].Name })
	h := sha256.New()
	observedText := ""
	if observed != nil {
		observedText = observed.UTC().Format(time.RFC3339Nano)
	}
	_, _ = fmt.Fprintf(h, "%s\n%s\n%s\n", provider, plan, observedText)
	for _, window := range copied {
		_, _ = fmt.Fprintf(h, "%s|%s|%s|%s|%s\n",
			window.Name,
			strconv.FormatFloat(window.Used, 'f', 4, 64),
			strconv.FormatFloat(window.Limit, 'f', 4, 64),
			window.Unit,
			window.ResetsAt.UTC().Format(time.RFC3339Nano),
		)
	}
	return "smp_" + hex.EncodeToString(h.Sum(nil))[:32]
}

func FingerprintReport(provider string, reason string) string {
	sum := sha256.Sum256([]byte(provider + "\n" + reason))
	return "rpt_" + hex.EncodeToString(sum[:])[:32]
}

func ParseTime(value string) (time.Time, error) {
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed.UTC(), nil
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed.UTC(), nil
	}
	whole, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	if whole > 1_000_000_000_000 {
		return time.UnixMilli(whole).UTC(), nil
	}
	return time.Unix(whole, 0).UTC(), nil
}

func validateSample(sample Sample, now time.Time) error {
	var fields []FieldError
	if !idPattern.MatchString(sample.SampleID) {
		fields = append(fields, FieldError{Field: "sample_id", Message: "Identificador inválido."})
	}
	if !validProvider(sample.Provider) {
		fields = append(fields, FieldError{Field: "provider", Message: "Provedor deve ser claude, codex ou antigravity."})
	}
	if sample.CollectedAt.IsZero() || sample.CollectedAt.After(now.Add(5*time.Minute)) {
		fields = append(fields, FieldError{Field: "collected_at", Message: "Instante ausente ou no futuro."})
	}
	if sample.SourceObservedAt != nil && sample.SourceObservedAt.After(now.Add(5*time.Minute)) {
		fields = append(fields, FieldError{Field: "source_observed_at", Message: "Instante da fonte está no futuro."})
	}
	if sample.Plan != "" && !planPattern.MatchString(sample.Plan) {
		fields = append(fields, FieldError{Field: "plan", Message: "Plano contém caracteres não permitidos."})
	}
	if len(sample.Windows) < 1 || len(sample.Windows) > 8 {
		fields = append(fields, FieldError{Field: "windows", Message: "É necessária de 1 a 8 janelas."})
	}
	seen := map[string]struct{}{}
	for i, window := range sample.Windows {
		field := fmt.Sprintf("windows[%d]", i)
		if !namePattern.MatchString(window.Name) {
			fields = append(fields, FieldError{Field: field + ".name", Message: "Nome de janela inválido."})
		}
		if _, ok := seen[window.Name]; ok {
			fields = append(fields, FieldError{Field: field + ".name", Message: "Janela repetida."})
		}
		seen[window.Name] = struct{}{}
		if window.Unit != UnitPercent || window.Limit != 100 {
			fields = append(fields, FieldError{Field: field + ".unit", Message: "Nesta etapa a unidade é percent e o limite é 100."})
		}
		if window.Used < 0 || window.Used > 1000 {
			fields = append(fields, FieldError{Field: field + ".used", Message: "Uso fora do intervalo aceito."})
		}
		if window.ResetsAt.IsZero() {
			fields = append(fields, FieldError{Field: field + ".resets_at", Message: "Reset ausente."})
		}
	}
	if len(fields) > 0 {
		return &ValidationError{Fields: fields}
	}
	return nil
}

func normalizeSample(sample Sample) Sample {
	sample.Provider = strings.TrimSpace(sample.Provider)
	sample.Plan = strings.TrimSpace(sample.Plan)
	sample.CollectedAt = sample.CollectedAt.UTC()
	if sample.SourceObservedAt != nil {
		observed := sample.SourceObservedAt.UTC()
		sample.SourceObservedAt = &observed
	}
	for i := range sample.Windows {
		sample.Windows[i].Name = strings.TrimSpace(sample.Windows[i].Name)
		sample.Windows[i].Unit = strings.TrimSpace(sample.Windows[i].Unit)
		sample.Windows[i].ResetsAt = sample.Windows[i].ResetsAt.UTC()
	}
	sort.Slice(sample.Windows, func(i, j int) bool {
		return sample.Windows[i].Name < sample.Windows[j].Name
	})
	return sample
}

func validProvider(provider string) bool {
	return provider == ProviderClaude || provider == ProviderCodex || provider == ProviderAntigravity
}

func sameOptionalTime(left *time.Time, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.UTC().Equal(right.UTC())
}

func rejectKeys(raw []byte, top map[string]struct{}, nested map[string]map[string]struct{}) error {
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	var fields []FieldError
	walkKeys(payload, "", top, nested, &fields)
	if len(fields) > 0 {
		sort.Slice(fields, func(i, j int) bool {
			return fields[i].Field < fields[j].Field
		})
		return &ValidationError{Fields: fields}
	}
	return nil
}

func rejectTrailing(dec *json.Decoder) error {
	var extra json.RawMessage
	err := dec.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return fmt.Errorf("%w: trailing", ErrMalformed)
	}
	return fmt.Errorf("%w: %v", ErrMalformed, err)
}

func walkKeys(value any, prefix string, allowedHere map[string]struct{}, nested map[string]map[string]struct{}, fields *[]FieldError) {
	object, ok := value.(map[string]any)
	if !ok {
		list, isList := value.([]any)
		if !isList {
			return
		}
		for i, item := range list {
			itemAllowed := allowedHere
			if nested != nil {
				itemAllowed = nil
			}
			walkKeys(item, fmt.Sprintf("%s[%d]", prefix, i), itemAllowed, nil, fields)
		}
		return
	}
	for key, child := range object {
		lower := strings.ToLower(key)
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		if _, blocked := forbiddenKeys[lower]; blocked {
			*fields = append(*fields, FieldError{Field: path, Message: "Campo proibido no contrato."})
			continue
		}
		if allowedHere != nil {
			if _, ok := allowedHere[key]; !ok {
				*fields = append(*fields, FieldError{Field: path, Message: "Campo não faz parte do contrato."})
				continue
			}
		}
		nextAllowed := map[string]struct{}(nil)
		nextNested := map[string]map[string]struct{}(nil)
		if nested != nil {
			if childAllowed, ok := nested[key]; ok {
				nextAllowed = childAllowed
			}
		}
		walkKeys(child, path, nextAllowed, nextNested, fields)
	}
}
