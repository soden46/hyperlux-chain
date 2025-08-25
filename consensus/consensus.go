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

var lastBlockTime int64
var lastTPS float64

const (
	// Target block time; sesuaikan sesuai throughput
	BlockTime = 350 * time.Millisecond
)

var (
	// PoH state
	pohChain []string
	pohSlot  int64 = 0

	// DPoS state (top-N by stake)
	Delegates []string
)

// InitConsensus → inisialisasi semua modul DPoH (PoH + DPoS + BFT)
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

	// pastikan wallet validator termuat
	ledger.AutoLoadValidatorWallets()

	// mulai block producer loop
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

// CommitBlock → ambil snapshot mempool, proses paralel (anti konflik),
// commit ke state sekali, buat block, lalu bersihkan mempool selektif.
func CommitBlock() {
	start := time.Now()

	mempoolBefore := ledger.GetMempoolSize()
	if mempoolBefore == 0 {
		return
	}

	// PoH next slot
	slotHash := nextPoH("block-commit")

	// pilih validator berbobot stake (VRF sederhana dari slotHash)
	if len(ledger.Validators) == 0 {
		fmt.Println("⚠️ No validators registered")
		return
	}
	validator := selectValidatorVRF(slotHash)
	valWallet := ledger.ValidatorWallets[validator.Address]
	if valWallet == nil {
		fmt.Printf("❌ Wallet not found for validator %s\n", validator.Address)
		return
	}

	fmt.Printf("🎲 Selected proposer: %s (stake=%d) | slotHash=%.12s...\n",
		validator.Address, validator.Stake, slotHash)

	// BFT vote simulasi
	if !bftVote(slotHash, ledger.Validators) {
		fmt.Println("❌ Block rejected by BFT")
		ledger.SlashValidator(validator.Address, 10)
		return
	}

	// Ambil snapshot mempool (tanpa argumen)
	snap := ledger.MempoolSnapshot()

	// Proses paralel + commit state sekali di akhir
	validTxs := ledger.ProcessTxListParallel(snap)

	// Buat block dari transaksi valid
	newBlock := ledger.AddBlockWithTxs(&validator, valWallet, validTxs)

	// Bersihkan mempool hanya TX yang sudah committed
	ledger.RemoveCommittedFromMempool(validTxs)

	// Checkpoint & siar ke jaringan (stub p2p aman untuk lokal)
	ledger.AddCheckpoint(newBlock)
	network.BroadcastBlock(newBlock)

	// Profiling helper
	elapsed := time.Since(start)
	mempoolAfter := ledger.GetMempoolSize()
	fmt.Printf("📈 Profiling → Goroutines=%d | Mempool(before)=%d after=%d | Latency=%.3fms | BlockTx=%d\n",
		runtime.NumGoroutine(), mempoolBefore, mempoolAfter, float64(elapsed.Microseconds())/1000.0, len(newBlock.Transactions))

	printMetrics(newBlock)
}

// ===================== DPoS (delegates) + VRF selection =====================

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

// selectValidatorVRF → pilih validator proporsional stake via slotHash
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

// validateBlock → placeholder validasi blok (signature, PoH, dll.)
func validateBlock(_ string) bool {
	return true
}

// ===================== Metrics =====================

func printMetrics(newBlock ledger.Block) {
	now := time.Now().Unix()
	if lastBlockTime > 0 {
		bt := now - lastBlockTime
		if bt > 0 {
			lastTPS = float64(len(newBlock.Transactions)) / float64(bt)
		}
		fmt.Printf("📊 Metrics → BlockTime=%ds, TPS=%.2f, Finality=BFT instant\n",
			now-lastBlockTime, lastTPS)
	}
	lastBlockTime = now
}

func GetLastBlockTime() int64   { return lastBlockTime }
func GetLastTPS() float64       { return lastTPS }
func GetFinalityStatus() string { return "BFT instant" }
