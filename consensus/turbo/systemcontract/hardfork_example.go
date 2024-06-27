package systemcontract

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/contracts"
	"github.com/ethereum/go-ethereum/contracts/system"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

var (
	addressListAdmin        = common.HexToAddress("0x0000000000000000000000000000000000000000")
	addressListAdminTestnet = common.HexToAddress("0x0000000000000000000000000000000000000000")

	onChainDaoAdmin        = common.HexToAddress("0x0000000000000000000000000000000000000000")
	onChainDaoAdminTestnet = common.HexToAddress("0x0000000000000000000000000000000000000000")

	AdminDevnet common.Address
)

func ExampleHardFork() []IUpgradeAction {
	return []IUpgradeAction{
		&ContractV2{},
		&AddressList{},
		&OnChainDao{},
	}
}

type ContractV2 struct {
}

func (h *ContractV2) GetName() string {
	return "ContractV2"
}

func (s *ContractV2) DoUpdate(state *state.StateDB, header *types.Header, chainContext core.ChainContext, config *params.ChainConfig) (err error) {
	contractCode := common.FromHex(system.StakingV2Code)
	//write code to sys contract
	state.SetCode(system.StakingContract, contractCode)
	log.Debug("Write code to system contract account", "addr", system.StakingContract, "code", system.StakingV2Code)
	return
}

// AddressList is used to manage tx by address
type AddressList struct {
}

func (s *AddressList) GetName() string {
	return "AddressList"
}

func (s *AddressList) DoUpdate(state *state.StateDB, header *types.Header, chainContext core.ChainContext, config *params.ChainConfig) (err error) {
	contractCode := common.FromHex(system.AddressListCode)
	//write addressListCode to sys contract
	state.SetCode(system.AddressListContract, contractCode)
	log.Debug("Write code to system contract account", "addr", system.AddressListContract, "code", system.AddressListCode)

	method := "initialize"

	admin := addressListAdminTestnet
	if config.ChainID.Cmp(params.MainnetChainConfig.ChainID) == 0 {
		admin = addressListAdmin
	} else if config.ChainID.Cmp(params.TestnetChainConfig.ChainID) != 0 && (AdminDevnet != common.Address{}) {
		admin = AdminDevnet
	}

	data, err := system.ABIPack(system.AddressListContract, method, admin)
	if err != nil {
		log.Error("Can't pack data for initialize", "error", err)
		return err
	}

	_, err = contracts.CallContract(&contracts.CallContext{
		Statedb:      state,
		Header:       header,
		ChainContext: chainContext,
		ChainConfig:  config,
	}, &system.AddressListContract, data)
	return err
}

// OnChainDao is used to manage proposal
type OnChainDao struct {
}

func (s *OnChainDao) GetName() string {
	return "OnChainDao"
}

func (s *OnChainDao) DoUpdate(state *state.StateDB, header *types.Header, chainContext core.ChainContext, config *params.ChainConfig) (err error) {
	contractCode := common.FromHex(system.OnChainDaoCode)
	//write Code to sys contract
	state.SetCode(system.OnChainDaoContract, contractCode)
	log.Debug("Write code to system contract account", "addr", system.OnChainDaoContract, "code", system.OnChainDaoCode)

	method := "initialize"

	admin := onChainDaoAdminTestnet
	if config.ChainID.Cmp(params.MainnetChainConfig.ChainID) == 0 {
		admin = onChainDaoAdmin
	} else if config.ChainID.Cmp(params.TestChainConfig.ChainID) != 0 && (AdminDevnet != common.Address{}) {
		admin = AdminDevnet
	}
	data, err := system.ABIPack(system.OnChainDaoContract, method, admin)
	if err != nil {
		log.Error("Can't pack data for initialize", "error", err)
		return err
	}
	_, err = contracts.CallContract(&contracts.CallContext{
		Statedb:      state,
		Header:       header,
		ChainContext: chainContext,
		ChainConfig:  config,
	}, &system.OnChainDaoContract, data)
	return err
}
