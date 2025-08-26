package consensus

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/soden46/hyperlux-chain/ledger"
	"github.com/soden46/hyperlux-chain/network"
)

const BlockTime = 350 * time.Millisecond

var (
	// PoH state
	pohChain []string
	pohSlot  int64

	// DPoS delegates (top-N)
	Delegates []string

	// metrics (berbasis wall-clock, bukan height delta)
	lastBlockWall time.Time
	lastTPS       float64

	// reentrancy guard: hindari CommitBlock overlap (auto-commit vs ticker)
	committing int32
)

// ===================== Init =====================

func InitConsensus() {
	fmt.Println("⚡ Consensus engine initialized")

	initPoH()
	initDPoS()
	initBFT()

	ledger.LoadValidators()
	if len(ledger.Validators) == 0 {
		fmt.Println("⚠️ Tidak ada validator terdaftar")
	} else {
		fmt.Printf("✅ Loaded %d validators from DB\n", len(ledger.Validators))
	}
	ledger.AutoLoadValidatorWallets()

	go blockProducer()
	fmt.Println("✅ Consensus modules ready")
}

func blockProducer() {
	ticker := time.NewTicker(BlockTime)
	defer ticker.Stop()
	for range ticker.C {
		CommitBlock()
	}
}

// ===================== CommitBlock =====================

func CommitBlock() {
	// hindari overlap
	if !atomic.CompareAndSwapInt32(&committing, 0, 1) {
		return
	}
	defer atomic.StoreInt32(&committing, 0)

	start := time.Now()

	mempoolBefore := ledger.GetMempoolSize()
	if mempoolBefore == 0 {
		return
	}

	// PoH slot
	slotHash := nextPoH("block-commit")

	// pilih proposer via VRF + skip yang suspended (propose)
	if len(ledger.Validators) == 0 {
		fmt.Println("⚠️ No validators registered")
		return
	}
	validator := selectValidatorVRFEligible(slotHash)
	valWallet := ledger.ValidatorWallets[validator.Address]
	if valWallet == nil {
		fmt.Printf("❌ Wallet not found for validator %s\n", validator.Address)
		return
	}

	fmt.Printf("🎲 Selected proposer: %s (stake=%d) | slotHash=%.12s...\n",
		validator.Address, validator.Stake, slotHash)

	// BFT vote (skip voter yang suspended vote/all)
	if !bftVote(slotHash, ledger.Validators) {
		fmt.Println("❌ Block rejected by BFT")
		ledger.SlashValidator(validator.Address, 10) // contoh penalti ringan
		return
	}

	// Snapshot & eksekusi paralel (mutasi state dilakukan di executor)
	snap := ledger.MempoolSnapshot()
	validTxs := ledger.ProcessTxListParallel(snap)

	// Build block & broadcast
	newBlock := ledger.AddBlockWithTxs(&validator, valWallet, validTxs)
	ledger.RemoveCommittedFromMempool(validTxs)
	ledger.AddCheckpoint(newBlock)
	network.BroadcastBlock(newBlock)

	// Profiling
	elapsed := time.Since(start)
	mempoolAfter := ledger.GetMempoolSize()
	fmt.Printf("📈 Profiling → Goroutines=%d | Mempool(before)=%d after=%d | Latency=%.3fms | BlockTx=%d\n",
		runtime.NumGoroutine(), mempoolBefore, mempoolAfter, float64(elapsed.Microseconds())/1000.0, len(newBlock.Transactions))

	printMetrics(newBlock)
}

// ===================== DPoS + VRF =====================

func initDPoS() {
	ledger.LoadValidators()
	if len(ledger.Validators) == 0 {
		fmt.Println("⚠️ No validators for DPoS")
		return
	}
	sort.Slice(ledger.Validators, func(i, j int) bool {
		return ledger.Validators[i].Stake > ledger.Validators[j].Stake
	})
	max := 5
	if len(ledger.Validators) < max {
		max = len(ledger.Validators)
	}
	Delegates = Delegates[:0]
	for i := 0; i < max; i++ {
		Delegates = append(Delegates, ledger.Validators[i].Address)
	}
	fmt.Printf("⚡ DPoS Consensus initialized with %d delegates\n", len(Delegates))
}

