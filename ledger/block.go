package ledger

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/soden46/hyperlux-chain/wallet"
)

// ================== Block Types & Chain ==================

type Block struct {
	Index        int           `json:"index"`
	Timestamp    int64         `json:"timestamp"` // unix seconds
	PrevHash     string        `json:"prev_hash"`
	Hash         string        `json:"hash"`
	MerkleRoot   string        `json:"merkle_root"`
	Proposer     string        `json:"proposer"` // validator address
	Transactions []Transaction `json:"transactions"`
}

var Blockchain []Block

// ================== Helpers ==================

func ComputeMerkleRoot(txs []Transaction) string {
	if len(txs) == 0 {
		return ""
	}
	// start from tx hashes
	hashes := make([][]byte, 0, len(txs))
	for _, tx := range txs {
		th := HashTransaction(tx)
		b, _ := hex.DecodeString(th)
		hashes = append(hashes, b)
	}
	// pairwise hash until one
	for len(hashes) > 1 {
		var next [][]byte
		for i := 0; i < len(hashes); i += 2 {
			if i+1 < len(hashes) {
				h := sha256.Sum256(append(hashes[i], hashes[i+1]...))
				next = append(next, h[:])
			} else {
				// odd → hash(x||x)
				h := sha256.Sum256(append(hashes[i], hashes[i]...))
				next = append(next, h[:])
			}
		}
		hashes = next
	}
	return hex.EncodeToString(hashes[0])
}

func hashBlockHeader(idx int, ts int64, prev, merkle, proposer string) string {
	header := fmt.Sprintf("%d|%d|%s|%s|%s", idx, ts, prev, merkle, proposer)
	sum := sha256.Sum256([]byte(header))
	return hex.EncodeToString(sum[:])
}

func NewBlock(index int, txs []Transaction, prevHash string, proposerWallet *wallet.Wallet) Block {
	ts := time.Now().Unix()
	proposer := ""
	if proposerWallet != nil {
		proposer = proposerWallet.AddressEd
	}
	mr := ComputeMerkleRoot(txs)
	h := hashBlockHeader(index, ts, prevHash, mr, proposer)
	return Block{
		Index:        index,
		Timestamp:    ts,
		PrevHash:     prevHash,
		Hash:         h,
		MerkleRoot:   mr,
		Proposer:     proposer,
		Transactions: txs,
	}
}

// ================== Checkpoint (stub) ==================

func AddCheckpoint(b Block) {
	// diimpl sederhana; bisa disambung ke snapshot/fast sync nantinya
	// no-op selain memastikan API tersedia
}

// ================== Committing blocks ==================

// AddBlock (legacy/dev): ambil TX dari mempool tanpa validasi batch state.
func AddBlock(val *ValidatorDef, valWallet *wallet.Wallet) Block {
	if len(Blockchain) == 0 {
		genesis := NewBlock(0, []Transaction{}, "0", nil)
		Blockchain = append(Blockchain, genesis)
	}

	last := Blockchain[len(Blockchain)-1]

	// snapshot mempool
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

// AddBlockWithTxs: commit block menggunakan TX valid & bagi fee + reward.
func AddBlockWithTxs(val *ValidatorDef, _ *wallet.Wallet, txs []Transaction) Block {
	if len(Blockchain) == 0 {
		genesis := NewBlock(0, []Transaction{}, "0", nil)
		Blockchain = append(Blockchain, genesis)
	}

	last := Blockchain[len(Blockchain)-1]
	newBlock := NewBlock(len(Blockchain), txs, last.Hash, nil)
	Blockchain = append(Blockchain, newBlock)

	// fee & reward tetap
	totalFees := 0
	for _, tx := range txs {
		totalFees += tx.Fee
	}
	BalanceMu.Lock()
	Balances[val.Address] += totalFees + 5 // contoh reward tetap
	BalanceMu.Unlock()

	SaveAllData()

	fmt.Printf("✅ Block %d committed by %s with %d txs\n",
		newBlock.Index, val.Address, len(newBlock.Transactions))
	fmt.Printf("   MerkleRoot: %s | Timestamp: %d\n",
		newBlock.MerkleRoot, newBlock.Timestamp)

	return newBlock
}
