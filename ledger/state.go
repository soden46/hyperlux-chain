package ledger

import (
	"sync"
	"time"
)

// ================= Global In-Memory State =================

// Account balances
var (
	Balances  = map[string]int{}
	BalanceMu sync.RWMutex
)

// Nonce per account
var (
	NonceTable   = map[string]int{}
	NonceTableMu sync.RWMutex
)

// Mempool
var (
	Mempool   []Transaction
	MempoolMu sync.RWMutex
)

// Monetary sinks
var (
	TreasuryBalance int
	BurnedSupply    int
)

// ================= Validator runtime status =================

type SuspensionScope int

const (
	ScopeNone    SuspensionScope = 0
	ScopePropose SuspensionScope = 1
	ScopeVote    SuspensionScope = 2
	ScopeAll     SuspensionScope = 3
)

type ValidatorRuntime struct {
	SuspendedUntil int64
	SuspendScope   SuspensionScope
}

var (
	ValidatorStatus   = map[string]*ValidatorRuntime{}
	ValidatorStatusMu sync.RWMutex
)

func nowUnix() int64 { return time.Now().Unix() }

func GetMempoolSize() int {
	MempoolMu.RLock()
	defer MempoolMu.RUnlock()
	return len(Mempool)
}
