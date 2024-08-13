package systemcontract

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/contracts/system"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

var (
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
	contractCode := common.FromHex(system.StakingV1Code)
	//write code to sys contract
	state.SetCode(system.StakingContract, contractCode)
	log.Debug("Write code to system contract account", "addr", system.StakingContract, "code", system.StakingV1Code)
	return
}

// AddressList is used to manage tx by address
type AddressList struct {
}

func (s *AddressList) GetName() string {
	return "AddressList"
}

func (s *AddressList) DoUpdate(state *state.StateDB, header *types.Header, chainContext core.ChainContext, config *params.ChainConfig) (err error) {
	return nil
}

// OnChainDao is used to manage proposal
type OnChainDao struct {
}

func (s *OnChainDao) GetName() string {
	return "OnChainDao"
}

func (s *OnChainDao) DoUpdate(state *state.StateDB, header *types.Header, chainContext core.ChainContext, config *params.ChainConfig) (err error) {
	return nil
}
