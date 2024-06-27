package systemcontract

import (
	"encoding/json"
	"math/big"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/contracts"
	"github.com/ethereum/go-ethereum/contracts/system"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/assert"
)

const testAbi = `[
	{
		"inputs": [],
		"name": "getActiveValidators",
		"outputs": [
			{
				"internalType": "address[]",
				"name": "",
				"type": "address[]"
			}
		],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [],
		"name": "currRewardsPerBlock",
		"outputs": [
			{
				"internalType": "uint256",
				"name": "",
				"type": "uint256"
			}
		],
		"stateMutability": "view",
		"type": "function"
	  },
	  {
		"inputs": [
		  {
			"internalType": "address",
			"name": "",
			"type": "address"
		  }
		],
		"name": "valMaps",
		"outputs": [
		  {
			"internalType": "contract IValidator",
			"name": "",
			"type": "address"
		  }
		],
		"stateMutability": "view",
		"type": "function"
	  },
	  {
		"inputs": [
			{
				"internalType": "address",
				"name": "_val",
				"type": "address"
			}
		],
		"name": "getPunishRecord",
		"outputs": [
			{
				"internalType": "uint256",
				"name": "",
				"type": "uint256"
			}
		],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [],
		"name": "currFeeRewards",
		"outputs": [
			{
				"internalType": "uint256",
				"name": "",
				"type": "uint256"
			}
		],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [
		  {
			"internalType": "address",
			"name": "a",
			"type": "address"
		  },
		  {
			"internalType": "enum AddressList.Direction",
			"name": "d",
			"type": "uint8"
		  }
		],
		"name": "addBlacklist",
		"outputs": [],
		"stateMutability": "nonpayable",
		"type": "function"
	},
    {
      "inputs": [],
      "name": "disableCheckInnerCreation",
      "outputs": [],
      "stateMutability": "nonpayable",
      "type": "function"
    },
    {
      "inputs": [],
      "name": "disableDevVerify",
      "outputs": [],
      "stateMutability": "nonpayable",
      "type": "function"
    },
    {
      "inputs": [],
      "name": "enableCheckInnerCreation",
      "outputs": [],
      "stateMutability": "nonpayable",
      "type": "function"
    },
	{
		"inputs": [],
		"name": "enableDevVerify",
		"outputs": [],
		"stateMutability": "nonpayable",
		"type": "function"
	},
	{
		"inputs": [
			{
				"internalType": "bytes32",
				"name": "sig",
				"type": "bytes32"
			},
			{
				"internalType": "uint128",
				"name": "checkIdx",
				"type": "uint128"
			}
		],
		"name": "removeRule",
		"outputs": [
			{
				"internalType": "bool",
				"name": "",
				"type": "bool"
			}
		],
		"stateMutability": "nonpayable",
		"type": "function"
	},
	{
		"inputs": [
			{
				"internalType": "uint256",
				"name": "action",
				"type": "uint256"
			},
			{
				"internalType": "address",
				"name": "from",
				"type": "address"
			},
			{
				"internalType": "address",
				"name": "to",
				"type": "address"
			},
			{
				"internalType": "uint256",
				"name": "value",
				"type": "uint256"
			},
			{
				"internalType": "bytes",
				"name": "input",
				"type": "bytes"
			}
		],
		"name": "commitProposal",
		"outputs": [],
		"stateMutability": "nonpayable",
		"type": "function"
	}
]`

var GenesisValidators = []common.Address{
	common.HexToAddress("0x1B5813bAA493742CEe5d2Eb7410b3014fe3cf2b6"),
	common.HexToAddress("0x8Cc5A1a0802DB41DB826C2FcB72423744338DcB0"),
}

func TestGetTopValidators(t *testing.T) {
	ctx, err := initCallContext()
	assert.NoError(t, err, "Init call context error")

	vals, err := GetTopValidators(ctx)
	if assert.NoError(t, err) {
		assert.Equal(t, GenesisValidators, vals)
	}
}

