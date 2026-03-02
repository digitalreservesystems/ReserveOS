import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"reserveos/core/chain"
	"reserveos/internal/reservestorage"
)

type GossipTxMsg struct {
	Hop int `json:"hop"`
	Tx  chain.Tx `json:"tx"`
}

func (n *Node) gossipTx(tx chain.Tx, hop int) {
	if !n.cfg.Gossip.Enabled { return }
	if hop >= n.cfg.Gossip.MaxHops { return }
	msg := GossipTxMsg{Hop: hop + 1, Tx: tx}
	b, _ := json.Marshal(msg)
	hc := &http.Client{Timeout: time.Duration(n.cfg.Gossip.TimeoutSeconds) * time.Second}
	peers := append([]string{}, n.cfg.Gossip.Peers...)
	if ps, err := reservestorage.ListPeers(n.db.DB, 5000); err == nil {
		for _, p := range ps { peers = append(peers, p.Addr) }
	}
	for _, peer := range peers {
		_, _ = hc.Post(peer+"/gossip/tx", "application/json", bytes.NewReader(b))
	}
}
