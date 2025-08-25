package storage

import (
	"log"

	"github.com/dgraph-io/badger/v3"
)

var DB *badger.DB

func InitDB(path string) {
	opts := badger.DefaultOptions(path).WithLogger(nil) // disable spam log
	db, err := badger.Open(opts)
	if err != nil {
		log.Fatal(err)
	}
	DB = db
}

func Save(key string, value []byte) error {
	return DB.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(key), value)
	})
}

func Load(key string) ([]byte, error) {
	var valCopy []byte
	err := DB.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err != nil {
			return err
		}
		valCopy, err = item.ValueCopy(nil)
		return err
	})
	return valCopy, err
}
