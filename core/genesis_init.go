package core

import (
	"errors"
	"fmt"
	"math"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/contracts/system"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/holiman/uint256"
)

const (
	initBatch   = 30
	extraVanity = 32                     // Fixed number of extra-data prefix bytes reserved for validator vanity
	extraSeal   = crypto.SignatureLength // Fixed number of extra-data suffix bytes reserved for validator seal
)

// fromGwei convert amount from gwei to wei
func fromGwei(gwei int64) *big.Int {
	return new(big.Int).Mul(big.NewInt(gwei), big.NewInt(1000000000))
}

// genesisInit is tools to init system contracts in genesis
type genesisInit struct {
	state   *state.StateDB
	header  *types.Header
	genesis *Genesis
}

// callContract executes contract in EVM
func (env *genesisInit) callContract(contract common.Address, method string, args ...interface{}) ([]byte, error) {
	// Pack method and args for data seg
	data, err := system.ABIPack(contract, method, args...)
	if err != nil {
		return nil, err
	}
	// Create EVM calling message
	msg := &Message{
		To:         &contract,
		From:       system.EngineCaller,
		Nonce:      0,
		Value:      common.Big0,
		GasLimit:   math.MaxUint64,
		GasPrice:   common.Big0,
		GasFeeCap:  common.Big0,
		GasTipCap:  common.Big0,
		Data:       data,
		AccessList: nil,
	}

	// Set up the initial access list.
	if rules := env.genesis.Config.Rules(env.header.Number, false, 0); rules.IsBerlin {
		env.state.Prepare(rules, msg.From, msg.From, msg.To, vm.ActivePrecompiles(rules), msg.AccessList)
	}
	// Create EVM
	evm := vm.NewEVM(NewEVMBlockContext(env.header, nil, &env.header.Coinbase), NewEVMTxContext(msg), env.state, env.genesis.Config, vm.Config{})
	// Run evm call
	v, _ := uint256.FromBig(msg.Value)
	ret, _, err := evm.Call(vm.AccountRef(msg.From), *msg.To, msg.Data, msg.GasLimit, v)

	if err == vm.ErrExecutionReverted {
		reason, errUnpack := abi.UnpackRevert(common.CopyBytes(ret))
		if errUnpack != nil {
			reason = "internal error"
		}
		err = fmt.Errorf("%s: %s", err.Error(), reason)
	}

	if err != nil {
		log.Error("ExecuteMsg failed", "err", err, "ret", string(ret))
	}
	env.state.Finalise(true)
	return ret, err
}

// initStaking initializes Staking Contract
func (env *genesisInit) initStaking() error {
	contract, ok := env.genesis.Alloc[system.StakingContract]
	if !ok {
		return errors.New("staking contract is missing in genesis")
	}

	if len(env.genesis.Validators) <= 0 {
		return errors.New("validators are missing in genesis")
	}

	totalValidatorStake := big.NewInt(0)
	for _, validator := range env.genesis.Validators {
		totalValidatorStake = new(big.Int).Add(totalValidatorStake, validator.Stake)
	}

	contract.Balance = new(big.Int).Add(totalValidatorStake, contract.Init.TotalRewards)
	balance, _ := uint256.FromBig(contract.Balance)
	env.state.SetBalance(system.StakingContract, balance, tracing.BalanceIncreaseGenesisBalance)

	_, err := env.callContract(system.StakingContract, "initialize",
		contract.Init.Admin,
		contract.Init.FirstLockPeriod,
		contract.Init.ReleasePeriod,
		contract.Init.ReleaseCnt,
		contract.Init.TotalRewards,
		contract.Init.RewardsPerBlock,
		big.NewInt(int64(env.genesis.Config.Turbo.Epoch)))
	return err
}

// initGenesisLock initializes GenesisLock Contract, including:
// 1. initialize PeriodTime
// 2. init locked accounts
func (env *genesisInit) initGenesisLock() error {
	contract, ok := env.genesis.Alloc[system.GenesisLockContract]
	if !ok {
		return errors.New("GenesisLock Contract is missing in genesis!")
	}

	contract.Balance = big.NewInt(0)
	for _, account := range contract.Init.LockedAccounts {
		contract.Balance = new(big.Int).Add(contract.Balance, account.LockedAmount)
	}
	balance, _ := uint256.FromBig(contract.Balance)
	env.state.SetBalance(system.GenesisLockContract, balance, tracing.BalanceIncreaseGenesisBalance)

	if _, err := env.callContract(system.GenesisLockContract, "initialize",
		contract.Init.PeriodTime); err != nil {
		return err
	}

	var (
		address      = make([]common.Address, 0, initBatch)
		typeId       = make([]*big.Int, 0, initBatch)
		lockedAmount = make([]*big.Int, 0, initBatch)
		lockedTime   = make([]*big.Int, 0, initBatch)
		periodAmount = make([]*big.Int, 0, initBatch)
	)
	sum := 0
	for _, account := range contract.Init.LockedAccounts {
		address = append(address, account.UserAddress)
		typeId = append(typeId, account.TypeId)
		lockedAmount = append(lockedAmount, account.LockedAmount)
		lockedTime = append(lockedTime, account.LockedTime)
		periodAmount = append(periodAmount, account.PeriodAmount)
		sum++
		if sum == initBatch {
			if _, err := env.callContract(system.GenesisLockContract, "init",
				address, typeId, lockedAmount, lockedTime, periodAmount); err != nil {
				return err
			}
			sum = 0
			address = make([]common.Address, 0, initBatch)
			typeId = make([]*big.Int, 0, initBatch)
			lockedAmount = make([]*big.Int, 0, initBatch)
			lockedTime = make([]*big.Int, 0, initBatch)
			periodAmount = make([]*big.Int, 0, initBatch)
		}
	}
	if len(address) > 0 {
		_, err := env.callContract(system.GenesisLockContract, "init",
			address, typeId, lockedAmount, lockedTime, periodAmount)
		return err
	}
	return nil
}

// initValidators add validators into Staking contracts
// and set validator addresses to header extra data
// and return new header extra data
func (env *genesisInit) initValidators() ([]byte, error) {
	if len(env.genesis.Validators) <= 0 {
		return env.header.Extra, errors.New("validators are missing in genesis!")
	}
	activeSet := make([]common.Address, 0, len(env.genesis.Validators))
	extra := make([]byte, 0, extraVanity+common.AddressLength*len(env.genesis.Validators)+extraSeal)
	extra = append(extra, env.header.Extra[:extraVanity]...)
	for _, v := range env.genesis.Validators {
		if _, err := env.callContract(system.StakingContract, "initValidator",
			v.Address, v.Manager, v.Rate, v.Stake, v.AcceptDelegation); err != nil {
			return env.header.Extra, err
		}
		extra = append(extra, v.Address[:]...)
		activeSet = append(activeSet, v.Address)
	}
	extra = append(extra, env.header.Extra[len(env.header.Extra)-extraSeal:]...)
	env.header.Extra = extra
	if _, err := env.callContract(system.StakingContract, "updateActiveValidatorSet", activeSet); err != nil {
		return extra, err
	}
	return env.header.Extra, nil
}
