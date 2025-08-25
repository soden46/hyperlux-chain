package ledger

import "sync"

// ======== Global In-Memory State (dipisah lock agar minim contention) ========

// Saldo on-chain
var (
	Balances   = map[string]int{}
	BalanceMu  sync.RWMutex
)

// Nonce per akun (anti replay / ordering by account)
var (
	NonceTable   = map[string]int{}
	NonceTableMu sync.RWMutex
)

// Mempool transaksi sementara (belum final)
var (
	Mempool   []Transaction
	MempoolMu sync.RWMutex
)

// Helper untuk profiling / commit loop
func GetMempoolSize() int {
	MempoolMu.RLock()
	defer MempoolMu.RUnlock()
	return len(Mempool)
}
