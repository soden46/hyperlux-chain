package ledger

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/soden46/hyperlux-chain/wallet"
)

// Definisi validator untuk konsensus DPoH/DPoS
type ValidatorDef struct {
	Address string `json:"address"`
	Stake   int    `json:"stake"`
}

// Daftar validator aktif
var Validators []ValidatorDef

// Peta address → wallet validator (untuk tanda tangan blok atau mini-block)
var ValidatorWallets = map[string]*wallet.Wallet{}

// AutoLoadValidatorWallets memetakan Validators ke file wallet di folder validators/
func AutoLoadValidatorWallets() {
	loaded := 0
	// Coba muat berdasarkan nama file <address>.json
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
		// Fallback: scan semua file & cari address yang cocok
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

// SlashValidator mengurangi stake (mis. untuk equivocation)
func SlashValidator(addr string, amount int) {
	for i := range Validators {
		if Validators[i].Address == addr {
			if Validators[i].Stake < amount {
				Validators[i].Stake = 0
			} else {
				Validators[i].Stake -= amount
			}
			fmt.Printf("⛔ Validator %s di-slashed %d, stake sekarang=%d\n",
				addr, amount, Validators[i].Stake)
			SaveValidators()
			return
		}
	}
}

// FixValidators memastikan ada validator + file wallet di folder validators/
// - Jika kosong, generate beberapa validator default
// - Simpan daftar Validators ke DB
func FixValidators() {
	_ = os.MkdirAll("validators", 0o755)

	// Jika DB belum punya daftar validator, coba load dari folder
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
					Stake:   100000, // default stake
				})
				fmt.Printf("✅ %s terdaftar (import dari %s)\n", w.AddressEd, f.Name())
			}
		}
	}

	// Jika tetap kosong, generate N wallet validator
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

	// Persist daftar validator
	SaveValidators()

	// Sinkronkan peta address → wallet
	AutoLoadValidatorWallets()
}

// Utility: export validators ke JSON (opsional untuk tooling)
func ExportValidatorsJSON() []byte {
	b, _ := json.MarshalIndent(Validators, "", "  ")
	return b
}
