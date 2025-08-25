package network

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/soden46/hyperlux-chain/ledger"
)

// ===== Roles =====

type Role string

const (
	RoleBoot   Role = "boot"
	RolePublic Role = "public"
	RoleMain   Role = "main"
	RoleSub    Role = "sub"
)

// ===== Peer registry (persisted) =====

type PeerInfo struct {
	ID   string `json:"id"`
	Role Role   `json:"role"`
	Addr string `json:"addr,omitempty"` // multiaddr, jika ada
	Seen int64  `json:"seen"`
	Boot bool   `json:"boot"`
}

var (
	nodeID      string
	currentRole Role = RolePublic

	regMu    sync.RWMutex
	peers             = map[string]PeerInfo{} // by ID
	peersFile         = "peers.json"
)

// ===== TX Ingress partitioning =====

var (
	shardCh     []chan ledger.Transaction
	numShards   int
	ingressOnce sync.Once

	ingressAccepted uint64
	ingressDropped  uint64
)

// ===== QoS lanes (token bucket) =====

type tokenBucket struct {
	mu      sync.Mutex
	tokens  float64
	lastRef time.Time
	rate    float64 // tokens per second
	burst   float64
}

func newBucket(rate, burst float64) *tokenBucket {
	return &tokenBucket{rate: rate, burst: burst, tokens: burst, lastRef: time.Now()}
}

func (b *tokenBucket) allow(weight float64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(b.lastRef).Seconds()
	b.lastRef = now
	b.tokens += elapsed * b.rate * weight
	max := b.burst * weight
	if b.tokens > max {
		b.tokens = max
	}
	if b.tokens < 1.0 {
		return false
	}
	b.tokens -= 1.0
	return true
}

// Lanes: fast / normal / slow
const (
	feeHigh = 5000
	feeMid  = 1200
)

var (
	bucketFast   = newBucket(4000, 8000)
	bucketNormal = newBucket(1800, 3600)
	bucketSlow   = newBucket(600, 1200)
)

// ===== Public helpers =====

func GetRole() Role     { return currentRole }
func GetNodeID() string { return nodeID }

func ListPeers() []PeerInfo {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]PeerInfo, 0, len(peers))
	for _, p := range peers {
		out = append(out, p)
	}
	return out
}

// ===== Init =====

func InitNetwork() {
	// Node ID
	if nodeID == "" {
		buf := make([]byte, 6)
		_, _ = rand.Read(buf)
		nodeID = "node-" + hex.EncodeToString(buf)
	}

	// Role via env
	switch strings.ToLower(os.Getenv("HYPERLUX_ROLE")) {
	case "boot":
		currentRole = RoleBoot
	case "main":
		currentRole = RoleMain
	case "sub":
		currentRole = RoleSub
	default:
		currentRole = RolePublic
	}

	// Load peer list (persisted)
	loadPeers()

	// Register self (and persist)
	registerSelf("", currentRole, currentRole == RoleBoot)

	// Start P2P (QUIC + GossipSub) — fallback ke stub bila tidak dibuild dengan tag p2p
	bootAddrs := getBootstrapAddrs()
	if err := StartP2P(bootAddrs); err != nil {
		fmt.Println("⚠️ P2P disabled (fallback in-memory):", err)
	} else {
		fmt.Printf("🌐 P2P online with %d bootstrap addrs\n", len(bootAddrs))
	}

	// Start gossip handlers (akan local-only bila P2P tidak aktif)
	StartGossip()

	// Start partitioned TX ingress workers
	startIngress()

	fmt.Printf("🌐 Network initialized: id=%s role=%s\n", nodeID, currentRole)
}

// ===== Peer persistence =====

func loadPeers() {
	data, err := os.ReadFile(peersFile)
	if err != nil {
		return
	}
	var list []PeerInfo
	if json.Unmarshal(data, &list) == nil {
		regMu.Lock()
		defer regMu.Unlock()
		for _, p := range list {
			peers[p.ID] = p
		}
	}
}