func TestUpdateActiveValidatorSet(t *testing.T) {
	ctx, err := initCallContext()
	assert.NoError(t, err, "Init call context error")

	getActiveValidators := func(ctx *contracts.CallContext) []common.Address {
		validators, ok := readSystemContract(t, ctx, "getActiveValidators").([]common.Address)
		assert.True(t, ok, "invalid validator format")
		return validators
	}

	newSet := []common.Address{
		common.BigToAddress(big.NewInt(111)),
		common.BigToAddress(big.NewInt(222)),
	}

	if assert.NoError(t, UpdateActiveValidatorSet(ctx, newSet)) {
		assert.Equal(t, newSet, getActiveValidators(ctx))
	}
}

func TestDecreaseMissedBlocksCounter(t *testing.T) {
	ctx, err := initCallContext()
	assert.NoError(t, err, "Init call context error")

	getPunishRecord := func(val common.Address) int {
		count, ok := readSystemContract(t, ctx, "getPunishRecord", val).(*big.Int)
		assert.True(t, ok, "invalid result format")
		return int(count.Int64())
	}

	LazyPunish(ctx, GenesisValidators[0])

	assert.Equal(t, 1, getPunishRecord(GenesisValidators[0]))

	assert.NoError(t, DecreaseMissedBlocksCounter(ctx))

	assert.Equal(t, 0, getPunishRecord(GenesisValidators[0]))
}

func TestGetRewardsUpdatePeroid(t *testing.T) {
	ctx, err := initCallContext()
	assert.NoError(t, err, "Init call context error")

	period, err := GetRewardsUpdatePeroid(ctx)
	if assert.NoError(t, err) {
		assert.Equal(t, 28800, int(period), "Read from system contract mismatch")
	}
}

func TestUpdateRewardsInfo(t *testing.T) {
	ctx, err := initCallContext()
	assert.NoError(t, err, "Init call context error")

	currRewardsPerBlock := func(ctx *contracts.CallContext) *big.Int {
		ret, ok := readSystemContract(t, ctx, "currRewardsPerBlock").(*big.Int)
		assert.True(t, ok, "invalid result format")
		return ret
	}

	period, err := GetRewardsUpdatePeroid(ctx)
	assert.NoError(t, err)

	ctx.Header.Number.SetUint64(period)

	assert.Equal(t, new(big.Int).Div(core.RewardsByMonth(0)[0], blocksPerMonth), currRewardsPerBlock(ctx))

	ctx.Header.Number.SetUint64(period * 33)

	assert.NoError(t, UpdateRewardsInfo(ctx))

	assert.Equal(t, new(big.Int).Div(core.RewardsByMonth(0)[1], blocksPerMonth), currRewardsPerBlock(ctx))
}

func TestDistributeBlockFee(t *testing.T) {
	ctx, err := initCallContext()
	assert.NoError(t, err, "Init call context error")

	getValidatorFee := func(val common.Address) *big.Int {
		contract, ok := readSystemContract(t, ctx, "valMaps", val).(common.Address)
		assert.True(t, ok, "invalid contract format")
		fee, ok := readContract(t, ctx, &contract, "currFeeRewards").(*big.Int)
		assert.True(t, ok, "invalid fee format")
		return fee
	}

	assert.NoError(t, UpdateActiveValidatorSet(ctx, GenesisValidators))

	origin := ctx.Statedb.GetBalance(ctx.Header.Coinbase)
	fee := big.NewInt(1000000000000000000)

	assert.NoError(t, DistributeBlockFee(ctx, fee))

	assert.Equal(t, new(big.Int).Sub(origin.ToBig(), fee), ctx.Statedb.GetBalance(ctx.Header.Coinbase))

	assert.Equal(t, big.NewInt(fee.Int64()/5), ctx.Statedb.GetBalance(system.CommunityPoolContract))

	valAmount := big.NewInt(fee.Int64() / 5 * 4 / 2)
	assert.Equal(t, valAmount, getValidatorFee(GenesisValidators[0]))
	assert.Equal(t, valAmount, getValidatorFee(GenesisValidators[1]))

}

