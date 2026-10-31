package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ai-usage-tracker/internal/agent"

	"github.com/mattn/go-isatty"
)

var errWizardCancelled = errors.New("setup cancelado")

type runMode int

const (
	modeFlags runMode = iota
	modeExecute
	modeWizard
	modeNeedTerminal
)

type wizardOptions struct {
	in          io.Reader
	out         io.Writer
	home        string
	now         time.Time
	interactive bool
	saved       agent.File
	tokenPath   string
	spoolPath   string
}

func chooseRun(explicit bool, saved agent.File, tty bool) runMode {
	if explicit {
		return modeFlags
	}
	if saved.Ready() {
		return modeExecute
	}
	if !tty {
		return modeNeedTerminal
	}
	return modeWizard
}

func terminal() bool {
	return isatty.IsTerminal(os.Stdin.Fd())
}

func runWizard(opts wizardOptions) (agent.File, string, string, error) {
	reader := bufio.NewReader(opts.in)
	current := opts.saved
	if current.TokenFile == "" {
		current.TokenFile = opts.tokenPath
	}
	if current.Spool == "" {
		current.Spool = opts.spoolPath
	}
	claudeSuggest, codexSuggest := agent.Suggest(opts.home, opts.now)
	if current.ClaudeFile == "" {
		current.ClaudeFile = claudeSuggest.Path
	}
	if current.CodexDir == "" {
		current.CodexDir = codexSuggest.Path
	}
	nameDefault := "notebook"
	for {
		api, err := ask(reader, opts, "Servidor", "Endereço do rastreador, como você abre no navegador.", current.API, false, false)
		if err != nil {
			return agent.File{}, "", "", err
		}
		if !validAPI(api) {
			fmt.Fprintln(opts.out, "Endereço inválido. Use http:// ou https://.")
			continue
		}
		name, err := ask(reader, opts, "Dispositivo", "Nome deste aparelho.", nameDefault, false, false)
		if err != nil {
			return agent.File{}, "", "", err
		}
		nameDefault = name
		code := ""
		if !tokenReady(opts.tokenPath) {
			code, err = ask(reader, opts, "Código", "Gere um código em Dispositivos e cole aqui. Ele não fica gravado na configuração.", "", false, false)
			if err != nil {
				return agent.File{}, "", "", err
			}
		}
		claudeBody := "Arquivo do Claude.\n" + agent.DescribeClaude(current.ClaudeFile, opts.now)
		claude, err := ask(reader, opts, "Claude", claudeBody, current.ClaudeFile, true, true)
		if err != nil {
			return agent.File{}, "", "", err
		}
		claude = expandPath(opts.home, claude)
		codexBody := "Pasta do Codex. O envio pergunta o limite ao comando local codex. Esta pasta fica de reserva quando essa leitura falha.\n" + agent.DescribeCodex(current.CodexDir, opts.now)
		codex, err := ask(reader, opts, "Codex", codexBody, current.CodexDir, true, true)
		if err != nil {
			return agent.File{}, "", "", err
		}
		codex = expandPath(opts.home, codex)
		agyCommand, err := askAgy(reader, opts, current.AgyCommand)
		if err != nil {
			return agent.File{}, "", "", err
		}
		if claude == "" && codex == "" && agyCommand == "" {
			fmt.Fprintln(opts.out, "Informe o Claude, o Codex ou o Antigravity.")
			continue
		}
		updateOn, err := askUpdate(reader, opts, current.AutoUpdate)
		if err != nil {
			return agent.File{}, "", "", err
		}
		file := agent.File{
			API:        api,
			TokenFile:  opts.tokenPath,
			Spool:      opts.spoolPath,
			ClaudeFile: claude,
			CodexDir:   codex,
			AgyCommand: agyCommand,
			AutoUpdate: updateOn,
		}
		ok, err := confirm(reader, opts, file, code != "")
		if err != nil {
			return agent.File{}, "", "", err
		}
		if !ok {
			current = file
			continue
		}
		return file, code, name, nil
	}
}

