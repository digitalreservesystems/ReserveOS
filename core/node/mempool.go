package node

import (
	"fmt"
	"container/heap"
	"sync"
	"time"

	"reserveos/core/chain"
	"reserveos/core/economics/fees"
	"reserveos/core/state"

	"github.com/syndtr/goleveldb/leveldb"
)

type memTx struct {
	Tx       chain.Tx
	TxID     string
	From     string
	Nonce    uint64
	Fee      int64
	SizeHint int
	FeeRate  float64
	AddedAt  int64
}

type Mempool struct {
	mu sync.Mutex

	cfg MempoolConfig

	byAcct map[string]map[uint64]*memTx

	totalTxs   int
	totalBytes int
}

func NewMempool(cfg MempoolConfig) *Mempool {
	return &Mempool{
		cfg:    cfg,
		byAcct: map[string]map[uint64]*memTx{},
	}
}

func (m *Mempool) pruneLocked(now int64) {
	cut := now - int64(m.cfg.TxTTLSeconds)
	for acct, mp := range m.byAcct {
		for nonce, mt := range mp {
			if mt.AddedAt <= cut {
				delete(mp, nonce)
				m.totalTxs--
				m.totalBytes -= mt.SizeHint
			}
		}
		if len(mp) == 0 {
			delete(m.byAcct, acct)
		}
	}
}

func (m *Mempool) Add(db *leveldb.DB, tx chain.Tx) (replaced bool, err error) {
	m.mu.Lock()
	if m.cfg.MaxTxsPerSender > 0 {
		cnt := 0
		for _, t := range m.txs { if t.From == tx.From { cnt++ } }
		if cnt >= m.cfg.MaxTxsPerSender { m.mu.Unlock(); return false, fmt.Errorf("sender_cap") }
	}

	defer m.mu.Unlock()

	now := time.Now().Unix()
	m.pruneLocked(now)

	acct := tx.From
	nonce := tx.Nonce
	if acct == "" {
		return false, ErrMempool("missing_from")
	}

	// future nonce policy (admission-time check)
	curNonce, _, _ := state.GetNonce(db, acct)
	expected := curNonce + 1
	if !m.cfg.AllowFutureNonces {
		if nonce != expected {
			return false, ErrMempool("nonce_not_next")
		}
	} else {
		if nonce < expected {
			return false, ErrMempool("nonce_too_low")
		}
		if nonce-expected > m.cfg.MaxNonceGap {
			return false, ErrMempool("nonce_gap_too_large")
		}
	}


	sz := fees.TxSizeHint(tx.ID(), len(tx.Outputs))
	feeRate := float64(tx.Fee) / float64(max(1, sz))

	mt := &memTx{Tx: tx, TxID: tx.ID(), From: acct, Nonce: nonce, Fee: tx.Fee, SizeHint: sz, FeeRate: feeRate, AddedAt: now}

	mp := m.byAcct[acct]
	if mp == nil {
		mp = map[uint64]*memTx{}
		m.byAcct[acct] = mp
	}

	if old, ok := mp[nonce]; ok {
		minNew := int64(float64(old.Fee) * m.cfg.RBFMinFeeBump)
		if tx.Fee < minNew {
			return false, ErrMempool("rbf_fee_bump_too_small")
		}
		mp[nonce] = mt
		m.totalBytes += (mt.SizeHint - old.SizeHint)
		return true, nil
	}

	if m.totalTxs+1 > m.cfg.MaxTxs {
		return false, ErrMempool("mempool_full")
	}
	if m.totalBytes+mt.SizeHint > m.cfg.MaxBytes {
		return false, ErrMempool("mempool_bytes_full")
	}

	mp[nonce] = mt
	m.totalTxs++
	m.totalBytes += mt.SizeHint
	return false, nil
}

