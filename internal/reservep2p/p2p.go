package reservep2p

import (
	"bufio"
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"reserveos/internal/reservekeyvault"
)

type Config struct {
	Bind string `json:"bind"`
	Port int    `json:"port"`
	KeyvaultPath string `json:"keyvault_path"`
	KeyvaultEnv string  `json:"keyvault_env"`
}

type Node struct {
	h Handler
	sessions map[string]*Session
	mu sync.Mutex

	cfg Config
	pubHex string
	priv ed25519.PrivateKey
	ln net.Listener
}

type hello struct {
	Type string `json:"type"` // "hello"
	Pub  string `json:"pub"`
	Nonce string `json:"nonce"`
	Sig string `json:"sig"`
}

func LoadOrGenIdentity(kv *reservekeyvault.Vault) (pubHex string, priv ed25519.PrivateKey, err error) {
	p, okP := kv.GetString("p2p.ed25519_pub")
	s, okS := kv.GetString("p2p.ed25519_priv")
	if okP && okS {
		pb, _ := hex.DecodeString(p)
		sb, _ := hex.DecodeString(s)
		if len(sb) == ed25519.PrivateKeySize {
			return p, ed25519.PrivateKey(sb), nil
		}
	}
	pub, privK, err := ed25519.GenerateKey(rand.Reader)
	if err != nil { return "", nil, err }
	pubHex = hex.EncodeToString(pub)
	privHex := hex.EncodeToString(privK)
	_ = kv.SetString("p2p.ed25519_pub", pubHex)
	_ = kv.SetString("p2p.ed25519_priv", privHex)
	return pubHex, privK, nil
}

func New(cfg Config) (*Node, error) {
	if cfg.Bind == "" { cfg.Bind = "0.0.0.0" }
	if cfg.Port == 0 { cfg.Port = 18444 }
	if cfg.KeyvaultPath == "" { cfg.KeyvaultPath = "runtime/keyvault/keyvault.enc" }
	if cfg.KeyvaultEnv == "" { cfg.KeyvaultEnv = "RESERVEOS_KEYMASTER" }

	kv, err := reservekeyvault.OpenOrInit(reservekeyvault.OpenOptions{Path: cfg.KeyvaultPath, KEKEnv: cfg.KeyvaultEnv, RotationDays: 5})
	if err != nil { return nil, err }
	pubHex, priv, err := LoadOrGenIdentity(kv)
	if err != nil { return nil, err }

	return &Node{cfg: cfg, pubHex: pubHex, priv: priv, sessions: map[string]*Session{}}, nil
}

func (n *Node) Start() error {
	addr := fmt.Sprintf("%s:%d", n.cfg.Bind, n.cfg.Port)
	ln, err := net.Listen("tcp", addr)
	if err != nil { return err }
	n.ln = ln
	go n.acceptLoop()
	return nil
}

func (n *Node) Stop() error {
	if n.ln != nil { return n.ln.Close() }
	return nil
}

func (n *Node) acceptLoop() {
	for {
		c, err := n.ln.Accept()
		if err != nil { return }
		go n.handleConn(c)
	}
}

func (n *Node) handleConn(c net.Conn) {
	defer c.Close()
	s, err := ServeSession(c, n.pubHex, n.priv, 10*time.Second)
	if err != nil { return }
	n.mu.Lock()
	n.sessions[s.PeerPub] = s
	n.mu.Unlock()
	go s.ReadLoop(func(peer string, m Msg){ if n.h != nil { n.h(peer, m) } })
	select {}
}

func bytesTrim(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r') { b = b[:len(b)-1] }
	return b
}

var _ = errors.New


func (n *Node) SetHandler(h Handler) { n.h = h }

func (n *Node) Sessions() []string {
	n.mu.Lock(); defer n.mu.Unlock()
	out := make([]string,0,len(n.sessions))
	for k := range n.sessions { out = append(out,k) }
	return out
}

func (n *Node) SendAll(msg Msg) {
	n.mu.Lock()
	ss := make([]*Session,0,len(n.sessions))
	for _, s := range n.sessions { ss = append(ss,s) }
	n.mu.Unlock()
	for _, s := range ss {
		_ = s.Send(msg)
	}
}

func (n *Node) Dial(addr string) error {
	s, err := DialSession(addr, n.pubHex, n.priv, 5*time.Second)
	if err != nil { return err }
	n.mu.Lock()
	n.sessions[s.PeerPub] = s
	n.mu.Unlock()
	go s.ReadLoop(func(peer string, m Msg){ if n.h != nil { n.h(peer, m) } })
	return nil
}


func (n *Node) SendTo(peerPub string, msg Msg) error {
	n.mu.Lock()
	s := n.sessions[peerPub]
	n.mu.Unlock()
	if s == nil { return fmt.Errorf("no_session") }
	return s.Send(msg)
}