func TestLazyPunish(t *testing.T) {
	ctx, err := initCallContext()
	assert.NoError(t, err, "Init call context error")
	getPunishRecord := func(val common.Address) int {
		count, ok := readSystemContract(t, ctx, "getPunishRecord", val).(*big.Int)
		assert.True(t, ok, "invalid validator format")
		return int(count.Int64())
	}

	assert.Equal(t, 0, getPunishRecord(GenesisValidators[0]))

	assert.NoError(t, LazyPunish(ctx, GenesisValidators[0]))

	assert.Equal(t, 1, getPunishRecord(GenesisValidators[0]))

}

func TestDoubleSignPunish(t *testing.T) {
	ctx, err := initCallContext()
	assert.NoError(t, err, "Init call context error")

	punishHash := common.BigToHash(big.NewInt(886))

	punished, err := IsDoubleSignPunished(ctx, punishHash)
	assert.NoError(t, err)
	assert.False(t, punished)

	assert.NoError(t, DoubleSignPunish(ctx, punishHash, GenesisValidators[0]))

	punished, err = IsDoubleSignPunished(ctx, punishHash)
	assert.NoError(t, err)
	assert.True(t, punished)
}

func TestDoubleSignPunishGivenEVM(t *testing.T) {
	ctx, err := initCallContext()
	assert.NoError(t, err, "Init call context error")

	punishHash := common.BigToHash(big.NewInt(886))

	punished, err := IsDoubleSignPunished(ctx, punishHash)
	assert.NoError(t, err)
	assert.False(t, punished)

	blockContext := core.NewEVMBlockContext(ctx.Header, ctx.ChainContext, nil)
	evm := vm.NewEVM(blockContext, vm.TxContext{}, ctx.Statedb, ctx.ChainConfig, vm.Config{})

	assert.NoError(t, DoubleSignPunishWithGivenEVM(evm, ctx.Header.Coinbase, punishHash, GenesisValidators[0]))

	punished, err = IsDoubleSignPunished(ctx, punishHash)
	assert.NoError(t, err)
	assert.True(t, punished)
}

func TestIsDoubleSignPunished(t *testing.T) {
	ctx, err := initCallContext()
	assert.NoError(t, err, "Init call context error")

	punishHash := common.BigToHash(big.NewInt(886))

	assert.NoError(t, DoubleSignPunish(ctx, punishHash, GenesisValidators[0]))

	check, err := IsDoubleSignPunished(ctx, punishHash)
	if assert.NoError(t, err) {
		assert.True(t, check)
	}
}

func TestGetBlacksFrom(t *testing.T) {
	ctx, err := initCallContext()
	assert.NoError(t, err, "Init call context error")

	assert.NoError(t, hardforksUpdate(ctx), "hardforksUpdate error")

	from, err := GetBlacksFrom(ctx)
	if assert.NoError(t, err) {
		assert.Equal(t, []common.Address{}, from)
	}

	ctx.Header.Coinbase = addressListAdmin
	addrFrom := common.BigToAddress(big.NewInt(111))
	var direct uint8 = 0
	writeContract(t, ctx, &system.AddressListContract, "addBlacklist", addrFrom, direct)

	from, err = GetBlacksFrom(ctx)
	if assert.NoError(t, err) {
		assert.Equal(t, []common.Address{addrFrom}, from)
	}
}

func TestGetBlacksTo(t *testing.T) {
	ctx, err := initCallContext()
	assert.NoError(t, err, "Init call context error")

	assert.NoError(t, hardforksUpdate(ctx), "hardforksUpdate error")

	ctx.Header.Coinbase = addressListAdmin
	addrTo := common.BigToAddress(big.NewInt(111))
	var direct uint8 = 1
	writeContract(t, ctx, &system.AddressListContract, "addBlacklist", addrTo, direct)

	to, err := GetBlacksTo(ctx)
	if assert.NoError(t, err) {
		assert.Equal(t, []common.Address{addrTo}, to)
	}
}

