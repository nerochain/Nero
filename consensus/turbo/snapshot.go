package turbo

import (
	"encoding/json"
	"math/big"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/turbo/systemcontract"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/params"
	lru "github.com/hashicorp/golang-lru"
)

// Snapshot is the state of the authorization voting at a given point in time.
type Snapshot struct {
	config   *params.ChainConfig // Consensus engine parameters to fine tune behavior
	sigcache *lru.ARCCache       // Cache of recent block signatures to speed up ecrecover

	Number     uint64                      `json:"number"`     // Block number where the snapshot was created
	Hash       common.Hash                 `json:"hash"`       // Block hash where the snapshot was created
	Validators map[common.Address]struct{} `json:"validators"` // Set of authorized validators at this moment
	Recents    map[uint64]common.Address   `json:"recents"`    // Set of recent validators for spam protections
}

// newSnapshot creates a new snapshot with the specified startup parameters. This
// method does not initialize the set of recent validators, so only ever use if for
// the genesis block.
func newSnapshot(config *params.ChainConfig, sigcache *lru.ARCCache, number uint64, hash common.Hash, validators []common.Address) *Snapshot {
	snap := &Snapshot{
		config:     config,
		sigcache:   sigcache,
		Number:     number,
		Hash:       hash,
		Validators: make(map[common.Address]struct{}),
		Recents:    make(map[uint64]common.Address),
	}
	for _, validator := range validators {
		snap.Validators[validator] = struct{}{}
	}
	return snap
}

// loadSnapshot loads an existing snapshot from the database.
func loadSnapshot(config *params.ChainConfig, sigcache *lru.ARCCache, db ethdb.Database, hash common.Hash) (*Snapshot, error) {
	blob, err := db.Get(append([]byte("turbo-"), hash[:]...))
	if err != nil {
		return nil, err
	}
	snap := new(Snapshot)
	if err := json.Unmarshal(blob, snap); err != nil {
		return nil, err
	}
	snap.config = config
	snap.sigcache = sigcache

	return snap, nil
}

// store inserts the snapshot into the database.
func (s *Snapshot) store(db ethdb.Database) error {
	blob, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return db.Put(append([]byte("turbo-"), s.Hash[:]...), blob)
}

// copy creates a deep copy of the snapshot, though not the individual votes.
func (s *Snapshot) copy() *Snapshot {
	cpy := &Snapshot{
		config:     s.config,
		sigcache:   s.sigcache,
		Number:     s.Number,
		Hash:       s.Hash,
		Validators: make(map[common.Address]struct{}),
		Recents:    make(map[uint64]common.Address),
	}
	for validator := range s.Validators {
		cpy.Validators[validator] = struct{}{}
	}
	for block, validator := range s.Recents {
		cpy.Recents[block] = validator
	}

	return cpy
}

// SignedRecently checks whether the validator signed block recently
func (s *Snapshot) SignedRecently(block uint64, validator common.Address) bool {
	continuousInturn := s.config.TurboContinuousInturn(big.NewInt(int64(block)))
	limit := uint64(len(s.Validators)/2+1) * continuousInturn
	var count uint64
	for blockNum, recent := range s.Recents {
		if blockNum != block-limit && recent == validator {
			count++
		}
	}
	return count >= continuousInturn
}

