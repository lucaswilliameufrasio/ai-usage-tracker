package agent

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

const (
	releaseRepo    = "lucaswilliameufrasio/ai-usage-tracker"
	releaseCommand = "ai-usage"
	archiveLimit   = 64 << 20
)

type UpdateOptions struct {
	CurrentVersion string
	BaseURL        string
	LatestURL      string
	Tag            string
	Client         *http.Client
	GOOS           string
	GOARCH         string
	Executable     string
	Exec           func(argv0 string, argv []string, env []string) error
	Args           []string
	Env            []string
}

func ApplyUpdate(ctx context.Context, opts UpdateOptions) error {
	if _, _, _, ok := parseSemver(opts.CurrentVersion); !ok {
		return nil
	}
	tag, err := resolveTag(ctx, opts)
	if err != nil {
		return err
	}
	next := strings.TrimPrefix(tag, "v")
	if !newerVersion(opts.CurrentVersion, next) {
		return nil
	}
	base := strings.TrimRight(opts.BaseURL, "/")
	if base == "" {
		base = "https://github.com/" + releaseRepo + "/releases/download/" + tag
	}
	return installRelease(ctx, opts, base, next)
}

func resolveTag(ctx context.Context, opts UpdateOptions) (string, error) {
	if opts.Tag != "" {
		if !stableTag(opts.Tag) {
			return "", errors.New("invalid stable release tag")
		}
		return opts.Tag, nil
	}
	url := opts.LatestURL
	if url == "" {
		url = "https://api.github.com/repos/" + releaseRepo + "/releases/latest"
	}
	raw, err := getBytes(ctx, opts.Client, url, 1<<20, "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	var body struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(raw, &body); err != nil || !stableTag(body.TagName) {
		return "", errors.New("could not read the release")
	}
	return body.TagName, nil
}

func installRelease(ctx context.Context, opts UpdateOptions, base string, version string) error {
	goos := opts.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	arch := opts.GOARCH
	if arch == "" {
		arch = runtime.GOARCH
	}
	asset := fmt.Sprintf("%s_%s_%s_%s.tar.gz", releaseCommand, version, goos, arch)
	checksums, err := getBytes(ctx, opts.Client, base+"/checksums.txt", 1<<20, "text/plain")
	if err != nil {
		return err
	}
	want, ok := checksumFor(string(checksums), asset)
	if !ok {
		return errors.New("checksum missing or invalid")
	}
	archive, err := getBytes(ctx, opts.Client, base+"/"+asset, archiveLimit, "application/octet-stream")
	if err != nil {
		return err
	}
	sum := sha256.Sum256(archive)
	if hex.EncodeToString(sum[:]) != want {
		return errors.New("checksum mismatch")
	}
	payload, err := extractAgent(archive)
	if err != nil {
		return err
	}
	return swapExecutable(opts, payload)
}

func getBytes(ctx context.Context, client *http.Client, url string, limit int64, accept string) ([]byte, error) {
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, errors.New("could not download the release")
	}
	req.Header.Set("User-Agent", "ai-usage-tracker")
	req.Header.Set("Accept", accept)
	res, err := client.Do(req)
	if err != nil {
		return nil, errors.New("could not download the release")
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return nil, errors.New("could not download the release")
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil || int64(len(raw)) > limit {
		return nil, errors.New("could not download the release")
	}
	return raw, nil
}

func checksumFor(raw string, asset string) (string, bool) {
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		name := strings.TrimPrefix(fields[1], "*")
		if name != asset {
			continue
		}
		sum := strings.ToLower(fields[0])
		if len(sum) != 64 || strings.Trim(sum, "0123456789abcdef") != "" {
			return "", false
		}
		return sum, true
	}
	return "", false
}

func extractAgent(raw []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, errors.New("package unreadable")
	}
	defer func() { _ = gz.Close() }()
	reader := tar.NewReader(io.LimitReader(gz, archiveLimit))
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, errors.New("package unreadable")
		}
		if header.Name != releaseCommand {
			continue
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != 0 {
			return nil, fmt.Errorf("the package does not contain %s", releaseCommand)
		}
		if header.Size <= 0 || header.Size > archiveLimit {
			return nil, errors.New("pacote grande demais")
		}
		payload, err := io.ReadAll(io.LimitReader(reader, archiveLimit+1))
		if err != nil || int64(len(payload)) != header.Size {
			return nil, errors.New("package unreadable")
		}
		return payload, nil
	}
	return nil, fmt.Errorf("the package does not contain %s", releaseCommand)
}

func swapExecutable(opts UpdateOptions, payload []byte) error {
	path := opts.Executable
	if path == "" {
		found, err := os.Executable()
		if err != nil {
			return errors.New("could not find the executable")
		}
		path = found
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".agent-new-*")
	if err != nil {
		return errors.New("could not replace the executable")
	}
	name := tmp.Name()
	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return errors.New("could not replace the executable")
	}
	if err := tmp.Chmod(0o755); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return errors.New("could not replace the executable")
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return errors.New("could not replace the executable")
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return errors.New("could not replace the executable")
	}
	execFn := opts.Exec
	if execFn == nil {
		execFn = syscall.Exec
	}
	args := opts.Args
	if args == nil {
		args = os.Args
	}
	env := opts.Env
	if env == nil {
		env = os.Environ()
	}
	if err := execFn(path, args, env); err != nil {
		return errors.New("could not restart the agent")
	}
	return nil
}

func stableTag(tag string) bool {
	if !strings.HasPrefix(tag, "v") {
		return false
	}
	_, _, _, ok := parseSemver(strings.TrimPrefix(tag, "v"))
	return ok
}

func newerVersion(current string, next string) bool {
	c1, c2, c3, ok := parseSemver(current)
	if !ok {
		return false
	}
	n1, n2, n3, ok := parseSemver(next)
	if !ok {
		return false
	}
	if n1 != c1 {
		return n1 > c1
	}
	if n2 != c2 {
		return n2 > c2
	}
	return n3 > c3
}

func parseSemver(value string) (int, int, int, bool) {
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return 0, 0, 0, false
	}
	nums := [3]int{}
	for i, part := range parts {
		if part == "" || (len(part) > 1 && part[0] == '0') {
			return 0, 0, 0, false
		}
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return 0, 0, 0, false
		}
		nums[i] = n
	}
	return nums[0], nums[1], nums[2], true
}
