// Copyright 2024 The go-ethereum Authors
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

package types

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
)

//go:generate go run github.com/fjl/gencodec -type Account -field-override accountMarshaling -out gen_account.go
//go:generate go run github.com/fjl/gencodec -type Init -field-override initMarshaling -out gen_init.go
//go:generate go run github.com/fjl/gencodec -type LockedAccount -field-override lockedAccountMarshaling -out gen_locked_account.go
//go:generate go run github.com/fjl/gencodec -type ValidatorInfo -field-override validatorInfoMarshaling -out gen_validator_info.go

// Account represents an Ethereum account and its attached data.
// This type is used to specify accounts in the genesis block state, and
// is also useful for JSON encoding/decoding of accounts.
type Account struct {
	Code    []byte                      `json:"code,omitempty"`
	Storage map[common.Hash]common.Hash `json:"storage,omitempty"`
	Balance *big.Int                    `json:"balance" gencodec:"required"`
	Nonce   uint64                      `json:"nonce,omitempty"`
	Init    *Init                       `json:"init,omitempty"`

	// used in tests
	PrivateKey []byte `json:"secretKey,omitempty"`
}

type Init struct {
	Admin           common.Address  `json:"admin,omitempty"`
	FirstLockPeriod *big.Int        `json:"firstLockPeriod,omitempty"`
	ReleasePeriod   *big.Int        `json:"releasePeriod,omitempty"`
	ReleaseCnt      *big.Int        `json:"releaseCnt,omitempty"`
	RuEpoch         *big.Int        `json:"ruEpoch,omitempty"`
	PeriodTime      *big.Int        `json:"periodTime,omitempty"`
	LockedAccounts  []LockedAccount `json:"lockedAccounts,omitempty"`
}

// LockedAccount represents the info of the locked account
type LockedAccount struct {
	UserAddress  common.Address `json:"userAddress,omitempty"`
	TypeId       *big.Int       `json:"typeId,omitempty"`
	LockedAmount *big.Int       `json:"lockedAmount,omitempty"`
	LockedTime   *big.Int       `json:"lockedTime,omitempty"`
	PeriodAmount *big.Int       `json:"periodAmount,omitempty"`
}

// ValidatorInfo represents the info of inital validators
type ValidatorInfo struct {
	Address          common.Address `json:"address"         gencodec:"required"`
	Manager          common.Address `json:"manager"         gencodec:"required"`
	Rate             *big.Int       `json:"rate,omitempty"`
	Stake            *big.Int       `json:"stake,omitempty"`
	AcceptDelegation bool           `json:"acceptDelegation,omitempty"`
}

// MakeValidator creates ValidatorInfo
func MakeValidator(address, manager, rate, stake string, acceptDelegation bool) ValidatorInfo {
	rateNum, ok := new(big.Int).SetString(rate, 10)
	if !ok {
		panic("Failed to make validator info due to invalid rate")
	}
	stakeNum, ok := new(big.Int).SetString(stake, 10)
	if !ok {
		panic("Failed to make validator info due to invalid stake")
	}

	return ValidatorInfo{
		Address:          common.HexToAddress(address),
		Manager:          common.HexToAddress(manager),
		Rate:             rateNum,
		Stake:            stakeNum,
		AcceptDelegation: acceptDelegation,
	}
}

type accountMarshaling struct {
	Code       hexutil.Bytes
	Balance    *math.HexOrDecimal256
	Nonce      math.HexOrDecimal64
	Storage    map[storageJSON]storageJSON
	PrivateKey hexutil.Bytes
}

type initMarshaling struct {
	FirstLockPeriod *math.HexOrDecimal256
	ReleasePeriod   *math.HexOrDecimal256
	ReleaseCnt      *math.HexOrDecimal256
	RuEpoch         *math.HexOrDecimal256
	PeriodTime      *math.HexOrDecimal256
}

type lockedAccountMarshaling struct {
	TypeId       *math.HexOrDecimal256
	LockedAmount *math.HexOrDecimal256
	LockedTime   *math.HexOrDecimal256
	PeriodAmount *math.HexOrDecimal256
}

type validatorInfoMarshaling struct {
	Rate  *math.HexOrDecimal256
	Stake *math.HexOrDecimal256
}

// storageJSON represents a 256 bit byte array, but allows less than 256 bits when
// unmarshalling from hex.
type storageJSON common.Hash

func (h *storageJSON) UnmarshalText(text []byte) error {
	text = bytes.TrimPrefix(text, []byte("0x"))
	if len(text) > 64 {
		return fmt.Errorf("too many hex characters in storage key/value %q", text)
	}
	offset := len(h) - len(text)/2 // pad on the left
	if _, err := hex.Decode(h[offset:], text); err != nil {
		return fmt.Errorf("invalid hex storage key/value %q", text)
	}
	return nil
}

func (h storageJSON) MarshalText() ([]byte, error) {
	return hexutil.Bytes(h[:]).MarshalText()
}

// GenesisAlloc specifies the initial state of a genesis block.
type GenesisAlloc map[common.Address]Account

func (ga *GenesisAlloc) UnmarshalJSON(data []byte) error {
	m := make(map[common.UnprefixedAddress]Account)
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	*ga = make(GenesisAlloc)
	for addr, a := range m {
		(*ga)[common.Address(addr)] = a
	}
	return nil
}
