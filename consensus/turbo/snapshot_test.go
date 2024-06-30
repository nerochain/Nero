// Copyright 2017 The go-ethereum Authors
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

package turbo

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/params"
	lru "github.com/hashicorp/golang-lru"
)

type fields struct {
	config     *params.ChainConfig
	sigcache   *lru.ARCCache
	Number     uint64
	Hash       common.Hash
	Validators map[common.Address]struct{}
	Recents    map[uint64]common.Address
}

func validatorAddress(index int64) common.Address {
	return common.BigToAddress(big.NewInt(6666 + index))
}

func genFields(block uint64) fields {
	result := fields{config: &params.ChainConfig{}, Number: block,
		Validators: make(map[common.Address]struct{}), Recents: make(map[uint64]common.Address)}
	continuousInturn := result.config.TurboContinuousInturn(big.NewInt(int64(block)))
	for i := int64(0); i < 21; i++ {
		addr := validatorAddress(i)
		result.Validators[addr] = struct{}{}
		if i < 11 {
			for r := uint64(0); r < continuousInturn; r++ {
				result.Recents[block-(11-uint64(i))*continuousInturn+r] = addr
			}
		}
	}
	return result
}

func TestSnapshot_SignedRecently(t *testing.T) {
	type args struct {
		block     uint64
		validator common.Address
	}

	tests := []struct {
		name   string
		fields fields
		args   args
		want   bool
	}{
		{"case1", genFields(100), args{101, validatorAddress(11)}, false},
		{"case2", genFields(100), args{101, validatorAddress(10)}, true},
		{"case3", genFields(99), args{100, validatorAddress(0)}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Snapshot{
				config:     tt.fields.config,
				sigcache:   tt.fields.sigcache,
				Number:     tt.fields.Number,
				Hash:       tt.fields.Hash,
				Validators: tt.fields.Validators,
				Recents:    tt.fields.Recents,
			}
			if got := s.SignedRecently(tt.args.block, tt.args.validator); got != tt.want {
				t.Errorf("Snapshot.SignedRecently() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSnapshot_inturn(t *testing.T) {
	type args struct {
		number    uint64
		validator common.Address
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   bool
	}{
		{"case1", genFields(100), args{100, validatorAddress(4)}, true},
		{"case2", genFields(100), args{101, validatorAddress(4)}, true},
		{"case3", genFields(100), args{102, validatorAddress(4)}, true},
		{"case4", genFields(100), args{103, validatorAddress(4)}, true},
		{"case5", genFields(100), args{104, validatorAddress(4)}, false},
		{"case5", genFields(100), args{104, validatorAddress(5)}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Snapshot{
				config:     tt.fields.config,
				sigcache:   tt.fields.sigcache,
				Number:     tt.fields.Number,
				Hash:       tt.fields.Hash,
				Validators: tt.fields.Validators,
				Recents:    tt.fields.Recents,
			}
			if got := s.inturn(tt.args.number, tt.args.validator); got != tt.want {
				t.Errorf("Snapshot.inturn() = %v, want %v", got, tt.want)
			}
		})
	}
}
