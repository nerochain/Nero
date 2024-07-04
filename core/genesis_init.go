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

var (
	// Rewards plan of rule 1
	rewardsByMonth1 = []*big.Int{
		fromGwei(460590277777778), fromGwei(516685474537037), fromGwei(573248131269290), fromGwei(630282143474312), fromGwei(687791439114376), fromGwei(745779978884773),
		fromGwei(838973978708813), fromGwei(932944595198053), fromGwei(1027698300158040), fromGwei(1123241619326020), fromGwei(1219581132820400), fromGwei(1316723475593910),
		fromGwei(1449397560112750), fromGwei(1583177262002570), fromGwei(1718071794741480), fromGwei(1854090448586550), fromGwei(1991242591213660), fromGwei(2129537668362660),
		fromGwei(2268985204487910), fromGwei(2409594803414200), fromGwei(2551376148998200), fromGwei(2694339005795410), fromGwei(2838493219732590), fromGwei(2983848718785920),
		fromGwei(3154721069220250), fromGwei(3327017355908200), fromGwei(3500749444985210), fromGwei(3675929301471200), fromGwei(3852568990094570), fromGwei(4030680676123140),
		fromGwei(4210276626201940), fromGwei(4391369209198070), fromGwei(4573970897052500), fromGwei(4758094265639040), fromGwei(4943751995630480), fromGwei(5130956873371840),
		fromGwei(5295416236205500), fromGwei(5461246093729430), fromGwei(5628457866732730), fromGwei(5797063071177730), fromGwei(5967073318993100), fromGwei(6138500318873600),
		fromGwei(6276633654864210), fromGwei(6415918101988080), fromGwei(6556363252837980), fromGwei(6697978779944960), fromGwei(6840774436444510), fromGwei(6984760056748220),
	}

	// Default rewards plan, which is rule 2
	rewardsByMonth2 = []*big.Int{
		fromGwei(384027777800000), fromGwei(446255787000000), fromGwei(509002363000000), fromGwei(572271827200000), fromGwei(636068536800000), fromGwei(700396885800000),
		fromGwei(799983526500000), fromGwei(900400055900000), fromGwei(1001653390000000), fromGwei(1095417168000000), fromGwei(1189962311000000), fromGwei(1285295330000000),
		fromGwei(1416145014000000), fromGwei(1548085111000000), fromGwei(1681124709000000), fromGwei(1815272970000000), fromGwei(1950539134000000), fromGwei(2086932516000000),
		fromGwei(2224462509000000), fromGwei(2363138585000000), fromGwei(2502970296000000), fromGwei(2643967271000000), fromGwei(2786139220000000), fromGwei(2929495936000000),
		fromGwei(3098352846000000), fromGwei(3268616898000000), fromGwei(3440299816000000), fromGwei(3613413426000000), fromGwei(3787969649000000), fromGwei(3963980507000000),
		fromGwei(4141458123000000), fromGwei(4320414718000000), fromGwei(4500862618000000), fromGwei(4682814251000000), fromGwei(4866282148000000), fromGwei(5051278944000000),
		fromGwei(5213511824000000), fromGwei(5377096644000000), fromGwei(5542044672000000), fromGwei(5708367267000000), fromGwei(5876075883000000), fromGwei(6045182071000000),
		fromGwei(6180975254000000), fromGwei(6317900048000000), fromGwei(6455965882000000), fromGwei(6595182264000000), fromGwei(6735558783000000), fromGwei(6877105106000000),
	}

	rewardsPlans = [][]*big.Int{
		rewardsByMonth1,
		rewardsByMonth2,
	}
)

// fromGwei convert amount from gwei to wei
func fromGwei(gwei int64) *big.Int {
	return new(big.Int).Mul(big.NewInt(gwei), big.NewInt(1000000000))
}

// RewardsByMonth returns RewardsByMonth info by different rules
// rule begins from 1, and 0 will return default value
func RewardsByMonth(rule uint64) []*big.Int {
	var idx int
	if rule == 0 { // 0 is the latest rule by default
		idx = len(rewardsPlans) - 1
	} else {
		idx = int(rule) - 1
	}
	if idx >= len(rewardsPlans) {
		panic(fmt.Sprintf("Unknown consensus rewards rule: %v\n", rule))
	}
	return rewardsPlans[idx]
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
		From:       env.genesis.Coinbase,
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
		return errors.New("Staking Contract is missing in genesis!")
	}

	if len(env.genesis.Validators) <= 0 {
		return errors.New("validators are missing in genesis!")
	}

	totalValidatorStake := big.NewInt(0)
	for _, validator := range env.genesis.Validators {
		totalValidatorStake = new(big.Int).Add(totalValidatorStake, new(big.Int).Mul(validator.Stake, big.NewInt(1000000000000000000)))
	}
	rewardsByMonth := RewardsByMonth(env.genesis.Config.Turbo.Rule)

	totalRewards := big.NewInt(0)
	for _, rewards := range rewardsByMonth {
		totalRewards = new(big.Int).Add(totalRewards, rewards)
	}

	contract.Balance = new(big.Int).Add(totalValidatorStake, totalRewards)
	balance, _ := uint256.FromBig(contract.Balance)
	env.state.SetBalance(system.StakingContract, balance, tracing.BalanceIncreaseGenesisBalance)

	blocksPerMonth := big.NewInt(60 * 60 * 24 / 3 * 30)
	rewardsPerBlock := new(big.Int).Div(rewardsByMonth[0], blocksPerMonth)

	_, err := env.callContract(system.StakingContract, "initialize",
		contract.Init.Admin,
		contract.Init.FirstLockPeriod,
		contract.Init.ReleasePeriod,
		contract.Init.ReleaseCnt,
		totalRewards,
		rewardsPerBlock,
		big.NewInt(int64(env.genesis.Config.Turbo.Epoch)),
		contract.Init.RuEpoch,
		system.CommunityPoolContract,
		system.BonusPoolContract)
	return err
}

// initCommunityPool initializes CommunityPool Contract
func (env *genesisInit) initCommunityPool() error {
	contract, ok := env.genesis.Alloc[system.CommunityPoolContract]
	if !ok {
		return errors.New("CommunityPool Contract is missing in genesis!")
	}
	_, err := env.callContract(system.CommunityPoolContract, "initialize", contract.Init.Admin)
	return err
}

// initBonusPool initializes BonusPool Contract
func (env *genesisInit) initBonusPool() error {
	if _, ok := env.genesis.Alloc[system.BonusPoolContract]; !ok {
		return errors.New("BonusPool Contract is missing in genesis!")
	}
	_, err := env.callContract(system.BonusPoolContract, "initialize", system.StakingContract)
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
