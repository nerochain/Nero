package turbo

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
)

const (
	DirectionFrom accessDirection = iota
	DirectionTo
	DirectionBoth
)

type EventCheckRule struct {
	EventSig common.Hash
	Checks   map[int]common.AddressCheckType
}

type accessDirection uint

type turboAccessFilter struct {
	accesses map[common.Address]accessDirection
	rules    map[common.Hash]*EventCheckRule
}

func (b *turboAccessFilter) IsAddressDenied(address common.Address, cType common.AddressCheckType) (hit bool) {
	return false
}

func (b *turboAccessFilter) IsLogDenied(evLog *types.Log) bool {
	return false
}

// CanCreate determines where a given address can create a new contract.
//
// This will queries the system Developers contract, by DIRECTLY to get the target slot value of the contract,
// it means that it's strongly relative to the layout of the Developers contract's state variables
func (c *Turbo) CanCreate(state consensus.StateReader, addr common.Address, isContract bool, height *big.Int) bool {
	return true
}

// FilterTx do a consensus-related validation on the given transaction at the given header and state.
// the parentState must be the state of the header's parent block.
func (c *Turbo) FilterTx(sender common.Address, tx *types.Transaction, header *types.Header, parentState *state.StateDB) error {
	return nil
}

func (c *Turbo) CreateEvmAccessFilter(header *types.Header, parentState *state.StateDB) vm.EvmAccessFilter {
	return &turboAccessFilter{
		accesses: make(map[common.Address]accessDirection),
		rules:    make(map[common.Hash]*EventCheckRule),
	}
}
