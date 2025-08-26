package ledger

import "fmt"

// InitLedger: init storage + load state + buat genesis jika kosong
func InitLedger() {
	InitDB()
	LoadBalances()
	LoadBlockchain()
	LoadMempool()
	LoadNonceTable()
	LoadValidators() // didefinisikan di validator.go

	if len(Blockchain) == 0 {
		genesis := NewBlock(0, []Transaction{}, "0", nil)
		Blockchain = append(Blockchain, genesis)
		SaveBlockchain()
		fmt.Println("✅ Genesis block created")
	} else {
		fmt.Println("✅ Storage engine initialized")
	}
}

// Utilities

func GetBalance(addr string) int {
	BalanceMu.RLock()
	defer BalanceMu.RUnlock()
	return Balances[addr]
}

func Airdrop(addr string, amount int) {
	BalanceMu.Lock()
	Balances[addr] += amount
	BalanceMu.Unlock()
	SaveBalances()
	fmt.Printf("💸 Airdropped %d HYLUX to %s\n", amount, addr)
}

// Load/Save everything

func LoadAllData() {
	LoadBalances()
	LoadBlockchain()
	LoadMempool()
	LoadNonceTable()
	LoadValidators()
}

func SaveAllData() {
	SaveBalances()
	SaveBlockchain()
	SaveMempool()
	SaveNonceTable()
	SaveValidators()
}
