// Copyright 2019 The go-ethereum Authors
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

func (p *Peer) broadcastAttestationsLoop() {
	for {
		select {
		case a := <-p.queuedAttestations:
			if err := p.SendNewAttestation(a.DeepCopy()); err != nil {
				p.Log().Trace(err.Error())
				return
			}
			p.Log().Trace("Propagated attestation", "number",
				a.TargetRangeEdge.Number.Uint64(), "hash", a.TargetRangeEdge.Hash)

		case <-p.term:
			return
		}
	}
}

func (p *Peer) broadcastJustifiedOrFinalizedBlockLoop() {
	for {
		select {
		case bs := <-p.queuedJustifiedOrFinalizedBlock:
			if err := p.SendNewJustifiedOrFinalizedBlock(bs.DeepCopy()); err != nil {
				return
			}
			p.Log().Trace("Propagated JustifiedOrFinalized", "number",
				bs.BlockNumber.Uint64(), "hash", bs.Hash)

		case <-p.term:
			return
		}
	}
}
