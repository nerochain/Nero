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
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p"
	"math/big"
)

// TODO In case of DDoS attack, the corresponding peer will be disconnected automatically
func handleNewAttestation(backend Backend, msg Decoder, peer *Peer) error {
	a := new(types.Attestation)
	if err := msg.Decode(a); err != nil {
		return fmt.Errorf("%w: message %v: %v", errDecode, msg, err)
	}
	if !peer.knownAttestations.Contains(a.Hash()) {
		peer.knownAttestations.Add(a.Hash())
	}
	err := backend.Chain().HandleAttestation(a)
	if err != nil {
		log.Warn(err.Error())
	}
	return nil
}

func handleNewJustifiedOrFinalizedBlock(backend Backend, msg Decoder, peer *Peer) error {
	var bs types.BlockStatus
	if err := msg.Decode(&bs); err != nil {
		return fmt.Errorf("%w: message %v: %v", errDecode, msg, err)
	}
	if bs.Status != types.BasJustified && bs.Status != types.BasFinalized {
		return fmt.Errorf("status is error  %d", bs.Status)
	}
	status, hash := backend.Chain().GetBlockStatusByNum(bs.BlockNumber.Uint64())
	if status == types.BasUnknown { // not found
		// need to request the current block
		return p2p.Send(peer.rw, GetAttestationsMsg, &types.RequestAttestation{BlockNumber: new(big.Int).Set(bs.BlockNumber), Hash: bs.Hash})
	} else if hash != bs.Hash { // Not in theory
		return fmt.Errorf("hash inequality %v: %v", hash.String(), bs.Hash.String())
	}
	if bs.Status == types.BasFinalized && status == types.BasJustified {
		// need to request the next block
		block := backend.Chain().GetBlockByNumber(bs.BlockNumber.Uint64() + 1)
		if block == nil {
			return fmt.Errorf("block not found %d", bs.BlockNumber.Uint64())
		}
		return p2p.Send(peer.rw, GetAttestationsMsg, &types.RequestAttestation{BlockNumber: new(big.Int).Set(block.Number()), Hash: block.Hash()})
	}
	// Ignore bs.Status == BasJustified, status == BasFinalized
	// bs.Status == BasJustified, status == BasJustified
	return nil
}

func handleGetAttestations(backend Backend, msg Decoder, peer *Peer) error {
	var ra types.RequestAttestation
	if err := msg.Decode(&ra); err != nil {
		return fmt.Errorf("%w: message %v: %v", errDecode, msg, err)
	}
	as, err := backend.Chain().GetHistoryAttestations(ra.BlockNumber, ra.Hash)
	if err != nil {
		return err
	}
	return p2p.Send(peer.rw, AttestationsMsg, as)
}

func handleAttestations(backend Backend, msg Decoder, peer *Peer) error {
	var as []types.Attestation
	if err := msg.Decode(&as); err != nil {
		return fmt.Errorf("%w: message %v: %v", errDecode, msg, err)
	}
	maxCount := backend.Chain().MaxValidators()
	if len(as) > int(maxCount) {
		return errors.New("the total number of attestations exceeds the maximum number of validators")
	}
	for _, a := range as {
		if !peer.knownAttestations.Contains(a.Hash()) {
			peer.knownAttestations.Add(a.Hash())
		}
		_ = backend.Chain().HandleAttestation(a.DeepCopy())
	}
	return nil
}