func confirm(reader *bufio.Reader, opts wizardOptions, file agent.File, enrolling bool) (bool, error) {
	tokenLine := "O token deste aparelho já está gravado."
	if enrolling {
		tokenLine = "Um código novo vai enrolar este aparelho."
	}
	updateLine := "Atualização automática desligada."
	if file.AutoUpdate {
		updateLine = "Atualização automática ligada."
	}
	agyLine := "Antigravity desligado."
	if file.AgyCommand != "" {
		agyLine = "Antigravity ligado."
	}
	body := fmt.Sprintf("Servidor\n%s\n\nClaude\n%s\n%s\n\nCodex\n%s\n%s\n\n%s\n\n%s\n\n%s\nO próximo run sem flags envia direto.",
		file.API,
		emptyPath(file.ClaudeFile),
		agent.DescribeClaude(file.ClaudeFile, opts.now),
		emptyPath(file.CodexDir),
		agent.DescribeCodex(file.CodexDir, opts.now),
		agyLine,
		updateLine,
		tokenLine,
	)
	paint(opts.out, opts.interactive, "Confirmar", body, "Enter grava e envia. n refaz. q cancela.")
	line, err := readLine(reader)
	if err != nil {
		return false, err
	}
	if line == "q" {
		return false, errWizardCancelled
	}
	if line == "" || line == "s" {
		return true, nil
	}
	return false, nil
}

func askAgy(reader *bufio.Reader, opts wizardOptions, current string) (string, error) {
	state := "desligado"
	if strings.TrimSpace(current) != "" {
		state = "ligado"
	}
	for {
		paint(opts.out, opts.interactive, "Antigravity", "O comando agy informa a janela do plano. O rastreador não lê o token. Agora está "+state+".", "Enter mantém. s liga. n desliga. q cancela.")
		line, err := readLine(reader)
		if err != nil {
			return "", err
		}
		if line == "q" {
			return "", errWizardCancelled
		}
		if line == "" {
			return current, nil
		}
		if line == "s" {
			return "agy", nil
		}
		if line == "n" {
			return "", nil
		}
		fmt.Fprintln(opts.out, "Responda s, n ou Enter.")
	}
}

func askUpdate(reader *bufio.Reader, opts wizardOptions, current bool) (bool, error) {
	state := "desligada"
	if current {
		state = "ligada"
	}
	for {
		paint(opts.out, opts.interactive, "Atualização", "Cada passagem pode conferir a release estável antes de enviar. Agora está "+state+".", "Enter mantém. s liga. n desliga. q cancela.")
		line, err := readLine(reader)
		if err != nil {
			return false, err
		}
		if line == "q" {
			return false, errWizardCancelled
		}
		if line == "" {
			return current, nil
		}
		if line == "s" {
			return true, nil
		}
		if line == "n" {
			return false, nil
		}
		fmt.Fprintln(opts.out, "Responda s, n ou Enter.")
	}
}

func ask(reader *bufio.Reader, opts wizardOptions, title string, body string, current string, allowEmpty bool, allowClear bool) (string, error) {
	for {
		hint := "Escreva o valor. q cancela."
		if current != "" {
			hint = "Enter aceita o sugerido. Um texto troca. q cancela."
		}
		if allowClear && current != "" {
			hint = "Enter aceita o sugerido. Um texto troca. - deixa vazio. q cancela."
		}
		shown := body
		if current != "" {
			shown += "\n\nSugerido: " + current
		}
		paint(opts.out, opts.interactive, title, shown, hint)
		line, err := readLine(reader)
		if err != nil {
			return "", err
		}
		if line == "q" {
			return "", errWizardCancelled
		}
		if line == "" {
			if current != "" || allowEmpty {
				return current, nil
			}
			fmt.Fprintln(opts.out, "Precisa de um valor.")
			continue
		}
		if line == "-" && allowClear {
			return "", nil
		}
		return line, nil
	}
}

func paint(out io.Writer, interactive bool, title string, body string, hint string) {
	if interactive {
		fmt.Fprint(out, "\033[H\033[2J")
	}
	fmt.Fprintf(out, "Agente do rastreador — %s\n\n%s\n\n%s\n> ", title, body, hint)
}

func readLine(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" && errors.Is(err, io.EOF) {
		return "", err
	}
	return line, nil
}

func validAPI(value string) bool {
	parsed, err := url.ParseRequestURI(value)
	if err != nil {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	return parsed.Host != ""
}

func expandPath(home string, value string) string {
	if value == "~" {
		return home
	}
	if strings.HasPrefix(value, "~/") {
		return filepath.Join(home, strings.TrimPrefix(value, "~/"))
	}
	return value
}

func emptyPath(value string) string {
	if value == "" {
		return "não usado"
	}
	return value
}

func tokenReady(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Size() > 0
}

func openWizard(saved agent.File, paths agent.Paths) (agent.File, string, string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return agent.File{}, "", "", err
	}
	return runWizard(wizardOptions{
		in:          os.Stdin,
		out:         os.Stdout,
		home:        home,
		now:         time.Now().UTC(),
		interactive: true,
		saved:       saved,
		tokenPath:   paths.Token,
		spoolPath:   paths.Spool,
	})
}
