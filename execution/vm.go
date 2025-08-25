package execution

import "fmt"

// Virtual Machine sederhana untuk eksekusi smart contract
type VM struct {
	Code string
}

func NewVM(code string) *VM {
	return &VM{Code: code}
}

func (vm *VM) Run() {
	// untuk sekarang cukup dummy saja
	fmt.Println("VM executing code:", vm.Code)
}
