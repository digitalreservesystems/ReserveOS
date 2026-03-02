package reservep2p

import (
	"bufio"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"time"
)

func Dial(addr string, pubHex string, priv ed25519.PrivateKey) (string, error) {
	c, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil { return "", err }
	defer c.Close()

	nonce := make([]byte, 16)
	_, _ = rand.Read(nonce)
	nonceHex := hex.EncodeToString(nonce)
	msg := []byte("hello|" + pubHex + "|" + nonceHex)
	sig := ed25519.Sign(priv, msg)

	h := hello{Type:"hello", Pub:pubHex, Nonce:nonceHex, Sig:hex.EncodeToString(sig)}
	b, _ := json.Marshal(h)
	b = append(b, '\n')
	_, _ = c.Write(b)

	_ = c.SetReadDeadline(time.Now().Add(5*time.Second))
	r := bufio.NewReader(c)
	line, err := r.ReadBytes('\n')
	if err != nil { return "", err }
	var resp hello
	if err := json.Unmarshal(bytesTrim(line), &resp); err != nil { return "", err }
	if resp.Type != "hello" { return "", fmt.Errorf("bad response") }
	return resp.Pub, nil
}
