package execution

import "fmt"

// Saldo akun
var Balances = map[string]int{}

func InitExecution() {
	fmt.Println("Execution layer ready")
	Balances["Alice"] = 100
	Balances["Bob"] = 50
}

func ExecuteTransfer(from string, to string, amount int) bool {
	if Balances[from] >= amount {
		Balances[from] -= amount
		Balances[to] += amount
		fmt.Printf("Executed transfer: %s -> %s : %d\n", from, to, amount)
		return true
	}
	fmt.Println("Transfer failed: insufficient balance")
	return false
}
