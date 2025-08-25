package wallet

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

// Struktur keystore JSON
type Keystore struct {
	// Ed25519
	AddressEd string `json:"address_ed"`
	CryptoEd  string `json:"crypto_ed"`
	NonceEd   string `json:"nonce_ed"`

	// secp256k1
	AddressSec string `json:"address_sec"`
	CryptoSec  string `json:"crypto_sec"`
	NonceSec   string `json:"nonce_sec"`
}

// ================== internal encrypt/decrypt ==================

// encrypt private key dengan password (AES-256-GCM)
func encryptPrivateKey(privKey []byte, password string) (cipherText, nonce []byte, err error) {
	key := sha256.Sum256([]byte(password)) // derive key dari password
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	nonce = make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}
	cipherText = gcm.Seal(nil, nonce, privKey, nil)
	return cipherText, nonce, nil
}

// decrypt private key dengan password
func decryptPrivateKey(cipherText, nonce []byte, password string) ([]byte, error) {
	key := sha256.Sum256([]byte(password))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plain, err := gcm.Open(nil, nonce, cipherText, nil)
	if err != nil {
		return nil, err
	}
	return plain, nil
}

// ================== public API ==================

// SaveKeystore menyimpan wallet ke file dengan password
func (w *Wallet) SaveKeystore(filename, password string) error {
	// encrypt Ed25519
	cipherEd, nonceEd, err := encryptPrivateKey(w.PrivEd, password)
	if err != nil {
		return err
	}

	// encrypt secp256k1
	cipherSec, nonceSec, err := encryptPrivateKey(w.PrivSec.Serialize(), password)
	if err != nil {
		return err
	}

	ks := Keystore{
		AddressEd:  w.AddressEd,
		CryptoEd:   hex.EncodeToString(cipherEd),
		NonceEd:    hex.EncodeToString(nonceEd),

		AddressSec: w.AddressSec,
		CryptoSec:  hex.EncodeToString(cipherSec),
		NonceSec:   hex.EncodeToString(nonceSec),
	}

	data, err := json.MarshalIndent(ks, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filename, data, 0600)
}

// LoadKeystore membuka wallet dari file keystore dengan password
func LoadKeystore(filename, password string) (*Wallet, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var ks Keystore
	if err := json.Unmarshal(data, &ks); err != nil {
		return nil, err
	}

	// decrypt Ed25519
	cipherEd, _ := hex.DecodeString(ks.CryptoEd)
	nonceEd, _ := hex.DecodeString(ks.NonceEd)
	privEd, err := decryptPrivateKey(cipherEd, nonceEd, password)
	if err != nil {
		return nil, fmt.Errorf("invalid password for Ed25519")
	}

	// decrypt secp256k1
	cipherSec, _ := hex.DecodeString(ks.CryptoSec)
	nonceSec, _ := hex.DecodeString(ks.NonceSec)
	privSecBytes, err := decryptPrivateKey(cipherSec, nonceSec, password)
	if err != nil {
		return nil, fmt.Errorf("invalid password for secp256k1")
	}
	privSec := secp256k1.PrivKeyFromBytes(privSecBytes)
	pubSec := privSec.PubKey().SerializeCompressed()

	// kalau AddressSec kosong di file, generate lagi dari pubSec
	addrSec := ks.AddressSec
	if addrSec == "" {
		h := sha256.Sum256(pubSec)
		addrSec = "hlcSec" + hex.EncodeToString(h[:4])
	}

	// reconstruct wallet dengan dua keypair
	return &Wallet{
		AddressEd:  ks.AddressEd,
		PrivEd:     privEd,
		PubEd:      privEd[32:], // Ed25519: pubKey = last 32 bytes
		AddressSec: addrSec,
		PrivSec:    privSec,
		PubSec:     pubSec,
	}, nil
}
