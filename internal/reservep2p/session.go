package reservep2p

import (
	"bufio"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

type Handler func(peerPub string, msg Msg)

type Session struct {
	PeerPub string
	Conn net.Conn
	R *bufio.Reader
	W *bufio.Writer
	AEAD cipher.AEAD
	mu sync.Mutex
}

func (s *Session) Send(msg Msg) error {
	b := EncodeMsg(msg)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := writeFrame(s.W, s.AEAD, b); err != nil { return err }
	return s.W.Flush()
}

func (s *Session) Close() error { return s.Conn.Close() }

// DialSession establishes hello handshake and AES-GCM session.
func DialSession(addr string, pubHex string, priv ed25519.PrivateKey, timeout time.Duration) (*Session, error) {
	c, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil { return nil, err }

	_ = c.SetDeadline(time.Now().Add(timeout))
	r := bufio.NewReader(c)
	w := bufio.NewWriter(c)

	nonceA := make([]byte, 16)
	_, _ = rand.Read(nonceA)
	nonceAHex := hex.EncodeToString(nonceA)
	msgA := []byte("hello|" + pubHex + "|" + nonceAHex)
	sigA := ed25519.Sign(priv, msgA)

	h := hello{Type:"hello", Pub: pubHex, Nonce: nonceAHex, Sig: hex.EncodeToString(sigA)}
	b, _ := json.Marshal(h)
	b = append(b, '\n')
	if _, err := w.Write(b); err != nil { c.Close(); return nil, err }
	if err := w.Flush(); err != nil { c.Close(); return nil, err }

	line, err := r.ReadBytes('\n')
	if err != nil { c.Close(); return nil, err }
	var resp hello
	if err := json.Unmarshal(bytesTrim(line), &resp); err != nil { c.Close(); return nil, err }
	if resp.Type != "hello" { c.Close(); return nil, fmt.Errorf("bad hello") }

	peerPubBytes, err := hex.DecodeString(resp.Pub)
	if err != nil || len(peerPubBytes) != ed25519.PublicKeySize { c.Close(); return nil, fmt.Errorf("bad peer pub") }
	peerSig, err := hex.DecodeString(resp.Sig)
	if err != nil || len(peerSig) != ed25519.SignatureSize { c.Close(); return nil, fmt.Errorf("bad peer sig") }
	msgB := []byte("hello|" + resp.Pub + "|" + resp.Nonce)
	if !ed25519.Verify(ed25519.PublicKey(peerPubBytes), msgB, peerSig) { c.Close(); return nil, fmt.Errorf("hello verify failed") }

	key := deriveSessionKey(pubHex, resp.Pub, nonceAHex, resp.Nonce)
	blk, _ := aes.NewCipher(key)
	aead, _ := cipher.NewGCM(blk)

	_ = c.SetDeadline(time.Time{})
	return &Session{PeerPub: resp.Pub, Conn: c, R: r, W: w, AEAD: aead}, nil
}

func ServeSession(c net.Conn, localPub string, localPriv ed25519.PrivateKey, timeout time.Duration) (*Session, error) {
	_ = c.SetDeadline(time.Now().Add(timeout))
	r := bufio.NewReader(c)
	w := bufio.NewWriter(c)

	line, err := r.ReadBytes('\n')
	if err != nil { return nil, err }
	var h hello
	if err := json.Unmarshal(bytesTrim(line), &h); err != nil { return nil, err }
	if h.Type != "hello" { return nil, fmt.Errorf("bad hello") }

	peerPubBytes, err := hex.DecodeString(h.Pub)
	if err != nil || len(peerPubBytes) != ed25519.PublicKeySize { return nil, fmt.Errorf("bad peer pub") }
	peerSig, err := hex.DecodeString(h.Sig)
	if err != nil || len(peerSig) != ed25519.SignatureSize { return nil, fmt.Errorf("bad peer sig") }
	msgA := []byte("hello|" + h.Pub + "|" + h.Nonce)
	if !ed25519.Verify(ed25519.PublicKey(peerPubBytes), msgA, peerSig) { return nil, fmt.Errorf("hello verify failed") }

	nonceB := make([]byte, 16)
	_, _ = rand.Read(nonceB)
	nonceBHex := hex.EncodeToString(nonceB)
	msgB := []byte("hello|" + localPub + "|" + nonceBHex)
	sigB := ed25519.Sign(localPriv, msgB)
	resp := hello{Type:"hello", Pub: localPub, Nonce: nonceBHex, Sig: hex.EncodeToString(sigB)}
	bb, _ := json.Marshal(resp)
	bb = append(bb, '\n')
	if _, err := w.Write(bb); err != nil { return nil, err }
	if err := w.Flush(); err != nil { return nil, err }

	key := deriveSessionKey(h.Pub, localPub, h.Nonce, nonceBHex)
	blk, _ := aes.NewCipher(key)
	aead, _ := cipher.NewGCM(blk)

	_ = c.SetDeadline(time.Time{})
	return &Session{PeerPub: h.Pub, Conn: c, R: r, W: w, AEAD: aead}, nil
}

func (s *Session) ReadLoop(h Handler) {
	for {
		pt, err := readFrame(s.R, s.AEAD)
		if err != nil { return }
		m, err := DecodeMsg(pt)
		if err != nil { continue }
		h(s.PeerPub, m)
	}
}

var _ = io.EOF
