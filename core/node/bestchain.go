package node

import (
	"reserveos/internal/reservestorage"
)

// BestChainHashAtHeight returns the header-hash that lies on the current best header tip at target height.
func (n *Node) BestChainHashAtHeight(target uint64) (string, bool) {
	hdrH, hdrHash, ok, _ := reservestorage.GetHeaderTip(n.db.DB)
	if tip, okT, _ := reservestorage.GetBestChainTip(n.db.DB); okT && tip != "" { hdrHash = tip; ok = true }
	if !ok || hdrHash == "" { return "", false }
	if target > hdrH { return "", false }
	if v, okC, _ := reservestorage.GetBestHashAtHeight(n.db.DB, target); okC && v != "" { return v, true }
	anc, okA, _ := reservestorage.GetHeaderAncestorHashAtHeight(n.db.DB, hdrHash, target)
	return anc, okA
}
