package store

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrSnapshotExists  = errors.New("snapshot exists")
	ErrSnapshotSame    = errors.New("snapshot same")
	ErrSnapshotDest    = errors.New("snapshot dest")
	ErrSnapshotInvalid = errors.New("snapshot invalid")
	ErrRestoreExists   = errors.New("restore exists")
)

type Census struct {
	UserVersion    int
	Accounts       int
	Devices        int
	Samples        int
	Sessions       int
	LoginProviders int
}

func SameTarget(source string, dest string) error {
	if err := readablePath(source); err != nil {
		return err
	}
	if err := readablePath(dest); err != nil {
		return err
	}
	left, err := filepath.Abs(source)
	if err != nil {
		return err
	}
	right, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
	if left == right {
		return ErrSnapshotSame
	}
	return nil
}

func (s *Store) Census() (Census, error) {
	return readCensus(s.db)
}

func (s *Store) Snapshot(dest string) (Census, error) {
	if err := readablePath(dest); err != nil {
		return Census{}, err
	}
	if _, err := os.Stat(dest); err == nil {
		return Census{}, ErrSnapshotExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return Census{}, err
	}
	before, err := s.Census()
	if err != nil {
		return Census{}, err
	}
	if before.UserVersion != schemaVersion {
		return Census{}, ErrSnapshotInvalid
	}
	if _, err := s.db.Exec(`VACUUM INTO ` + sqlString(dest)); err != nil {
		_ = os.Remove(dest)
		return Census{}, err
	}
	if err := os.Chmod(dest, 0o600); err != nil {
		return Census{}, err
	}
	after, err := Inspect(dest)
	if err != nil {
		_ = os.Remove(dest)
		return Census{}, err
	}
	if after != before {
		_ = os.Remove(dest)
		return Census{}, ErrSnapshotInvalid
	}
	return after, nil
}

func Inspect(path string) (Census, error) {
	if err := readablePath(path); err != nil {
		return Census{}, err
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=busy_timeout(5000)&_pragma=query_only(1)")
	if err != nil {
		return Census{}, ErrSnapshotInvalid
	}
	defer db.Close()
	rows, err := db.Query(`PRAGMA integrity_check`)
	if err != nil {
		return Census{}, ErrSnapshotInvalid
	}
	defer rows.Close()
	var messages []string
	for rows.Next() {
		var message string
		if err := rows.Scan(&message); err != nil {
			return Census{}, ErrSnapshotInvalid
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return Census{}, ErrSnapshotInvalid
	}
	if len(messages) != 1 || messages[0] != "ok" {
		return Census{}, ErrSnapshotInvalid
	}
	census, err := readCensus(db)
	if err != nil || census.UserVersion != schemaVersion {
		return Census{}, ErrSnapshotInvalid
	}
	return census, nil
}

func Restore(snapshot string, destDir string) (Census, error) {
	census, err := Inspect(snapshot)
	if err != nil {
		return Census{}, err
	}
	if err := readablePath(destDir); err != nil {
		return Census{}, err
	}
	if err := os.MkdirAll(destDir, 0o700); err != nil {
		return Census{}, err
	}
	target := filepath.Join(destDir, "tracker.db")
	if _, err := os.Stat(target); err == nil {
		return Census{}, ErrRestoreExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return Census{}, err
	}
	in, err := os.Open(snapshot)
	if err != nil {
		return Census{}, err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return Census{}, ErrRestoreExists
		}
		return Census{}, err
	}
	copied := false
	defer func() {
		if !copied {
			_ = os.Remove(target)
		}
	}()
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return Census{}, err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return Census{}, err
	}
	if err := out.Close(); err != nil {
		return Census{}, err
	}
	if err := os.Chmod(target, 0o600); err != nil {
		return Census{}, err
	}
	restored, err := Inspect(target)
	if err != nil {
		return Census{}, err
	}
	if restored != census {
		return Census{}, ErrSnapshotInvalid
	}
	copied = true
	return restored, nil
}

type rowQuery interface {
	QueryRow(query string, args ...any) *sql.Row
}

func readCensus(q rowQuery) (Census, error) {
	var census Census
	if err := q.QueryRow(`PRAGMA user_version`).Scan(&census.UserVersion); err != nil {
		return Census{}, err
	}
	counts := []struct {
		query string
		dest  *int
	}{
		{`SELECT COUNT(*) FROM accounts`, &census.Accounts},
		{`SELECT COUNT(*) FROM devices`, &census.Devices},
		{`SELECT COUNT(*) FROM samples`, &census.Samples},
		{`SELECT COUNT(*) FROM sessions`, &census.Sessions},
		{`SELECT COUNT(*) FROM login_providers`, &census.LoginProviders},
	}
	for _, item := range counts {
		if err := q.QueryRow(item.query).Scan(item.dest); err != nil {
			return Census{}, err
		}
	}
	return census, nil
}

func readablePath(path string) error {
	if strings.TrimSpace(path) == "" || strings.ContainsAny(path, "\x00?#\n\r") {
		return ErrSnapshotDest
	}
	return nil
}

func sqlString(path string) string {
	return "'" + strings.ReplaceAll(path, "'", "''") + "'"
}

func FormatCensus(census Census) string {
	return fmt.Sprintf(
		"versao=%d contas=%d dispositivos=%d amostras=%d sessoes=%d provedores=%d\n",
		census.UserVersion,
		census.Accounts,
		census.Devices,
		census.Samples,
		census.Sessions,
		census.LoginProviders,
	)
}
