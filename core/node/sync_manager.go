package node

import (
	"encoding/json"
	"reserveos/core/chain"
	"sync"
	"time"

	"reserveos/internal/reservep2p"
	"reserveos/internal/reservestorage"
)

type SyncStatus struct {
	Mode string `json:"mode"`
	Peers int `json:"peers"`
	LastRequestedUnix int64 `json:"last_requested_unix"`
	LastHeadersCount int `json:"last_headers_count"`
	LastHeaderHashes []string `json:"last_header_hashes"`
	InFlightBlocks int `json:"in_flight_blocks"`
	PendingBlocks int `json:"pending_blocks"`
	TipHeight uint64 `json:"tip_height"`
}

type syncManager struct {
	paused bool
	
	failCount map[string]int
	lastFail map[string]int64
	
	anchorOK map[string]bool
	
	mu sync.Mutex
	status SyncStatus
	inflight map[string]time.Time
	pending []string
	lastAnchorHash string
	lastAnchorHeight uint64
}

func (sm *syncManager) setHeaders(count int, hashes []string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.status.LastHeadersCount = count
	sm.status.LastHeaderHashes = hashes

	exists := map[string]bool{}
	for _, h := range sm.pending { exists[h] = true }
	for h := range sm.inflight { exists[h] = true }
	for _, h := range hashes {
		if h == "" || exists[h] { continue }
		sm.pending = append(sm.pending, h)
		exists[h] = true
	}
	sm.status.PendingBlocks = len(sm.pending)
}

func (sm *syncManager) touchRequest() {
	sm.mu.Lock(); defer sm.mu.Unlock()
	sm.status.LastRequestedUnix = time.Now().Unix()
}

func (sm *syncManager) addInFlight(h string) {
	sm.mu.Lock(); defer sm.mu.Unlock()
	sm.inflight[h] = time.Now()
	sm.status.InFlightBlocks = len(sm.inflight)
}

func (sm *syncManager) clearInFlight(h string) {
	sm.mu.Lock(); defer sm.mu.Unlock()
	delete(sm.inflight, h)
	sm.status.InFlightBlocks = len(sm.inflight)
}

func (sm *syncManager) popPending() (string, bool) {
	sm.mu.Lock(); defer sm.mu.Unlock()
	if len(sm.pending) == 0 { return "", false }
	h := sm.pending[0]
	sm.pending = sm.pending[1:]
	sm.status.PendingBlocks = len(sm.pending)
	return h, true
}

func (sm *syncManager) snapshot(tip uint64, peers int) SyncStatus {
	sm.mu.Lock(); defer sm.mu.Unlock()
	s := sm.status
	s.TipHeight = tip
	s.Peers = peers
	if s.Mode == "" { s.Mode = "headers_first" }
	s.InFlightBlocks = len(sm.inflight)
	s.PendingBlocks = len(sm.pending)
	return s
}

