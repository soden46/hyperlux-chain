package ledger

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/syndtr/goleveldb/leveldb"
)

var (
	db   *leveldb.DB
	dbMu sync.Mutex // serialize open/close per process
)

// InitDB buka LevelDB satu kali
func InitDB() {
	dbMu.Lock()
	defer dbMu.Unlock()
	if db != nil {
		return
	}
	var err error
	db, err = leveldb.OpenFile("hyperlux_db", nil)
	if err != nil {
		log.Fatalf("❌ gagal buka DB: %v", err)
	}
	fmt.Println("✅ Storage engine initialized")
}

// ================= BALANCES =================
func SaveBalances() {
	InitDB()
	BalanceMu.RLock()
	defer BalanceMu.RUnlock()
	data, _ := json.Marshal(Balances)
	_ = db.Put([]byte("balances"), data, nil)
}

func LoadBalances() {
	InitDB()
	data, _ := db.Get([]byte("balances"), nil)
	if len(data) > 0 {
		_ = json.Unmarshal(data, &Balances)
	}
}

// ================= BLOCKCHAIN =================
func SaveBlockchain() {
	InitDB()
	data, _ := json.Marshal(Blockchain)
	_ = db.Put([]byte("blockchain"), data, nil)
}

func LoadBlockchain() {
	InitDB()
	data, _ := db.Get([]byte("blockchain"), nil)
	if len(data) > 0 {
		_ = json.Unmarshal(data, &Blockchain)
	}
}

// ================= MEMPOOL =================
func SaveMempool() {
	InitDB()
	MempoolMu.RLock()
	defer MempoolMu.RUnlock()
	data, _ := json.Marshal(Mempool)
	_ = db.Put([]byte("mempool"), data, nil)
}

func LoadMempool() {
	InitDB()
	data, _ := db.Get([]byte("mempool"), nil)
	if len(data) > 0 {
		_ = json.Unmarshal(data, &Mempool)
	}
}

// ================= NONCE TABLE =================
func SaveNonceTable() {
	InitDB()
	NonceTableMu.RLock()
	defer NonceTableMu.RUnlock()
	data, _ := json.Marshal(NonceTable)
	_ = db.Put([]byte("nonce_table"), data, nil)
}

func LoadNonceTable() {
	InitDB()
	data, _ := db.Get([]byte("nonce_table"), nil)
	if len(data) > 0 {
		_ = json.Unmarshal(data, &NonceTable)
	}
}

// ================= VALIDATORS =================
func SaveValidators() {
	InitDB()
	data, _ := json.Marshal(Validators)
	_ = db.Put([]byte("validators"), data, nil)
}

func LoadValidators() {
	InitDB()
	data, _ := db.Get([]byte("validators"), nil)
	if len(data) > 0 {
		_ = json.Unmarshal(data, &Validators)
	}
}
