package node

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"reserveos/core/chain"
	"reserveos/internal/reservestorage"
)

type GossipBlockMsg struct {
	Hop int `json:"hop"`
	Block chain.Block `json:"block"`
	Hash string `json:"hash"`
}

func (n *Node) gossipBlock(b chain.Block, hash string, hop int) {
	if !n.cfg.Gossip.Enabled { return }
	if hop >= n.cfg.Gossip.MaxHops { return }
	msg := GossipBlockMsg{Hop: hop + 1, Block: b, Hash: hash}
	bb, _ := json.Marshal(msg)
	hc := &http.Client{Timeout: time.Duration(n.cfg.Gossip.TimeoutSeconds) * time.Second}
	peers := append([]string{}, n.cfg.Gossip.Peers...)
	if ps, err := reservestorage.ListPeers(n.db.DB, 5000); err == nil {
		for _, p := range ps { peers = append(peers, p.Addr) }
	}
	for _, peer := range peers {
		_, _ = hc.Post(peer+"/gossip/block", "application/json", bytes.NewReader(bb))
	}
}
