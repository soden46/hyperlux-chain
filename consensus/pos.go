package consensus

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/soden46/hyperlux-chain/ledger"
)

// InitPoS → load validator dari ledger
func InitPoS() {
	ledger.LoadValidators()
	fmt.Println("⚡ Proof of Stake module initialized")
	fmt.Printf("✅ PoS initialized with %d validators\n", len(ledger.Validators))
}

// SelectValidator → pilih validator probabilistik berdasarkan stake
func SelectValidator() ledger.ValidatorDef {
	totalStake := 0
	for _, v := range ledger.Validators {
		totalStake += v.Stake
	}

	if totalStake == 0 {
		fmt.Println("⚠️ No validators registered")
		return ledger.ValidatorDef{}
	}

	rand.Seed(time.Now().UnixNano())
	r := rand.Intn(totalStake)

	sum := 0
	for _, v := range ledger.Validators {
		sum += v.Stake
		if r < sum {
			return v
		}
	}
	return ledger.Validators[0]
}
