package reservestorage

import "github.com/syndtr/goleveldb/leveldb"

type LevelDB struct{ DB *leveldb.DB }

func OpenLevelDB(path string) (*LevelDB, error) {
	db, err := leveldb.OpenFile(path, nil)
	if err != nil {
		return nil, err
	}
	return &LevelDB{DB: db}, nil
}

func (l *LevelDB) Close() error { return l.DB.Close() }
