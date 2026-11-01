package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"ai-usage-tracker/internal/agent"
	"ai-usage-tracker/internal/service"
)

func serviceCommand(args []string) {
	if len(args) == 0 {
		serviceUsage()
		os.Exit(2)
	}
	switch args[0] {
	case "install":
		os.Exit(serviceInstall(args[1:], os.Stdout, os.Stderr))
	case "uninstall":
		os.Exit(serviceUninstall(os.Stdout, os.Stderr))
	case "doctor":
		os.Exit(serviceDoctor(os.Stdout, os.Stderr))
	default:
		serviceUsage()
		os.Exit(2)
	}
}

func serviceUsage() {
	name := filepath.Base(os.Args[0])
	fmt.Fprintf(os.Stderr, "usage: %s service install [--dry-run] | %s service uninstall | %s service doctor\n", name, name, name)
}

func serviceManager() (service.Manager, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return service.Manager{}, fmt.Errorf("could not find the home directory: %w", err)
	}
	exe, err := os.Executable()
	if err != nil {
		return service.Manager{}, fmt.Errorf("could not find this binary: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return service.Manager{
		GOOS:     runtime.GOOS,
		Home:     home,
		UID:      os.Getuid(),
		User:     os.Getenv("USER"),
		Exe:      exe,
		Runner:   service.ExecRunner{},
		LookPath: exec.LookPath,
	}, nil
}

func loadConfig() (agent.File, string, error) {
	paths, err := agent.DefaultPaths()
	if err != nil {
		return agent.File{}, "", err
	}
	file, err := agent.Load(paths.Config)
	return file, paths.Config, err
}

func serviceInstall(args []string, out io.Writer, errOut io.Writer) int {
	flags := flag.NewFlagSet("service install", flag.ContinueOnError)
	flags.SetOutput(errOut)
	dryRun := flags.Bool("dry-run", false, "show the service file and the commands without changing anything")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	manager, err := serviceManager()
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	cfg, _, err := loadConfig()
	if err != nil {
		fmt.Fprintln(errOut, service.ErrNotReady)
		return 1
	}
	report, err := manager.Install(context.Background(), cfg, service.InstallOptions{DryRun: *dryRun})
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	writeInstall(out, report, *dryRun)
	return 0
}

func writeInstall(out io.Writer, report service.Report, dryRun bool) {
	if dryRun {
		fmt.Fprintf(out, "dry run: nothing was changed\nwould write %s:\n\n%s\nwould run:\n", report.File, report.Content)
		for _, command := range report.Commands {
			fmt.Fprintf(out, "  %s\n", command)
		}
		return
	}
	fmt.Fprintf(out, "service installed and started\nfile: %s\n", report.File)
	for _, note := range report.Notes {
		fmt.Fprintln(out, note)
	}
	fmt.Fprintln(out, "Check it any time with: ai-usage service doctor")
}

func serviceUninstall(out io.Writer, errOut io.Writer) int {
	manager, err := serviceManager()
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	report, err := manager.Uninstall(context.Background())
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	if !report.Changed {
		fmt.Fprintln(out, "the service was not installed; nothing to do")
		return 0
	}
	fmt.Fprintf(out, "service stopped and removed: %s\n", report.File)
	for _, note := range report.Notes {
		fmt.Fprintln(out, note)
	}
	return 0
}

func serviceDoctor(out io.Writer, errOut io.Writer) int {
	manager, err := serviceManager()
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	cfg, path, cfgErr := loadConfig()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	checks := manager.Doctor(ctx, service.DoctorInput{Config: cfg, ConfigPath: path, ConfigErr: cfgErr})
	for _, line := range formatChecks(checks) {
		fmt.Fprintln(out, line)
	}
	if service.Failed(checks) {
		return 1
	}
	return 0
}

func formatChecks(checks []service.Check) []string {
	var lines []string
	problems, warnings := 0, 0
	for _, check := range checks {
		lines = append(lines, fmt.Sprintf("%-5s %-13s %s", check.Status, check.Name, check.Detail))
		if check.Status != service.OK && check.Fix != "" {
			lines = append(lines, fmt.Sprintf("%-5s %-13s fix: %s", "", "", check.Fix))
		}
		switch check.Status {
		case service.Fail:
			problems++
		case service.Warn:
			warnings++
		}
	}
	summary := "all checks passed"
	if problems > 0 || warnings > 0 {
		summary = strings.TrimSpace(fmt.Sprintf("%d problem(s), %d warning(s)", problems, warnings))
	}
	return append(lines, "", summary)
}
