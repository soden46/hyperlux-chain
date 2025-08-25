package network

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type Peer struct {
	ID   string `json:"id"`
	Addr string `json:"addr"`
	Role Role   `json:"role"`
	Seen int64  `json:"seen"`
}

func ConnectPeer(p Peer) {
	// registry + persist (P2P real akan connect via bootstrap)
	RegisterPeerRemote(p.ID, p.Role, p.Addr, p.Role == RoleBoot)
	fmt.Printf("Connected (registry) to peer %s (%s)\n", p.ID, p.Addr)
}

func ExportPeers(path string) error {
	list := ListPeers()
	b, _ := json.MarshalIndent(list, "", "  ")
	return os.WriteFile(path, b, 0644)
}

func ImportPeers(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var list []Peer
	if err := json.Unmarshal(b, &list); err != nil {
		return err
	}
	for _, p := range list {
		RegisterPeerRemote(p.ID, p.Role, p.Addr, p.Role == RoleBoot)
	}
	return nil
}

func TouchSelfAddr(addr string) {
	regMu.Lock()
	if me, ok := peers[GetNodeID()]; ok {
		me.Addr = addr
		me.Seen = time.Now().Unix()
		peers[GetNodeID()] = me
	}
	regMu.Unlock()
	savePeers()
}
