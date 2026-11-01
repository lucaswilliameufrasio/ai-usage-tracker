package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"ai-usage-tracker/internal/agent"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "enroll":
		enroll(os.Args[2:])
	case "run":
		run(os.Args[2:])
	case "setup":
		configure(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func enroll(args []string) {
	flags := flag.NewFlagSet("enroll", flag.ExitOnError)
	api := flags.String("api", "", "URL do rastreador")
	code := flags.String("code", "", "código de enrolamento de uso único")
	name := flags.String("name", "", "nome deste dispositivo")
	out := flags.String("out", "", "arquivo em que o token será gravado")
	_ = flags.Parse(args)
	if *api == "" || *code == "" || *name == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "enroll exige --api, --code, --name e --out")
		os.Exit(2)
	}
	result, err := agent.Enroll(context.Background(), nil, *api, *code, *name)
	if err != nil {
		fmt.Fprintln(os.Stderr, "enrolamento recusado")
		os.Exit(1)
	}
	if err := os.WriteFile(*out, []byte(result.Token+"\n"), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "não foi possível gravar o token")
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "dispositivo %s\n", result.DeviceID)
}

func run(args []string) {
	flags := flag.NewFlagSet("run", flag.ExitOnError)
	api := flags.String("api", "", "URL do rastreador")
	tokenFile := flags.String("token-file", "", "arquivo do token de dispositivo")
	spoolPath := flags.String("spool", "spool.json", "fila local")
	claudeFile := flags.String("claude-file", "", "arquivo local de uso do Claude")
	codexDir := flags.String("codex-dir", "", "diretório de rollouts do Codex")
	_ = flags.Parse(args)
	explicit := false
	flags.Visit(func(*flag.Flag) {
		explicit = true
	})
	paths, pathErr := agent.DefaultPaths()
	saved := agent.File{}
	if pathErr == nil {
		loaded, err := agent.Load(paths.Config)
		if err == nil {
			saved = loaded
		} else if explicit == false && !os.IsNotExist(err) {
			fmt.Fprintln(os.Stderr, "configuração ilegível")
			os.Exit(1)
		}
	}
	switch chooseRun(explicit, saved, terminal()) {
	case modeFlags:
		if *api == "" || *tokenFile == "" || (*claudeFile == "" && *codexDir == "") {
			fmt.Fprintln(os.Stderr, "run exige --api, --token-file e ao menos uma fonte")
			os.Exit(2)
		}
		execute(agent.File{
			API: *api, TokenFile: *tokenFile, Spool: *spoolPath, ClaudeFile: *claudeFile, CodexDir: *codexDir,
		})
		return
	case modeExecute:
		executeLoop(saved)
		return
	case modeNeedTerminal:
		fmt.Fprintln(os.Stderr, "Rode num terminal para configurar o agente, ou informe --api, --token-file e uma fonte.")
		os.Exit(2)
	}
	if pathErr != nil {
		fmt.Fprintln(os.Stderr, "não foi possível achar a configuração local")
		os.Exit(1)
	}
	finishSetup(saved, paths)
}

func configure(args []string) {
	flags := flag.NewFlagSet("setup", flag.ExitOnError)
	_ = flags.Parse(args)
	if !terminal() {
		fmt.Fprintln(os.Stderr, "O setup precisa de um terminal.")
		os.Exit(2)
	}
	paths, err := agent.DefaultPaths()
	if err != nil {
		fmt.Fprintln(os.Stderr, "não foi possível achar a configuração local")
		os.Exit(1)
	}
	saved, err := agent.Load(paths.Config)
	if err != nil && !os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, "configuração ilegível")
		os.Exit(1)
	}
	finishSetup(saved, paths)
}

