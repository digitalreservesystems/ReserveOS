package finality

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

type Validator struct {
	Name      string
	PubkeyHex string
	Weight    int64
}

type Vote struct {
	Height    uint64 `json:"height"`
	BlockHash string `json:"block_hash"`
	ChainID   string `json:"chain_id"`
	PubkeyHex string `json:"pubkey_hex"`
	SigHex    string `json:"sig_hex"`
	TimeUnix  int64  `json:"time_unix"`
}

func VoteMessage(chainID string, height uint64, blockHash string) []byte {
	// msg = SHA256("ReserveOS:PoSFinality:v1"||chainID||height||hash)
	h := sha256.New()
	h.Write([]byte("ReserveOS:PoSFinality:v1|"))
	h.Write([]byte(chainID))
	h.Write([]byte("|"))
	h.Write([]byte(fmt.Sprintf("%d", height)))
	h.Write([]byte("|"))
	h.Write([]byte(blockHash))
	sum := h.Sum(nil)
	return sum
}

func SignVote(priv ed25519.PrivateKey, chainID string, height uint64, blockHash string) (sigHex string) {
	msg := VoteMessage(chainID, height, blockHash)
	sig := ed25519.Sign(priv, msg)
	return hex.EncodeToString(sig)
}

func VerifyVote(pub ed25519.PublicKey, sigHex string, chainID string, height uint64, blockHash string) bool {
	sig, err := hex.DecodeString(sigHex)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return false
	}
	msg := VoteMessage(chainID, height, blockHash)
	return ed25519.Verify(pub, msg, sig)
}

func EnsureLocalValidatorKey(get func(k string) (string, bool), set func(k, v string) error) (pubHex string, privHex string, err error) {
	// Store in keyvault:
	//  pos.ed25519_pub, pos.ed25519_priv
	if p, ok := get("pos.ed25519_pub"); ok {
		if s, ok2 := get("pos.ed25519_priv"); ok2 {
			return p, s, nil
		}
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	pubHex = hex.EncodeToString(pub)
	privHex = hex.EncodeToString(priv)
	if err := set("pos.ed25519_pub", pubHex); err != nil { return "", "", err }
	if err := set("pos.ed25519_priv", privHex); err != nil { return "", "", err }
	return pubHex, privHex, nil
}

func ParsePriv(privHex string) (ed25519.PrivateKey, error) {
	b, err := hex.DecodeString(privHex)
	if err != nil { return nil, err }
	if len(b) != ed25519.PrivateKeySize {
		return nil, errors.New("bad ed25519 priv size")
	}
	return ed25519.PrivateKey(b), nil
}

func ParsePub(pubHex string) (ed25519.PublicKey, error) {
	b, err := hex.DecodeString(pubHex)
	if err != nil { return nil, err }
	if len(b) != ed25519.PublicKeySize {
		return nil, errors.New("bad ed25519 pub size")
	}
	return ed25519.PublicKey(b), nil
}

func TotalWeight(vs []Validator) int64 {
	var t int64
	for _, v := range vs {
		t += v.Weight
	}
	return t
}

func ReachedThreshold(sum int64, total int64, num int64, den int64) bool {
	if total <= 0 { return false }
	// sum/total >= num/den  => sum*den >= total*num
	return sum*den >= total*num
}
