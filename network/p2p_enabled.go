//go:build p2p

package network

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	libp2p "github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	host "github.com/libp2p/go-libp2p/core/host"
	peer "github.com/libp2p/go-libp2p/core/peer"
	quic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/soden46/hyperlux-chain/ledger"
)

var (
	Host        host.Host
	PubSub      *pubsub.PubSub
	TopicBlocks *pubsub.Topic
	TopicMini   *pubsub.Topic

	subBlocks *pubsub.Subscription
	subMini   *pubsub.Subscription
)

const (
	topicBlocks = "hyperlux/blocks/v1"
	topicMini   = "hyperlux/miniblocks/v1"
)

func StartP2P(bootstrap []string) error {
	h, err := libp2p.New(
		libp2p.Transport(quic.NewTransport),
	)
	if err != nil {
		return err
	}
	Host = h

	// expose listen addrs & persist
	for _, a := range h.Addrs() {
		addr := fmt.Sprintf("%s/p2p/%s", a.String(), h.ID().String())
		RegisterPeerRemote(GetNodeID(), GetRole(), addr, GetRole() == RoleBoot)
		fmt.Println("📡 Listening on", addr)
	}

	// connect bootstrap
	for _, bs := range bootstrap {
		_ = connectMultiaddr(h, bs)
	}

	ps, err := pubsub.NewGossipSub(context.Background(), h)
	if err != nil { return err }
	PubSub = ps

	if TopicBlocks, err = ps.Join(topicBlocks); err != nil { return err }
	if TopicMini, err = ps.Join(topicMini); err != nil { return err }

	if subBlocks, err = TopicBlocks.Subscribe(); err != nil { return err }
	if subMini, err = TopicMini.Subscribe(); err != nil { return err }

	return nil
}

func connectMultiaddr(h host.Host, addr string) error {
	maddr, err := ma.NewMultiaddr(addr)
	if err != nil {
		return err
	}
	info, err := peer.AddrInfoFromP2pAddr(maddr)
	if err != nil {
		return err
	}
	if err := h.Connect(context.Background(), *info); err != nil {
		return err
	}
	fmt.Println("🤝 Connected to", addr)
	return nil
}

func BroadcastBlock(block ledger.Block) error {
	if Host == nil || PubSub == nil || TopicBlocks == nil {
		return errors.New("p2p not ready")
	}
	data, _ := json.Marshal(block)
	if err := TopicBlocks.Publish(context.Background(), data); err != nil {
		return err
	}
	fmt.Printf("🌐 Broadcasted block %d (hash=%.12s...)\n", block.Index, block.Hash)
	return nil
}

func PublishMiniBlockP2P(mb MiniBlock) error {
	if Host == nil || PubSub == nil || TopicMini == nil {
		return errors.New("p2p not ready")
	}
	return TopicMini.Publish(context.Background(), encodeMiniBlock(mb))
}

func getSubs() (*pubsub.Subscription, *pubsub.Subscription) {
	return subBlocks, subMini
}
