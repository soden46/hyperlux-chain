package ledger

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/soden46/hyperlux-chain/wallet"
)

// ================== Data & Registry ==================

type ValidatorDef struct {
	Address string `json:"address"`
	Stake   int    `json:"stake"`
}

var (
	Validators       []ValidatorDef
	ValidatorWallets = map[string]*wallet.Wallet{}
)

const validatorsDBFile = "validators.json"

// ================== Wallet loading helpers ==================

func AutoLoadValidatorWallets() {
	loaded := 0
	for _, v := range Validators {
		path := filepath.Join("validators", v.Address+".json")
		if _, err := os.Stat(path); err == nil {
			if w, err := wallet.LoadWallet(path); err == nil {
				ValidatorWallets[v.Address] = w
				fmt.Printf("✅ %s sinkron (validators/%s.json)\n", v.Address, v.Address)
				loaded++
				continue
			}
		}
		// Fallback scan
		files, _ := os.ReadDir("validators")
		found := false
		for _, f := range files {
			if f.IsDir() || filepath.Ext(f.Name()) != ".json" {
				continue
			}
			w, err := wallet.LoadWallet(filepath.Join("validators", f.Name()))
			if err == nil && w.AddressEd == v.Address {
				ValidatorWallets[v.Address] = w
				fmt.Printf("✅ %s sinkron (validators/%s)\n", v.Address, f.Name())
				found = true
				loaded++
				break
			}
		}
		if !found {
			fmt.Printf("⚠️ Wallet untuk validator %s tidak ditemukan di validators/\n", v.Address)
		}
	}
	if loaded == 0 {
		fmt.Println("⚠️ Tidak ada wallet validator yang berhasil dimuat")
	}
}

// ================== Load/Save Validators ==================

func LoadValidators() {
	data, err := os.ReadFile(validatorsDBFile)
	if err != nil {
		return
	}
	var list []ValidatorDef
	if err := json.Unmarshal(data, &list); err != nil {
		fmt.Println("⚠️ LoadValidators warn:", err)
		return
	}
	Validators = list
}

func SaveValidators() {
	b, err := json.MarshalIndent(Validators, "", "  ")
	if err != nil {
		fmt.Println("⚠️ SaveValidators warn:", err)
		return
	}
	_ = os.WriteFile(validatorsDBFile, b, 0644)
}

// ================== Status / Suspension ==================

func getOrCreateRuntime(addr string) *ValidatorRuntime {
	ValidatorStatusMu.Lock()
	defer ValidatorStatusMu.Unlock()
	rt, ok := ValidatorStatus[addr]
	if !ok {
		rt = &ValidatorRuntime{}
		ValidatorStatus[addr] = rt
	}
	return rt
}

func IsSuspended(addr string, need SuspensionScope) bool {
	ValidatorStatusMu.RLock()
	rt, ok := ValidatorStatus[addr]
	ValidatorStatusMu.RUnlock()
	if !ok || rt == nil {
		return false
	}
	if nowUnix() >= rt.SuspendedUntil {
		return false
	}
	if rt.SuspendScope == ScopeAll {
		return true
	}
	if need == ScopePropose && (rt.SuspendScope == ScopePropose) {
		return true
	}
	if need == ScopeVote && (rt.SuspendScope == ScopeVote) {
		return true
	}
	return false
}

func SuspendValidator(addr string, scope SuspensionScope, dur time.Duration) {
	rt := getOrCreateRuntime(addr)
	ValidatorStatusMu.Lock()
	rt.SuspendedUntil = time.Now().Add(dur).Unix()
	rt.SuspendScope = scope
	ValidatorStatusMu.Unlock()
	fmt.Printf("⏸️ Validator %s suspended scope=%d until=%d\n", addr, scope, rt.SuspendedUntil)
}

// ================== Slashing & Distribution ==================

type SlashKind int

