package node

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"reserveos/core/chain"
	"reserveos/core/consensus/finality"
	"reserveos/core/consensus/pow"
	"reserveos/core/economics/fees"
	"reserveos/core/economics/pop"
	"reserveos/core/crypto/sig"
	"reserveos/core/crypto/schnorr"
	"filippo.io/edwards25519"
	"reserveos/core/state"
	"reserveos/internal/reserveconfig"
	"reserveos/internal/reservekeyvault"
	"reserveos/internal/reserverpc"
	"reserveos/internal/reservestorage"
	"reserveos/internal/reservep2p"
)

type Node struct {
	sm *syncManager

	cfg     *Config
	db      *reservestorage.LevelDB
	vault   *reservekeyvault.Vault
	rpc     *reserverpc.Server
	genesis *chain.Genesis
	mp      *Mempool
	p2p     *reservep2p.Node

	// finality / PoS voting (demo)
	finalityEnabled    bool
	checkpointInterval uint64
	thresholdNum       int64
	thresholdDen       int64
	validators         []finality.Validator
	localPubHex        string
	localPrivHex       string

	started time.Time
	ready   bool
}

func NewFromConfig(path string) (*Node, error) {
	var cfg Config
	if err := reserveconfig.LoadJSON(path, &cfg); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Node{cfg: &cfg}, nil
}

