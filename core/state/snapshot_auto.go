package state

import "strings"

// ReadSnapshotAuto reads either json.gz or gob.gz snapshot based on filename suffix.
func ReadSnapshotAuto(path string) (Snapshot, error) {
	if strings.HasSuffix(path, ".gob.gz") {
		return ReadSnapshotBin(path)
	}
	return ReadSnapshot(path)
}