func TestGetRuleByIndex(t *testing.T) {
	ctx, err := initCallContext()
	assert.NoError(t, err, "Init call context error")

	assert.NoError(t, hardforksUpdate(ctx), "hardforksUpdate error")

	for i, rule := range []struct {
		sig   common.Hash
		idx   int
		ctype common.AddressCheckType
	}{
		{sig: common.HexToHash("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"), idx: 1, ctype: 1},
		{sig: common.HexToHash("0x06b541ddaa720db2b10a4d0cdac39b8d360425fc073085fac19bc82614677987"), idx: 2, ctype: 1},
		{sig: common.HexToHash("0xc3d58168c5ae7397731d063d5bbf3d657854427343f4c083240f7aacaa2d0f62"), idx: 2, ctype: 1},
		{sig: common.HexToHash("0x4a39dc06d4c0dbc64b70af90fd698a233a518aa5d07e595d983b8c0526c8f7fb"), idx: 2, ctype: 1},
	} {
		sig, idx, ctype, err := GetRuleByIndex(ctx, uint32(i))
		assert.NoError(t, err, "GetRuleByIndex error")
		assert.Equal(t, rule.sig, sig)
		assert.Equal(t, rule.idx, idx)
		assert.Equal(t, rule.ctype, ctype)
	}

	_, _, _, err = GetRuleByIndex(ctx, uint32(4))
	assert.Error(t, err)

}

func TestGetRulesLen(t *testing.T) {
	ctx, err := initCallContext()
	assert.NoError(t, err, "Init call context error")

	assert.NoError(t, hardforksUpdate(ctx), "hardforksUpdate error")

	length, err := GetRulesLen(ctx)
	assert.NoError(t, err, "GetRulesLen error")

	assert.Equal(t, uint32(4), length)
}

func TestIsDeveloperVerificationEnabled(t *testing.T) {
	ctx, err := initCallContext()
	assert.NoError(t, err, "Init call context error")

	assert.NoError(t, hardforksUpdate(ctx), "hardforksUpdate error")

	enabled, checkInnerCreation := IsDeveloperVerificationEnabled(ctx.Statedb)
	assert.Equal(t, false, enabled)
	assert.Equal(t, false, checkInnerCreation)

	ctx.Header.Coinbase = addressListAdmin
	writeContract(t, ctx, &system.AddressListContract, "enableDevVerify")
	enabled, checkInnerCreation = IsDeveloperVerificationEnabled(ctx.Statedb)
	assert.Equal(t, true, enabled)
	assert.Equal(t, false, checkInnerCreation)

	writeContract(t, ctx, &system.AddressListContract, "disableDevVerify")
	writeContract(t, ctx, &system.AddressListContract, "enableCheckInnerCreation")
	enabled, checkInnerCreation = IsDeveloperVerificationEnabled(ctx.Statedb)
	assert.Equal(t, false, enabled)
	assert.Equal(t, true, checkInnerCreation)

	writeContract(t, ctx, &system.AddressListContract, "enableDevVerify")
	enabled, checkInnerCreation = IsDeveloperVerificationEnabled(ctx.Statedb)
	assert.Equal(t, true, enabled)
	assert.Equal(t, true, checkInnerCreation)
}

func TestLastBlackUpdatedNumber(t *testing.T) {
	ctx, err := initCallContext()
	assert.NoError(t, err, "Init call context error")

	assert.NoError(t, hardforksUpdate(ctx), "hardforksUpdate error")

	number := LastBlackUpdatedNumber(ctx.Statedb)
	assert.Equal(t, uint64(200), number)

	ctx.Header.Coinbase = addressListAdmin
	ctx.Header.Number = big.NewInt(300)
	var direct uint8 = 1
	writeContract(t, ctx, &system.AddressListContract, "addBlacklist", common.BigToAddress(big.NewInt(111)), direct)

	number = LastBlackUpdatedNumber(ctx.Statedb)
	assert.Equal(t, uint64(300), number)

}

func TestLastRulesUpdatedNumber(t *testing.T) {
	ctx, err := initCallContext()
	assert.NoError(t, err, "Init call context error")

	assert.NoError(t, hardforksUpdate(ctx), "hardforksUpdate error")

	number := LastRulesUpdatedNumber(ctx.Statedb)
	assert.Equal(t, uint64(200), number)

	ctx.Header.Coinbase = addressListAdmin
	ctx.Header.Number = big.NewInt(300)
	writeContract(t, ctx, &system.AddressListContract, "removeRule",
		common.HexToHash("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"), common.Big1)

	number = LastRulesUpdatedNumber(ctx.Statedb)
	assert.Equal(t, uint64(300), number)
}

