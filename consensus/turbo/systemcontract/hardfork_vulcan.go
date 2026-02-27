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

func VulcanHardFork() []IUpgradeAction {
	return []IUpgradeAction{
		&StakingV2{},
	}
}

type StakingV2 struct {
}

func (s *StakingV2) GetName() string {
	return "StakingV2"
}

func (s *StakingV2) DoUpdate(state *state.StateDB, header *types.Header, chainContext core.ChainContext, config *params.ChainConfig) (err error) {
	contractCode := common.FromHex(system.StakingV2Code)
	//write code to sys contract
	state.SetCode(system.StakingContract, contractCode)
	log.Debug("Write code to system contract account", "addr", system.StakingContract, "code", system.StakingV2Code)
	return
}

func VulcanV2HardFork() []IUpgradeAction {
	return []IUpgradeAction{
		&StakingV3{},
	}
}

type StakingV3 struct {
}

func (s *StakingV3) GetName() string {
	return "StakingV3"
}

func (s *StakingV3) DoUpdate(state *state.StateDB, header *types.Header, chainContext core.ChainContext, config *params.ChainConfig) (err error) {
	oldCode := state.GetCode(system.StakingContract)
	log.Info("StakingV3 upgrade begin",
		"addr", system.StakingContract,
		"oldLen", len(oldCode),
		"height", header.Number,
		"chainId", config.ChainID)

	contractCode := common.FromHex(system.StakingV3Code)
	state.SetCode(system.StakingContract, contractCode)

	newCode := state.GetCode(system.StakingContract)
	log.Info("StakingV3 upgrade done",
		"addr", system.StakingContract,
		"newLen", len(newCode),
		"height", header.Number,
		"chainId", config.ChainID)
	return
}