func (n *Node) Start(ctx context.Context) error {
	n.started = time.Now()
	n.mp = NewMempool(n.cfg.Mempool)
	// reload persisted mempool
	if txs, err := reservestorage.LoadMempoolTxs(n.db.DB, 5000); err == nil {
		for _, tx := range txs { _, _ = n.mp.Add(n.db.DB, tx) }
	}

	gen, err := chain.LoadGenesis(n.cfg.Chain.GenesisFile)
	if err != nil {
		return fmt.Errorf("genesis: %w", err)
	}
	n.genesis = gen

	v, err := reservekeyvault.OpenOrInit(reservekeyvault.OpenOptions{Path: n.cfg.Keyvault.Path, KEKEnv: n.cfg.Keyvault.KEKEnv})
	if err != nil {
		return fmt.Errorf("keyvault: %w", err)
	}
	n.vault = v

	db, err := reservestorage.OpenLevelDB(n.cfg.Storage.LevelDBPath)
	if err != nil {
		return fmt.Errorf("leveldb: %w", err)
	}
	n.db = db

	if err := n.ensureGenesisStored(); err != nil {
		return err
	}

	// Finality config: prefer config, otherwise inherit from genesis.finality/validators
	fc := n.cfg.Finality
	if len(fc.Validators) == 0 && n.genesis != nil && len(n.genesis.Validators) > 0 {
		for _, gv := range n.genesis.Validators {
			fc.Validators = append(fc.Validators, FinalityValidator{Name: gv.Name, PubkeyHex: gv.PubkeyHex, Weight: gv.Weight})
		}
	}
	if (fc.CheckpointInterval == 0 || fc.ThresholdNum == 0 || fc.ThresholdDen == 0) && n.genesis != nil && n.genesis.Finality != nil {
		if fc.CheckpointInterval == 0 {
			fc.CheckpointInterval = n.genesis.Finality.CheckpointInterval
		}
		if fc.ThresholdNum == 0 {
			fc.ThresholdNum = n.genesis.Finality.ThresholdNum
		}
		if fc.ThresholdDen == 0 {
			fc.ThresholdDen = n.genesis.Finality.ThresholdDen
		}
	}

	n.finalityEnabled = fc.Enabled
	n.checkpointInterval = fc.CheckpointInterval
	n.thresholdNum = fc.ThresholdNum
	n.thresholdDen = fc.ThresholdDen

	if n.finalityEnabled {
		pubHex, privHex, err := finality.EnsureLocalValidatorKey(n.vault.GetString, n.vault.SetString)
		if err != nil {
			return fmt.Errorf("finality key: %w", err)
		}
		n.localPubHex = pubHex
		n.localPrivHex = privHex

		for _, vcfg := range fc.Validators {
			pk := vcfg.PubkeyHex
			if pk == "AUTO_FROM_KEYVAULT" || pk == "" {
				pk = n.localPubHex
			}
			w := vcfg.Weight
			if w == 0 {
				w = 1
			}
			n.validators = append(n.validators, finality.Validator{Name: vcfg.Name, PubkeyHex: pk, Weight: w})
		}
	}

	r := reserverpc.New(reserverpc.Options{Bind: n.cfg.RPC.Bind, Port: n.cfg.RPC.Port})

	r.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	r.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if n.ready {
			w.Write([]byte("ready"))
			return
		}
		w.WriteHeader(503)
		w.Write([]byte("not ready"))
	})



	r.HandleFunc("/p2p/peerstat", func(w http.ResponseWriter, req *http.Request) {
		pub := req.URL.Query().Get("pub")
		if pub == "" { w.WriteHeader(400); return }
		ps, _, err := reservestorage.GetPeerStat(n.db.DB, pub)
		if err != nil { w.WriteHeader(500); return }
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(ps)
	})


	r.HandleFunc("/p2p/bans", func(w http.ResponseWriter, _ *http.Request) {
		ps, err := reservestorage.ListPeerStats(n.db.DB, 2000)
		if err != nil { w.WriteHeader(500); return }
		bans := make([]reservestorage.PeerStat, 0, len(ps))
		now := time.Now().Unix()
		for _, s := range ps { if s.BannedUntil > now { bans = append(bans, s) } }
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"count": len(bans), "bans": bans})
	})
	r.HandleFunc("/p2p/unban", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "POST" { w.WriteHeader(405); return }
		var body struct{ Pub string `json:"pub"` }
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil { w.WriteHeader(400); return }
		if body.Pub == "" { w.WriteHeader(400); return }
		_ = reservestorage.UnbanPeer(n.db.DB, body.Pub)
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	})
	r.HandleFunc("/p2p/ban", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "POST" { w.WriteHeader(405); return }
		var body struct{ Pub string `json:"pub"`; Seconds int64 `json:"seconds"` }
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil { w.WriteHeader(400); return }
		if body.Pub == "" { w.WriteHeader(400); return }
		if body.Seconds == 0 { body.Seconds = 600 }
		_ = reservestorage.BanPeer(n.db.DB, body.Pub, body.Seconds)
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	})
	r.HandleFunc("/p2p/sessions", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type","application/json")
		if n.p2p == nil { _ = json.NewEncoder(w).Encode(map[string]any{"count":0,"sessions":[]string{}}); return }
		ss := n.p2p.Sessions()
		_ = json.NewEncoder(w).Encode(map[string]any{"count": len(ss), "sessions": ss})
	})
	r.HandleFunc("/p2p/info", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"enabled": n.cfg.P2P.Enabled, "bind": n.cfg.P2P.Bind, "port": n.cfg.P2P.Port})
	})

	r.HandleFunc("/node/role", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"role": n.cfg.Node.Role, "read_only": n.isReadOnly()})
	})

	r.HandleFunc("/rpc", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" { w.WriteHeader(405); return }
		var req struct{ Method string `json:"method"`; Params map[string]any `json:"params"` }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil { w.WriteHeader(400); return }
		w.Header().Set("Content-Type","application/json")
		switch req.Method {
		case "chain.info":
			_ = json.NewEncoder(w).Encode(fetchChainInfo(n))
		case "mempool.info":
			_ = json.NewEncoder(w).Encode(n.mp.Info())
		case "gossip.info":
			_ = json.NewEncoder(w).Encode(map[string]any{"enabled": n.cfg.Gossip.Enabled, "peers": n.cfg.Gossip.Peers, "max_hops": n.cfg.Gossip.MaxHops})
		case "tx.get":
			txid, _ := req.Params["txid"].(string)
			tx, ok, _ := reservestorage.GetTx(n.db.DB, txid)
			_ = json.NewEncoder(w).Encode(map[string]any{"found": ok, "tx": tx})
		default:
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"error":"unknown_method"})
		}
	})

	r.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		tipH, _, ok, _ := reservestorage.GetTip(n.db.DB)
		if !ok { tipH = 0 }
		mp := n.mp.Info()
		w.Header().Set("Content-Type","text/plain; version=0.0.4")
		w.Write([]byte("reserveos_tip_height " + fmt.Sprintf("%d", tipH) + "\n"))
		hh, _, okH, _ := reservestorage.GetHeaderTip(n.db.DB)
		if !okH { hh = tipH }
		fh, _, _ := reservestorage.GetFinalizedHeight(n.db.DB)
		w.Write([]byte("reserveos_header_tip_height " + fmt.Sprintf("%d", hh) + "\n"))
		w.Write([]byte("reserveos_finalized_height " + fmt.Sprintf("%d", fh) + "\n"))
		pend := 0
		infl := 0
		if n.sm != nil { st := n.sm.snapshot(tipH, 0); pend = st.PendingBlocks; infl = st.InFlightBlocks }
		pe := 0
		if n.p2p != nil { pe = len(n.p2p.Sessions()) }
		w.Write([]byte("reserveos_sync_pending " + fmt.Sprintf("%d", pend) + "\n"))
		w.Write([]byte("reserveos_sync_inflight " + fmt.Sprintf("%d", infl) + "\n"))
		w.Write([]byte("reserveos_p2p_peers " + fmt.Sprintf("%d", pe) + "\n"))
		w.Write([]byte("reserveos_mempool_txs " + fmt.Sprintf("%v", mp["total_txs"]) + "\n"))
	})


	r.HandleFunc("/sync/last_headers", func(w http.ResponseWriter, _ *http.Request) {
		if n.sm == nil { w.WriteHeader(404); return }
		st := n.sm.snapshot(0, 0)
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"count": len(st.LastHeaderHashes), "hashes": st.LastHeaderHashes})
	})




	r.HandleFunc("/sync/candidates", func(w http.ResponseWriter, r *http.Request) {
		hQ := r.URL.Query().Get("height")
		h, _ := strconv.ParseUint(hQ, 10, 64)
		c, err := reservestorage.ListCandidatesAtHeight(n.db.DB, h, 200)
		if err != nil { w.WriteHeader(500); return }
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"height": h, "count": len(c), "candidates": c})
	})





	r.HandleFunc("/sync/retry", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" { w.WriteHeader(405); return }
		if n.sm != nil { _ = n.sm.requeueTimedOut(n.cfg.Sync.InFlightTimeoutSec) }
		_ = n.TryConvergeToBest()
		h, hash, ok, _ := reservestorage.GetTip(n.db.DB)
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "tip_height": h, "tip_hash": hash, "set": ok})
	})
	r.HandleFunc("/sync/converge", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" { w.WriteHeader(405); return }
		if err := n.TryConvergeToBest(); err != nil { w.WriteHeader(400); w.Write([]byte(err.Error())); return }
		h, hash, ok, _ := reservestorage.GetTip(n.db.DB)
		hh, hhash, okH, _ := reservestorage.GetHeaderTip(n.db.DB)
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "block_tip": h, "block_tip_hash": hash, "block_tip_set": ok, "header_tip": hh, "header_tip_hash": hhash, "header_tip_set": okH})
	})

	r.HandleFunc("/sync/reorg_best_check", func(w http.ResponseWriter, r *http.Request) {
		// dry run: checks if blocks for best header chain are present
		curH, _, okC, _ := reservestorage.GetTip(n.db.DB)
		if !okC { curH = 0 }
		hH, hHash, okH, _ := reservestorage.GetHeaderTip(n.db.DB)
		missing := 0
		if okH && hHash != "" && hH > curH {
			path, err := n.headerPathFromTo(curH+1, hH, hHash)
			if err == nil {
				for _, bh := range path {
					var blk chain.Block
					okb, _, _ := reservestorage.GetBlockByHash(n.db.DB, bh, &blk)
					if !okb { missing++ }
				}
			}
		}
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "missing_blocks": missing})
	})
	r.HandleFunc("/sync/reorg_best", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" { w.WriteHeader(405); return }
		if err := n.ReorgToBestHeaderChain(); err != nil { w.WriteHeader(400); w.Write([]byte(err.Error())); return }
		h, hash, ok, _ := reservestorage.GetTip(n.db.DB)
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "tip_height": h, "tip_hash": hash, "set": ok})
	})
	r.HandleFunc("/sync/rebuild", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" { w.WriteHeader(405); return }
		hQ := r.URL.Query().Get("height")
		h, _ := strconv.ParseUint(hQ, 10, 64)
		if err := n.RebuildStateToHeight(h); err != nil { w.WriteHeader(400); w.Write([]byte(err.Error())); return }
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "height": h})
	})

	r.HandleFunc("/sync/commit_until", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" { w.WriteHeader(405); return }
		nQ := r.URL.Query().Get("n")
		nn, _ := strconv.Atoi(nQ)
		if nn <= 0 { nn = 100 }
		for i:=0; i<nn; i++ { n.TryCommitNext() }
		h, hash, ok, _ := reservestorage.GetTip(n.db.DB)
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "tip_height": h, "tip_hash": hash, "set": ok})
	})
	r.HandleFunc("/sync/commit", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" { w.WriteHeader(405); return }
		n.TryCommitNext()
		h, hash, ok, _ := reservestorage.GetTip(n.db.DB)
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"tip_height": h, "tip_hash": hash, "ok": ok})
	})
	r.HandleFunc("/sync/advance", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" { w.WriteHeader(405); return }
		n.TryCommitNext()
		h, hash, ok, _ := reservestorage.GetTip(n.db.DB)
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"tip_height": h, "tip_hash": hash, "ok": ok})
	})
	r.HandleFunc("/sync/reset", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" { w.WriteHeader(405); return }
		if n.sm != nil {
			n.sm.mu.Lock()
			n.sm.pending = []string{}
			n.sm.inflight = map[string]time.Time{}
			n.sm.status.PendingBlocks = 0
			n.sm.status.InFlightBlocks = 0
			n.sm.mu.Unlock()
		}
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	})
	r.HandleFunc("/sync/peers", func(w http.ResponseWriter, _ *http.Request) {
		if n.p2p == nil { w.WriteHeader(200); w.Header().Set("Content-Type","application/json"); _=json.NewEncoder(w).Encode(map[string]any{"count":0,"peers":[]any{}}); return }
		ss := n.p2p.Sessions()
		peers := make([]any, 0, len(ss))
		for _, p := range ss {
			ok := true
			if n.sm != nil { ok = n.sm.isPeerAnchorOK(p) }
			peers = append(peers, map[string]any{"pub": p, "anchor_ok": ok})
		}
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"count": len(ss), "peers": peers})
	})



	r.HandleFunc("/sync/auto_converge", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" { w.WriteHeader(405); return }
		var body struct{ Enabled bool `json:"enabled"` }
		_ = json.NewDecoder(r.Body).Decode(&body)
		n.cfg.Sync.AutoConverge = body.Enabled
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "enabled": n.cfg.Sync.AutoConverge})
	})
	r.HandleFunc("/sync/pause", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" { w.WriteHeader(405); return }
		var body struct{ Paused bool `json:"paused"` }
		_ = json.NewDecoder(r.Body).Decode(&body)
		if n.sm != nil { n.sm.setPaused(body.Paused) }
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "paused": body.Paused})
	})




	r.HandleFunc("/sync/headers_at_height", func(w http.ResponseWriter, r *http.Request) {
		hQ := r.URL.Query().Get("h")
		h, _ := strconv.ParseUint(hQ, 10, 64)
		hashes, err := reservestorage.ListHeaderHashesAtHeight(n.db.DB, h, 200)
		if err != nil { w.WriteHeader(500); return }
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"height": h, "count": len(hashes), "hashes": hashes})
	})
	r.HandleFunc("/sync/plan", func(w http.ResponseWriter, r *http.Request) {
		limitQ := r.URL.Query().Get("n")
		n, _ := strconv.Atoi(limitQ)
		if n <= 0 { n = 50 }
		if n > 500 { n = 500 }
		blockH, _, okB, _ := reservestorage.GetTip(n.db.DB)
		if !okB { blockH = 0 }
		hdrH, hdrHash, okH, _ := reservestorage.GetHeaderTip(n.db.DB)
		if !okH || hdrHash == "" || hdrH <= blockH { 
			w.Header().Set("Content-Type","application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"count":0,"hashes":[]string{}})
			return
		}
		start := blockH + 1
		seq := make([]string,0,n)
		cur := hdrHash
		for len(seq) < n {
			hdr, ok, _ := reservestorage.GetHeaderByHash(n.db.DB, cur)
			if !ok { break }
			if hdr.Height < start { break }
			seq = append(seq, cur)
			if hdr.Height == start { break }
			cur = hdr.PrevHash
		}
		for i, j := 0, len(seq)-1; i < j; i, j = i+1, j-1 { seq[i], seq[j] = seq[j], seq[i] }
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"count": len(seq), "hashes": seq, "start_height": start, "header_tip": hdrH})
	})

	r.HandleFunc("/sync/state_height", func(w http.ResponseWriter, r *http.Request) {
		h, hash, ok, _ := reservestorage.GetTip(n.db.DB)
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"height": h, "hash": hash, "set": ok, "finalized_hash": func() string { fh, okh, _ := reservestorage.GetFinalizedHash(n.db.DB); if okh { return fh }; return "" }()})
	})
	r.HandleFunc("/sync/missing", func(w http.ResponseWriter, r *http.Request) {
		blockH, _, okB, _ := reservestorage.GetTip(n.db.DB)
		if !okB { blockH = 0 }
		hdrH, _, okH, _ := reservestorage.GetHeaderTip(n.db.DB)
		if !okH { hdrH = blockH }
		missing := int64(hdrH) - int64(blockH)
		if missing < 0 { missing = 0 }
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"block_tip": blockH, "header_tip": hdrH, "missing_blocks": missing})
	})


	r.HandleFunc("/sync/cumwork", func(w http.ResponseWriter, r *http.Request) {
		hash := r.URL.Query().Get("hash")
		cw, ok, err := reservestorage.GetCumWork(n.db.DB, hash)
		if err != nil { w.WriteHeader(500); return }
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"hash": hash, "cumwork": cw, "set": ok})
	})
	r.HandleFunc("/sync/best_header_tip", func(w http.ResponseWriter, r *http.Request) {
		h, hash, ok, err := reservestorage.GetHeaderTip(n.db.DB)
		if err != nil { w.WriteHeader(500); return }
		cw := uint64(0)
		if ok && hash != "" { c, okc, _ := reservestorage.GetCumWork(n.db.DB, hash); if okc { cw = c } }
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"height": h, "hash": hash, "cumwork": cw, "set": ok})
	})
	r.HandleFunc("/sync/header_tip", func(w http.ResponseWriter, r *http.Request) {
		h, hash, ok, err := reservestorage.GetHeaderTip(n.db.DB)
		if err != nil { w.WriteHeader(500); return }
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"height": h, "hash": hash, "set": ok, "finalized_hash": func() string { fh, okh, _ := reservestorage.GetFinalizedHash(n.db.DB); if okh { return fh }; return "" }()})
	})
	r.HandleFunc("/sync/frontier", func(w http.ResponseWriter, r *http.Request) {
		h, hash, ok, err := reservestorage.GetHeaderFrontier(n.db.DB)
		if err != nil { w.WriteHeader(500); return }
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"height": h, "hash": hash, "set": ok, "finalized_hash": func() string { fh, okh, _ := reservestorage.GetFinalizedHash(n.db.DB); if okh { return fh }; return "" }()})
	})
	r.HandleFunc("/sync/status", func(w http.ResponseWriter, _ *http.Request) {
		tipH, _, ok, _ := reservestorage.GetTip(n.db.DB)
		if !ok { tipH = 0 }
		peers := 0
		if n.p2p != nil { peers = len(n.p2p.Sessions()) }
		st := SyncStatus{Mode:"headers_first", Peers: peers, TipHeight: tipH}
		if n.sm != nil { st = n.sm.snapshot(tipH, peers) }
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(st)
	})

	r.HandleFunc("/mine/template", func(w http.ResponseWriter, r *http.Request) {
		// build a deterministic candidate set without mining
		txs := n.mp.EligibleSnapshot(n.db.DB)
		txs = n.FilterTxsBySimState(txs)
		ids := make([]string, 0, len(txs))
		for _, tx := range txs { ids = append(ids, tx.ID()) }
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"count": len(ids), "txids": ids})
	})
	r.HandleFunc("/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"service":"core-node","api_version":1}`))
	})


	r.HandleFunc("/chain/locate", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "POST" { w.WriteHeader(405); return }
		var body struct {
			Locator []string `json:"locator"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			w.WriteHeader(400); w.Write([]byte(`{"error":"bad_json"}`)); return
		}
		hh, h, ok, err := n.FindFirstKnown(body.Locator)
		if err != nil { w.WriteHeader(500); w.Write([]byte(`{"error":"db"}`)); return }
		w.Header().Set("Content-Type","application/json")
		if !ok { _ = json.NewEncoder(w).Encode(map[string]any{"found": false}); return }
		_ = json.NewEncoder(w).Encode(map[string]any{"found": true, "hash": hh, "height": h})
	})
	r.HandleFunc("/chain/info", func(w http.ResponseWriter, _ *http.Request) {
		h, tip, ok, err := reservestorage.GetTip(n.db.DB)
		if err != nil {
			w.WriteHeader(500)
			w.Write([]byte(`{"error":"db"}`))
			return
		}
		if !ok {
			w.WriteHeader(503)
			w.Write([]byte(`{"error":"no_tip"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"chain_id":                  n.genesis.ChainID,
			"height":                    h,
			"tip":                       tip,
			"block_time_target_seconds": pow.BlockTimeTargetSeconds,
			"difficulty_window_blocks":  pow.DifficultyWindowBlocks,
		})
	})

	r.HandleFunc("/finality/info", func(w http.ResponseWriter, _ *http.Request) {
		h, hh, ok, err := reservestorage.GetFinalized(n.db.DB)
		if err != nil {
			w.WriteHeader(500)
			w.Write([]byte(`{"error":"db"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if !ok {
			w.Write([]byte(`{"finalized":false}`))
			return
		}
		w.Write([]byte(fmt.Sprintf(`{"finalized":true,"height":%d,"hash":%q}`, h, hh)))
	})


	r.HandleFunc("/pos/register", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "POST" { w.WriteHeader(405); return }
		var body struct{ PubkeyHex string `json:"pubkey_hex"`; Name string `json:"name"`; Weight int64 `json:"weight"` }
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil { w.WriteHeader(400); return }
		if body.PubkeyHex == "" { w.WriteHeader(400); w.Write([]byte(`{"error":"missing_pubkey"}`)); return }
		if body.Weight <= 0 { body.Weight = 1 }
		_ = reservestorage.PutValidator(n.db.DB, reservestorage.Validator{PubkeyHex: body.PubkeyHex, Name: body.Name, Weight: body.Weight})
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	})

	r.HandleFunc("/pos/validators/source", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"validator_source": n.cfg.PoS.ValidatorSource})
	})

	r.HandleFunc("/pos/slashing/status", func(w http.ResponseWriter, _ *http.Request) {
		vals, _ := reservestorage.ListValidators(n.db.DB, 5000)
		var total, slashed int
		for _, v := range vals { total++; if v.Slashed { slashed++ } }
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"validators": total, "slashed": slashed})
	})
	r.HandleFunc("/pos/validators", func(w http.ResponseWriter, _ *http.Request) {
		vals, err := reservestorage.ListValidators(n.db.DB, 5000)
		if err != nil { w.WriteHeader(500); return }
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"count": len(vals), "validators": vals})
	})
	r.HandleFunc("/finality/validators", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"enabled":             n.finalityEnabled,
			"threshold":           fmt.Sprintf("%d/%d", n.thresholdNum, n.thresholdDen),
			"checkpoint_interval": n.checkpointInterval,
			"local_pubkey":        n.localPubHex,
			"validators":          n.validators,
		})
	})

	// Submit a PoS finality vote from a remote validator
	r.HandleFunc("/finality/submit_vote", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		var vr reservestorage.VoteRecord
		if err := json.NewDecoder(req.Body).Decode(&vr); err != nil {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"bad_json"}`))
			return
		}
		if !n.finalityEnabled {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"finality_disabled"}`))
			return
		}
		if vr.ChainID != n.genesis.ChainID {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"wrong_chain"}`))
			return
		}
		// must be a known validator
		var val *finality.Validator
		for i := range n.validators {
			if n.validators[i].PubkeyHex == vr.PubkeyHex {
				val = &n.validators[i]
				break
			}
		}
		if val == nil {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"unknown_validator"}`))
			return
		}
		pub, err := finality.ParsePub(vr.PubkeyHex)
		if err != nil || !finality.VerifyVote(pub, vr.SigHex, vr.ChainID, vr.Height, vr.BlockHash) {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"bad_signature"}`))
			return
		}
		has, err := reservestorage.HasVote(n.db.DB, vr.Height, vr.PubkeyHex)
		if err != nil {
			w.WriteHeader(500)
			w.Write([]byte(`{"error":"db"}`))
			return
		}
		if !has {
			_ = reservestorage.PutVote(n.db.DB, vr)
		// try finalize using weighted quorum
		n.TryFinalizeHeight(vr.Height)
		}
		// Attempt finalize
		_ = n.tryFinalizeHeight(vr.Height, vr.BlockHash)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	})

	// List votes recorded for a checkpoint height




	r.HandleFunc("/finality/anchor", func(w http.ResponseWriter, _ *http.Request) {
		finH, ok, _ := reservestorage.GetFinalizedHeight(n.db.DB)
		var h uint64 = finH
		var hash string
		if ok && finH > 0 {
			var b chain.Block
			okb, _, _ := reservestorage.GetBlockByHeight(n.db.DB, finH, &b)
			if okb { hash = b.Hash().String() }
		}
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"height": h, "hash": hash, "set": ok, "finalized_hash": func() string { fh, okh, _ := reservestorage.GetFinalizedHash(n.db.DB); if okh { return fh }; return "" }()})
	})
	r.HandleFunc("/finality/status", func(w http.ResponseWriter, _ *http.Request) {
		finH, ok, _ := reservestorage.GetFinalizedHeight(n.db.DB)
		cps, _ := reservestorage.ListCheckpoints(n.db.DB, 5000)
		lastCP := uint64(0)
		if len(cps) > 0 { lastCP = cps[len(cps)-1].Height }
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"finalized_height": finH,
			"finalized_set": ok,
			"latest_checkpoint_height": lastCP,
		})
	})
	r.HandleFunc("/finality/finalized", func(w http.ResponseWriter, _ *http.Request) {
		h, ok, err := reservestorage.GetFinalizedHeight(n.db.DB)
		if err != nil { w.WriteHeader(500); return }
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"finalized_height": h, "set": ok, "finalized_hash": func() string { fh, okh, _ := reservestorage.GetFinalizedHash(n.db.DB); if okh { return fh }; return "" }()})
	})

	r.HandleFunc("/finality/tally", func(w http.ResponseWriter, r *http.Request) {
		hQ := r.URL.Query().Get("height")
		h, _ := strconv.ParseUint(hQ, 10, 64)
		votes, err := reservestorage.ListVotes(n.db.DB, h, 5000)
		if err != nil { w.WriteHeader(500); return }
		vals, _ := reservestorage.ListValidators(n.db.DB, 5000)
		wBy := map[string]int64{}
		var totalW int64
		for _, v := range vals { if v.Slashed || v.Weight<=0 { continue }; wBy[v.PubkeyHex]=v.Weight; totalW += v.Weight }
		tally := map[string]int64{}
		for _, vr := range votes { wv := wBy[vr.PubkeyHex]; if wv>0 { tally[vr.BlockHash]+=wv } }
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"height": h, "checkpoint_id": fmt.Sprintf("%d:%s", h, func() string { _, hh, ok, _ := reservestorage.GetHeaderTip(n.db.DB); if ok { return hh }; return "" }()), "total_weight": totalW, "tally": tally, "votes": len(votes)})
	})
	r.HandleFunc("/finality/checkpoints", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "GET" { w.WriteHeader(405); return }
		fromQ := req.URL.Query().Get("from")
		countQ := req.URL.Query().Get("count")
		from, err := strconv.ParseUint(fromQ, 10, 64)
		if err != nil { from = 0 }
		count, err := strconv.Atoi(countQ)
		if err != nil || count <= 0 { count = 50 }
		if count > 200 { count = 200 }
		cps, err := reservestorage.ListCheckpoints(n.db.DB, 5000)
		if err != nil { w.WriteHeader(500); return }
		if from > 0 {
			flt := make([]finality.Checkpoint, 0, len(cps))
			for _, cp := range cps { if cp.Height >= from { flt = append(flt, cp) } }
			cps = flt
		}
		if len(cps) > count { cps = cps[:count] }
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"from": from, "count": len(cps), "checkpoints": cps})
		return

		// legacy
		for i := 0; i < count; i++ {
			h := from + uint64(i)*n.checkpointInterval
			if h == 0 { continue }
			// checkpoint is stored under key finality:checkpoint:<height>, but we stored via PutCheckpoint (JSON) in DB
			var cp any
			ok, err := reservestorage.GetBlockByHash(n.db.DB, "", &cp) // placeholder
			_ = ok; _ = err
			// We don't have direct get; just return height list for now
			out = append(out, map[string]any{"height": h})
		}
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"from": from, "count": len(out), "checkpoints": out})
	})
	r.HandleFunc("/finality/votes", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "GET" {
			w.WriteHeader(405)
			return
		}
		q := req.URL.Query().Get("height")
		h64, err := strconv.ParseUint(q, 10, 64)
		if err != nil {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"bad_height"}`))
			return
		}
		votes, err := reservestorage.ListVotesForHeight(n.db.DB, h64)
		if err != nil {
			w.WriteHeader(500)
			w.Write([]byte(`{"error":"db"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"height": h64, "votes": votes})
	})




	r.HandleFunc("/state/nonce", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "GET" { w.WriteHeader(405); return }
		id := req.URL.Query().Get("id")
		if id == "" { w.WriteHeader(400); w.Write([]byte(`{"error":"missing id"}`)); return }
		n, _, _ := state.GetNonce(n.db.DB, id)
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "nonce": n})
	})

	r.HandleFunc("/state/info", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "GET" { w.WriteHeader(405); return }
		id := req.URL.Query().Get("id")
		if id == "" { w.WriteHeader(400); w.Write([]byte(`{"error":"missing id"}`)); return }
		bal, _, _ := state.GetBalance(n.db.DB, id)
		nn, _, _ := state.GetNonce(n.db.DB, id)
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "balance": bal, "nonce": nn})
	})


	r.HandleFunc("/state/snapshot/info", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Query().Get("path")
		if path == "" { w.WriteHeader(400); return }
		ss, err := state.ReadSnapshotAuto(path)
		if err != nil { w.WriteHeader(500); return }
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"version": ss.Version,
			"height": ss.Height,
			"tip_hash": ss.TipHash,
			"created_unix": ss.CreatedUnix,
			"balances": len(ss.Balances),
			"nonces": len(ss.Nonces),
		})
	})
	r.HandleFunc("/state/snapshots", func(w http.ResponseWriter, _ *http.Request) {
		paths, err := state.ListSnapshots(n.cfg.State.SnapshotDir)
		if err != nil { w.WriteHeader(500); return }
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"count": len(paths), "paths": paths})
	})

	r.HandleFunc("/state/snapshot/trigger", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "POST" { w.WriteHeader(405); return }
		h, tip, ok, err := reservestorage.GetTip(n.db.DB)
		_ = ok
		if err != nil { w.WriteHeader(500); return }
		var path string
		var err error
		if n.cfg.State.SnapshotFormat == "bin" { path, err = func() (string,error){ return state.WriteSnapshotBin(n.db.DB, n.cfg.State.SnapshotDir, h, tip) }() } else { path, err = state.WriteSnapshot(n.db.DB, n.cfg.State.SnapshotDir, h, tip) }
		if err != nil { w.WriteHeader(500); return }
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "path": path, "height": h})
	})
	r.HandleFunc("/state/balance", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "GET" { w.WriteHeader(405); return }
		id := req.URL.Query().Get("id")
		if id == "" { w.WriteHeader(400); w.Write([]byte(`{"error":"missing id"}`)); return }
		bal, _, _ := state.GetBalance(n.db.DB, id)
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "balance": bal})
	})


	r.HandleFunc("/otap/bucket", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "GET" { w.WriteHeader(405); return }
		p := req.URL.Query().Get("p")
		if p == "" { w.WriteHeader(400); w.Write([]byte(`{"error":"missing p"}`)); return }
		bal, _, _ := state.GetBalance(n.db.DB, "otap:"+p)
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"p": p, "bucket_balance": bal})
	})
	r.HandleFunc("/pop/register", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "POST" { w.WriteHeader(405); return }
		var body struct{ ID string `json:"id"`; Weight int64 `json:"weight"` }
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil { w.WriteHeader(400); return }
		if body.ID == "" { w.WriteHeader(400); w.Write([]byte(`{"error":"missing id"}`)); return }
		if body.Weight <= 0 { body.Weight = 1 }
		p := reservestorage.Participant{ID: body.ID, Weight: body.Weight, AddedAt: time.Now().Unix()}
		_ = reservestorage.PutParticipant(n.db.DB, p)
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	})



	r.HandleFunc("/pop/event", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "POST" { w.WriteHeader(405); return }
		var body struct{ ID string `json:"id"`; Type string `json:"type"`; Delta uint64 `json:"delta"` }
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil { w.WriteHeader(400); return }
		if body.ID == "" { w.WriteHeader(400); w.Write([]byte(`{"error":"missing id"}`)); return }
		if body.Type == "" { body.Type = "heartbeat" }
		if body.Delta == 0 { body.Delta = 1 }
		mul := uint64(1)
		switch body.Type {
		case "relay": mul = 5
		case "storage": mul = 3
		case "uptime": mul = 2
		default: mul = 1
		}
		_ = reservestorage.PutPoPEvent(n.db.DB, reservestorage.PoPEvent{ID: body.ID, Type: body.Type, Delta: body.Delta})
		_ = reservestorage.AddScore(n.db.DB, body.ID, body.Delta*mul)
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "applied": body.Delta*mul})
	})
	r.HandleFunc("/pop/heartbeat", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "POST" { w.WriteHeader(405); return }
		var body struct{ ID string `json:"id"`; Delta uint64 `json:"delta"` }
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil { w.WriteHeader(400); return }
		if body.ID == "" { w.WriteHeader(400); w.Write([]byte(`{"error":"missing id"}`)); return }
		if body.Delta == 0 { body.Delta = 1 }
		_ = reservestorage.AddScore(n.db.DB, body.ID, body.Delta)
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	})

	r.HandleFunc("/pop/score", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "GET" { w.WriteHeader(405); return }
		id := req.URL.Query().Get("id")
		if id == "" { w.WriteHeader(400); return }
		sc, _ := reservestorage.GetScore(n.db.DB, id)
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "score": sc})
	})
	r.HandleFunc("/pop/participants", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "GET" { w.WriteHeader(405); return }
		parts, err := reservestorage.ListParticipants(n.db.DB, 5000)
		if err != nil { w.WriteHeader(500); return }
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"count": len(parts), "participants": parts})
	})



	r.HandleFunc("/peers/add", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "POST" { w.WriteHeader(405); return }
		var body struct{ Addr string `json:"addr"` }
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil { w.WriteHeader(400); return }
		if body.Addr == "" { w.WriteHeader(400); return }
		_ = reservestorage.PutPeer(n.db.DB, reservestorage.Peer{Addr: body.Addr, AddedUnix: time.Now().Unix()})
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	})
	r.HandleFunc("/peers/list", func(w http.ResponseWriter, _ *http.Request) {
		ps, err := reservestorage.ListPeers(n.db.DB, 5000)
		if err != nil { w.WriteHeader(500); return }
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"count": len(ps), "peers": ps})
	})
	r.HandleFunc("/gossip/info", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"enabled": n.cfg.Gossip.Enabled, "peers": n.cfg.Gossip.Peers, "max_hops": n.cfg.Gossip.MaxHops})
	})


	r.HandleFunc("/gossip/block", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "POST" { w.WriteHeader(405); return }
		var msg GossipBlockMsg
		if err := json.NewDecoder(req.Body).Decode(&msg); err != nil { w.WriteHeader(400); return }
		b := msg.Block
		// accept block (fork-safe) and forward
		if err := n.SubmitBlock(&b); err != nil { w.WriteHeader(400); w.Write([]byte(`{"error":"rejected"}`)); return }
		n.gossipBlock(b, msg.Hash, msg.Hop)
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	})
	r.HandleFunc("/gossip/tx", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "POST" { w.WriteHeader(405); return }
		var msg GossipTxMsg
		if err := json.NewDecoder(req.Body).Decode(&msg); err != nil { w.WriteHeader(400); return }
		tx := msg.Tx
		if tx.Type == "" { tx.Type = "transfer" }
		if err := n.validateTxAdmission(&tx); err != nil { w.WriteHeader(400); w.Write([]byte(`{"error":"rejected"}`)); return }
		if _, err := n.mp.Add(n.db.DB, tx); err != nil { w.WriteHeader(400); w.Write([]byte(`{"error":"mempool"}`)); return }
		n.gossipTx(tx, msg.Hop)
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	})

	r.HandleFunc("/mempool/eligible", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type","application/json")
		txs := n.mp.EligibleSnapshot(n.db.DB)
		_ = json.NewEncoder(w).Encode(map[string]any{"count": len(txs), "txs": txs})
	})

	r.HandleFunc("/mempool/queue", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type","application/json")
		q := n.mp.QueueSnapshot()
		_ = json.NewEncoder(w).Encode(map[string]any{"accounts": len(q), "queue": q})
	})

	r.HandleFunc("/mempool/persisted", func(w http.ResponseWriter, r *http.Request) {
		items, err := reservestorage.ListPersistedMempool(n.db.DB, 1000)
		if err != nil { w.WriteHeader(500); return }
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"count": len(items), "items": items})
	})
	r.HandleFunc("/mempool/prune", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" { w.WriteHeader(405); return }
		// reload path prunes expired automatically
		_, _ = reservestorage.LoadMempoolTxs(n.db.DB, 5000)
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	})
	r.HandleFunc("/mempool/info", func(w http.ResponseWriter, _ *http.Request) {
		// lightweight view
		w.Header().Set("Content-Type","application/json")
		info := n.mp.Info()
		_ = json.NewEncoder(w).Encode(info)
	})

	r.HandleFunc("/fees/reconcile", func(w http.ResponseWriter, r *http.Request) {
		from, _ := strconv.ParseUint(r.URL.Query().Get("from"), 10, 64)
		to, _ := strconv.ParseUint(r.URL.Query().Get("to"), 10, 64)
		if to == 0 { th, _, ok, _ := reservestorage.GetTip(n.db.DB); if ok { to = th } }
		if to < from { w.WriteHeader(400); return }
		var blocks int
		var total int64
		for h := from; h <= to; h++ {
			s, ok := reservestorage.GetBlockFeeSum(n.db.DB, h)
			if ok { total += s; blocks++ }
		}
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"from": from, "to": to, "blocks": blocks, "fee_sum_total": total})
	})
	r.HandleFunc("/fees/info", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type","application/json")
		pools := reservestorage.GetFeePools(n.db.DB)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"gas_asset": n.cfg.Fees.GasAsset,
			"mode": n.cfg.Fees.Mode,
			"base_fee_min": n.cfg.Fees.BaseFeeMin,
			"per_byte": n.cfg.Fees.PerByte,
			"otap_multiplier": n.cfg.Fees.OTAPMultiplier,
			"pools": pools,
		})
	})

	r.HandleFunc("/tx/get", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "GET" { w.WriteHeader(405); return }
		txid := req.URL.Query().Get("txid")
		if txid == "" { w.WriteHeader(400); return }
		tx, ok, err := reservestorage.GetTx(n.db.DB, txid)
		if err != nil { w.WriteHeader(500); return }
		w.Header().Set("Content-Type","application/json")
		if !ok { _ = json.NewEncoder(w).Encode(map[string]any{"found": false}); return }
		_ = json.NewEncoder(w).Encode(map[string]any{"found": true, "tx": tx})
	})
	r.HandleFunc("/tx/submit", func(w http.ResponseWriter, req *http.Request) {
		if n.isReadOnly() { w.WriteHeader(403); w.Write([]byte(`{"error":"read_only"}`)); return }
		if !n.cfg.Node.AllowPublicSubmit { w.WriteHeader(403); w.Write([]byte(`{"error":"submit_disabled"}`)); return }
		if req.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		var tx chain.Tx
		if err := json.NewDecoder(req.Body).Decode(&tx); err != nil {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"bad_json"}`))
			return
		}
		if len(tx.Outputs) == 0 {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"no_outputs"}`))
			return
		}
		for i := range tx.Outputs {
			o := &tx.Outputs[i]
			if o.Asset == "" {
				o.Asset = "USDR"
			}
			if o.Amount <= 0 {
				w.WriteHeader(400)
				w.Write([]byte(`{"error":"bad_amount"}`))
				return
			}
			if o.Address == "" {
				if o.P == "" || o.R == "" {
					w.WriteHeader(400)
					w.Write([]byte(`{"error":"missing_destination"}`))
					return
				}
			}
		}
		// Fee policy (USDR gas): require tx.fee >= estimate based on size hint and OTAP flag.
		isOTAP := false
		for _, o := range tx.Outputs {
			if o.P != "" && o.R != "" { isOTAP = true; break }
		}
		if tx.GasAsset == "" { tx.GasAsset = n.cfg.Fees.GasAsset }
		if tx.GasAsset != n.cfg.Fees.GasAsset {
			w.WriteHeader(400); w.Write([]byte(`{"error":"bad_gas_asset"}`)); return
		}
		sz := fees.TxSizeHint(tx.ID(), len(tx.Outputs))
		est := fees.EstimateFee(sz, isOTAP, fees.Params{GasAsset: n.cfg.Fees.GasAsset, BaseFeeMin: n.cfg.Fees.BaseFeeMin, PerByte: n.cfg.Fees.PerByte, OTAPMultiplier: n.cfg.Fees.OTAPMultiplier, Mode: fees.Mode(n.cfg.Fees.Mode)})
		if tx.Fee < est {
			w.WriteHeader(400); w.Write([]byte(fmt.Sprintf(`{"error":"insufficient_fee","need":%d,"have":%d}`, est, tx.Fee))); return
		}

		// Account tx validation (v1): require signature and nonce ordering for From
		if tx.Type == "" { tx.Type = "transfer" }
		if tx.From == "" || tx.PubKey == "" || tx.SigHex == "" {
			w.WriteHeader(400); w.Write([]byte(`{"error":"missing_from_or_sig"}`)); return
		}
		if tx.From != tx.PubKey {
			w.WriteHeader(400); w.Write([]byte(`{"error":"from_pubkey_mismatch"}`)); return
		}
		if !sig.Verify(tx.PubKey, tx.SigHex, tx.SigningBytes()) {
			w.WriteHeader(400); w.Write([]byte(`{"error":"bad_signature"}`)); return
		}
