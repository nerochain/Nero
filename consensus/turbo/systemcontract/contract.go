package systemcontract

import (
	"bytes"
	"errors"
	"math"
	"math/big"
	"sort"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/contracts"
	"github.com/ethereum/go-ethereum/contracts/system"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/log"
	"github.com/holiman/uint256"
)

const TopValidatorNum uint8 = 25

// AddrAscend implements the sort interface to allow sorting a list of addresses
type AddrAscend []common.Address

func (s AddrAscend) Len() int           { return len(s) }
func (s AddrAscend) Less(i, j int) bool { return bytes.Compare(s[i][:], s[j][:]) < 0 }
func (s AddrAscend) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

type Proposal struct {
	Id     *big.Int
	Action *big.Int
	From   common.Address
	To     common.Address
	Value  *big.Int
	Data   []byte
}

// GetTopValidators return the result of calling method `getTopValidators` in Staking contract
func GetTopValidators(ctx *contracts.CallContext) ([]common.Address, error) {
	const method = "getTopValidators"
	result, err := contractRead(ctx, system.StakingContract, method, TopValidatorNum)
	if err != nil {
		log.Error("GetTopValidators contractRead failed", "err", err)
		return []common.Address{}, err
	}
	validators, ok := result.([]common.Address)
	if !ok {
		return []common.Address{}, errors.New("GetTopValidators: invalid validator format")
	}
	sort.Sort(AddrAscend(validators))
	return validators, nil
}

// UpdateActiveValidatorSet return the result of calling method `updateActiveValidatorSet` in Staking contract
func UpdateActiveValidatorSet(ctx *contracts.CallContext, newValidators []common.Address) error {
	const method = "updateActiveValidatorSet"
	err := contractWrite(ctx, system.EngineCaller, system.StakingContract, method, newValidators)
	if err != nil {
		log.Error("UpdateActiveValidatorSet failed", "newValidators", newValidators, "err", err)
	}
	return err
}

// DecreaseMissedBlocksCounter return the result of calling method `decreaseMissedBlocksCounter` in Staking contract
func DecreaseMissedBlocksCounter(ctx *contracts.CallContext) error {
	const method = "decreaseMissedBlocksCounter"
	err := contractWrite(ctx, system.EngineCaller, system.StakingContract, method)
	if err != nil {
		log.Error("DecreaseMissedBlocksCounter failed", "err", err)
	}
	return err
}

// DistributeBlockFee return the result of calling method `distributeBlockFee` in Staking contract
func DistributeBlockFee(ctx *contracts.CallContext, fee *uint256.Int) error {
	const method = "distributeBlockFee"
	data, err := system.ABIPack(system.StakingContract, method)
	if err != nil {
		log.Error("Can't pack data for distributeBlockFee", "error", err)
		return err
	}
	if _, err := contracts.CallContractWithValue(ctx, system.EngineCaller, &system.StakingContract, data, fee); err != nil {
		log.Error("DistributeBlockFee failed", "fee", fee, "err", err)
		return err
	}
	return nil
}

// LazyPunish return the result of calling method `lazyPunish` in Staking contract
func LazyPunish(ctx *contracts.CallContext, validator common.Address) error {
	const method = "lazyPunish"
	err := contractWrite(ctx, system.EngineCaller, system.StakingContract, method, validator)
	if err != nil {
		log.Error("LazyPunish failed", "validator", validator, "err", err)
	}
	return err
}

// DoubleSignPunish return the result of calling method `doubleSignPunish` in Staking contract
func DoubleSignPunish(ctx *contracts.CallContext, punishHash common.Hash, validator common.Address) error {
	const method = "doubleSignPunish"
	err := contractWrite(ctx, system.EngineCaller, system.StakingContract, method, punishHash, validator)
	if err != nil {
		log.Error("DoubleSignPunish failed", "punishHash", punishHash, "validator", validator, "err", err)
	}
	return err
}

// DoubleSignPunishWithGivenEVM return the result of calling method `doubleSignPunish` in Staking contract with given EVM
func DoubleSignPunishWithGivenEVM(evm *vm.EVM, from common.Address, punishHash common.Hash, validator common.Address) error {
	// execute contract
	data, err := system.ABIPack(system.StakingContract, "doubleSignPunish", punishHash, validator)
	if err != nil {
		log.Error("Can't pack data for doubleSignPunish", "error", err)
		return err
	}
	if _, err := contracts.VMCallContract(evm, from, &system.StakingContract, data, math.MaxUint64); err != nil {
		log.Error("DoubleSignPunishWithGivenEVM failed", "punishHash", punishHash, "validator", validator, "err", err)
		return err
	}
	return nil
}

// IsDoubleSignPunished return the result of calling method `isDoubleSignPunished` in Staking contract
func IsDoubleSignPunished(ctx *contracts.CallContext, punishHash common.Hash) (bool, error) {
	const method = "isDoubleSignPunished"
	result, err := contractRead(ctx, system.StakingContract, method, punishHash)
	if err != nil {
		log.Error("IsDoubleSignPunished contractRead failed", "punishHash", punishHash, "err", err)
		return true, err
	}
	punished, ok := result.(bool)
	if !ok {
		return true, errors.New("IsDoubleSignPunished: invalid result format, punishHash" + punishHash.Hex())
	}
	return punished, nil
}

// contractRead perform contract read
func contractRead(ctx *contracts.CallContext, contract common.Address, method string, args ...interface{}) (interface{}, error) {
	ret, err := contractReadAll(ctx, contract, method, args...)
	if err != nil {
		return nil, err
	}
	if len(ret) != 1 {
		return nil, errors.New(method + ": invalid result length")
	}
	return ret[0], nil
}

// contractReadAll perform contract Read and return all results
func contractReadAll(ctx *contracts.CallContext, contract common.Address, method string, args ...interface{}) ([]interface{}, error) {
	abi := system.ABI(contract)
	result, err := contractReadBytes(ctx, contract, &abi, method, args...)
	if err != nil {
		return nil, err
	}
	// unpack data
	ret, err := abi.Unpack(method, result)
	if err != nil {
		return nil, err
	}
	return ret, nil
}

// contractReadBytes perform read contract and returns bytes
func contractReadBytes(ctx *contracts.CallContext, contract common.Address, abi *abi.ABI, method string, args ...interface{}) ([]byte, error) {
	data, err := abi.Pack(method, args...)
	if err != nil {
		log.Error("Can't pack data", "method", method, "error", err)
		return nil, err
	}
	result, err := contracts.CallContract(ctx, ctx.Header.Coinbase, &contract, data)
	if err != nil {
		log.Error("Failed to execute", "method", method, "err", err)
		return nil, err
	}
	return result, nil
}

// contractWrite perform write contract
func contractWrite(ctx *contracts.CallContext, from common.Address, contract common.Address, method string, args ...interface{}) error {
	data, err := system.ABIPack(contract, method, args...)
	if err != nil {
		log.Error("Can't pack data", "method", method, "error", err)
		return err
	}
	if _, err := contracts.CallContract(ctx, from, &contract, data); err != nil {
		log.Error("Failed to execute", "method", method, "err", err)
		return err
	}
	return nil
}
