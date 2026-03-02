package schnorr

import (
	"crypto/rand"
	"crypto/sha512"
	"encoding/hex"
	"errors"

	"filippo.io/edwards25519"
)

type Signature struct {
	RHex string `json:"R"` // 32-byte point encoding hex
	SHex string `json:"s"` // 32-byte scalar encoding hex
}

// Sign signs msg with secret scalar x and public key P = x*G.
// Returns signature encoded as hex of R||s (64 bytes) by default helper.
func Sign(x *edwards25519.Scalar, P *edwards25519.Point, msg []byte) (sigHex string, err error) {
	k, err := randomScalar()
	if err != nil { return "", err }
	R := new(edwards25519.Point).ScalarBaseMult(k)
	e, err := challenge(R, P, msg)
	if err != nil { return "", err }
	ex := new(edwards25519.Scalar).Multiply(e, x)
	s := new(edwards25519.Scalar).Add(k, ex)

	out := append(R.Bytes(), s.Bytes()...)
	return hex.EncodeToString(out), nil
}

func Verify(P *edwards25519.Point, msg []byte, sigHex string) bool {
	sig, err := hex.DecodeString(sigHex)
	if err != nil || len(sig) != 64 { return false }
	Rbytes := sig[:32]
	sbytes := sig[32:]

	R := new(edwards25519.Point)
	if _, err := R.SetBytes(Rbytes); err != nil { return false }

	s := new(edwards25519.Scalar)
	if _, err := s.SetCanonicalBytes(sbytes); err != nil { return false }

	e, err := challenge(R, P, msg)
	if err != nil { return false }

	lhs := new(edwards25519.Point).ScalarBaseMult(s) // sG
	eP := new(edwards25519.Point).ScalarMult(e, P)
	rhs := new(edwards25519.Point).Add(R, eP) // R + eP

	return lhs.Equal(rhs) == 1
}

func challenge(R *edwards25519.Point, P *edwards25519.Point, msg []byte) (*edwards25519.Scalar, error) {
	h := sha512.New()
	h.Write([]byte("ReserveOS:Schnorr25519:v1|"))
	h.Write(R.Bytes())
	h.Write(P.Bytes())
	h.Write(msg)
	sum := h.Sum(nil)
	if len(sum) != 64 { return nil, errors.New("bad hash length") }
	sc := new(edwards25519.Scalar)
	sc, err := sc.SetUniformBytes(sum)
	if err != nil { return nil, err }
	return sc, nil
}

func randomScalar() (*edwards25519.Scalar, error) {
	buf := make([]byte, 64)
	if _, err := rand.Read(buf); err != nil { return nil, err }
	s := new(edwards25519.Scalar)
	s, err := s.SetUniformBytes(buf)
	if err != nil { return nil, err }
	return s, nil
}