curNonce, _, _ := state.GetNonce(n.db.DB, tx.From)
		if tx.Nonce != curNonce+1 {
			w.WriteHeader(400); w.Write([]byte(fmt.Sprintf(`{"error":"bad_nonce","need":%d,"have":%d}`, curNonce+1, tx.Nonce))); return
		}
		// balance check: require From balance >= outputs_total + fee
		var outTotal int64
		for _, o := range tx.Outputs { outTotal += o.Amount }
		bal, _, _ := state.GetBalance(n.db.DB, tx.From)
		if bal < outTotal+tx.Fee {
			w.WriteHeader(400); w.Write([]byte(fmt.Sprintf(`{"error":"insufficient_funds","need":%d,"have":%d}`, outTotal+tx.Fee, bal))); return
		}

		// OTAP claim validation: claims are paid from otap:<P> bucket, not From balance.
		if tx.Type == "otap_claim" {
			if tx.OTAPClaim == nil || tx.OTAPClaim.P == "" || tx.OTAPClaim.R == "" || tx.OTAPClaim.To == "" {
				w.WriteHeader(400); w.Write([]byte(`{"error":"bad_otap_claim"}`)); return
			}
			if tx.OTAPClaim.To != tx.From {
				w.WriteHeader(400); w.Write([]byte(`{"error":"claim_to_must_equal_from"}`)); return
			}
			// require claim bucket has sufficient funds
			bucket := "otap:" + tx.OTAPClaim.P
			bBal, _, _ := state.GetBalance(n.db.DB, bucket)
			if bBal < tx.OTAPClaim.Amount {
				w.WriteHeader(400); w.Write([]byte(fmt.Sprintf(`{"error":"insufficient_otap_bucket","need":%d,"have":%d}`, tx.OTAPClaim.Amount, bBal))); return
			}
			if tx.OTAPClaim.ClaimSigHex == "" {
				w.WriteHeader(400); w.Write([]byte(`{"error":"missing_claim_sig"}`)); return
			}
			// Verify claim signature with one-time pubkey P
			Ppt := new(edwards25519.Point)
			if _, err := Ppt.SetBytes(mustHex32(tx.OTAPClaim.P)); err != nil { w.WriteHeader(400); w.Write([]byte(`{"error":"bad_claim_P"}`)); return }
			if !schnorr.Verify(Ppt, tx.SigningBytes(), tx.OTAPClaim.ClaimSigHex) {
				w.WriteHeader(400); w.Write([]byte(`{"error":"bad_claim_signature"}`)); return
			}
			// override outTotal requirement: claim tx has no Outputs; amount is in OTAPClaim.Amount
			// still must pay fee from sender balance
			 // fee is paid from the same OTAP bucket in v1 so first-time receivers can claim.
			if bBal < tx.OTAPClaim.Amount+tx.Fee {
				w.WriteHeader(400); w.Write([]byte(fmt.Sprintf(`{"error":"insufficient_otap_bucket_for_fee","need":%d,"have":%d}`, tx.OTAPClaim.Amount+tx.Fee, bBal))); return
			}
		}

		if _, err := n.mp.Add(n.db.DB, tx); err != nil { w.WriteHeader(400); w.Write([]byte(fmt.Sprintf(`{"error":"%s"}`, err.Error()))); return }
		// broadcast to gossip peers
		 _ = reservestorage.PutTx(n.db.DB, tx)
		n.gossipTx(tx, 0)
		n.p2pBroadcastTx(tx)
		reservestorage.PutMempoolTx(n.db.DB, tx, 3600)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(fmt.Sprintf(`{"ok":true,"txid":%q}`, tx.ID())))
	})

	r.HandleFunc("/chain/mine_one", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		hashHex, nonce, err := n.MineOne()
		if err != nil {
			w.WriteHeader(400)
			w.Write([]byte(fmt.Sprintf(`{"error":%q}`, err.Error())))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(fmt.Sprintf(`{"ok":true,"hash":%q,"nonce":%d}`, hashHex, nonce)))
	})


	r.HandleFunc("/chain/block_by_hash", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "GET" { w.WriteHeader(405); return }
		h := req.URL.Query().Get("hash")
		if h == "" { w.WriteHeader(400); w.Write([]byte(`{"error":"missing hash"}`)); return }
		var b chain.Block
		ok, err := reservestorage.GetBlockByHash(n.db.DB, h, &b)
		if err != nil || !ok { w.WriteHeader(404); w.Write([]byte(`{"error":"not found"}`)); return }
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"hash": h, "block": b})
	})

	r.HandleFunc("/chain/header_by_hash", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "GET" { w.WriteHeader(405); return }
		h := req.URL.Query().Get("hash")
		if h == "" { w.WriteHeader(400); w.Write([]byte(`{"error":"missing hash"}`)); return }
		var b chain.Block
		ok, err := reservestorage.GetBlockByHash(n.db.DB, h, &b)
		if err != nil || !ok { w.WriteHeader(404); w.Write([]byte(`{"error":"not found"}`)); return }
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"hash": h, "header": b.Header})
	})

	r.HandleFunc("/chain/headers", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "GET" { w.WriteHeader(405); return }
		fromQ := req.URL.Query().Get("from")
		countQ := req.URL.Query().Get("count")
		from, err := strconv.ParseUint(fromQ, 10, 64)
		if err != nil { w.WriteHeader(400); w.Write([]byte(`{"error":"bad from"}`)); return }
		count, err := strconv.Atoi(countQ)
		if err != nil || count <= 0 { w.WriteHeader(400); w.Write([]byte(`{"error":"bad count"}`)); return }
		if count > 2000 { count = 2000 }
		headers := make([]chain.BlockHeader, 0, count)
		for i := 0; i < count; i++ {
			var b chain.Block
			ok, _, err := reservestorage.GetBlockByHeight(n.db.DB, from+uint64(i), &b)
			if err != nil || !ok { break }
			headers = append(headers, b.Header)
		}
		w.Header().Set("Content-Type","application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"from": from, "count": len(headers), "headers": headers})
	})
	r.HandleFunc("/chain/block", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "GET" {
			w.WriteHeader(405)
			return
		}
		q := req.URL.Query().Get("height")
		if q == "" {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"missing height"}`))
			return
		}
		h64, err := strconv.ParseUint(q, 10, 64)
		if err != nil {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"bad height"}`))
			return
		}
		var b chain.Block
		ok, hashHex, err := reservestorage.GetBlockByHeight(n.db.DB, h64, &b)
		if err != nil || !ok {
			w.WriteHeader(404)
			w.Write([]byte(`{"error":"not found"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"height": h64, "hash": hashHex, "block": b})
	})

	if err := r.Start(); err != nil {
		return fmt.Errorf("rpc: %w", err)
	}
	n.rpc = r
	if n.cfg.P2P.Enabled {
		p2pn, _ := reservep2p.New(reservep2p.Config{Bind: n.cfg.P2P.Bind, Port: n.cfg.P2P.Port, KeyvaultPath: n.cfg.Keyvault.Path, KeyvaultEnv: n.cfg.Keyvault.KEKEnv})
		n.p2p = p2pn
		if n.p2p != nil { _ = n.p2p.Start(); n.startP2PGossip() }
	n.startSyncManager()
	}
	n.ready = true
	if n.finalityEnabled {
		n.startFinalityGossip()
	}
	if n.cfg.Sync.Enabled {
		n.startSyncLoop()
	}
	return nil
}

func (n *Node) Stop(ctx context.Context) error {
	n.ready = false
	if n.rpc != nil {
		_ = n.rpc.Stop(ctx)
	}
	if n.db != nil {
		_ = n.db.Close()
	}
	return nil
}

func (n *Node) ensureGenesisStored() error {
	has, err := reservestorage.HasGenesis(n.db.DB)
	if err != nil {
		return fmt.Errorf("genesis-check: %w", err)
	}

	gb := chain.GenesisBlock(n.genesis)
	gh := gb.Header.Hash().String()

	if !has {
		if err := reservestorage.PutGenesis(n.db.DB, n.genesis); err != nil {
			return fmt.Errorf("genesis-store: %w", err)
		}
		if err := reservestorage.PutBlock(n.db.DB, gh, 0, gb); err != nil {
			return fmt.Errorf("genesis-block-store: %w", err)
		}
		if err := reservestorage.PutTip(n.db.DB, 0, []byte(gh)); err != nil {
			return fmt.Errorf("tip-store: %w", err)
		}
		_ = reservestorage.PutFinalized(n.db.DB, 0, gh)
		_ = reservestorage.PutWork(n.db.DB, gh, pow.Work64(gb.Header.Bits))
		_ = reservestorage.PutTipWork(n.db.DB, pow.Work64(gb.Header.Bits))
		return nil
	}

	var existing chain.Genesis
	ok, err := reservestorage.GetGenesis(n.db.DB, &existing)
	if err != nil || !ok {
		return fmt.Errorf("genesis-load-stored: %w", err)
	}
	if existing.ChainID != n.genesis.ChainID {
		return fmt.Errorf("genesis mismatch: db=%s file=%s", existing.ChainID, n.genesis.ChainID)
	}
	return nil
}

func (n *Node) SubmitBlock(b *chain.Block) error {
	if b.Header.ChainID != n.genesis.ChainID {
		return errors.New("wrong chain_id")
	}
	if err := n.ValidateLimits(b); err != nil { return err }
	if err := n.ValidateBlockStateOnMainParent(b); err != nil { return err }
	if err := n.ValidateBlockBasic(b); err != nil { return err }

	// Validate PoW first
	h := b.Header.Hash()
	if !pow.MeetsTarget(h, b.Header.Bits) {
		return errors.New("pow does not meet target")
	}
	hashHex := h.String()

	// Parent validation (genesis has height 0 and zero prev)
	if b.Header.Height == 0 {
		// genesis already handled elsewhere
		return nil
	}
	prevHex := b.Header.PrevHash.String()

	// Ensure parent exists
	var prevB chain.Block
	okPrev, err := reservestorage.GetBlockByHash(n.db.DB, prevHex, &prevB)
	if err != nil || !okPrev {
		return errors.New("missing parent")
	}
	if prevB.Header.Height+1 != b.Header.Height {
		return errors.New("bad height linkage")
	}

	// Difficulty rule for this height must match what we'd compute on the MAIN chain;
	// for v1 demo we enforce it using the best chain view (height index).
	getHeader := func(height uint64) (*chain.BlockHeader, bool) {
		var bb chain.Block
		ok3, _, _ := reservestorage.GetBlockByHeight(n.db.DB, height, &bb)
		if !ok3 {
			return nil, false
		}
		return &bb.Header, true
	}
	getBits := func(height uint64) (uint32, bool) {
		hdr, ok := getHeader(height)
		if !ok {
			return 0, false
		}
		return hdr.Bits, true
	}
	expectedBits, ok2 := pow.NextWorkRequired(getHeader, getBits, b.Header.Height)
	if ok2 && b.Header.Bits != expectedBits {
		return fmt.Errorf("bad bits: got %08x want %08x", b.Header.Bits, expectedBits)
	}

	// Store block by hash (always)
	if err := reservestorage.PutBlock(n.db.DB, hashHex, b.Header.Height, b); err != nil {
		return err
	}
	_ = reservestorage.PutHeader(n.db.DB, hashHex, b.Header)

	// Compute cumulative work for this block
	parentWork, _, err := reservestorage.GetWork(n.db.DB, prevHex)
	if err != nil {
		return err
	}
	wAdd := pow.Work64(b.Header.Bits)
	cum := pow.Cumulative(parentWork, wAdd)
	_ = reservestorage.PutWork(n.db.DB, hashHex, cum)

	// If this block extends our current tip on main chain, advance quickly
	tipH, tipHash, okTip, _ := reservestorage.GetTip(n.db.DB)
	if okTip && prevHex == tipHash && b.Header.Height == tipH+1 {
		if err := n.ValidateBlockStateTip(b); err != nil { return err }

		_ = reservestorage.PutTip(n.db.DB, b.Header.Height, []byte(hashHex))
		_ = reservestorage.PutTipWork(n.db.DB, cum)
		_ = reservestorage.PutHashByHeightOverwrite(n.db.DB, b.Header.Height, hashHex)
		// checkpoint processing
		if n.finalityEnabled && b.Header.Height%n.checkpointInterval == 0 {
			cp := finality.Checkpoint{Height: b.Header.Height, HashHex: hashHex, TimeUnix: b.Header.TimeUnix}
			_ = reservestorage.PutCheckpoint(n.db.DB, b.Header.Height, cp)
			_ = n.processCheckpointVotes(cp)
		}
		return nil
	}

	// Otherwise, fork-choice: if cumulative work beats current tip AND chain is anchored at finalized, switch main tip.
	curWork, _, _ := reservestorage.GetTipWork(n.db.DB)
	if cum > curWork {
		if err := n.trySwitchMainChain(hashHex, b.Header.Height); err == nil {
			_ = reservestorage.PutTipWork(n.db.DB, cum)
		}
	}
	return nil
}

func (n *Node) isLocalValidator() bool {
	for _, v := range n.validators { if v.PubkeyHex == n.localPubHex { return true } }
	return false
}

func (n *Node) processCheckpointVotes(cp finality.Checkpoint) error {
	if n.cfg.Node.Role == "observer" || n.isReadOnly() { return nil }
	if !n.isLocalValidator() { return nil }
	priv, err := finality.ParsePriv(n.localPrivHex)
	if err != nil {
		return err
	}

	has, err := reservestorage.HasVote(n.db.DB, cp.Height, n.localPubHex)
	if err != nil {
		return err
	}
	if !has {
		sigHex := finality.SignVote(priv, n.genesis.ChainID, cp.Height, cp.HashHex)
		_ = reservestorage.PutVote(n.db.DB, reservestorage.VoteRecord{
			Height:    cp.Height,
			BlockHash: cp.HashHex,
			ChainID:   n.genesis.ChainID,
			PubkeyHex: n.localPubHex,
			SigHex:    sigHex,
			TimeUnix:  time.Now().Unix(),
		})
	}
	return n.tryFinalizeHeight(cp.Height, cp.HashHex)
}

func (n *Node) tryFinalizeHeight(height uint64, hashHex string) error {
	votes, err := reservestorage.ListVotesForHeight(n.db.DB, height)
	if err != nil { return err }

	// Prefer on-disk validator registry (non-slashed). Fallback to compiled/config validators.
	valsReg, err2 := reservestorage.ListValidators(n.db.DB, 5000)
	useReg := (err2 == nil && len(valsReg) > 0)

	total := int64(0)
	signed := int64(0)

	for _, vr := range votes {
		if vr.BlockHash != hashHex || vr.ChainID != n.genesis.ChainID { continue }
		if useReg {
			for _, v := range valsReg {
				if v.Slashed { continue }
				total += v.Weight
				if v.PubkeyHex != vr.PubkeyHex { continue }
				pub, err := finality.ParsePub(v.PubkeyHex)
				if err != nil { continue }
				if finality.VerifyVote(pub, vr.SigHex, n.genesis.ChainID, height, hashHex) {
					signed += v.Weight
				}
			}
		} else {
			total = finality.TotalWeight(n.validators)
			for _, v := range n.validators {
				if v.PubkeyHex != vr.PubkeyHex { continue }
				pub, err := finality.ParsePub(v.PubkeyHex)
				if err != nil { continue }
				if finality.VerifyVote(pub, vr.SigHex, n.genesis.ChainID, height, hashHex) {
					signed += v.Weight
				}
			}
		}
	}

	if total == 0 { return nil }
	if finality.ReachedThreshold(signed, total, n.thresholdNum, n.thresholdDen) {
		_ = reservestorage.PutFinalized(n.db.DB, height, hashHex)
		// store checkpoint record for observability
		_ = reservestorage.PutCheckpoint(n.db.DB, height, finality.Checkpoint{Height: height, HashHex: hashHex, TimeUnix: time.Now().Unix()})
	}
	return nil
}

func (n *Node) BuildNextBlockHeader(txs []chain.Tx) (*chain.BlockHeader, error) {
	tipH, tipHash, ok, err := reservestorage.GetTip(n.db.DB)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("no tip")
	}

	var tipBlock chain.Block
	ok2, err := reservestorage.GetBlockByHash(n.db.DB, tipHash, &tipBlock)
	if err != nil || !ok2 {
		return nil, errors.New("missing tip block")
	}

	nextHeight := tipH + 1

	getHeader := func(height uint64) (*chain.BlockHeader, bool) {
		var bb chain.Block
		ok3, _, _ := reservestorage.GetBlockByHeight(n.db.DB, height, &bb)
		if !ok3 {
			return nil, false
		}
		return &bb.Header, true
	}
	getBits := func(height uint64) (uint32, bool) {
		hdr, ok := getHeader(height)
		if !ok {
			return 0, false
		}
		return hdr.Bits, true
	}

	expectedBits, ok4 := pow.NextWorkRequired(getHeader, getBits, nextHeight)
	if !ok4 {
		return nil, errors.New("cannot compute expected difficulty")
	}

	h := &chain.BlockHeader{
		Version:   uint32(n.genesis.ProtocolVersion),
		ChainID:   n.genesis.ChainID,
		Height:    nextHeight,
		TimeUnix:  time.Now().Unix(),
		PrevHash:  tipBlock.Header.Hash(),
		StateRoot: tipBlock.Header.StateRoot,
		TxRoot:    chain.TxRoot(txs),
		Bits:      expectedBits,
		Nonce:     0,
	}
	return h, nil
}

func (n *Node) MineOne() (string, uint64, error) {
	if n.isReadOnly() || n.cfg.Node.Role == "observer" { return "", 0, fmt.Errorf("mining_disabled") }

	txs := n.mp.Drain(n.db.DB, 500)
	// block assembly correctness: filter by simulated state application
	txs = n.FilterTxsBySimState(txs)
	for _, tx := range txs { reservestorage.DelMempoolTx(n.db.DB, tx.ID()) }
	// coinbase issuance (v1)
	if n.cfg.Issuance.Enabled {
		cb := chain.Tx{Type:"coinbase", From:"", PubKey:"", SigHex:"", Nonce:0, Fee:0, GasAsset:n.cfg.Fees.GasAsset,
			Outputs: []chain.TxOutput{{Amount:n.cfg.Issuance.BlockReward, Asset:n.cfg.Issuance.RewardAsset, Address:n.cfg.Issuance.CoinbaseTo}},
		}
		txs = append([]chain.Tx{cb}, txs...)
	}

	h, err := n.BuildNextBlockHeader(txs)
	if err != nil {
		return "", 0, err
	}
	hash, nonce := pow.MineHeader(h)
	b := &chain.Block{Header: *h, Txs: txs}
	if err := n.SubmitBlock(b); err != nil {
		return "", 0, err
	}
	return hash.String(), nonce, nil
}


func (n *Node) isReadOnly() bool {
	if n.cfg.Node.ReadOnly { return true }
	if n.cfg.Node.Role == "observer" { return true }
	return false
}


func fetchChainInfo(n *Node) map[string]any {
	tipH, tipHash, ok, _ := reservestorage.GetTip(n.db.DB)
	if !ok { tipH = 0; tipHash = "" }
	return map[string]any{"tip_height": tipH, "tip_hash": tipHash}
}


func (n *Node) enforceFinalizedOnCandidate(candidateTipHash string) error {
	finH, ok, _ := reservestorage.GetFinalizedHeight(n.db.DB)
	if !ok || finH == 0 { return nil }
	// main chain hash at finH
	var mainB chain.Block
	okb, _, _ := reservestorage.GetBlockByHeight(n.db.DB, finH, &mainB)
	if !okb { return nil }
	mainHash := mainB.Hash().String()
	candHash, okc, _ := reservestorage.GetAncestorHashAtHeight(n.db.DB, candidateTipHash, finH)
	if !okc { return nil }
	if candHash != mainHash { return fmt.Errorf("violates_finalized") }
	return nil
}


func (n *Node) chooseSyncPeer() (string, bool) {
	if n.p2p == nil { return "", false }
	ss := n.p2p.Sessions()
	if len(ss) == 0 { return "", false }
	// filter by anchorOK if finalized set
	finH, ok, _ := reservestorage.GetFinalizedHeight(n.db.DB)
	cands := make([]string, 0, len(ss))
	for _, p := range ss {
		if ok && finH > 0 && n.sm != nil && !n.sm.isPeerAnchorOK(p) { continue }
		cands = append(cands, p)
	}
	if len(cands) == 0 { return "", false }
	return cands[rand.Intn(len(cands))], true
}


func (n *Node) TryAdvanceTip() {
	// GC candidates/headers below finalized
	finH, okF, _ := reservestorage.GetFinalizedHeight(n.db.DB)
	if okF && finH > 0 {
		reservestorage.DeleteHeadersBelowHeight(n.db.DB, finH)
		// best-effort: delete candidates for finH (and below) by scanning next few heights
		for h := uint64(0); h <= finH; h++ {
			cands, _ := reservestorage.ListCandidatesAtHeight(n.db.DB, h, 200)
			for _, c := range cands { reservestorage.DelCandidate(n.db.DB, h, c.Hash) }
		}
	}

	if !n.cfg.Sync.StrictLinear { return }
	for {
		tipH, tipHash, okTip, _ := reservestorage.GetTip(n.db.DB)
		if !okTip { tipH = 0; tipHash = "" }
		nextH := tipH + 1
		cands, err := reservestorage.ListCandidatesAtHeight(n.db.DB, nextH, 64)
		if err != nil || len(cands) == 0 { return }
		chosen := ""
		for _, c := range cands {
			if c.PrevHash == tipHash { chosen = c.Hash; break }
		}
		if chosen == "" { return }
		// enforce finalized constraint before setting tip
		if err := n.enforceFinalizedOnCandidate(chosen); err != nil { return }
		if err := n.ApplyBlock(&blk); err != nil { return }
		_ = reservestorage.SetTip(n.db.DB, blk.Header.Height, chosen)
		// keep scheduler moving
		n.enqueueMissingFromHeaderTip()
		// remove all candidates at this height to avoid reprocessing
		for _, c := range cands { reservestorage.DelCandidate(n.db.DB, nextH, c.Hash) }
	}
}


func (n *Node) TryCommitNext() {
	for {
		tipH, tipHash, okTip, _ := reservestorage.GetTip(n.db.DB)
		if !okTip { tipH = 0; tipHash = "" }
		nextH := tipH + 1

		// If best chain knows the hash at nextH, require the block hash to match it
		want, okW := n.BestChainHashAtHeight(nextH)

		// Look for candidates at nextH that connect to tip
		cands, _ := reservestorage.ListCandidatesAtHeight(n.db.DB, nextH, 256)
		chosen := ""
		for _, c := range cands {
			if c.PrevHash != tipHash { continue }
			if okW && want != "" && c.Hash != want { continue }
			chosen = c.Hash
			break
		}
		if chosen == "" { return }

		// enforce finalized boundary
		if err := n.enforceFinalizedOnCandidate(chosen); err != nil { return }

		// load block and require it matches stored header fields
		var blk chain.Block
		okB, _, _ := reservestorage.GetBlockByHash(n.db.DB, chosen, &blk)
		if !okB { return }
		if err := n.BlockMatchesStoredHeader(&blk); err != nil { return }


		// advance canonical tip
		if err := n.ApplyBlock(&blk); err != nil { return }
		_ = reservestorage.SetTip(n.db.DB, blk.Header.Height, chosen)
		for _, c := range cands { reservestorage.DelCandidate(n.db.DB, nextH, c.Hash) }

		// keep scheduler moving
		n.enqueueMissingFromHeaderTip()
	}
}


func (n *Node) TryConvergeToBest() error {
	// 1) enqueue missing blocks along best chain
	n.enqueueMissingFromHeaderTip()
	// 2) try commit forward along best chain
	n.TryCommitNext()
	// 3) if still diverged from best header tip and blocks are present, try reorg
	_, curHash, okC, _ := reservestorage.GetTip(n.db.DB)
	_, bestHash, okH, _ := reservestorage.GetHeaderTip(n.db.DB)
	if okC && okH && curHash != "" && bestHash != "" && curHash != bestHash {
		// attempt reorg (finalized-safe)
		return n.ReorgToBestHeaderChain()
	}
	return nil
}