func savePeers() {
	regMu.RLock()
	defer regMu.RUnlock()
	list := make([]PeerInfo, 0, len(peers))
	for _, p := range peers {
		list = append(list, p)
	}
	b, _ := json.MarshalIndent(list, "", "  ")
	_ = os.WriteFile(peersFile, b, 0644)
}

func registerSelf(addr string, role Role, boot bool) {
	regMu.Lock()
	peers[nodeID] = PeerInfo{ID: nodeID, Role: role, Addr: addr, Seen: time.Now().Unix(), Boot: boot}
	regMu.Unlock()
	savePeers()
}

func RegisterPeerRemote(id string, role Role, addr string, boot bool) {
	regMu.Lock()
	peers[id] = PeerInfo{ID: id, Role: role, Addr: addr, Seen: time.Now().Unix(), Boot: boot}
	regMu.Unlock()
	savePeers()
}

func getBootstrapAddrs() []string {
	if env := os.Getenv("HYPERLUX_BOOTSTRAP"); env != "" {
		parts := strings.Split(env, ",")
		out := make([]string, 0, len(parts))
		for _, s := range parts {
			if trimmed := strings.TrimSpace(s); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	regMu.RLock()
	defer regMu.RUnlock()
	var out []string
	for _, p := range peers {
		if p.Boot && p.Addr != "" {
			out = append(out, p.Addr)
		}
	}
	return out
}

// ===== TX ingress partitioning =====

func startIngress() {
	ingressOnce.Do(func() {
		ncpu := runtime.NumCPU()
		if ncpu < 2 {
			ncpu = 2
		}
		numShards = ncpu * 2

		shardCh = make([]chan ledger.Transaction, numShards)
		for i := 0; i < numShards; i++ {
			ch := make(chan ledger.Transaction, 4096)
			shardCh[i] = ch
			// single-threaded per shard → minim konflik nonce by sender
			go func(in <-chan ledger.Transaction) {
				for tx := range in {
					_ = ledger.ValidateAndAddToMempool(tx)
				}
			}(ch)
		}
		fmt.Printf("🔀 TX ingress started with %d shards (≈ %d cores)\n", numShards, ncpu)
	})
}

func shardFor(key string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return int(h.Sum32()) % numShards
}

// ===== QoS / Gateway =====

func isValidator(addr string) (bool, int) {
	for _, v := range ledger.Validators {
		if v.Address == addr {
			return true, v.Stake
		}
	}
	return false, 0
}

func laneFor(tx ledger.Transaction) (bucket *tokenBucket, weight float64) {
	if ok, stake := isValidator(tx.From); ok {
		w := 1.0 + float64(stake)/10000.0
		if w > 4 {
			w = 4
		}
		return bucketFast, w
	}
	if tx.Fee >= feeHigh {
		return bucketFast, 1.0
	}
	if tx.Fee >= feeMid {
		return bucketNormal, 1.0
	}
	return bucketSlow, 1.0
}

func GatewayAcceptTx(tx ledger.Transaction) error {
	// Non-public node → langsung antrikan
	if currentRole != RolePublic {
		enqueueShard(tx)
		atomic.AddUint64(&ingressAccepted, 1)
		return nil
	}
	b, w := laneFor(tx)
	if !b.allow(w) {
		atomic.AddUint64(&ingressDropped, 1)
		return fmt.Errorf("rate limited")
	}
	enqueueShard(tx)
	atomic.AddUint64(&ingressAccepted, 1)
	return nil
}

func enqueueShard(tx ledger.Transaction) {
	if numShards == 0 {
		startIngress()
	}
	idx := shardFor(tx.From)
	select {
	case shardCh[idx] <- tx:
	default:
		atomic.AddUint64(&ingressDropped, 1)
	}
}

// ===== Stats =====

func GatewayStats() (accepted, dropped uint64) {
	return atomic.LoadUint64(&ingressAccepted), atomic.LoadUint64(&ingressDropped)
}