const (
	SlashKindDowntime SlashKind = 1
	SlashKindSafety   SlashKind = 2 // double-sign, invalid block, dsb.
)

type SlashParams struct {
	// Absolute amount (token) to slash. Jika 0 dan Percent>0 → gunakan persentase stake.
	Amount  int
	Percent float64 // e.g., 0.0001 = 0.01%

	// Distribusi (default 70/15/10/5 untuk safety fault)
	BurnPct     float64 // 0.70
	TreasuryPct float64 // 0.15
	WhistlePct  float64 // 0.10
	HonestPct   float64 // 0.05

	Reporter       string    // optional (penerima whistle reward); jika kosong → masuk treasury
	Kind           SlashKind // Downtime / Safety
	CorrelationMul float64   // multiplier jika koralitif (e.g., 1.0..3.0)

	// Suspension
	SuspendScope SuspensionScope
	SuspendFor   time.Duration
}

func defaultSafetyPolicy() SlashParams {
	return SlashParams{
		Amount:         0,
		Percent:        0,
		BurnPct:        0.70,
		TreasuryPct:    0.15,
		WhistlePct:     0.10,
		HonestPct:      0.05,
		Reporter:       "",
		Kind:           SlashKindSafety,
		CorrelationMul: 1.0,
		SuspendScope:   ScopeAll,
		SuspendFor:     24 * time.Hour,
	}
}

func defaultDowntimePolicy() SlashParams {
	return SlashParams{
		Amount:         0,
		Percent:        0.0001, // 0.01% stake
		BurnPct:        1.0,    // burn semua
		TreasuryPct:    0.0,
		WhistlePct:     0.0,
		HonestPct:      0.0,
		Reporter:       "",
		Kind:           SlashKindDowntime,
		CorrelationMul: 1.0,
		SuspendScope:   ScopePropose,
		SuspendFor:     5 * time.Minute,
	}
}

func findValidator(addr string) (idx int, ok bool) {
	for i := range Validators {
		if Validators[i].Address == addr {
			return i, true
		}
	}
	return -1, false
}

func slashSingle(addr string, amt int) int {
	if amt <= 0 {
		return 0
	}
	i, ok := findValidator(addr)
	if !ok {
		return 0
	}
	if Validators[i].Stake < amt {
		amt = Validators[i].Stake
	}
	Validators[i].Stake -= amt
	return amt
}

func distributeSlashed(total int, reporter string, offender string) {
	if total <= 0 {
		return
	}
	// 70% burn, 15% treasury, 10% whistle, 5% honest
	burn := int(float64(total) * 0.70)
	trea := int(float64(total) * 0.15)
	whis := int(float64(total) * 0.10)
	hon := total - burn - trea - whis // residu → honest

	// update sinks
	BurnedSupply += burn
	TreasuryBalance += trea

	// whistle
	if reporter != "" && whis > 0 {
		BalanceMu.Lock()
		Balances[reporter] += whis
		BalanceMu.Unlock()
	} else {
		// jika tidak ada reporter, masuk treasury
		TreasuryBalance += whis
		whis = 0
	}

	// honest redistribution pro-rata stake (kecuali offender)
	if hon > 0 {
		totalStake := 0
		for _, v := range Validators {
			if v.Address == offender {
				continue
			}
			totalStake += v.Stake
		}
		if totalStake > 0 {
			BalanceMu.Lock()
			for _, v := range Validators {
				if v.Address == offender {
					continue
				}
				share := (hon * v.Stake) / totalStake
				if share > 0 {
					Balances[v.Address] += share
				}
			}
			BalanceMu.Unlock()
		} else {
			// fallback → treasury
			TreasuryBalance += hon
		}
	}

	fmt.Printf("💥 Slashed=%d | burn=%d treasury=%d whistle=%d honest=%d\n", total, burn, trea, whis, hon)
}

// === Public helpers (HANYA SATU DEFINISI) ===

