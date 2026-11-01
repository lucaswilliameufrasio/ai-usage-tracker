package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"ai-usage-tracker/internal/store"
)

func runBackup(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("backup", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	data := flags.String("data", "data", "SQLite directory")
	out := flags.String("out", "", "snapshot file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *out == "" {
		return errors.New("backup requires --out")
	}
	source := filepath.Join(*data, "tracker.db")
	if err := store.SameTarget(source, *out); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o700); err != nil {
		return err
	}
	db, err := store.Open(source)
	if err != nil {
		return err
	}
	defer db.Close()
	census, err := db.Snapshot(*out)
	if err != nil {
		return err
	}
	_, err = io.WriteString(stdout, store.FormatCensus(census))
	return err
}

func runRestore(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("restore", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	snapshot := flags.String("snapshot", "", "snapshot file")
	dest := flags.String("dest", "", "new directory for the restore")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *snapshot == "" || *dest == "" {
		return errors.New("restore requires --snapshot and --dest")
	}
	census, err := store.Restore(*snapshot, *dest)
	if err != nil {
		return err
	}
	_, err = io.WriteString(stdout, store.FormatCensus(census))
	return err
}

func explainMaintenance(err error) string {
	switch {
	case errors.Is(err, store.ErrSnapshotExists):
		return "The snapshot destination already exists."
	case errors.Is(err, store.ErrSnapshotSame):
		return "The snapshot cannot replace the database in use."
	case errors.Is(err, store.ErrRestoreExists):
		return "A restore only writes into a directory without a database."
	case errors.Is(err, store.ErrSnapshotInvalid):
		return "The snapshot failed verification."
	case errors.Is(err, store.ErrSnapshotDest):
		return "The backup path is not usable."
	default:
		return "The backup operation did not complete."
	}
}

func maintenance(command string, args []string) int {
	var err error
	switch command {
	case "backup":
		err = runBackup(args, os.Stdout)
	case "restore":
		err = runRestore(args, os.Stdout)
	default:
		fmt.Fprintln(os.Stderr, "Use backup ou restore.")
		return 2
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, explainMaintenance(err))
		return 1
	}
	return 0
}
