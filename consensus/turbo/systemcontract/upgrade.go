package systemcontract

import (
	"math/big"

	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

const (
	Vulcan = "vulcan"
)

var hardforkContracts map[string][]IUpgradeAction = map[string][]IUpgradeAction{
	// Add forks here
	Vulcan: VulcanHardFork(),
}

// IUpgradeAction is the interface for system contracts upgrades
type IUpgradeAction interface {
	// GetName returns the name of the updated system contract
	GetName() string

	// DoUpdate is used to add/update system contracts as well as initialization
	DoUpdate(state *state.StateDB, header *types.Header, chainContext core.ChainContext, config *params.ChainConfig) error
}

// Hardfork conbines name and number
type Hardfork struct {
	Name   string
	Number *big.Int
}

// ApplySystemContractUpgrade updates the system contract when hardfork happens
// NOTE: this function will always returl nil error in order to not break the consensus when fail
func ApplySystemContractUpgrade(hardfork string, state *state.StateDB, header *types.Header, chainContext core.ChainContext, config *params.ChainConfig) (err error) {
	if config == nil || header == nil || state == nil {
		log.Error("System contract upgrade failed due to unexpected env", "hardfork", hardfork, "config", config, "header", header, "state", state)
		return
	}
	if contracts, ok := hardforkContracts[hardfork]; ok {
		log.Info("Begin system contacts upgrade", "hardfork", hardfork, "height", header.Number, "chainId", config.ChainID)
		for _, contract := range contracts {
			log.Info("Upgrade system contract", "name", contract.GetName())
			if err = contract.DoUpdate(state, header, chainContext, config); err != nil {
				log.Error("Upgrade system contract error", "hardfork", hardfork, "name", contract.GetName(), "err", err)
				return
			}
		}
		log.Info("System contacts upgrade success", "hardfork", hardfork, "height", header.Number, "chainId", config.ChainID)
		return
	}
	log.Error("System contract upgrade failed due to unsupported hardfork", "hardfork", hardfork, "height", header.Number)
	return
}