func TestGetPassedProposalCount(t *testing.T) {
	ctx, err := initCallContext()
	assert.NoError(t, err, "Init call context error")

	assert.NoError(t, hardforksUpdate(ctx), "hardforksUpdate error")

	ctx.Header.Coinbase = onChainDaoAdmin
	writeContract(t, ctx, &system.OnChainDaoContract, "commitProposal", big.NewInt(1),
		common.BigToAddress(big.NewInt(1)), common.BigToAddress(big.NewInt(2)),
		big.NewInt(0), common.Hex2BytesFixed("0xred", 24))

	count, err := GetPassedProposalCount(ctx)
	assert.NoError(t, err, "GetPassedProposalCount error")

	assert.Equal(t, uint32(1), count)

}

func TestGetPassedProposalByIndex(t *testing.T) {
	ctx, err := initCallContext()
	assert.NoError(t, err, "Init call context error")

	assert.NoError(t, hardforksUpdate(ctx), "hardforksUpdate error")

	var (
		action = big.NewInt(1)
		from   = common.BigToAddress(big.NewInt(1))
		to     = common.BigToAddress(big.NewInt(2))
		value  = big.NewInt(0)
		data   = common.Hex2BytesFixed("0xfade", 24)
	)

	ctx.Header.Coinbase = onChainDaoAdmin
	writeContract(t, ctx, &system.OnChainDaoContract, "commitProposal", action, from, to, value, data)

	prop, err := GetPassedProposalByIndex(ctx, 0)
	assert.NoError(t, err, "GetPassedProposalByIndex error")

	assert.Equal(t, action, prop.Action)
	assert.Equal(t, from, prop.From)
	assert.Equal(t, to, prop.To)
	assert.Equal(t, value.Uint64(), prop.Value.Uint64())
	assert.Equal(t, data, prop.Data)
}

func TestFinishProposalById(t *testing.T) {
	ctx, err := initCallContext()
	assert.NoError(t, err, "Init call context error")

	assert.NoError(t, hardforksUpdate(ctx), "hardforksUpdate error")

	ctx.Header.Coinbase = onChainDaoAdmin
	writeContract(t, ctx, &system.OnChainDaoContract, "commitProposal", big.NewInt(1),
		common.BigToAddress(big.NewInt(1)), common.BigToAddress(big.NewInt(2)),
		big.NewInt(0), common.Hex2BytesFixed("0xred", 24))
	writeContract(t, ctx, &system.OnChainDaoContract, "commitProposal", big.NewInt(2),
		common.BigToAddress(big.NewInt(3)), common.BigToAddress(big.NewInt(4)),
		big.NewInt(0), common.Hex2BytesFixed("0xdad", 24))

	count, err := GetPassedProposalCount(ctx)
	assert.NoError(t, err, "FinishProposalById error")

	assert.Equal(t, uint32(2), count)

	err = FinishProposalById(ctx, big.NewInt(1))
	assert.NoError(t, err, "FinishProposalById error")

	count, err = GetPassedProposalCount(ctx)
	assert.NoError(t, err, "FinishProposalById error")

	assert.Equal(t, uint32(1), count)
}

