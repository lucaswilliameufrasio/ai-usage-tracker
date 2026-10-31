package vault

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	magic    = "ATV1"
	maxPlain = 8 << 10
	fileMode = 0o600
	dirMode  = 0o700
)

var (
	ErrNotFound    = errors.New("vault: ausente")
	ErrInvalidName = errors.New("vault: nome")
	ErrInvalidKey  = errors.New("vault: chave")
	ErrTooLarge    = errors.New("vault: tamanho")
	ErrSealed      = errors.New("vault: selo")
	ErrUnavailable = errors.New("vault: indisponível")
)

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,80}$`)

// Box guarda segredos do servidor por nome. O valor só volta para o processo.
type Box interface {
	Put(ctx context.Context, name string, plaintext []byte) error
	Get(ctx context.Context, name string) ([]byte, error)
	Delete(ctx context.Context, name string) error
	Present(ctx context.Context, name string) (bool, error)
}

// Own é o cofre da instalação. A chave fica fora do diretório cifrado.
type Own struct {
	dir  string
	aead cipher.AEAD
}

// DecodeKey aceita 32 bytes em base64. A chave não é gravada por este pacote.
func DecodeKey(encoded string) ([]byte, error) {
	trimmed := strings.TrimSpace(encoded)
	if trimmed == "" {
		return nil, ErrInvalidKey
	}
	raw, err := base64.StdEncoding.DecodeString(trimmed)
	if err != nil {
		raw, err = base64.RawStdEncoding.DecodeString(trimmed)
	}
	if err != nil || len(raw) != 32 {
		return nil, ErrInvalidKey
	}
	return raw, nil
}

func OpenOwn(dir string, key []byte) (*Own, error) {
	if len(key) != 32 {
		return nil, ErrInvalidKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrInvalidKey
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return nil, fmt.Errorf("criar cofre: %w", err)
	}
	return &Own{dir: dir, aead: aead}, nil
}

func (o *Own) Put(ctx context.Context, name string, plaintext []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !namePattern.MatchString(name) {
		return ErrInvalidName
	}
	if len(plaintext) == 0 {
		return ErrInvalidName
	}
	if len(plaintext) > maxPlain {
		return ErrTooLarge
	}
	nonce := make([]byte, o.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	sealed := o.aead.Seal(nil, nonce, plaintext, []byte(name))
	payload := make([]byte, 0, len(magic)+len(nonce)+len(sealed))
	payload = append(payload, magic...)
	payload = append(payload, nonce...)
	payload = append(payload, sealed...)
	path := o.path(name)
	tmp := path + ".tmp"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, fileMode)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(payload)
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		_ = os.Remove(tmp)
		if writeErr != nil {
			return writeErr
		}
		if syncErr != nil {
			return syncErr
		}
		return closeErr
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func (o *Own) Get(ctx context.Context, name string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !namePattern.MatchString(name) {
		return nil, ErrInvalidName
	}
	payload, err := os.ReadFile(o.path(name))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	nonceSize := o.aead.NonceSize()
	if len(payload) < len(magic)+nonceSize {
		return nil, ErrSealed
	}
	if string(payload[:len(magic)]) != magic {
		return nil, ErrSealed
	}
	nonce := payload[len(magic) : len(magic)+nonceSize]
	sealed := payload[len(magic)+nonceSize:]
	plain, err := o.aead.Open(nil, nonce, sealed, []byte(name))
	if err != nil {
		return nil, ErrSealed
	}
	return plain, nil
}

func (o *Own) Delete(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !namePattern.MatchString(name) {
		return ErrInvalidName
	}
	err := os.Remove(o.path(name))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (o *Own) Present(ctx context.Context, name string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if !namePattern.MatchString(name) {
		return false, ErrInvalidName
	}
	_, err := os.Stat(o.path(name))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (o *Own) path(name string) string {
	sum := sha256.Sum256([]byte(name))
	return filepath.Join(o.dir, hex.EncodeToString(sum[:]))
}
