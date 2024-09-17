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

func ExampleHardFork() []IUpgradeAction {
	return []IUpgradeAction{
		&ContractV2{},
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
