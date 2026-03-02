package otap

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"

	"filippo.io/edwards25519"
)

const DomainSep = "ReserveOS:OTAP:v1"

type RegistryKeys struct {
	ScanPub  string `json:"S_pub"` // hex compressed
	SpendPub string `json:"V_pub"` // hex compressed
}

type PrivateKeys struct {
	ScanPriv  string `json:"s"` // hex scalar 32
	SpendPriv string `json:"v"` // hex scalar 32
}

// GenerateKeys generates (s,v) and corresponding (S_pub,V_pub).
func GenerateKeys() (RegistryKeys, PrivateKeys, error) {
	s, err := randomScalar()
	if err != nil { return RegistryKeys{}, PrivateKeys{}, err }
	v, err := randomScalar()
	if err != nil { return RegistryKeys{}, PrivateKeys{}, err }

	S := new(edwards25519.Point).ScalarBaseMult(s)
	V := new(edwards25519.Point).ScalarBaseMult(v)

	return RegistryKeys{
		ScanPub:  hex.EncodeToString(S.Bytes()),
		SpendPub: hex.EncodeToString(V.Bytes()),
	}, PrivateKeys{
		ScanPriv:  hex.EncodeToString(s.Bytes()),
		SpendPriv: hex.EncodeToString(v.Bytes()),
	}, nil
}

type OTAPOutput struct {
	P   string `json:"P"`   // hex compressed
	R   string `json:"R"`   // hex compressed
	Tag string `json:"tag"` // 4 hex chars (16-bit) optional
}

func BuildOutput(chainID string, context string, reg RegistryKeys) (OTAPOutput, []byte, error) {
	S, err := decodePoint(reg.ScanPub)
	if err != nil { return OTAPOutput{}, nil, err }
	V, err := decodePoint(reg.SpendPub)
	if err != nil { return OTAPOutput{}, nil, err }

	r, err := randomScalar()
	if err != nil { return OTAPOutput{}, nil, err }
	R := new(edwards25519.Point).ScalarBaseMult(r)

	K := new(edwards25519.Point).ScalarMult(r, S) // r*S_pub

	k, err := hToScalar(K.Bytes(), chainID, context)
	if err != nil { return OTAPOutput{}, nil, err }

	kG := new(edwards25519.Point).ScalarBaseMult(k)
	P := new(edwards25519.Point).Add(V, kG)

	tag := tag16(K.Bytes())

	return OTAPOutput{
		P: hex.EncodeToString(P.Bytes()),
		R: hex.EncodeToString(R.Bytes()),
		Tag: tag,
	}, K.Bytes(), nil // return shared secret bytes for memo encryption
}

func Detect(chainID, context string, out OTAPOutput, sPrivHex string, VpubHex string) (matched bool, Kbytes []byte, err error) {
	s, err := decodeScalar(sPrivHex)
	if err != nil { return false, nil, err }
	R, err := decodePoint(out.R)
	if err != nil { return false, nil, err }
	V, err := decodePoint(VpubHex)
	if err != nil { return false, nil, err }
	P, err := decodePoint(out.P)
	if err != nil { return false, nil, err }

	K := new(edwards25519.Point).ScalarMult(s, R) // s*R
	k, err := hToScalar(K.Bytes(), chainID, context)
	if err != nil { return false, nil, err }
	kG := new(edwards25519.Point).ScalarBaseMult(k)
	Pp := new(edwards25519.Point).Add(V, kG)

	if Pp.Equal(P) != 1 {
		return false, nil, nil
	}
	return true, K.Bytes(), nil
}

func tag16(K []byte) string {
	h := sha256.Sum256(append(append([]byte{}, K...), []byte("tag")...))
	return hex.EncodeToString(h[:2])
}

func hToScalar(K []byte, chainID string, context string) (*edwards25519.Scalar, error) {
	// Use SHA-512 -> 64 bytes -> SetUniformBytes
	h := sha512.New()
	h.Write([]byte(DomainSep))
	h.Write([]byte("|"))
	h.Write([]byte(chainID))
	h.Write([]byte("|"))
	h.Write([]byte(context))
	h.Write([]byte("|"))
	h.Write(K)
	sum := h.Sum(nil)
	if len(sum) != 64 { return nil, errors.New("bad sha512 length") }
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

func decodePoint(hexComp string) (*edwards25519.Point, error) {
	b, err := hex.DecodeString(hexComp)
	if err != nil { return nil, err }
	if len(b) != 32 { return nil, errors.New("point must be 32 bytes") }
	p := new(edwards25519.Point)
	_, err = p.SetBytes(b)
	if err != nil { return nil, err }
	return p, nil
}

func decodeScalar(hex32 string) (*edwards25519.Scalar, error) {
	b, err := hex.DecodeString(hex32)
	if err != nil { return nil, err }
	if len(b) != 32 { return nil, errors.New("scalar must be 32 bytes") }
	s := new(edwards25519.Scalar)
	s, err = s.SetCanonicalBytes(b)
	if err != nil { return nil, err }
	return s, nil
}


// DetectWithK returns whether output matches and also returns k' scalar (derived from K'=s*R).
func DetectWithK(chainID, context string, out OTAPOutput, sPrivHex string, VpubHex string) (matched bool, Kbytes []byte, k *edwards25519.Scalar, err error) {
	s, err := decodeScalar(sPrivHex)
	if err != nil { return false, nil, nil, err }
	R, err := decodePoint(out.R)
	if err != nil { return false, nil, nil, err }
	V, err := decodePoint(VpubHex)
	if err != nil { return false, nil, nil, err }
	P, err := decodePoint(out.P)
	if err != nil { return false, nil, nil, err }

	K := new(edwards25519.Point).ScalarMult(s, R) // s*R
	k2, err := hToScalar(K.Bytes(), chainID, context)
	if err != nil { return false, nil, nil, err }
	kG := new(edwards25519.Point).ScalarBaseMult(k2)
	Pp := new(edwards25519.Point).Add(V, kG)

	if Pp.Equal(P) != 1 {
		return false, nil, nil, nil
	}
	return true, K.Bytes(), k2, nil
}

// OneTimePriv derives x = v + k' (mod L), where v is spend private scalar.
func OneTimePriv(vPrivHex string, k *edwards25519.Scalar) (*edwards25519.Scalar, error) {
	v, err := decodeScalar(vPrivHex)
	if err != nil { return nil, err }
	x := new(edwards25519.Scalar).Add(v, k)
	return x, nil
}


// DecodePointHex is an exported helper for wallet services.
func DecodePointHex(hexStr string) (*edwards25519.Point, error) {
	return decodePoint(hexStr)
}
