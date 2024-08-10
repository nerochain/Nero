package systemcontract

import (
	"bytes"
	"errors"
	"math"
	"math/big"
	"sort"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/contracts"
	"github.com/ethereum/go-ethereum/contracts/system"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/log"
	"github.com/holiman/uint256"
)

const TopValidatorNum uint8 = 21

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

// GetBlacksFrom return the access tx-from list
func GetBlacksFrom(ctx *contracts.CallContext) ([]common.Address, error) {
	const method = "getBlacksFrom"
	result, err := contractRead(ctx, system.AddressListContract, method)
	if err != nil {
		log.Error("GetBlacksFrom contractRead failed", "err", err)
		return []common.Address{}, err
	}
	from, ok := result.([]common.Address)
	if !ok {
		return []common.Address{}, errors.New("GetBlacksFrom: invalid result format")
	}
	return from, nil
}

// GetBlacksTo return access tx-to list
func GetBlacksTo(ctx *contracts.CallContext) ([]common.Address, error) {
	const method = "getBlacksTo"
	result, err := contractRead(ctx, system.AddressListContract, method)
	if err != nil {
		log.Error("GetBlacksTo contractRead failed", "err", err)
		return []common.Address{}, err
	}
	to, ok := result.([]common.Address)
	if !ok {
		return []common.Address{}, errors.New("GetBlacksTo: invalid result format")
	}
	return to, nil
}

// GetRuleByIndex return event log rules
func GetRuleByIndex(ctx *contracts.CallContext, idx uint32) (common.Hash, int, common.AddressCheckType, error) {
	const method = "getRuleByIndex"
	results, err := contractReadAll(ctx, system.AddressListContract, method, idx)
	if err != nil {
		log.Error("GetRuleByIndex contractRead failed", "err", err)
		return common.Hash{}, 0, common.CheckNone, err
	}
	if len(results) != 3 {
		return common.Hash{}, 0, common.CheckNone, errors.New("GetRuleByIndex: invalid results' length")
	}
	var (
		ok    bool
		sig   common.Hash
		index *big.Int
		ctype uint8
	)
	if sig, ok = results[0].([32]byte); !ok {
		return common.Hash{}, 0, common.CheckNone, errors.New("GetRuleByIndex: invalid result sig format")
	}
	if index, ok = results[1].(*big.Int); !ok {
		return common.Hash{}, 0, common.CheckNone, errors.New("GetRuleByIndex: invalid result index format")
	}
	if ctype, ok = results[2].(uint8); !ok {
		return common.Hash{}, 0, common.CheckNone, errors.New("GetRuleByIndex: invalid result checktype format")
	}
	return sig, int(index.Int64()), common.AddressCheckType(ctype), nil
}

// GetRulesLen return event log rules length
func GetRulesLen(ctx *contracts.CallContext) (uint32, error) {
	const method = "rulesLen"
	result, err := contractRead(ctx, system.AddressListContract, method)
	if err != nil {
		log.Error("GetRulesLen contractRead failed", "err", err)
		return 0, err
	}
	n, ok := result.(uint32)
	if !ok {
		return 0, errors.New("GetRulesLen: invalid result format")
	}
	return n, nil
}

// IsDeveloperVerificationEnabled returns developer verification flags (devVerifyEnabled,checkInnerCreation).
// Since the state variables are as follows:
//
//	   bool public initialized;
//	   bool public devVerifyEnabled;
//		  bool public checkInnerCreation;
//	   address public admin;
//	   address public pendingAdmin;
//	   mapping(address => bool) private devs;
//
// according to [Layout of State Variables in Storage](https://docs.soliditylang.org/en/v0.8.4/internals/layout_in_storage.html),
// and after optimizer enabled, the `initialized`, `devVerifyEnabled`, `checkInnerCreation` and `admin` will be packed, and stores at slot 0,
// `pendingAdmin` stores at slot 1, and the position for `devs` is 2.
func IsDeveloperVerificationEnabled(state consensus.StateReader) (devVerifyEnabled, checkInnerCreation bool) {
	compactValue := state.GetState(system.AddressListContract, common.Hash{}).Bytes()
	// Layout of slot 0:
	// [0   -    8][9-28][        29        ][       30       ][    31     ]
	// [zero bytes][admin][checkInnerCreation][devVerifyEnabled][initialized]
	devVerifyEnabled = compactValue[30] == 0x01
	checkInnerCreation = compactValue[29] == 0x01
	return
}

// LastBlackUpdatedNumber returns LastBlackUpdatedNumber of address list
func LastBlackUpdatedNumber(state consensus.StateReader) uint64 {
	value := state.GetState(system.AddressListContract, system.BlackLastUpdatedNumberPosition)
	return value.Big().Uint64()
}

// LastRulesUpdatedNumber returns LastRulesUpdatedNumber of address list
func LastRulesUpdatedNumber(state consensus.StateReader) uint64 {
	value := state.GetState(system.AddressListContract, system.RulesLastUpdatedNumberPosition)
	return value.Big().Uint64()
}

// GetPassedProposalCount returns passed proposal count
func GetPassedProposalCount(ctx *contracts.CallContext) (uint32, error) {
	const method = "getPassedProposalCount"
	result, err := contractRead(ctx, system.OnChainDaoContract, method)
	if err != nil {
		log.Error("GetPassedProposalCount contractRead failed", "err", err)
		return 0, err
	}
	count, ok := result.(uint32)
	if !ok {
		return 0, errors.New("GetPassedProposalCount: invalid result format")
	}
	return count, nil
}

// GetPassedProposalByIndex returns passed proposal by index
func GetPassedProposalByIndex(ctx *contracts.CallContext, idx uint32) (*Proposal, error) {
	const method = "getPassedProposalByIndex"
	abi := system.ABI(system.OnChainDaoContract)
	result, err := contractReadBytes(ctx, system.OnChainDaoContract, &abi, method, idx)
	if err != nil {
		log.Error("GetPassedProposalByIndex contractReadBytes failed", "idx", idx, "err", err)
		return nil, err
	}
	// unpack data
	prop := &Proposal{}
	if err = abi.UnpackIntoInterface(prop, method, result); err != nil {
		log.Error("GetPassedProposalByIndex UnpackIntoInterface failed", "idx", idx, "err", err)
		return nil, err
	}
	return prop, nil
}

// FinishProposalById finish passed proposal by id
func FinishProposalById(ctx *contracts.CallContext, id *big.Int) error {
	const method = "finishProposalById"
	err := contractWrite(ctx, ctx.Header.Coinbase, system.OnChainDaoContract, method, id)
	if err != nil {
		log.Error("FinishProposalById failed", "id", id, "err", err)
	}
	return err
}

// ExecuteProposal executes proposal
func ExecuteProposal(ctx *contracts.CallContext, prop *Proposal) error {
	value, _ := uint256.FromBig(prop.Value)
	_, err := contracts.CallContractWithValue(ctx, prop.From, &prop.To, prop.Data, value)
	if err != nil {
		log.Error("ExecuteProposal failed", "proposal", prop, "err", err)
	}
	return err
}

// ExecuteProposalWithGivenEVM executes proposal by given evm
func ExecuteProposalWithGivenEVM(evm *vm.EVM, prop *Proposal, gas uint64) (ret []byte, err error) {
	if ret, err = contracts.VMCallContract(evm, prop.From, &prop.To, prop.Data, gas); err != nil {
		log.Error("ExecuteProposalWithGivenEVM failed", "proposal", prop, "err", err)
	}
	return
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
