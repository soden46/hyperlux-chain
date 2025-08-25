package ledger

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"runtime"
	"sort"

	"github.com/soden46/hyperlux-chain/wallet"
)

// ===================== Data Types =====================

type Transaction struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Amount    int    `json:"amount"`
	Fee       int    `json:"fee"`
	Nonce     int    `json:"nonce"`
	Signature string `json:"signature"`
	PubKey    string `json:"pubkey"`
}

// ===================== TX Construction =====================

// NewTransaction → buat TX baru (fee super murah berbasis ukuran, mirip Solana)
func NewTransaction(w *wallet.Wallet, to string, amount int) Transaction {
	nonce := GetNextNonce(w.AddressEd)

	data := fmt.Sprintf("%s|%s|%d|%d", w.AddressEd, to, amount, nonce)
	sig := w.SignEd([]byte(data))

	tx := Transaction{
		From:      w.AddressEd,
		To:        to,
		Amount:    amount,
		Nonce:     nonce,
		Signature: hex.EncodeToString(sig),
		PubKey:    hex.EncodeToString(w.PubEd),
	}
	tx.Fee = CalculateFee(tx)
	return tx
}

// GetNextNonce → baca nonce (thread-safe)
func GetNextNonce(addr string) int {
	NonceTableMu.RLock()
	defer NonceTableMu.RUnlock()
	return NonceTable[addr] + 1
}

// ===================== Mempool Ops (ingest) =====================

// ValidateAndAddToMempool → validasi dasar, masukkan ke mempool.
// Tidak mengubah Balances/NonceTable: itu dilakukan saat block commit.
func ValidateAndAddToMempool(tx Transaction) error {
	// Snapshot cepat untuk expected nonce & balance
	NonceTableMu.RLock()
	expected := NonceTable[tx.From] + 1
	NonceTableMu.RUnlock()

	BalanceMu.RLock()
	balance := Balances[tx.From]
	BalanceMu.RUnlock()

	if tx.Nonce != expected {
		return fmt.Errorf("❌ invalid nonce (expected %d, got %d)", expected, tx.Nonce)
	}
	if balance < tx.Amount+tx.Fee {
		return fmt.Errorf("❌ insufficient balance")
	}
	if !VerifyTransaction(tx) {
		return fmt.Errorf("❌ invalid signature")
	}

	MempoolMu.Lock()
	Mempool = append(Mempool, tx)
	MempoolMu.Unlock()
	return nil
}

// ===================== Mempool Helpers (untuk consensus) =====================

// MempoolSnapshot → salin seluruh mempool saat ini (thread-safe)
func MempoolSnapshot() []Transaction {
	MempoolMu.RLock()
	defer MempoolMu.RUnlock()
	out := make([]Transaction, len(Mempool))
	copy(out, Mempool)
	return out
}

// RemoveCommittedFromMempool → hapus TX yang sudah masuk block dari mempool
func RemoveCommittedFromMempool(committed []Transaction) {
	if len(committed) == 0 {
		return
	}
	// Buat set hash TX yang masuk block
	toDelete := make(map[string]struct{}, len(committed))
	for _, tx := range committed {
		toDelete[HashTransaction(tx)] = struct{}{}
	}

	MempoolMu.Lock()
	defer MempoolMu.Unlock()

	if len(Mempool) == 0 {
		return
	}
	filtered := Mempool[:0]
	for _, tx := range Mempool {
		if _, ok := toDelete[HashTransaction(tx)]; !ok {
			filtered = append(filtered, tx)
		}
	}
	// shrink jika perlu
	Mempool = filtered
}

// ===================== Batch Processing (untuk consensus) =====================

