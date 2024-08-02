// Copyright 2020 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package cons

import (
	mapset "github.com/deckarep/golang-set"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p"
)

const (
	maxKnownAttestationHash            = 1024
	maxQueuedAttestations              = 100
	maxQueuedJustifiedOrFinalizedBlock = 100
	maxKnownJustifiedOrFinalizedBlock  = 100
)

// Peer is a collection of relevant information we have about a `cons` peer.
type Peer struct {
	id string // Unique ID for the peer, cached

	*p2p.Peer                   // The embedded P2P package peer
	rw        p2p.MsgReadWriter // Input/output streams for cons
	version   uint              // Protocol version negotiated

	logger log.Logger // Contextual logger with the peer id injected

	knownAttestations  *knownCache             // Set of attestation hashes known to be known by this peer
	queuedAttestations chan *types.Attestation // Queue of attestation to broadcast to the peer

	knownJustifiedOrFinalizedBlock  *knownCache
	queuedJustifiedOrFinalizedBlock chan *types.BlockStatus

	term chan struct{} // Termination channel to stop the broadcasters
}

// newPeer create a wrapper for a network connection and negotiated  protocol
// version.
func newPeer(version uint, p *p2p.Peer, rw p2p.MsgReadWriter) *Peer {
	id := p.ID().String()
	peer := &Peer{
		id:                              id,
		Peer:                            p,
		rw:                              rw,
		version:                         version,
		logger:                          log.New("peer", id[:8]),
		knownAttestations:               newKnownCache(maxKnownAttestationHash),
		queuedAttestations:              make(chan *types.Attestation, maxQueuedAttestations),
		knownJustifiedOrFinalizedBlock:  newKnownCache(maxKnownJustifiedOrFinalizedBlock),
		queuedJustifiedOrFinalizedBlock: make(chan *types.BlockStatus, maxQueuedJustifiedOrFinalizedBlock),
		term:                            make(chan struct{}),
	}
	// Start up all the broadcasters
	go peer.broadcastAttestationsLoop()
	go peer.broadcastJustifiedOrFinalizedBlockLoop()
	return peer
}

// Close signals the broadcast goroutine to terminate. Only ever call this if
// you created the peer yourself via NewPeer. Otherwise let whoever created it
// clean it up!
func (p *Peer) Close() {
	close(p.term)
}

// ID retrieves the peer's unique identifier.
func (p *Peer) ID() string {
	return p.id
}

// Version retrieves the peer's negoatiated `cons` protocol version.
func (p *Peer) Version() uint {
	return p.version
}

// Log overrides the P2P logget with the higher level one containing only the id.
func (p *Peer) Log() log.Logger {
	return p.logger
}

// max is a helper function which returns the larger of the two given integers.
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// knownCache is a cache for known hashes.
type knownCache struct {
	hashes mapset.Set
	max    int
}

// newKnownCache creates a new knownCache with a max capacity.
func newKnownCache(max int) *knownCache {
	return &knownCache{
		max:    max,
		hashes: mapset.NewSet(),
	}
}

// Add adds a list of elements to the set.
func (k *knownCache) Add(hashes ...common.Hash) {
	for k.hashes.Cardinality() > max(0, k.max-len(hashes)) {
		k.hashes.Pop()
	}
	for _, hash := range hashes {
		k.hashes.Add(hash)
	}
}

// Contains returns whether the given item is in the set.
func (k *knownCache) Contains(hash common.Hash) bool {
	return k.hashes.Contains(hash)
}

// Cardinality returns the number of elements in the set.
func (k *knownCache) Cardinality() int {
	return k.hashes.Cardinality()
}

func (p *Peer) KnownAttestation(hash common.Hash) bool {
	return p.knownAttestations.Contains(hash)
}

func (p *Peer) KnownJustifiedOrFinalizedBlock(hash common.Hash) bool {
	return p.knownJustifiedOrFinalizedBlock.Contains(hash)
}

func (p *Peer) SendNewAttestation(a *types.Attestation) error {
	// Mark all the block hash as known, but ensure we don't overflow our limits
	p.knownAttestations.Add(a.Hash())
	return p2p.Send(p.rw, NewAttestationMsg, &NewAttestationPacket{
		SourceRangeEdge: a.SourceRangeEdge,
		TargetRangeEdge: a.TargetRangeEdge,
		R:               a.R,
		S:               a.S,
		V:               a.V,
	})
}

func (p *Peer) AsyncSendNewAttestation(a *types.Attestation) {
	select {
	case p.queuedAttestations <- a.DeepCopy():
		// Mark all the block hash as known, but ensure we don't overflow our limits
		p.knownAttestations.Add(a.Hash())
		p.Log().Trace("AsyncSendNewAttestation", "number",
			a.TargetRangeEdge.Number.Uint64())
	default:
		p.Log().Debug("Dropping attestation propagation", "number",
			a.TargetRangeEdge.Number.Uint64(), "hash", a.TargetRangeEdge.Hash)
	}
}

func (p *Peer) SendNewJustifiedOrFinalizedBlock(bs *types.BlockStatus) error {
	// Mark all the block hash as known, but ensure we don't overflow our limits
	p.knownJustifiedOrFinalizedBlock.Add(bs.CacheHash())
	return p2p.Send(p.rw, NewJustifiedOrFinalizedBlockMsg, bs.DeepCopy())
}

func (p *Peer) AsyncSendNewJustifiedOrFinalizedBlock(bs *types.BlockStatus) {
	select {
	case p.queuedJustifiedOrFinalizedBlock <- bs.DeepCopy():
		// Mark all the block hash as known, but ensure we don't overflow our limits
		p.knownJustifiedOrFinalizedBlock.Add(bs.CacheHash())
	default:
		p.Log().Debug("Dropping JustifiedOrFinalized propagation", "number",
			bs.BlockNumber.Uint64(), "hash", bs.Hash)
	}
}