func SlashDowntime(addr string) {
	p := defaultDowntimePolicy()
	ApplySlash(addr, p, "")
}

func SlashSafetyFault(addr string, amount int, reporter string, correlationMul float64) {
	p := defaultSafetyPolicy()
	p.Amount = amount
	if correlationMul > 0 {
		p.CorrelationMul = correlationMul
	}
	ApplySlash(addr, p, reporter)
}

func SlashValidator(addr string, amount int) {
	SlashSafetyFault(addr, amount, "", 1.0)
}

func SlashCluster(mainAddr, subAddr string, totalAmount int, reporter string) {
	if totalAmount <= 0 {
		return
	}
	mainAmt := (totalAmount * 40) / 100
	subAmt := totalAmount - mainAmt

	// sub
	if subAmt > 0 && subAddr != "" {
		SlashSafetyFault(subAddr, subAmt, reporter, 1.0)
	}
	// main
	if mainAmt > 0 && mainAddr != "" {
		SlashSafetyFault(mainAddr, mainAmt, reporter, 1.0)
	}
}

// core slashing executor
func ApplySlash(offender string, params SlashParams, reporter string) {
	// resolve amount
	amt := params.Amount
	if amt <= 0 && params.Percent > 0 {
		if i, ok := findValidator(offender); ok {
			amt = int(float64(Validators[i].Stake) * params.Percent)
			if amt <= 0 && Validators[i].Stake > 0 {
				amt = 1 // minimal 1 token
			}
		}
	}
	if params.CorrelationMul > 0 && params.CorrelationMul != 1.0 {
		amt = int(float64(amt) * params.CorrelationMul)
	}
	if amt <= 0 {
		return
	}

	// apply slash (mutate stake)
	actual := slashSingle(offender, amt)
	if actual <= 0 {
		return
	}
	fmt.Printf("⛔ Validator %s slashed %d (kind=%d)\n", offender, actual, params.Kind)

	// distribution by policy
	switch params.Kind {
	case SlashKindDowntime:
		// burn all
		BurnedSupply += actual
	default:
		// 70/15/10/5
		distributeSlashed(actual, reporter, offender)
	}

	// suspension
	if params.SuspendFor > 0 && params.SuspendScope != ScopeNone {
		SuspendValidator(offender, params.SuspendScope, params.SuspendFor)
	}

	SaveValidators()
}

// ================== Fix/Init Validators ==================

func FixValidators() {
	_ = os.MkdirAll("validators", 0o755)

	// coba load dari folder jika DB kosong (LoadValidators dipanggil di luar)
	if len(Validators) == 0 {
		files, _ := os.ReadDir("validators")
		for _, f := range files {
			if f.IsDir() || filepath.Ext(f.Name()) != ".json" {
				continue
			}
			w, err := wallet.LoadWallet(filepath.Join("validators", f.Name()))
			if err == nil {
				Validators = append(Validators, ValidatorDef{
					Address: w.AddressEd,
					Stake:   100000,
				})
				fmt.Printf("✅ %s terdaftar (import dari %s)\n", w.AddressEd, f.Name())
			}
		}
	}

	// Jika tetap kosong → generate default N
	if len(Validators) == 0 {
		const N = 6
		for i := 0; i < N; i++ {
			w := wallet.GenerateWallet()
			filename := filepath.Join("validators", w.AddressEd+".json")
			if err := w.SaveToFile(filename); err == nil {
				Validators = append(Validators, ValidatorDef{
					Address: w.AddressEd,
					Stake:   100000,
				})
				fmt.Printf("✅ %s dibuat & terdaftar (validators/%s.json)\n", w.AddressEd, w.AddressEd)
			}
		}
	}

	SaveValidators()
	AutoLoadValidatorWallets()
}

// Utility export
func ExportValidatorsJSON() []byte {
	b, _ := json.MarshalIndent(Validators, "", "  ")
	return b
}
