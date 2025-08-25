package wallet

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
)

// Wallet simpan 2 jenis keypair (Ed25519 + secp256k1)
type Wallet struct {
	// Ed25519
	AddressEd string
	PubEd     ed25519.PublicKey
	PrivEd    ed25519.PrivateKey

	// secp256k1
	AddressSec string
	PubSec     []byte
	PrivSec    *secp256k1.PrivateKey
}

// GenerateWallet membuat wallet baru dengan Ed25519 + secp256k1
func GenerateWallet() *Wallet {
	// ===== Ed25519 =====
	pubEd, privEd, _ := ed25519.GenerateKey(nil)
	addrEd := "hlcEd" + hex.EncodeToString(pubEd[:4])

	// ===== secp256k1 =====
	privSec, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		log.Fatal("❌ Gagal generate secp256k1:", err)
	}
	pubSec := privSec.PubKey().SerializeCompressed()

	// buat address dari hash public key
	h := sha256.Sum256(pubSec)
	addrSec := "hlcSec" + hex.EncodeToString(h[:4])

	return &Wallet{
		AddressEd:  addrEd,
		PubEd:      pubEd,
		PrivEd:     privEd,
		AddressSec: addrSec,
		PubSec:     pubSec,
		PrivSec:    privSec,
	}
}

// SaveToFile menyimpan wallet ke JSON (raw, tanpa enkripsi)
func (w *Wallet) SaveToFile(filename string) error {
	data := map[string]string{
		"address_ed":  w.AddressEd,
		"pub_ed":      hex.EncodeToString(w.PubEd),
		"priv_ed":     hex.EncodeToString(w.PrivEd),
		"address_sec": w.AddressSec,
		"pub_sec":     hex.EncodeToString(w.PubSec),
		"priv_sec":    hex.EncodeToString(w.PrivSec.Serialize()),
	}
	file, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filename, file, 0600)
}

// LoadWallet membuka wallet dari file JSON
func LoadWallet(filename string) (*Wallet, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}

	privEd, _ := hex.DecodeString(m["priv_ed"])
	pubEd, _ := hex.DecodeString(m["pub_ed"])
	privSecBytes, _ := hex.DecodeString(m["priv_sec"])
	privSec := secp256k1.PrivKeyFromBytes(privSecBytes)
	pubSec, _ := hex.DecodeString(m["pub_sec"])

	return &Wallet{
		AddressEd:  m["address_ed"],
		PubEd:      ed25519.PublicKey(pubEd),
		PrivEd:     ed25519.PrivateKey(privEd),
		AddressSec: m["address_sec"],
		PubSec:     pubSec,
		PrivSec:    privSec,
	}, nil
}

// SignEd menandatangani data pakai Ed25519
func (w *Wallet) SignEd(data []byte) []byte {
	return ed25519.Sign(w.PrivEd, data)
}

// VerifyEd verifikasi tanda tangan Ed25519
func VerifyEd(pub ed25519.PublicKey, data []byte, sig []byte) bool {
	return ed25519.Verify(pub, data, sig)
}

// SignSec menandatangani data pakai secp256k1 (ECDSA)
func (w *Wallet) SignSec(data []byte) []byte {
	sig := ecdsa.Sign(w.PrivSec, data)
	return sig.Serialize()
}

// Print detail wallet
func (w *Wallet) Print() {
	fmt.Println("🔑 Ed25519 Address :", w.AddressEd)
	fmt.Println("🔑 secp256k1 Address:", w.AddressSec)
}
