package network

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/soden46/hyperlux-chain/ledger"
	"github.com/soden46/hyperlux-chain/wallet"
)

type MiniBlock struct {
	Slot       string               `json:"slot"`
	Timestamp  int64                `json:"timestamp"`
	TxList     []ledger.Transaction `json:"tx_list"`
	MerkleRoot string               `json:"merkle_root"`
	ProducerID string               `json:"producer_id"`
	PubKey     string               `json:"pubkey"`
	Signature  string               `json:"signature"`
}

var miniBlockBus = make(chan MiniBlock, 8192)

func SignMiniBlock(w *wallet.Wallet, slot string, txs []ledger.Transaction) (MiniBlock, error) {
	mb := MiniBlock{
		Slot:       slot,
		Timestamp:  time.Now().UnixNano(),
		TxList:     txs,
		MerkleRoot: ledger.ComputeMerkleRoot(txs),
		ProducerID: GetNodeID(),
		PubKey:     hex.EncodeToString(w.PubEd),
	}
	header := fmt.Sprintf("%s|%d|%s|%s|%s", mb.Slot, mb.Timestamp, mb.MerkleRoot, mb.ProducerID, mb.PubKey)
	h := sha256.Sum256([]byte(header))
	sig := w.SignEd(h[:])
	mb.Signature = hex.EncodeToString(sig)
	return mb, nil
}

func VerifyMiniBlock(mb MiniBlock) bool {
	header := fmt.Sprintf("%s|%d|%s|%s|%s", mb.Slot, mb.Timestamp, mb.MerkleRoot, mb.ProducerID, mb.PubKey)
	h := sha256.Sum256([]byte(header))
	pub, err1 := hex.DecodeString(mb.PubKey)
	sig, err2 := hex.DecodeString(mb.Signature)
	if err1 != nil || err2 != nil {
		return false
	}
	return ed25519.Verify(ed25519.PublicKey(pub), h[:], sig)
}

func PublishMiniBlock(mb MiniBlock) {
	select {
	case miniBlockBus <- mb:
	default:
	}
	_ = PublishMiniBlockP2P(mb) // no-op jika P2P dimatikan
}

func CollectMiniBlocks(slot string, timeout time.Duration) []MiniBlock {
	var out []MiniBlock
	deadline := time.After(timeout)
loop:
	for {
		select {
		case mb := <-miniBlockBus:
			if mb.Slot == slot && VerifyMiniBlock(mb) {
				out = append(out, mb)
			}
		case <-deadline:
			break loop
		}
	}
	return out
}

func encodeMiniBlock(mb MiniBlock) []byte {
	b, _ := json.Marshal(mb)
	return b
}

func decodeMiniBlock(b []byte) (MiniBlock, error) {
	var mb MiniBlock
	return mb, json.Unmarshal(b, &mb)
}