// ProcessTxListParallel → validasi batch (partisi by sender, anti konflik nonce),
// eksekusi paralel, lalu COMMIT ke state global SEKALI di akhir.
// Return daftar TX yang benar-benar valid untuk dimasukkan ke block.
func ProcessTxListParallel(txs []Transaction) []Transaction {
	if len(txs) == 0 {
		return []Transaction{}
	}

	// Partisi by sender → minim konflik nonce
	partitions := map[string][]Transaction{}
	for _, tx := range txs {
		partitions[tx.From] = append(partitions[tx.From], tx)
	}
	// Urutkan tiap partisi by nonce ascending
	for sender := range partitions {
		sort.Slice(partitions[sender], func(i, j int) bool {
			return partitions[sender][i].Nonce < partitions[sender][j].Nonce
		})
	}

	// Snapshot nonce sender & saldo sender (yang dibutuhkan untuk debit)
	nonceSnap := make(map[string]int, len(partitions))
	balSnap := make(map[string]int, len(partitions))

	NonceTableMu.RLock()
	for sender := range partitions {
		nonceSnap[sender] = NonceTable[sender]
	}
	NonceTableMu.RUnlock()

	BalanceMu.RLock()
	for sender := range partitions {
		balSnap[sender] = Balances[sender]
	}
	BalanceMu.RUnlock()

	// Worker pool dinamis
	numWorkers := runtime.NumCPU()
	if numWorkers > len(partitions) {
		numWorkers = len(partitions)
	}
	if numWorkers < 1 {
		numWorkers = 1
	}

	type job struct {
		sender string
		txs    []Transaction
	}
	jobs := make(chan job, len(partitions))
	out := make(chan []Transaction, len(partitions))

	for sender, list := range partitions {
		jobs <- job{sender: sender, txs: list}
	}
	close(jobs)

	worker := func() {
		for j := range jobs {
			accepted := make([]Transaction, 0, len(j.txs))
			localNonce := nonceSnap[j.sender]
			localBal := balSnap[j.sender]

			for _, tx := range j.txs {
				// Nonce harus berurutan (monotonik per pengirim)
				if tx.Nonce != localNonce+1 {
					break
				}
				// Verifikasi tanda tangan
				if !VerifyTransaction(tx) {
					break
				}
				// Cek cukup saldo (debit + fee)
				cost := tx.Amount + tx.Fee
				if localBal < cost {
					break
				}
				// terima & update local view
				localBal -= cost
				localNonce = tx.Nonce
				accepted = append(accepted, tx)
			}
			out <- accepted
		}
	}

	for i := 0; i < numWorkers; i++ {
		go worker()
	}

	final := make([]Transaction, 0, len(txs))
	for i := 0; i < len(partitions); i++ {
		accepted := <-out
		if len(accepted) > 0 {
			final = append(final, accepted...)
		}
	}
	close(out)

	// Commit ke state global SEKALI (minimize contention)
	if len(final) > 0 {
		BalanceMu.Lock()
		NonceTableMu.Lock()
		for _, tx := range final {
			Balances[tx.From] -= tx.Amount + tx.Fee
			Balances[tx.To] += tx.Amount
			NonceTable[tx.From] = tx.Nonce
		}
		NonceTableMu.Unlock()
		BalanceMu.Unlock()
	}

	return final
}

// ProcessMempoolParallel → wrapper nyaman untuk memproses snapshot mempool
func ProcessMempoolParallel() []Transaction {
	snap := MempoolSnapshot()
	return ProcessTxListParallel(snap)
}

// ClearMempool → reset penuh (biasanya tidak perlu jika kita selective remove)
func ClearMempool() {
	MempoolMu.Lock()
	Mempool = []Transaction{}
	MempoolMu.Unlock()
}

// ===================== Utils =====================

func CalculateFee(tx Transaction) int {
	b, _ := json.Marshal(tx)
	size := len(b)
	feePerByte := 1 // super murah, bisa dituning sesuai ekonomi jaringan
	return size * feePerByte
}

func VerifyTransaction(tx Transaction) bool {
	data := fmt.Sprintf("%s|%s|%d|%d", tx.From, tx.To, tx.Amount, tx.Nonce)
	pubBytes, err1 := hex.DecodeString(tx.PubKey)
	sigBytes, err2 := hex.DecodeString(tx.Signature)
	if err1 != nil || err2 != nil {
		return false
	}
	return ed25519.Verify(ed25519.PublicKey(pubBytes), []byte(data), sigBytes)
}

func EncodeTransaction(tx Transaction) []byte {
	b, _ := json.Marshal(tx)
	return b
}

func DecodeTransaction(data []byte) (Transaction, error) {
	var tx Transaction
	err := json.Unmarshal(data, &tx)
	return tx, err
}

func HashTransaction(tx Transaction) string {
	data := fmt.Sprintf("%s|%s|%d|%d|%s",
		tx.From, tx.To, tx.Amount, tx.Nonce, tx.Signature)
	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:])
}
