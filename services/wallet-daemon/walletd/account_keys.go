package walletd

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"

	"reserveos/internal/reservekeyvault"
)

const (
	kvWalletPub  = "wallet.ed25519_pub"
	kvWalletPriv = "wallet.ed25519_priv"
)

type WalletAccount struct {
	PubHex  string
	PrivHex string
}

func LoadOrGenWalletAccount(v *reservekeyvault.Vault) (*WalletAccount, error) {
	if v == nil { return nil, errors.New("nil keyvault") }
	pub, okP := v.GetString(kvWalletPub)
	priv, okS := v.GetString(kvWalletPriv)
	if okP && okS {
		return &WalletAccount{PubHex: pub, PrivHex: priv}, nil
	}
	p, s, err := ed25519.GenerateKey(rand.Reader)
	if err != nil { return nil, err }
	pub = hex.EncodeToString(p)
	priv = hex.EncodeToString(s)
	if err := v.SetString(kvWalletPub, pub); err != nil { return nil, err }
	if err := v.SetString(kvWalletPriv, priv); err != nil { return nil, err }
	return &WalletAccount{PubHex: pub, PrivHex: priv}, nil
}

func (a *WalletAccount) Sign(msg []byte) (string, error) {
	b, err := hex.DecodeString(a.PrivHex)
	if err != nil { return "", err }
	if len(b) != ed25519.PrivateKeySize { return "", errors.New("bad priv size") }
	sig := ed25519.Sign(ed25519.PrivateKey(b), msg)
	return hex.EncodeToString(sig), nil
}