// apply creates a new authorization snapshot by applying the given headers to
// the original one.
func (s *Snapshot) apply(headers []*types.Header, chain consensus.ChainHeaderReader, parents []*types.Header) (*Snapshot, error) {
	// Allow passing in no headers for cleaner code
	if len(headers) == 0 {
		return s, nil
	}
	// Sanity check that the headers can be applied
	for i := 0; i < len(headers)-1; i++ {
		if headers[i+1].Number.Uint64() != headers[i].Number.Uint64()+1 {
			return nil, errInvalidVotingChain
		}
	}
	if headers[0].Number.Uint64() != s.Number+1 {
		return nil, errInvalidVotingChain
	}
	// Iterate through the headers and create a new snapshot
	snap := s.copy()

	for i, header := range headers {
		// Remove any votes on checkpoint blocks
		number := header.Number.Uint64()
		continuousInturn := s.config.TurboContinuousInturn(header.Number)
		if limit := uint64(len(snap.Validators)/2+1) * continuousInturn; number >= limit {
			// Delete the oldest validator from the recent list to allow it signing again
			delete(snap.Recents, number-limit)
		}
		// Resolve the authorization key and check against validators
		validator, err := ecrecover(header, s.sigcache)
		if err != nil {
			return nil, err
		}
		if _, ok := snap.Validators[validator]; !ok {
			return nil, errUnauthorizedValidator
		}
		if snap.SignedRecently(number, validator) {
			return nil, errRecentlySigned
		}
		snap.Recents[number] = validator

		// Before the first epoch block after Waterdrop hard-fork: update validators at the first block at epoch;
		// Starting from the first epoch block after Waterdrop hard-fork: use a look-back validator.
		// Which means: the blocks in range [1, ((waterdropBlock/EpochPeriod)+1)*EpochPeriod ] are using the latest validators set;
		// the blocks ≥ ((waterdropBlock/EpochPeriod)+1)*EpochPeriod + 1 are using the look-back validators set.
		if number > 0 && number%s.config.Turbo.Epoch == 0 {
			var checkpointHeader *types.Header
			// For a large chain insertion, the previous blocks may not have been written to db,
			// so we need to find it through both previous `headers` and parents
			if uint64(i) >= s.config.Turbo.Epoch {
				checkpointHeader = headers[i-int(s.config.Turbo.Epoch)]
			} else {
				// i < epoch ==> epoch -i >= 1
				idxInParents := len(parents) - (int(s.config.Turbo.Epoch) - i)
				if idxInParents >= 0 {
					checkpointHeader = parents[idxInParents]
				} else {
					checkpointHeader = chain.GetHeaderByNumber(number - s.config.Turbo.Epoch)
					if checkpointHeader == nil {
						return nil, consensus.ErrUnknownAncestor
					}
				}
			}

			// get validators from headers and use that for new validator set
			validators := make([]common.Address, (len(checkpointHeader.Extra)-extraVanity-extraSeal)/common.AddressLength)
			for i := 0; i < len(validators); i++ {
				copy(validators[i][:], checkpointHeader.Extra[extraVanity+i*common.AddressLength:])
			}

			newValidators := make(map[common.Address]struct{})
			for _, validator := range validators {
				newValidators[validator] = struct{}{}
			}

			// need to delete recorded recent seen blocks if necessary, it may pause whole chain when validators length
			// decreases.
			limit := uint64(len(newValidators)/2+1) * continuousInturn
			for i := 0; i < (len(snap.Validators)/2-len(newValidators)/2)*int(continuousInturn); i++ {
				delete(snap.Recents, number-limit-uint64(i))
			}

			snap.Validators = newValidators
		}
	}

	snap.Number += uint64(len(headers))
	snap.Hash = headers[len(headers)-1].Hash()

	return snap, nil
}

// validators retrieves the list of authorized validators in ascending order.
func (s *Snapshot) validators() []common.Address {
	sigs := make([]common.Address, 0, len(s.Validators))
	for sig := range s.Validators {
		sigs = append(sigs, sig)
	}
	sort.Sort(systemcontract.AddrAscend(sigs))
	return sigs
}

// inturn returns if a validator at a given block height is in-turn or not.
func (s *Snapshot) inturn(number uint64, validator common.Address) bool {
	validators, offset := s.validators(), 0
	for offset < len(validators) && validators[offset] != validator {
		offset++
	}
	continuousInturn := s.config.TurboContinuousInturn(big.NewInt(int64(number)))
	return (number%(uint64(len(validators))*continuousInturn))/continuousInturn == uint64(offset)
}

func (s *Snapshot) IsAuthorized(addr common.Address) bool {
	_, exist := s.Validators[addr]
	return exist
}

func (s *Snapshot) Len() int {
	return len(s.Validators)
}
