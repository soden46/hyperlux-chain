package ledger

import (
	"fmt"

	"github.com/soden46/hyperlux-chain/wallet"
)

// InitLedger inisialisasi LevelDB + load semua state, buat genesis kalau belum ada
func InitLedger() {
	fmt.Println("✅ Storage engine initialized")

	InitDB()
	LoadBalances()
	LoadBlockchain()
	LoadMempool()
	LoadNonceTable()
	LoadValidators()

	if len(Blockchain) == 0 {
		genesis := NewBlock(0, []Transaction{}, "0", nil)
		Blockchain = append(Blockchain, genesis)
		SaveBlockchain()
		fmt.Println("✅ Genesis block created")
	}
}

// AddBlock (legacy/dev) — ambil TX dari mempool apa adanya, buat block, reset mempool
// (Tidak menyentuh balances/nonce — gunakan ProcessMempoolParallel untuk komit state)
func AddBlock(val *ValidatorDef, valWallet *wallet.Wallet) Block {
	if len(Blockchain) == 0 {
		genesis := NewBlock(0, []Transaction{}, "0", nil)
		Blockchain = append(Blockchain, genesis)
	}

	last := Blockchain[len(Blockchain)-1]
	// pakai snapshot mempool (tanpa mutasi)
	MempoolMu.RLock()
	txs := make([]Transaction, len(Mempool))
	copy(txs, Mempool)
	MempoolMu.RUnlock()

	newBlock := NewBlock(len(Blockchain), txs, last.Hash, valWallet)
	Blockchain = append(Blockchain, newBlock)

	// reset mempool
	ClearMempool()

	SaveAllData()

	fmt.Printf("✅ Block %d committed by %s with %d txs\n",
		newBlock.Index, val.Address, len(newBlock.Transactions))
	fmt.Printf("   MerkleRoot: %s | Timestamp: %d\n",
		newBlock.MerkleRoot, newBlock.Timestamp)

	return newBlock
}

// AddBlockWithTxs — commit block menggunakan TX valid (state sudah diupdate di ProcessMempoolParallel),
// lalu distribusi fee + reward untuk validator (tambahan).
func AddBlockWithTxs(val *ValidatorDef, valWallet *wallet.Wallet, txs []Transaction) Block {
	if len(Blockchain) == 0 {
		genesis := NewBlock(0, []Transaction{}, "0", nil)
		Blockchain = append(Blockchain, genesis)
	}

	last := Blockchain[len(Blockchain)-1]
	newBlock := NewBlock(len(Blockchain), txs, last.Hash, valWallet)
	Blockchain = append(Blockchain, newBlock)

	// Hitung total fee & rewardkan ke validator
	totalFees := 0
	for _, tx := range txs {
		totalFees += tx.Fee
	}

	// Kredit fee + reward tetap ke validator
	BalanceMu.Lock()
	Balances[val.Address] += totalFees + 5 // contoh reward tetap 5
	BalanceMu.Unlock()

	SaveAllData()

	fmt.Printf("✅ Block %d committed by %s with %d txs\n",
		newBlock.Index, val.Address, len(newBlock.Transactions))
	fmt.Printf("   MerkleRoot: %s | Timestamp: %d\n",
		newBlock.MerkleRoot, newBlock.Timestamp)

	return newBlock
}

// GetBalance → ambil saldo address (thread-safe read)
func GetBalance(addr string) int {
	BalanceMu.RLock()
	defer BalanceMu.RUnlock()
	return Balances[addr]
}

// Airdrop → tambah saldo manual (thread-safe)
func Airdrop(addr string, amount int) {
	BalanceMu.Lock()
	Balances[addr] += amount
	BalanceMu.Unlock()
	SaveBalances()
	fmt.Printf("💸 Airdropped %d HYLUX to %s\n", amount, addr)
}

// LoadAllData memuat semua data (state) ke memori
func LoadAllData() {
	LoadBalances()
	LoadBlockchain()
	LoadMempool()
	LoadNonceTable()
	LoadValidators()
}

// SaveAllData menyimpan semua data dari memori ke LevelDB
func SaveAllData() {
	SaveBalances()
	SaveBlockchain()
	SaveMempool()
	SaveNonceTable()
	SaveValidators()
}
