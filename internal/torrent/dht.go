package torrent

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"net/netip"
	"slices"
	"time"
)

// The DHT (BEP 5) is only read: Polyfin looks a torrent's peers up as a
// read-only node (BEP 43), answers no query and announces nothing, as no
// peer can connect in. A lookup starts from the nodes the addon's sources
// name, the engine's bootstrap nodes (see BootstrapNodes), and those peers
// tell (PORT).
const (
	// dhtParallel is how many queries a lookup has out at once,
	// dhtQueries how many it sends at most, and dhtWait how long one
	// waits for its answer.
	dhtParallel = 4
	dhtQueries  = 64
	dhtWait     = 3 * time.Second
	// dhtKeep is how many of the closest nodes a lookup follows.
	dhtKeep = 16
)

type dhtNode struct {
	id      Hash
	address string
	asked   bool
}

// lookupPeers looks hash up on the DHT from the nodes starts, and sends the
// peers found to found until the lookup ends or ctx is done.
func lookupPeers(ctx context.Context, conn PacketConn, hash Hash, starts []string, found func([]netip.AddrPort)) error {
	if len(starts) == 0 {
		return errors.New("no DHT node to start from")
	}
	var self Hash
	_, _ = rand.Read(self[:])
	nodes := make([]*dhtNode, 0, len(starts))
	known := map[string]bool{}
	for _, address := range starts {
		if !known[address] {
			known[address] = true
			// Starting nodes' IDs are unknown: they are asked first.
			nodes = append(nodes, &dhtNode{address: address})
		}
	}
	pending := map[string]*dhtNode{}
	sent := 0
	var txid uint16
	buffer := make([]byte, 4096)
	seen := map[netip.AddrPort]bool{}
	for {
		for len(pending) < dhtParallel && sent < dhtQueries {
			var next *dhtNode
			for _, n := range nodes {
				if !n.asked {
					next = n
					break
				}
			}
			if next == nil {
				break
			}
			next.asked = true
			txid++
			t := string([]byte{byte(txid >> 8), byte(txid)})
			query := encode(map[string]any{"t": t, "y": "q", "q": "get_peers", "ro": 1,
				"a": map[string]any{"id": string(self[:]), "info_hash": string(hash[:])}})
			if err := conn.WriteTo(query, next.address); err != nil {
				continue
			}
			pending[t] = next
			sent++
		}
		if len(pending) == 0 {
			return ctx.Err()
		}
		deadline := time.Now().Add(dhtWait)
		if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
			deadline = d
		}
		_ = conn.SetReadDeadline(deadline)
		n, from, err := conn.ReadFrom(buffer)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			// Those that did not answer in time are given up.
			clear(pending)
			continue
		}
		message, _, err := decodeDict(buffer[:n])
		if err != nil || str(message, "y") != "r" {
			continue
		}
		node := pending[str(message, "t")]
		if node == nil {
			continue
		}
		delete(pending, str(message, "t"))
		reply := dict(message, "r")
		copy(node.id[:], str(reply, "id"))
		var peers []netip.AddrPort
		for _, value := range list(reply, "values") {
			if raw, ok := value.(string); ok {
				for _, peer := range compactPeers([]byte(raw), 4) {
					if !seen[peer] {
						seen[peer] = true
						peers = append(peers, peer)
					}
				}
			}
		}
		if len(peers) > 0 {
			found(peers)
		}
		compact := []byte(str(reply, "nodes"))
		for len(compact) >= 26 {
			var id Hash
			copy(id[:], compact[:20])
			for _, address := range compactPeers(compact[20:26], 4) {
				if a := address.String(); !known[a] && address != from {
					known[a] = true
					nodes = append(nodes, &dhtNode{id: id, address: a})
				}
			}
			compact = compact[26:]
		}
		// Only the closest nodes are followed.
		slices.SortStableFunc(nodes, func(a, b *dhtNode) int { return closer(hash, a.id, b.id) })
		if len(nodes) > dhtKeep {
			nodes = nodes[:dhtKeep]
		}
	}
}

// closer compares the distance of a and b to target.
func closer(target, a, b Hash) int {
	var da, db Hash
	for i := range target {
		da[i], db[i] = a[i]^target[i], b[i]^target[i]
	}
	return bytes.Compare(da[:], db[:])
}