func TestExecuteProposal(t *testing.T) {
	ctx, err := initCallContext()
	assert.NoError(t, err, "Init call context error")

	assert.NoError(t, hardforksUpdate(ctx), "hardforksUpdate error")

	enabled, checkInnerCreation := IsDeveloperVerificationEnabled(ctx.Statedb)
	assert.NoError(t, err, "ExecuteProposal error")
	assert.Equal(t, false, enabled)
	assert.Equal(t, false, checkInnerCreation)

	tabi, err := abi.JSON(strings.NewReader(testAbi))
	data, err := tabi.Pack("enableDevVerify")
	assert.NoError(t, err, "ExecuteProposal error")
	prop := &Proposal{
		From:  addressListAdmin,
		To:    system.AddressListContract,
		Value: common.Big0,
		Data:  data,
	}
	err = ExecuteProposal(ctx, prop)
	assert.NoError(t, err, "ExecuteProposal error")

	enabled, checkInnerCreation = IsDeveloperVerificationEnabled(ctx.Statedb)
	assert.NoError(t, err, "ExecuteProposal error")
	assert.Equal(t, true, enabled)
	assert.Equal(t, false, checkInnerCreation)

	// enableCheckInnerCreation
	tabi, err = abi.JSON(strings.NewReader(testAbi))
	data, err = tabi.Pack("enableCheckInnerCreation")
	assert.NoError(t, err, "ExecuteProposal error")
	prop = &Proposal{
		From:  addressListAdmin,
		To:    system.AddressListContract,
		Value: common.Big0,
		Data:  data,
	}
	err = ExecuteProposal(ctx, prop)
	assert.NoError(t, err, "ExecuteProposal error")

	enabled, checkInnerCreation = IsDeveloperVerificationEnabled(ctx.Statedb)
	assert.NoError(t, err, "ExecuteProposal error")
	assert.Equal(t, true, enabled)
	assert.Equal(t, true, checkInnerCreation)
}

func TestExecuteProposalWithGivenEVM(t *testing.T) {
	ctx, err := initCallContext()
	assert.NoError(t, err, "Init call context error")

	assert.NoError(t, hardforksUpdate(ctx), "hardforksUpdate error")

	enabled, checkInnerCreation := IsDeveloperVerificationEnabled(ctx.Statedb)
	assert.NoError(t, err, "ExecuteProposalWithGivenEVM error")
	assert.Equal(t, false, enabled)
	assert.Equal(t, false, checkInnerCreation)

	tabi, err := abi.JSON(strings.NewReader(testAbi))
	data, err := tabi.Pack("enableDevVerify")
	assert.NoError(t, err, "ExecuteProposal error")
	prop := &Proposal{
		From:  addressListAdmin,
		To:    system.AddressListContract,
		Value: common.Big0,
		Data:  data,
	}

	blockContext := core.NewEVMBlockContext(ctx.Header, ctx.ChainContext, nil)
	evm := vm.NewEVM(blockContext, vm.TxContext{}, ctx.Statedb, ctx.ChainConfig, vm.Config{})
	_, err = ExecuteProposalWithGivenEVM(evm, prop, 100000)
	assert.NoError(t, err, "ExecuteProposalWithGivenEVM error")
	// enableCheckInnerCreation
	tabi, err = abi.JSON(strings.NewReader(testAbi))
	data, err = tabi.Pack("enableCheckInnerCreation")
	assert.NoError(t, err, "ExecuteProposal error")
	prop = &Proposal{
		From:  addressListAdmin,
		To:    system.AddressListContract,
		Value: common.Big0,
		Data:  data,
	}
	_, err = ExecuteProposalWithGivenEVM(evm, prop, 100000)
	assert.NoError(t, err, "ExecuteProposalWithGivenEVM error")

	enabled, checkInnerCreation = IsDeveloperVerificationEnabled(ctx.Statedb)
	assert.NoError(t, err, "ExecuteProposalWithGivenEVM error")
	assert.Equal(t, true, enabled)
	assert.Equal(t, true, checkInnerCreation)
}

// Utils function to do system contracts update of hardforks
func hardforksUpdate(ctx *contracts.CallContext) error {
	for _, hardfork := range []string{} {
		if err := ApplySystemContractUpgrade(hardfork, ctx.Statedb, ctx.Header, ctx.ChainContext, ctx.ChainConfig); err != nil {
			return err
		}
	}
	return nil
}

