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

//go:build none
// +build none

/*
The mkalloc tool creates the genesis allocation constants in genesis_alloc.go
It outputs a const declaration that contains an RLP-encoded list of (address, balance) tuples.

	go run mkalloc.go genesis.json
*/
package main

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"slices"
	"strconv"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/rlp"
)

type allocItem struct {
	Addr    *big.Int
	Balance *big.Int
	Misc    *allocItemMisc `rlp:"optional"`
}

type allocItemMisc struct {
	Nonce uint64
	Code  []byte
	Slots []allocItemStorageItem
	Init  *initArgs `rlp:"optional"`
}

type initArgs struct {
	Admin           *big.Int        `rlp:"optional"`
	FirstLockPeriod *big.Int        `rlp:"optional"`
	ReleasePeriod   *big.Int        `rlp:"optional"`
	ReleaseCnt      *big.Int        `rlp:"optional"`
	TotalRewards    *big.Int        `rlp:"optional"`
	RewardsPerBlock *big.Int        `rlp:"optional"`
	PeriodTime      *big.Int        `rlp:"optional"`
	LockedAccounts  []lockedAccount `rlp:"optional"`
}

// LockedAccount represents the info of the locked account
type lockedAccount struct {
	UserAddress  *big.Int
	TypeId       *big.Int
	LockedAmount *big.Int
	LockedTime   *big.Int
	PeriodAmount *big.Int
}

type allocItemStorageItem struct {
	Key common.Hash
	Val common.Hash
}

func makelist(g *core.Genesis) []allocItem {
	items := make([]allocItem, 0, len(g.Alloc))
	for addr, account := range g.Alloc {
		var misc *allocItemMisc
		if len(account.Storage) > 0 || len(account.Code) > 0 || account.Nonce != 0 || account.Init != nil {
			misc = &allocItemMisc{
				Nonce: account.Nonce,
				Code:  account.Code,
				Slots: make([]allocItemStorageItem, 0, len(account.Storage)),
			}
			for key, val := range account.Storage {
				misc.Slots = append(misc.Slots, allocItemStorageItem{key, val})
			}
			slices.SortFunc(misc.Slots, func(a, b allocItemStorageItem) int {
				return a.Key.Cmp(b.Key)
			})
			if account.Init != nil {
				misc.Init = &initArgs{
					Admin:           new(big.Int).SetBytes(account.Init.Admin.Bytes()),
					FirstLockPeriod: account.Init.FirstLockPeriod,
					ReleasePeriod:   account.Init.ReleasePeriod,
					ReleaseCnt:      account.Init.ReleaseCnt,
					TotalRewards:    account.Init.TotalRewards,
					RewardsPerBlock: account.Init.RewardsPerBlock,
					PeriodTime:      account.Init.PeriodTime,
				}
				if len(account.Init.LockedAccounts) > 0 {
					misc.Init.LockedAccounts = make([]lockedAccount, 0, len(account.Init.LockedAccounts))
					for _, locked := range account.Init.LockedAccounts {
						misc.Init.LockedAccounts = append(misc.Init.LockedAccounts, lockedAccount{new(big.Int).SetBytes(locked.UserAddress.Bytes()),
							locked.TypeId, locked.LockedAmount, locked.LockedTime, locked.PeriodAmount})
					}
				}
			}
		}
		bigAddr := new(big.Int).SetBytes(addr.Bytes())
		items = append(items, allocItem{bigAddr, account.Balance, misc})
	}
	slices.SortFunc(items, func(a, b allocItem) int {
		return a.Addr.Cmp(b.Addr)
	})
	return items
}

func makealloc(g *core.Genesis) string {
	a := makelist(g)
	data, err := rlp.EncodeToBytes(a)
	if err != nil {
		panic(err)
	}
	return strconv.QuoteToASCII(string(data))
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "Usage: mkalloc genesis.json")
		os.Exit(1)
	}

	g := new(core.Genesis)
	file, err := os.Open(os.Args[1])
	if err != nil {
		panic(err)
	}
	defer file.Close()
	if err := json.NewDecoder(file).Decode(g); err != nil {
		panic(err)
	}
	fmt.Println("const allocData =", makealloc(g))
}
