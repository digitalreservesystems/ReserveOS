package pow

// Retarget computes next Bits based on a simple window.
// This is a placeholder: clamp actual timespan to [target/4, target*4] then scale.
func Retarget(prevBits uint32, actualSpanSec int64, targetSpanSec int64) uint32 {
	if targetSpanSec <= 0 { return prevBits }
	min := targetSpanSec / 4
	max := targetSpanSec * 4
	if actualSpanSec < min { actualSpanSec = min }
	if actualSpanSec > max { actualSpanSec = max }
	// scale bits linearly in v1 (not bitcoin-accurate)
	// smaller span => harder => bits-1; larger span => easier => bits+1
	if actualSpanSec < targetSpanSec { return prevBits + 1 }
	if actualSpanSec > targetSpanSec { if prevBits > 1 { return prevBits - 1 }; return prevBits }
	return prevBits
}
