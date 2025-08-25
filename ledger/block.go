package ledger

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/soden46/hyperlux-chain/wallet"
)

type Block struct {
	Index        int           `json:"index"`
	Timestamp    int64         `json:"timestamp"`
	Transactions []Transaction `json:"transactions"`
	PrevHash     string        `json:"prev_hash"`
	Hash         string        `json:"hash"`
	MerkleRoot   string        `json:"merkle_root"`
	Validator    string        `json:"validator"`
	ValidatorSig string        `json:"validator_sig"`
	Nonce        int           `json:"nonce"`
}

// Blockchain disimpan di memori (persist di LevelDB lewat storage.go)
var Blockchain []Block

// Checkpoint untuk sync cepat
type Checkpoint struct {
	BlockIndex int    `json:"block_index"`
	Hash       string `json:"hash"`
}

var Checkpoints []Checkpoint

// NewBlock membuat block baru, menandatangani header dengan wallet validator bila ada
func NewBlock(index int, txs []Transaction, prevHash string, valWallet *wallet.Wallet) Block {
	ts := time.Now().Unix()
	merkle := ComputeMerkleRoot(txs)

	// Block header yang di-hash
	header := struct {
		Index      int
		Timestamp  int64
		PrevHash   string
		MerkleRoot string
	}{
		Index:      index,
		Timestamp:  ts,
		PrevHash:   prevHash,
		MerkleRoot: merkle,
	}
	headerBytes, _ := json.Marshal(header)
	hash := sha256.Sum256(headerBytes)

	// Tanda tangan validator (jika ada)
	var sig string
	if valWallet != nil {
		sigBytes := valWallet.SignEd(hash[:])
		sig = hex.EncodeToString(sigBytes)
	}

	valAddr := "genesis"
	if valWallet != nil {
		valAddr = valWallet.AddressEd
	}

	return Block{
		Index:        index,
		Timestamp:    ts,
		Transactions: txs,
		PrevHash:     prevHash,
		Hash:         hex.EncodeToString(hash[:]),
		MerkleRoot:   merkle,
		Validator:    valAddr,
		ValidatorSig: sig,
		Nonce:        0,
	}
}

// ComputeMerkleRoot hitung root sederhana dari daftar transaksi
func ComputeMerkleRoot(txs []Transaction) string {
	if len(txs) == 0 {
		return ""
	}
	var hashes []string
	for _, tx := range txs {
		h := HashTransaction(tx)
		hashes = append(hashes, h)
	}

	for len(hashes) > 1 {
		var newHashes []string
		for i := 0; i < len(hashes); i += 2 {
			if i+1 < len(hashes) {
				combined := hashes[i] + hashes[i+1]
				h := sha256.Sum256([]byte(combined))
				newHashes = append(newHashes, hex.EncodeToString(h[:]))
			} else {
				newHashes = append(newHashes, hashes[i])
			}
		}
		hashes = newHashes
	}
	return hashes[0]
}

// AddCheckpoint tiap kelipatan 100 block
func AddCheckpoint(block Block) {
	if block.Index%100 == 0 {
		cp := Checkpoint{BlockIndex: block.Index, Hash: block.Hash}
		Checkpoints = append(Checkpoints, cp)
	}
}
