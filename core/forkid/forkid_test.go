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

package forkid

import (
	"bytes"
	"hash/crc32"
	"math"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
)

// Tests that IDs are properly RLP encoded (specifically important because we
// use uint32 to store the hash, but we need to encode it as [4]byte).
func TestEncoding(t *testing.T) {
	tests := []struct {
		id   ID
		want []byte
	}{
		{ID{Hash: checksumToBytes(0), Next: 0}, common.Hex2Bytes("c6840000000080")},
		{ID{Hash: checksumToBytes(0xdeadbeef), Next: 0xBADDCAFE}, common.Hex2Bytes("ca84deadbeef84baddcafe,")},
		{ID{Hash: checksumToBytes(math.MaxUint32), Next: math.MaxUint64}, common.Hex2Bytes("ce84ffffffff88ffffffffffffffff")},
	}
	for i, tt := range tests {
		have, err := rlp.EncodeToBytes(tt.id)
		if err != nil {
			t.Errorf("test %d: failed to encode forkid: %v", i, err)
			continue
		}
		if !bytes.Equal(have, tt.want) {
			t.Errorf("test %d: RLP mismatch: have %x, want %x", i, have, tt.want)
		}
	}
}

// Tests that time-based forks which are active at genesis are not included in
// forkid hash.
func TestTimeBasedForkInGenesis(t *testing.T) {
	var (
		time       = uint64(1690475657)
		genesis    = types.NewBlockWithHeader(&types.Header{Time: time})
		forkidHash = checksumToBytes(crc32.ChecksumIEEE(genesis.Hash().Bytes()))
		config     = func(shanghai, cancun uint64) *params.ChainConfig {
			return &params.ChainConfig{
				ChainID:                       big.NewInt(1337),
				HomesteadBlock:                big.NewInt(0),
				DAOForkBlock:                  nil,
				DAOForkSupport:                true,
				EIP150Block:                   big.NewInt(0),
				EIP155Block:                   big.NewInt(0),
				EIP158Block:                   big.NewInt(0),
				ByzantiumBlock:                big.NewInt(0),
				ConstantinopleBlock:           big.NewInt(0),
				PetersburgBlock:               big.NewInt(0),
				IstanbulBlock:                 big.NewInt(0),
				MuirGlacierBlock:              big.NewInt(0),
				BerlinBlock:                   big.NewInt(0),
				LondonBlock:                   big.NewInt(0),
				TerminalTotalDifficulty:       big.NewInt(0),
				TerminalTotalDifficultyPassed: true,
				MergeNetsplitBlock:            big.NewInt(0),
				ShanghaiTime:                  &shanghai,
				CancunTime:                    &cancun,
				Ethash:                        new(params.EthashConfig),
			}
		}
	)
	tests := []struct {
		config *params.ChainConfig
		want   ID
	}{
		// Shanghai active before genesis, skip
		{config(time-1, time+1), ID{Hash: forkidHash, Next: time + 1}},

		// Shanghai active at genesis, skip
		{config(time, time+1), ID{Hash: forkidHash, Next: time + 1}},

		// Shanghai not active, skip
		{config(time+1, time+2), ID{Hash: forkidHash, Next: time + 1}},
	}
	for _, tt := range tests {
		if have := NewID(tt.config, genesis, 0, time); have != tt.want {
			t.Fatalf("incorrect forkid hash: have %x, want %x", have, tt.want)
		}
	}
}