func selectValidatorVRF(seed string) ledger.ValidatorDef {
	hash := sha256.Sum256([]byte(seed))
	rnd := new(big.Int).SetBytes(hash[:])

	totalStake := big.NewInt(0)
	for _, v := range ledger.Validators {
		totalStake.Add(totalStake, big.NewInt(int64(v.Stake)))
	}
	if totalStake.Cmp(big.NewInt(0)) == 0 {
		return ledger.ValidatorDef{}
	}

	r := new(big.Int).Mod(rnd, totalStake)
	acc := big.NewInt(0)
	for _, v := range ledger.Validators {
		acc.Add(acc, big.NewInt(int64(v.Stake)))
		if r.Cmp(acc) < 0 {
			return v
		}
	}
	return ledger.Validators[0]
}

// pilih validator eligible (tidak suspended propose). Jika pick suspended → fallback linear scan
func selectValidatorVRFEligible(seed string) ledger.ValidatorDef {
	pick := selectValidatorVRF(seed)
	if !ledger.IsSuspended(pick.Address, ledger.ScopePropose) {
		return pick
	}
	for _, v := range ledger.Validators {
		if !ledger.IsSuspended(v.Address, ledger.ScopePropose) {
			return v
		}
	}
	return pick
}

// ===================== PoH =====================

func initPoH() {
	fmt.Println("Proof of History module initialized")
	if len(pohChain) == 0 {
		genesis := generatePoH("genesis", "init", time.Now().UnixNano())
		pohChain = append(pohChain, genesis)
		fmt.Println("✅ PoH genesis slot:", genesis)
	}
}

func generatePoH(prevHash, data string, timestamp int64) string {
	input := fmt.Sprintf("%s|%s|%d", prevHash, data, timestamp)
	hash := sha256.Sum256([]byte(input))
	return hex.EncodeToString(hash[:])
}

func nextPoH(data string) string {
	if len(pohChain) == 0 {
		initPoH()
	}
	prev := pohChain[len(pohChain)-1]
	pohSlot++
	hash := generatePoH(prev, data, time.Now().UnixNano())
	pohChain = append(pohChain, hash)
	return hash
}

// ===================== BFT (simulasi) =====================

func initBFT() {
	fmt.Println("⚡ BFT Consensus initialized")
}

func bftVote(blockHash string, validators []ledger.ValidatorDef) bool {
	var yesCount int32
	var wg sync.WaitGroup
	results := make(chan bool, len(validators))

	for _, v := range validators {
		if ledger.IsSuspended(v.Address, ledger.ScopeVote) || ledger.IsSuspended(v.Address, ledger.ScopeAll) {
			continue
		}
		wg.Add(1)
		go func(val ledger.ValidatorDef) {
			defer wg.Done()
			isValid := validateBlock(blockHash)
			results <- isValid
			if isValid {
				fmt.Printf("🗳️ %s voted YES for block %.12s\n", val.Address, blockHash)
			} else {
				fmt.Printf("🗳️ %s voted NO for block %.12s\n", val.Address, blockHash)
			}
		}(v)
	}

	wg.Wait()
	close(results)

	for res := range results {
		if res {
			atomic.AddInt32(&yesCount, 1)
		}
	}

	yes := int(yesCount)
	threshold := (len(validators)*2)/3 + 1
	if yes >= threshold {
		fmt.Printf("✅ BFT reached consensus: %d/%d YES (%.2f%%)\n",
			yes, len(validators), float64(yes)/float64(len(validators))*100)
		return true
	}
	fmt.Println("❌ BFT failed to reach consensus")
	return false
}

func validateBlock(_ string) bool { return true }

// ===================== Metrics =====================

func printMetrics(newBlock ledger.Block) {
	now := time.Now()
	if !lastBlockWall.IsZero() {
		dt := now.Sub(lastBlockWall).Seconds()
		if dt <= 0 {
			dt = 1e-6
		}
		lastTPS = float64(len(newBlock.Transactions)) / dt
		fmt.Printf("📊 Metrics → BlockTime=%.2fs, TPS=%.2f, Finality=BFT instant\n", dt, lastTPS)
	}
	lastBlockWall = now
}

func GetLastBlockTime() int64 {
	if lastBlockWall.IsZero() {
		return 0
	}
	return int64(time.Since(lastBlockWall).Seconds())
}
func GetLastTPS() float64       { return lastTPS }
func GetFinalityStatus() string { return "BFT instant" }
