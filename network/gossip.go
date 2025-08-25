package network

import (
	"encoding/json"
	"fmt"

	"github.com/soden46/hyperlux-chain/ledger"
)

func StartGossip() {
	sb, sm := getSubs()
	if sb == nil && sm == nil {
		fmt.Println("🗣️ Gossip (local-only): P2P not active")
		return
	}
	fmt.Println("🗣️ Gossip protocol started")

	go func() {
		if sb == nil { return }
		for {
			msg, err := sb.Next(nil)
			if err != nil { return }
			var blk ledger.Block
			if err := json.Unmarshal(msg.Data, &blk); err != nil {
				continue
			}
			// TODO: verifikasi blok (validator sig, root, dll)
			ledger.Blockchain = append(ledger.Blockchain, blk)
			ledger.SaveBlockchain()
			fmt.Printf("📥 Received block %d from peer (hash=%.12s...)\n", blk.Index, blk.Hash)
		}
	}()

	go func() {
		if sm == nil { return }
		for {
			msg, err := sm.Next(nil)
			if err != nil { return }
			mb, err := decodeMiniBlock(msg.Data)
			if err != nil || !VerifyMiniBlock(mb) {
				continue
			}
			select {
			case miniBlockBus <- mb:
			default:
			}
		}
	}()
}
