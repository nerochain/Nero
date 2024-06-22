package turbo

import (
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/params"
	lru "github.com/hashicorp/golang-lru"
)

// StateFn gets state by the state root hash.
type StateFn func(hash common.Hash) (*state.StateDB, error)

// ValidatorFn hashes and signs the data to be signed by a backing account.
type ValidatorFn func(validator accounts.Account, mimeType string, message []byte) ([]byte, error)
type SignTxFn func(account accounts.Account, tx *types.Transaction, chainID *big.Int) (*types.Transaction, error)

type Turbo struct {
	chainConfig *params.ChainConfig // ChainConfig to execute evm
	config      *params.TurboConfig // Consensus engine configuration parameters
	db          ethdb.Database      // Database to store and retrieve snapshot checkpoints

	recents    *lru.ARCCache // Snapshots for recent block to speed up reorgs
	signatures *lru.ARCCache // Signatures of recent blocks to speed up mining

	accesslist      *lru.Cache // accesslists caches recent accesslist to speed up transactions validation
	accessLock      sync.Mutex // Make sure only get accesslist once for each block
	eventCheckRules *lru.Cache // eventCheckRules caches recent EventCheckRules to speed up log validation
	rulesLock       sync.Mutex // Make sure only get eventCheckRules once for each block

	signer types.Signer // the signer instance to recover tx sender

	validator common.Address // Ethereum address of the signing key
	signFn    ValidatorFn    // Validator function to authorize hashes with
	signTxFn  SignTxFn
	isReady   bool         // isReady indicates whether the engine is ready for mining
	lock      sync.RWMutex // Protects the validator fields

	stateFn StateFn // Function to get state by state root

	rewardsUpdatePeroid uint64 // block rewards update perroid in number of blocks

	chain consensus.ChainHeaderReader

	// The fields below are for testing only
	fakeDiff bool // Skip difficulty verifications

	attestationStatus uint8
}

// New creates a Turbo proof-of-stake-authority consensus engine with the initial
// validators set to the ones provided by the user.
func New(chainConfig *params.ChainConfig, db ethdb.Database) *Turbo {
	return &Turbo{}
}
