package crypto

import (
	"crypto/sha256"
	"encoding/hex"
)

func Hash(data string) string {
	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:])
}

// TODO: implement Ed25519 signatures, Merkle trees
