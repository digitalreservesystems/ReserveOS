package reservekeyvault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
)

type OpenOptions struct {
	RotationDays int

	Path   string
	KEKEnv string
}

type Vault struct {
	rotationDays int

	mu   sync.Mutex
	path string
	key  [32]byte
	data map[string]any
}

func OpenOrInit(opt OpenOptions) (*Vault, error) {
	kek := os.Getenv(opt.KEKEnv)
	if kek == "" {
		return nil, errors.New("missing keymaster env: " + opt.KEKEnv)
	}
	keyID := "k1"
	key := deriveKey(kek, keyID)
	_ = os.MkdirAll(filepath.Dir(opt.Path), 0700)

	if _, err := os.Stat(opt.Path); errors.Is(err, os.ErrNotExist) {
		v := &Vault{path: opt.Path, key: key, rotationDays: opt.RotationDays, data: map[string]any{"version": 1, "kv": map[string]any{}}}
		if err := v.persistLocked(); err != nil {
			return nil, err
		}
		return v, nil
	}

	plain, err := readDecrypted(opt.Path, key[:])
	if err != nil {
		return nil, err
	}
	var obj map[string]any
	if err := json.Unmarshal(plain, &obj); err != nil {
		return nil, err
	}
	if _, ok := obj["kv"]; !ok {
		obj["kv"] = map[string]any{}
	}
	v := &Vault{path: opt.Path, key: key, rotationDays: opt.RotationDays, data: obj}
	_ = v.maybeRotateLocked(kek)
	return v, nil
}

func (v *Vault) GetString(key string) (string, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	kv, _ := v.data["kv"].(map[string]any)
	val, ok := kv[key]
	if !ok {
		return "", false
	}
	s, ok := val.(string)
	return s, ok
}

func (v *Vault) SetString(key string, value string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	kv, ok := v.data["kv"].(map[string]any)
	if !ok || kv == nil {
		kv = map[string]any{}
		v.data["kv"] = kv
	}
	kv[key] = value
	return v.persistLocked()
}

func (v *Vault) persistLocked() error {
	b, err := json.Marshal(v.data)
	if err != nil {
		return err
	}
	block, err := aes.NewCipher(v.key[:])
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return err
	}
	ct := gcm.Seal(nil, nonce, b, nil)
	out := append([]byte("KVAULT1:"), nonce...)
	out = append(out, ct...)
	return os.WriteFile(v.path, out, 0600)
}

func readDecrypted(path string, key []byte) ([]byte, error) {
	in, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(in) < len("KVAULT1:")+12 {
		return nil, errors.New("invalid keyvault file")
	}
	if string(in[:8]) != "KVAULT1:" {
		return nil, errors.New("invalid keyvault header")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	nonce := in[8 : 8+ns]
	ct := in[8+ns:]
	return gcm.Open(nil, nonce, ct, nil)
}


func deriveKey(kek string, keyID string) [32]byte {
	return sha256.Sum256([]byte(kek + "|" + keyID))
}
