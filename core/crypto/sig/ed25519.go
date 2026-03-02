package sig

import (
	"crypto/ed25519"
	"encoding/hex"
	"errors"
)

func ParsePub(hexPub string) (ed25519.PublicKey, error) {
	b, err := hex.DecodeString(hexPub)
	if err != nil { return nil, err }
	if len(b) != ed25519.PublicKeySize { return nil, errors.New("bad pubkey size") }
	return ed25519.PublicKey(b), nil
}

func ParseSig(hexSig string) ([]byte, error) {
	b, err := hex.DecodeString(hexSig)
	if err != nil { return nil, err }
	if len(b) != ed25519.SignatureSize { return nil, errors.New("bad sig size") }
	return b, nil
}

func Verify(pubHex string, sigHex string, msg []byte) bool {
	pub, err := ParsePub(pubHex)
	if err != nil { return false }
	sig, err := ParseSig(sigHex)
	if err != nil { return false }
	return ed25519.Verify(pub, msg, sig)
}


func VerifyWithPubHex(pubHex string, sigHex string, msg []byte) bool {
	return Verify(pubHex, sigHex, msg)
}
