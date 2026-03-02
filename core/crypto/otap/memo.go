package otap

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
)

func EncryptMemo(sharedSecretK []byte, plaintext []byte, aad []byte) (string, error) {
	memoKey, err := deriveMemoKey(sharedSecretK)
	if err != nil { return "", err }
	aead, err := chacha20poly1305.New(memoKey)
	if err != nil { return "", err }
	nonce := make([]byte, chacha20poly1305.NonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil { return "", err }
	ct := aead.Seal(nil, nonce, plaintext, aad)
	out := append(nonce, ct...)
	return base64.StdEncoding.EncodeToString(out), nil
}

func DecryptMemo(sharedSecretK []byte, enc string, aad []byte) ([]byte, error) {
	memoKey, err := deriveMemoKey(sharedSecretK)
	if err != nil { return nil, err }
	aead, err := chacha20poly1305.New(memoKey)
	if err != nil { return nil, err }
	b, err := base64.StdEncoding.DecodeString(enc)
	if err != nil { return nil, err }
	ns := chacha20poly1305.NonceSize
	if len(b) < ns { return nil, errors.New("enc_memo too short") }
	nonce := b[:ns]
	ct := b[ns:]
	return aead.Open(nil, nonce, ct, aad)
}

func deriveMemoKey(K []byte) ([]byte, error) {
	khash := sha256.Sum256(K)
	r := hkdf.New(sha256.New, khash[:], nil, []byte("ReserveOS:OTAP:memo"))
	key := make([]byte, 32)
	if _, err := io.ReadFull(r, key); err != nil { return nil, err }
	return key, nil
}