func (n *Node) startSyncManager() {
	n.sm = &syncManager{inflight: map[string]time.Time{}, pending: []string{}, anchorOK: map[string]bool{}, failCount: map[string]int{}, lastFail: map[string]int64{}}

	for i := 0; i < n.cfg.Sync.Workers; i++ {
		go func() {
			for {
			if n.p2p != nil {
				if n.sm != nil && n.sm.isPaused() { time.Sleep(1 * time.Second); continue }
				n.sm.touchRequest()
				tipH, _, ok, _ := reservestorage.GetTip(n.db.DB)
				if !ok { tipH = 0 }
				frontH, _, okF, _ := reservestorage.GetHeaderFrontier(n.db.DB)
				start := tipH + 1
				if okF && frontH >= tipH { start = frontH + 1 }
				loc := make([]string,0,32)
				// build locator from tip
				h := tipH
				step := uint64(1)
				for len(loc) < 32 {
					var b chain.Block
					okb, _, _ := reservestorage.GetBlockByHeight(n.db.DB, h, &b)
					if okb { loc = append(loc, b.Hash().String()) }
					if h == 0 { break }
					if len(loc) > 10 { step *= 2 }
					if h < step { h = 0 } else { h -= step }
				}
				req := map[string]any{"from": start, "count": 200, "locator": loc}
				b, _ := json.Marshal(req)
				n.p2p.SendAll(reservep2p.Msg{Type: "get_anchor"})
				n.p2p.SendAll(reservep2p.Msg{Type: "get_headers", Data: b})
			}
			time.Sleep(5 * time.Second)
		}
	}()

	for i := 0; i < n.cfg.Sync.Workers; i++ {
		go func() {
			for {
			if n.p2p == nil || n.sm == nil {
				time.Sleep(250 * time.Millisecond)
				continue
			}
			if n.sm.inflightCount() >= n.cfg.Sync.MaxInFlightBlocks { time.Sleep(100 * time.Millisecond); continue }
			h, ok := n.sm.popPending()
			if !ok {
				time.Sleep(200 * time.Millisecond)
				continue
			}
			// request block from network
			n.sm.addInFlight(h)
			peer, okp := n.chooseSyncPeer()
			if !okp { time.Sleep(200 * time.Millisecond); continue }
			if n.sm != nil {
				if bo := n.sm.peerBackoff(peer); bo > 0 { time.Sleep(bo); }
			}
			if reservestorage.PeerGetBlockRecently(n.db.DB, peer, h, 2) { time.Sleep(50*time.Millisecond); continue }
			reservestorage.MarkPeerGetBlock(n.db.DB, peer, h)
			_ = n.p2p.SendTo(peer, reservep2p.Msg{Type:"get_block", Hash: h})
			time.Sleep(50 * time.Millisecond)
		}
	}()
}


func (sm *syncManager) inflightCount() int {
	sm.mu.Lock(); defer sm.mu.Unlock()
	return len(sm.inflight)
}

func (sm *syncManager) setPeerAnchorOK(peer string, ok bool) {
	sm.mu.Lock(); defer sm.mu.Unlock()
	sm.anchorOK[peer] = ok
}

func (sm *syncManager) isPeerAnchorOK(peer string) bool {
	sm.mu.Lock(); defer sm.mu.Unlock()
	v, ok := sm.anchorOK[peer]
	return ok && v
}

func (sm *syncManager) setLocalAnchor(h uint64, hash string) {
	sm.mu.Lock(); defer sm.mu.Unlock()
	sm.lastAnchorHeight = h
	sm.lastAnchorHash = hash
}


func (sm *syncManager) markFail(peer string) {
	sm.mu.Lock(); defer sm.mu.Unlock()
	sm.failCount[peer]++
	sm.lastFail[peer] = time.Now().Unix()
}

func (sm *syncManager) peerBackoff(peer string) time.Duration {
	sm.mu.Lock(); defer sm.mu.Unlock()
	c := sm.failCount[peer]
	lf := sm.lastFail[peer]
	if c == 0 { return 0 }
	// exponential backoff, capped
	wait := int64(1<<minInt(c, 6)) // up to 64s
	now := time.Now().Unix()
	if now-lf >= wait { return 0 }
	return time.Duration(wait-(now-lf)) * time.Second
}

func minInt(a,b int) int { if a<b { return a }; return b }


func (sm *syncManager) setPaused(v bool) {
	sm.mu.Lock(); defer sm.mu.Unlock()
	sm.paused = v
}

func (sm *syncManager) isPaused() bool {
	sm.mu.Lock(); defer sm.mu.Unlock()
	return sm.paused
}


func (n *Node) enqueueMissingFromHeaderTip() {
	if n.sm == nil { return }
	blockH, _, okB, _ := reservestorage.GetTip(n.db.DB)
	if !okB { blockH = 0 }
	hdrH, hdrHash, okH, _ := reservestorage.GetHeaderTip(n.db.DB)
	if !okH || hdrHash == "" { return }
	// walk back from best header tip to block tip+1 to build the missing sequence
	if hdrH <= blockH { return }
	start := blockH + 1
	// find ancestor hash at start height
	anc, okA, _ := reservestorage.GetHeaderAncestorHashAtHeight(n.db.DB, hdrHash, start)
	if !okA { return }
	// Now reconstruct forward hashes by walking backwards from tip and collecting, then reverse
	// We do a backward collection from hdrHash down to start
	seq := make([]string, 0, int(hdrH-start+1))
	cur := hdrHash
	for {
		h, ok, _ := reservestorage.GetHeaderByHash(n.db.DB, cur)
		if !ok { break }
		if h.Height < start { break }
		seq = append(seq, cur)
		if h.Height == start { break }
		cur = h.PrevHash
	}
	// reverse to ascending order
	for i, j := 0, len(seq)-1; i < j; i, j = i+1, j-1 { seq[i], seq[j] = seq[j], seq[i] }

	filled := 0
	for _, hash := range seq {
		// skip if block present
		var b chain.Block
		okb, _, _ := reservestorage.GetBlockByHash(n.db.DB, hash, &b)
		if okb { continue }
		n.sm.setHeaders(0, []string{hash})
		filled++
		if filled >= n.cfg.Sync.MaxQueueFill { return }
		if n.sm.snapshot(0,0).PendingBlocks > 5000 { return }
	}
	_ = anc
}


func (sm *syncManager) requeueTimedOut(timeoutSec int) []string {
	sm.mu.Lock(); defer sm.mu.Unlock()
	now := time.Now()
	req := make([]string, 0, 16)
	for h, t := range sm.inflight {
		if now.Sub(t) > time.Duration(timeoutSec)*time.Second {
			req = append(req, h)
			delete(sm.inflight, h)
		}
	}
	sm.status.InFlightBlocks = len(sm.inflight)
	// prepend to pending
	if len(req) > 0 {
		sm.pending = append(req, sm.pending...)
		sm.status.PendingBlocks = len(sm.pending)
	}
	return req
}


func (sm *syncManager) prioritizeNearTip(maxFront int) {
	sm.mu.Lock(); defer sm.mu.Unlock()
	if maxFront <= 0 || len(sm.pending) <= maxFront { return }
	// keep first maxFront as-is; nothing else for v1
}
