package ledger

import (
	"sync"
	"time"
)

// ================= Global In-Memory State (fine-grained locks) =================

// Account balances
var (
	Balances  = map[string]int{}
	BalanceMu sync.RWMutex
)

// Nonce per account (anti-replay, ordering-by-account)
var (
	NonceTable   = map[string]int{}
	NonceTableMu sync.RWMutex
)

// Mempool (pending, not-final)
var (
	Mempool   []Transaction
	MempoolMu sync.RWMutex
)

// Monetary sinks & pools
var (
	TreasuryBalance int // 15% dari slashing (plus akumulasi lain)
	BurnedSupply    int // 70% dari slashing
	// Catatan: 5% honest redistribution langsung dibagikan pro-rata stake
)

// ================= Validator runtime status =================

type SuspensionScope int

const (
	ScopeNone   SuspensionScope = 0
	ScopePropose SuspensionScope = 1
	ScopeVote    SuspensionScope = 2
	ScopeAll     SuspensionScope = 3
)

type ValidatorRuntime struct {
	SuspendedUntil int64           // unix seconds
	SuspendScope   SuspensionScope // Propose, Vote, All
	// Bisa ditambah counter pelanggaran, dsb.
}

var (
	ValidatorStatus   = map[string]*ValidatorRuntime{} // address -> runtime status
	ValidatorStatusMu sync.RWMutex
)

// Helpers
func nowUnix() int64 { return time.Now().Unix() }

func GetMempoolSize() int {
	MempoolMu.RLock()
	defer MempoolMu.RUnlock()
	return len(Mempool)
}
