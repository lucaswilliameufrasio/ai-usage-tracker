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

var errWizardCancelled = errors.New("setup cancelled")

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
		api, err := ask(reader, opts, "Server", "Tracker address, as you open it in the browser.", current.API, false, false)
		if err != nil {
			return agent.File{}, "", "", err
		}
		if !validAPI(api) {
			fmt.Fprintln(opts.out, "Invalid address. Use http:// or https://.")
			continue
		}
		name, err := ask(reader, opts, "Device", "Name of this device.", nameDefault, false, false)
		if err != nil {
			return agent.File{}, "", "", err
		}
		nameDefault = name
		code := ""
		serverChanged := opts.saved.API != "" && api != opts.saved.API
		if !tokenReady(opts.tokenPath) || serverChanged {
			codeBody := "Create a code under Devices and paste it here. It is not saved in the configuration."
			if serverChanged && tokenReady(opts.tokenPath) {
				codeBody = "The server changed, so this device must be enrolled again. Create a code under Devices on the new server and paste it here. The old token is replaced."
			}
			code, err = ask(reader, opts, "Code", codeBody, "", false, false)
			if err != nil {
				return agent.File{}, "", "", err
			}
		}
		claudeBody := "Claude file.\n" + agent.DescribeClaude(current.ClaudeFile, opts.now)
		claude, err := ask(reader, opts, "Claude", claudeBody, current.ClaudeFile, true, true)
		if err != nil {
			return agent.File{}, "", "", err
		}
		claude = expandPath(opts.home, claude)
		codexBody := "Codex folder. Each run asks the local codex command for the limit. This folder is the fallback when that read fails.\n" + agent.DescribeCodex(current.CodexDir, opts.now)
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
			fmt.Fprintln(opts.out, "Provide Claude, Codex or Antigravity.")
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
	tokenLine := "This device's token is already saved."
	if enrolling {
		tokenLine = "A new code will enroll this device."
	}
	updateLine := "Automatic update off."
	if file.AutoUpdate {
		updateLine = "Automatic update on."
	}
	agyLine := "Antigravity off."
	if file.AgyCommand != "" {
		agyLine = "Antigravity on."
	}
	body := fmt.Sprintf("Server\n%s\n\nClaude\n%s\n%s\n\nCodex\n%s\n%s\n\n%s\n\n%s\n\n%s\nThe next run without flags sends directly.",
		file.API,
		emptyPath(file.ClaudeFile),
		agent.DescribeClaude(file.ClaudeFile, opts.now),
		emptyPath(file.CodexDir),
		agent.DescribeCodex(file.CodexDir, opts.now),
		agyLine,
		updateLine,
		tokenLine,
	)
	paint(opts.out, opts.interactive, "Confirm", body, "Enter saves and sends. n starts over. q cancels.")
	line, err := readLine(reader)
	if err != nil {
		return false, err
	}
	if line == "q" {
		return false, errWizardCancelled
	}
	if line == "" || line == "y" {
		return true, nil
	}
	return false, nil
}

func askAgy(reader *bufio.Reader, opts wizardOptions, current string) (string, error) {
	state := "off"
	if strings.TrimSpace(current) != "" {
		state = "on"
	}
	for {
		paint(opts.out, opts.interactive, "Antigravity", "The agy command reports the plan window. The tracker does not read the token. It is currently "+state+".", "Enter keeps. y turns on. n turns off. q cancels.")
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
		if line == "y" {
			return "agy", nil
		}
		if line == "n" {
			return "", nil
		}
		fmt.Fprintln(opts.out, "Answer y, n or Enter.")
	}
}

func askUpdate(reader *bufio.Reader, opts wizardOptions, current bool) (bool, error) {
	state := "off"
	if current {
		state = "on"
	}
	for {
		paint(opts.out, opts.interactive, "Update", "Each pass can check the stable release before sending. It is currently "+state+".", "Enter keeps. y turns on. n turns off. q cancels.")
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
		if line == "y" {
			return true, nil
		}
		if line == "n" {
			return false, nil
		}
		fmt.Fprintln(opts.out, "Answer y, n or Enter.")
	}
}

func ask(reader *bufio.Reader, opts wizardOptions, title string, body string, current string, allowEmpty bool, allowClear bool) (string, error) {
	for {
		hint := "Type the value. q cancels."
		if current != "" {
			hint = "Enter accepts the suggestion. Typing replaces it. q cancels."
		}
		if allowClear && current != "" {
			hint = "Enter accepts the suggestion. Typing replaces it. - clears it. q cancels."
		}
		shown := body
		if current != "" {
			shown += "\n\nSuggested: " + current
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
			fmt.Fprintln(opts.out, "A value is required.")
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
	fmt.Fprintf(out, "Tracker agent — %s\n\n%s\n\n%s\n> ", title, body, hint)
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
		return "not used"
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
