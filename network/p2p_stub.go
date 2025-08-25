//go:build !p2p

package network

import (
	"errors"

	"github.com/soden46/hyperlux-chain/ledger"
)

// Stub: no-op P2P so local tests can run without libp2p.

func StartP2P(_ []string) error { return nil }

func BroadcastBlock(_ ledger.Block) error { return nil }

func PublishMiniBlockP2P(_ MiniBlock) error { return nil }

func getSubs() (interface{ Next(interface{}) (*msg, error) }, interface{ Next(interface{}) (*msg, error) }) {
	// dummy to satisfy StartGossip(); we won't use it.
	return nil, nil
}

// minimal type to satisfy interface in stub; not used.
type msg struct {
	Data []byte
}

var ErrP2PDisabled = errors.New("p2p disabled")