func (m *Mempool) Drain(db *leveldb.DB, maxCount int) []chain.Tx {
	m.mu.Lock()
	if m.cfg.MaxTxsPerSender > 0 {
		cnt := 0
		for _, t := range m.txs { if t.From == tx.From { cnt++ } }
		if cnt >= m.cfg.MaxTxsPerSender { m.mu.Unlock(); return false, fmt.Errorf("sender_cap") }
	}

	defer m.mu.Unlock()

	now := time.Now().Unix()
	m.pruneLocked(now)

	pq := &txHeap{}
	heap.Init(pq)

	expected := map[string]uint64{}
	for acct := range m.byAcct {
		nn, _, _ := state.GetNonce(db, acct)
		expected[acct] = nn + 1
		if mt := m.byAcct[acct][nn+1]; mt != nil {
			heap.Push(pq, mt)
		}
	}

	out := make([]chain.Tx, 0, maxCount)
	for pq.Len() > 0 && (maxCount <= 0 || len(out) < maxCount) {
		mt := heap.Pop(pq).(*memTx)
		acct := mt.From
		exp := expected[acct]
		cur := m.byAcct[acct][exp]
		if cur == nil || cur.TxID != mt.TxID {
			continue
		}

		out = append(out, cur.Tx)
		delete(m.byAcct[acct], exp)
		m.totalTxs--
		m.totalBytes -= cur.SizeHint
		if len(m.byAcct[acct]) == 0 {
			delete(m.byAcct, acct)
		}
		expected[acct] = exp + 1
		if next := m.byAcct[acct][exp+1]; next != nil {
			heap.Push(pq, next)
		}
	}
	return out
}

type txHeap []*memTx

func (h txHeap) Len() int { return len(h) }
func (h txHeap) Less(i, j int) bool {
	if h[i].FeeRate == h[j].FeeRate {
		return h[i].Fee > h[j].Fee
	}
	return h[i].FeeRate > h[j].FeeRate
}
func (h txHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *txHeap) Push(x any)    { *h = append(*h, x.(*memTx)) }
func (h *txHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

func max(a, b int) int { if a > b { return a }; return b }

type ErrMempool string

func (e ErrMempool) Error() string { return string(e) }


func (m *Mempool) Info() map[string]any {
	m.mu.Lock()
	if m.cfg.MaxTxsPerSender > 0 {
		cnt := 0
		for _, t := range m.txs { if t.From == tx.From { cnt++ } }
		if cnt >= m.cfg.MaxTxsPerSender { m.mu.Unlock(); return false, fmt.Errorf("sender_cap") }
	}

	defer m.mu.Unlock()
	perAcct := map[string]int{}
	for acct, mp := range m.byAcct {
		perAcct[acct] = len(mp)
	}
	return map[string]any{
		"total_txs": m.totalTxs,
		"total_bytes": m.totalBytes,
		"accounts": len(m.byAcct),
		"per_account": perAcct,
		"config": m.cfg,
	}
}


// QueueSnapshot returns a shallow view of queued nonces per account.
func (m *Mempool) QueueSnapshot() map[string][]uint64 {
	m.mu.Lock()
	if m.cfg.MaxTxsPerSender > 0 {
		cnt := 0
		for _, t := range m.txs { if t.From == tx.From { cnt++ } }
		if cnt >= m.cfg.MaxTxsPerSender { m.mu.Unlock(); return false, fmt.Errorf("sender_cap") }
	}

	defer m.mu.Unlock()
	out := map[string][]uint64{}
	for acct, mp2 := range m.byAcct {
		arr := make([]uint64, 0, len(mp2))
		for n := range mp2 { arr = append(arr, n) }
		// no sort to keep minimal
		out[acct] = arr
	}
	return out
}

// EligibleSnapshot returns the next-eligible tx per account (nonce == stateNonce+1), without removing.
func (m *Mempool) EligibleSnapshot(db *leveldb.DB) []chain.Tx {
	// nonce lanes: mine only contiguous nonce chains per sender

	m.mu.Lock()
	if m.cfg.MaxTxsPerSender > 0 {
		cnt := 0
		for _, t := range m.txs { if t.From == tx.From { cnt++ } }
		if cnt >= m.cfg.MaxTxsPerSender { m.mu.Unlock(); return false, fmt.Errorf("sender_cap") }
	}

	defer m.mu.Unlock()
	out := make([]chain.Tx, 0, len(m.byAcct))
	for acct, mp2 := range m.byAcct {
		nn, _, _ := state.GetNonce(db, acct)
		if mt := mp2[nn+1]; mt != nil {
			out = append(out, mt.Tx)
		}
	}
	return out
}


// TODO(v1.0.148): implement eviction by fee-rate/age when mempool exceeds limits.