func finishSetup(saved agent.File, paths agent.Paths) {
	file, code, name, err := openWizard(saved, paths)
	if errors.Is(err, errWizardCancelled) {
		fmt.Fprintln(os.Stderr, "setup cancelado")
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "setup incompleto")
		os.Exit(1)
	}
	if code != "" {
		result, err := agent.Enroll(context.Background(), nil, file.API, code, name)
		if err != nil {
			fmt.Fprintln(os.Stderr, "enrolamento recusado")
			os.Exit(1)
		}
		if err := os.WriteFile(file.TokenFile, []byte(result.Token+"\n"), 0o600); err != nil {
			fmt.Fprintln(os.Stderr, "não foi possível gravar o token")
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "dispositivo %s\n", result.DeviceID)
	}
	if err := agent.Save(paths.Config, file); err != nil {
		fmt.Fprintln(os.Stderr, "não foi possível gravar a configuração")
		os.Exit(1)
	}
	execute(file)
}

func execute(file agent.File) {
	os.Exit(runOnce(context.Background(), file))
}

func runOnce(ctx context.Context, file agent.File) int {
	result, err := runFile(ctx, file)
	return report(result, err)
}

func runFile(ctx context.Context, file agent.File) (agent.Result, error) {
	raw, err := os.ReadFile(file.TokenFile)
	if err != nil {
		return agent.Result{}, errToken
	}
	return agent.Run(ctx, agent.Config{
		API:          file.API,
		Token:        string(trim(raw)),
		SpoolPath:    file.Spool,
		ClaudeFile:   file.ClaudeFile,
		CodexDir:     file.CodexDir,
		CodexCommand: codexBinary(file.CodexDir),
		AgyCommand:   file.AgyCommand,
		Now:          time.Now().UTC(),
	})
}

func report(result agent.Result, err error) int {
	if result.CodexLive == "read" {
		fmt.Fprintln(os.Stderr, "codex: read live from the local command")
	}
	if result.CodexLive == "fallback" {
		fmt.Fprintln(os.Stderr, "codex: live read unavailable, kept the file observation")
	}
	if result.AgyLive == "read" {
		fmt.Fprintln(os.Stderr, "antigravity: read live from the local command")
	}
	if result.AgyLive == "miss" {
		fmt.Fprintln(os.Stderr, "antigravity: live read unavailable, no new sample from this provider")
	}
	for _, line := range outcomeLines(result.Outcomes) {
		fmt.Fprintln(os.Stderr, line)
	}
	if errors.Is(err, errToken) {
		fmt.Fprintln(os.Stderr, "token unreadable")
		return 1
	}
	if err != nil && !result.Blocked {
		fmt.Fprintln(os.Stderr, "send incomplete; the local queue was kept")
		return 1
	}
	if result.Blocked {
		fmt.Fprintln(os.Stderr, "device revoked; the queue will not be resent with this credential")
		return 3
	}
	fmt.Fprintf(os.Stderr, "total accepted=%d duplicate=%d rejected=%d pending=%d\n", result.Accepted, result.Duplicate, result.Rejected, result.Pending)
	return 0
}

// outcomeLines escreve uma linha por item: provedor, tipo, estado e, se houve, o código.
func outcomeLines(outcomes []agent.Outcome) []string {
	lines := make([]string, 0, len(outcomes))
	for _, outcome := range outcomes {
		provider := outcome.Provider
		if provider == "" {
			provider = "unknown"
		}
		line := fmt.Sprintf("%s %s %s", provider, outcome.Kind, outcome.Status)
		if outcome.Code != "" {
			line += " code=" + outcome.Code
		}
		lines = append(lines, line)
	}
	return lines
}

func codexBinary(dir string) string {
	if dir == "" {
		return ""
	}
	return "codex"
}

func trim(raw []byte) []byte {
	start := 0
	end := len(raw)
	for start < end && (raw[start] == ' ' || raw[start] == '\n' || raw[start] == '\r' || raw[start] == '\t') {
		start++
	}
	for end > start && (raw[end-1] == ' ' || raw[end-1] == '\n' || raw[end-1] == '\r' || raw[end-1] == '\t') {
		end--
	}
	return raw[start:end]
}

func usage() {
	name := filepath.Base(os.Args[0])
	fmt.Fprintf(os.Stderr, "uso: %s enroll | %s run | %s setup\n", name, name, name)
}
