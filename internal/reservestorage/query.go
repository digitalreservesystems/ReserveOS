package reservestorage

import "github.com/syndtr/goleveldb/leveldb"

func GetBlockByHeight(db *leveldb.DB, height uint64, out any) (bool, string, error) {
	h, ok, err := GetHashByHeight(db, height)
	if err != nil || !ok {
		return false, "", err
	}
	ok2, err := GetBlockByHash(db, h, out)
	return ok2, h, err
}
