package chain

import "crypto/sha256"

func TxRoot(txs []Tx) Hash {
	if len(txs) == 0 {
		return sha256d([]byte("txroot:empty"))
	}
	var level [][]byte
	for _, tx := range txs {
		level = append(level, []byte(tx.ID()))
	}
	for len(level) > 1 {
		var next [][]byte
		for i := 0; i < len(level); i += 2 {
			if i+1 == len(level) {
				next = append(next, sha(level[i], level[i]))
			} else {
				next = append(next, sha(level[i], level[i+1]))
			}
		}
		level = next
	}
	sum := sha256.Sum256(level[0])
	var out Hash
	copy(out[:], sum[:])
	return out
}

func sha(a, b []byte) []byte {
	h := sha256.New()
	h.Write(a)
	h.Write(b)
	return h.Sum(nil)
}