// Utils function to create call context
func initCallContext() (*contracts.CallContext, error) {
	file, err := os.Open("../../../core/testdata/test-genesis.json")
	if err != nil {
		return nil, err
	}
	defer file.Close()
	genesis := new(core.Genesis)
	if err := json.NewDecoder(file).Decode(genesis); err != nil {
		return nil, err
	}

	db := rawdb.NewMemoryDatabase()

	genesisBlock := genesis.ToBlock()

	header := &types.Header{
		ParentHash: genesisBlock.Hash(),
		Number:     big.NewInt(200),
		Difficulty: common.Big2,
		Time:       uint64(time.Now().Unix()),
		Coinbase:   common.HexToAddress("0x352BbF453fFdcba6b126a73eD684260D7968dDc8"),
	}

	var statedb *state.StateDB
	if statedb, err = state.New(genesisBlock.Root(), state.NewDatabase(db), nil); err != nil {
		return nil, err
	}

	return &contracts.CallContext{
		Statedb:      statedb,
		Header:       header,
		ChainContext: &MockChainContext{header, &MockConsensusEngine{}},
		ChainConfig:  genesis.Config,
	}, nil
}

func readSystemContract(t *testing.T, ctx *contracts.CallContext, method string, args ...interface{}) interface{} {
	return readContract(t, ctx, &system.StakingContract, method, args...)
}

func readContract(t *testing.T, ctx *contracts.CallContext, contract *common.Address, method string, args ...interface{}) interface{} {
	abi, err := abi.JSON(strings.NewReader(testAbi))
	// execute contract
	data, err := abi.Pack(method, args...)
	assert.NoError(t, err)

	result, err := contracts.CallContract(ctx, contract, data)
	assert.NoError(t, err)

	// unpack data
	ret, err := abi.Unpack(method, result)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(ret), "invalid result length")
	return ret[0]
}

func writeContract(t *testing.T, ctx *contracts.CallContext, contract *common.Address, method string, args ...interface{}) {
	abi, err := abi.JSON(strings.NewReader(testAbi))
	// execute contract
	data, err := abi.Pack(method, args...)
	assert.NoError(t, err)

	_, err = contracts.CallContract(ctx, contract, data)
	assert.NoError(t, err)
}

// MockChainContext implements ChainContext for unit test
type MockChainContext struct {
	header *types.Header
	engine consensus.Engine
}

func (c *MockChainContext) Engine() consensus.Engine {
	return c.engine
}

func (c *MockChainContext) GetHeader(common.Hash, uint64) *types.Header {
	return c.header
}

// MockConsensusEngine implements Engine for unit test
type MockConsensusEngine struct {
}

func (c *MockConsensusEngine) Author(header *types.Header) (common.Address, error) {
	return header.Coinbase, nil
}

func (c *MockConsensusEngine) VerifyHeader(chain consensus.ChainHeaderReader, header *types.Header) error {
	return nil
}

func (c *MockConsensusEngine) VerifyHeaders(chain consensus.ChainHeaderReader, headers []*types.Header) (chan<- struct{}, <-chan error) {
	return make(chan struct{}), make(chan error, len(headers))
}

func (c *MockConsensusEngine) VerifyUncles(chain consensus.ChainReader, block *types.Block) error {
	return nil
}

func (c *MockConsensusEngine) Prepare(chain consensus.ChainHeaderReader, header *types.Header) error {
	return nil
}

func (c *MockConsensusEngine) Finalize(chain consensus.ChainHeaderReader, header *types.Header, state *state.StateDB, body *types.Body,
	receipts *[]*types.Receipt, punishTxs []*types.Transaction, proposalTxs []*types.Transaction) error {
	return nil
}

func (c *MockConsensusEngine) FinalizeAndAssemble(chain consensus.ChainHeaderReader, header *types.Header, state *state.StateDB, body *types.Body, receipts []*types.Receipt) (*types.Block, []*types.Receipt, error) {
	return nil, receipts, nil
}

func (c *MockConsensusEngine) Seal(chain consensus.ChainHeaderReader, block *types.Block, results chan<- *types.Block, stop <-chan struct{}) error {
	return nil
}

func (c *MockConsensusEngine) CalcDifficulty(chain consensus.ChainHeaderReader, time uint64, parent *types.Header) *big.Int {
	return common.Big2
}

func (c *MockConsensusEngine) SealHash(header *types.Header) common.Hash {
	return common.Hash{}
}

func (c *MockConsensusEngine) Close() error {
	return nil
}

func (c *MockConsensusEngine) APIs(chain consensus.ChainHeaderReader) []rpc.API {
	return []rpc.API{}
}
