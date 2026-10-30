package main

import (
	"context"
	"flag"
	"fmt"
	"os"
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
	if *api == "" || *tokenFile == "" || (*claudeFile == "" && *codexDir == "") {
		fmt.Fprintln(os.Stderr, "run exige --api, --token-file e ao menos uma fonte")
		os.Exit(2)
	}
	raw, err := os.ReadFile(*tokenFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "token ilegível")
		os.Exit(1)
	}
	token := string(trim(raw))
	result, err := agent.Run(context.Background(), agent.Config{
		API:        *api,
		Token:      token,
		SpoolPath:  *spoolPath,
		ClaudeFile: *claudeFile,
		CodexDir:   *codexDir,
		Now:        time.Now().UTC(),
	})
	if err != nil && !result.Blocked {
		fmt.Fprintln(os.Stderr, "envio incompleto; a fila local foi preservada")
		os.Exit(1)
	}
	if result.Blocked {
		fmt.Fprintln(os.Stderr, "dispositivo revogado; a fila não será reenviada com esta credencial")
		os.Exit(3)
	}
	fmt.Fprintf(os.Stderr, "aceitas %d duplicadas %d rejeitadas %d pendentes %d\n", result.Accepted, result.Duplicate, result.Rejected, result.Pending)
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
	fmt.Fprintln(os.Stderr, "uso: agent enroll | agent run")
}
