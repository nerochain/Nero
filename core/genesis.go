// Copyright 2014 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/contracts/system"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/ethereum/go-ethereum/triedb"
	"github.com/ethereum/go-ethereum/triedb/pathdb"
	"github.com/holiman/uint256"
)

//go:generate go run github.com/fjl/gencodec -type Genesis -field-override genesisSpecMarshaling -out gen_genesis.go

var errGenesisNoConfig = errors.New("genesis has no chain configuration")

// Deprecated: use types.Account instead.
type GenesisAccount = types.Account

// Deprecated: use types.GenesisAlloc instead.
type GenesisAlloc = types.GenesisAlloc

// Genesis specifies the header fields, state of a genesis block. It also defines hard
// fork switch-over blocks through the chain configuration.
type Genesis struct {
	Config     *params.ChainConfig   `json:"config"`
	Nonce      uint64                `json:"nonce"`
	Timestamp  uint64                `json:"timestamp"`
	ExtraData  []byte                `json:"extraData"`
	GasLimit   uint64                `json:"gasLimit"   gencodec:"required"`
	Difficulty *big.Int              `json:"difficulty" gencodec:"required"`
	Mixhash    common.Hash           `json:"mixHash"`
	Coinbase   common.Address        `json:"coinbase"`
	Alloc      types.GenesisAlloc    `json:"alloc"      gencodec:"required"`
	Validators []types.ValidatorInfo `json:"validators"`

	// These fields are used for consensus tests. Please don't use them
	// in actual genesis blocks.
	Number        uint64      `json:"number"`
	GasUsed       uint64      `json:"gasUsed"`
	ParentHash    common.Hash `json:"parentHash"`
	BaseFee       *big.Int    `json:"baseFeePerGas"` // EIP-1559
	ExcessBlobGas *uint64     `json:"excessBlobGas"` // EIP-4844
	BlobGasUsed   *uint64     `json:"blobGasUsed"`   // EIP-4844
}

func ReadGenesis(db ethdb.Database) (*Genesis, error) {
	var genesis Genesis
	stored := rawdb.ReadCanonicalHash(db, 0)
	if (stored == common.Hash{}) {
		return nil, fmt.Errorf("invalid genesis hash in database: %x", stored)
	}
	blob := rawdb.ReadGenesisStateSpec(db, stored)
	if blob == nil {
		return nil, errors.New("genesis state missing from db")
	}
	if len(blob) != 0 {
		if err := genesis.Alloc.UnmarshalJSON(blob); err != nil {
			return nil, fmt.Errorf("could not unmarshal genesis state json: %s", err)
		}
	}
	genesis.Config = rawdb.ReadChainConfig(db, stored)
	if genesis.Config == nil {
		return nil, errors.New("genesis config missing from db")
	}
	genesisBlock := rawdb.ReadBlock(db, stored, 0)
	if genesisBlock == nil {
		return nil, errors.New("genesis block missing from db")
	}
	genesisHeader := genesisBlock.Header()
	genesis.Nonce = genesisHeader.Nonce.Uint64()
	genesis.Timestamp = genesisHeader.Time
	genesis.ExtraData = genesisHeader.Extra
	genesis.GasLimit = genesisHeader.GasLimit
	genesis.Difficulty = genesisHeader.Difficulty
	genesis.Mixhash = genesisHeader.MixDigest
	genesis.Coinbase = genesisHeader.Coinbase
	genesis.BaseFee = genesisHeader.BaseFee
	genesis.ExcessBlobGas = genesisHeader.ExcessBlobGas
	genesis.BlobGasUsed = genesisHeader.BlobGasUsed

	return &genesis, nil
}

// hashAlloc computes the state root according to the genesis specification.
func hashAlloc(ga *types.GenesisAlloc, isVerkle bool) (common.Hash, state.Database, error) {
	// If a genesis-time verkle trie is requested, create a trie config
	// with the verkle trie enabled so that the tree can be initialized
	// as such.
	var config *triedb.Config
	if isVerkle {
		config = &triedb.Config{
			PathDB:   pathdb.Defaults,
			IsVerkle: true,
		}
	}
	// Create an ephemeral in-memory database for computing hash,
	// all the derived states will be discarded to not pollute disk.
	db := state.NewDatabaseWithConfig(rawdb.NewMemoryDatabase(), config)
	statedb, err := state.New(types.EmptyRootHash, db, nil)
	if err != nil {
		return common.Hash{}, nil, err
	}
	for addr, account := range *ga {
		if account.Balance != nil {
			statedb.AddBalance(addr, uint256.MustFromBig(account.Balance), tracing.BalanceIncreaseGenesisBalance)
		}
		statedb.SetCode(addr, account.Code)
		statedb.SetNonce(addr, account.Nonce)
		for key, value := range account.Storage {
			statedb.SetState(addr, key, value)
		}
	}
	root, err := statedb.Commit(0, false)
	return root, db, err
}

// flushAlloc is very similar with hash, but the main difference is all the generated
// states will be persisted into the given database. Also, the genesis state
// specification will be flushed as well.
func flushAlloc(g *Genesis, db ethdb.Database, triedb *triedb.Database, block *types.Block) error {
	statedb, err := state.New(types.EmptyRootHash, state.NewDatabaseWithNodeDB(db, triedb), nil)
	if err != nil {
		return err
	}
	ga := &g.Alloc
	for addr, account := range *ga {
		if account.Balance != nil {
			// This is not actually logged via tracer because OnGenesisBlock
			// already captures the allocations.
			statedb.AddBalance(addr, uint256.MustFromBig(account.Balance), tracing.BalanceIncreaseGenesisBalance)
		}
		statedb.SetCode(addr, account.Code)
		statedb.SetNonce(addr, account.Nonce)
		for key, value := range account.Storage {
			statedb.SetState(addr, key, value)
		}
	}
	// Handle the Turbo related
	if g.Config != nil && g.Config.Turbo != nil {
		// init system contract
		head := block.Header()
		gInit := &genesisInit{statedb, head, g}
		for name, initSystemContract := range map[string]func() error{
			"Staking":     gInit.initStaking,
			"GenesisLock": gInit.initGenesisLock,
		} {
			if err = initSystemContract(); err != nil {
				log.Crit("Failed to init system contract", "contract", name, "err", err)
			}
		}
		// Set validoter info
		if head.Extra, err = gInit.initValidators(); err != nil {
			log.Crit("Failed to init Validators", "err", err)
		}
	}
	root, err := statedb.Commit(0, false)
	if err != nil {
		return err
	}
	// Commit newly generated states into disk if it's not empty.
	if root != types.EmptyRootHash {
		if err := triedb.Commit(root, true); err != nil {
			return err
		}
	}
	// Marshal the genesis state specification and persist.
	blob, err := json.Marshal(ga)
	if err != nil {
		return err
	}
	rawdb.WriteGenesisStateSpec(db, block.Hash(), blob)
	return nil
}

func getGenesisState(db ethdb.Database, blockhash common.Hash) (alloc types.GenesisAlloc, err error) {
	blob := rawdb.ReadGenesisStateSpec(db, blockhash)
	if len(blob) != 0 {
		if err := alloc.UnmarshalJSON(blob); err != nil {
			return nil, err
		}

		return alloc, nil
	}

	// Genesis allocation is missing and there are several possibilities:
	// the node is legacy which doesn't persist the genesis allocation or
	// the persisted allocation is just lost.
	// - supported networks(mainnet, testnets), recover with defined allocations
	// - private network, can't recover
	var genesis *Genesis
	switch blockhash {
	case params.MainnetGenesisHash:
		genesis = DefaultGenesisBlock()
	case params.GoerliGenesisHash:
		genesis = DefaultGoerliGenesisBlock()
	case params.SepoliaGenesisHash:
		genesis = DefaultSepoliaGenesisBlock()
	case params.HoleskyGenesisHash:
		genesis = DefaultHoleskyGenesisBlock()
	}
	if genesis != nil {
		return genesis.Alloc, nil
	}

	return nil, nil
}

// field type overrides for gencodec
type genesisSpecMarshaling struct {
	Nonce         math.HexOrDecimal64
	Timestamp     math.HexOrDecimal64
	ExtraData     hexutil.Bytes
	GasLimit      math.HexOrDecimal64
	GasUsed       math.HexOrDecimal64
	Number        math.HexOrDecimal64
	Difficulty    *math.HexOrDecimal256
	Alloc         map[common.UnprefixedAddress]types.Account
	BaseFee       *math.HexOrDecimal256
	ExcessBlobGas *math.HexOrDecimal64
	BlobGasUsed   *math.HexOrDecimal64
}

// GenesisMismatchError is raised when trying to overwrite an existing
// genesis block with an incompatible one.
type GenesisMismatchError struct {
	Stored, New common.Hash
}

func (e *GenesisMismatchError) Error() string {
	return fmt.Sprintf("database contains incompatible genesis (have %x, new %x)", e.Stored, e.New)
}

// ChainOverrides contains the changes to chain config.
type ChainOverrides struct {
	OverrideCancun *uint64
	OverrideVerkle *uint64
}

// SetupGenesisBlock writes or updates the genesis block in db.
// The block that will be used is:
//
//	                     genesis == nil       genesis != nil
//	                  +------------------------------------------
//	db has no genesis |  main-net default  |  genesis
//	db has genesis    |  from DB           |  genesis (if compatible)
//
// The stored chain configuration will be updated if it is compatible (i.e. does not
// specify a fork block below the local head block). In case of a conflict, the
// error is a *params.ConfigCompatError and the new, unwritten config is returned.
//
// The returned chain configuration is never nil.
func SetupGenesisBlock(db ethdb.Database, triedb *triedb.Database, genesis *Genesis) (*params.ChainConfig, common.Hash, error) {
	return SetupGenesisBlockWithOverride(db, triedb, genesis, nil)
}

func SetupGenesisBlockWithOverride(db ethdb.Database, triedb *triedb.Database, genesis *Genesis, overrides *ChainOverrides) (*params.ChainConfig, common.Hash, error) {
	if genesis != nil && genesis.Config == nil {
		return params.AllEthashProtocolChanges, common.Hash{}, errGenesisNoConfig
	}
	applyOverrides := func(config *params.ChainConfig) {
		if config != nil {
			if overrides != nil && overrides.OverrideCancun != nil {
				config.CancunTime = overrides.OverrideCancun
			}
			if overrides != nil && overrides.OverrideVerkle != nil {
				config.VerkleTime = overrides.OverrideVerkle
			}
		}
	}
	// Just commit the new block if there is no stored genesis block.
	stored := rawdb.ReadCanonicalHash(db, 0)
	if (stored == common.Hash{}) {
		if genesis == nil {
			log.Info("Writing default main-net genesis block")
			genesis = DefaultGenesisBlock()
		} else {
			log.Info("Writing custom genesis block")
		}

		applyOverrides(genesis.Config)
		block, err := genesis.Commit(db, triedb)
		if err != nil {
			return genesis.Config, common.Hash{}, err
		}
		return genesis.Config, block.Hash(), nil
	}
	// The genesis block is present(perhaps in ancient database) while the
	// state database is not initialized yet. It can happen that the node
	// is initialized with an external ancient store. Commit genesis state
	// in this case.
	header := rawdb.ReadHeader(db, stored, 0)
	if header.Root != types.EmptyRootHash && !triedb.Initialized(header.Root) {
		if genesis == nil {
			genesis = DefaultGenesisBlock()
		}
		applyOverrides(genesis.Config)
		// Ensure the stored genesis matches with the given one.
		hash := genesis.ToBlock().Hash()
		if hash != stored {
			return genesis.Config, hash, &GenesisMismatchError{stored, hash}
		}
		block, err := genesis.Commit(db, triedb)
		if err != nil {
			return genesis.Config, hash, err
		}
		return genesis.Config, block.Hash(), nil
	}
	// Check whether the genesis block is already written.
	if genesis != nil {
		applyOverrides(genesis.Config)
		hash := genesis.ToBlock().Hash()
		if hash != stored {
			return genesis.Config, hash, &GenesisMismatchError{stored, hash}
		}
	}
	// Get the existing chain configuration.
	newcfg := genesis.configOrDefault(stored)
	applyOverrides(newcfg)
	if err := newcfg.CheckConfigForkOrder(); err != nil {
		return newcfg, common.Hash{}, err
	}
	storedcfg := rawdb.ReadChainConfig(db, stored)
	if storedcfg == nil {
		log.Warn("Found genesis block without chain config")
		rawdb.WriteChainConfig(db, stored, newcfg)
		return newcfg, stored, nil
	}
	storedData, _ := json.Marshal(storedcfg)
	// Special case: if a private network is being used (no genesis and also no
	// mainnet hash in the database), we must not apply the `configOrDefault`
	// chain config as that would be AllProtocolChanges (applying any new fork
	// on top of an existing private network genesis block). In that case, only
	// apply the overrides.
	if genesis == nil && stored != params.MainnetGenesisHash {
		newcfg = storedcfg
		applyOverrides(newcfg)
	}
	// Check config compatibility and write the config. Compatibility errors
	// are returned to the caller unless we're already at block zero.
	head := rawdb.ReadHeadHeader(db)
	if head == nil {
		return newcfg, stored, errors.New("missing head header")
	}
	compatErr := storedcfg.CheckCompatible(newcfg, head.Number.Uint64(), head.Time)
	if compatErr != nil && ((head.Number.Uint64() != 0 && compatErr.RewindToBlock != 0) || (head.Time != 0 && compatErr.RewindToTime != 0)) {
		return newcfg, stored, compatErr
	}
	// Don't overwrite if the old is identical to the new
	if newData, _ := json.Marshal(newcfg); !bytes.Equal(storedData, newData) {
		rawdb.WriteChainConfig(db, stored, newcfg)
	}
	return newcfg, stored, nil
}

// LoadChainConfig loads the stored chain config if it is already present in
// database, otherwise, return the config in the provided genesis specification.
func LoadChainConfig(db ethdb.Database, genesis *Genesis) (*params.ChainConfig, error) {
	// Load the stored chain config from the database. It can be nil
	// in case the database is empty. Notably, we only care about the
	// chain config corresponds to the canonical chain.
	stored := rawdb.ReadCanonicalHash(db, 0)
	if stored != (common.Hash{}) {
		storedcfg := rawdb.ReadChainConfig(db, stored)
		if storedcfg != nil {
			return storedcfg, nil
		}
	}
	// Load the config from the provided genesis specification
	if genesis != nil {
		// Reject invalid genesis spec without valid chain config
		if genesis.Config == nil {
			return nil, errGenesisNoConfig
		}
		// If the canonical genesis header is present, but the chain
		// config is missing(initialize the empty leveldb with an
		// external ancient chain segment), ensure the provided genesis
		// is matched.
		if stored != (common.Hash{}) && genesis.ToBlock().Hash() != stored {
			return nil, &GenesisMismatchError{stored, genesis.ToBlock().Hash()}
		}
		return genesis.Config, nil
	}
	// There is no stored chain config and no new config provided,
	// In this case the default chain config(mainnet) will be used
	return params.MainnetChainConfig, nil
}

func (g *Genesis) configOrDefault(ghash common.Hash) *params.ChainConfig {
	switch {
	case g != nil:
		return g.Config
	case ghash == params.MainnetGenesisHash:
		return params.MainnetChainConfig
	case ghash == params.TestnetGenesisHash:
		return params.TestnetChainConfig
	case ghash == params.HoleskyGenesisHash:
		return params.HoleskyChainConfig
	case ghash == params.SepoliaGenesisHash:
		return params.SepoliaChainConfig
	case ghash == params.GoerliGenesisHash:
		return params.GoerliChainConfig
	default:
		return params.AllEthashProtocolChanges
	}
}

// IsVerkle indicates whether the state is already stored in a verkle
// tree at genesis time.
func (g *Genesis) IsVerkle() bool {
	return g.Config.IsVerkle(new(big.Int).SetUint64(g.Number), g.Timestamp)
}

// ToBlock returns the genesis block according to genesis specification.
func (g *Genesis) ToBlock() *types.Block {
	root, db, err := hashAlloc(&g.Alloc, g.IsVerkle())
	if err != nil {
		panic(err)
	}
	head := &types.Header{
		Number:     new(big.Int).SetUint64(g.Number),
		Nonce:      types.EncodeNonce(g.Nonce),
		Time:       g.Timestamp,
		ParentHash: g.ParentHash,
		Extra:      g.ExtraData,
		GasLimit:   g.GasLimit,
		GasUsed:    g.GasUsed,
		BaseFee:    g.BaseFee,
		Difficulty: g.Difficulty,
		MixDigest:  g.Mixhash,
		Coinbase:   g.Coinbase,
		Root:       root,
	}
	if g.GasLimit == 0 {
		head.GasLimit = params.GenesisGasLimit
	}
	if g.Difficulty == nil && g.Mixhash == (common.Hash{}) {
		head.Difficulty = params.GenesisDifficulty
	}
	if g.Config != nil && g.Config.IsLondon(common.Big0) {
		if g.BaseFee != nil {
			head.BaseFee = g.BaseFee
		} else {
			head.BaseFee = new(big.Int).SetUint64(params.InitialBaseFee)
		}
	}
	// Handle the Turbo related
	if g.Config != nil && g.Config.Turbo != nil {
		statedb, err := state.New(head.Root, db, nil)
		if err != nil {
			panic(err)
		}
		// init system contract
		gInit := &genesisInit{statedb, head, g}
		for name, initSystemContract := range map[string]func() error{
			"Staking":     gInit.initStaking,
			"GenesisLock": gInit.initGenesisLock,
		} {
			if err = initSystemContract(); err != nil {
				log.Crit("Failed to init system contract", "contract", name, "err", err)
			}
		}
		// Set validoter info
		if head.Extra, err = gInit.initValidators(); err != nil {
			log.Crit("Failed to init Validators", "err", err)
		}
		if head.Root, err = statedb.Commit(0, false); err != nil {
			panic(err)
		}
	}

	var withdrawals []*types.Withdrawal
	if conf := g.Config; conf != nil {
		num := big.NewInt(int64(g.Number))
		if conf.IsShanghai(num, g.Timestamp) {
			head.WithdrawalsHash = &types.EmptyWithdrawalsHash
			withdrawals = make([]*types.Withdrawal, 0)
		}
		if conf.IsCancun(num, g.Timestamp) {
			// EIP-4788: The parentBeaconBlockRoot of the genesis block is always
			// the zero hash. This is because the genesis block does not have a parent
			// by definition.
			head.ParentBeaconRoot = new(common.Hash)
			// EIP-4844 fields
			head.ExcessBlobGas = g.ExcessBlobGas
			head.BlobGasUsed = g.BlobGasUsed
			if head.ExcessBlobGas == nil {
				head.ExcessBlobGas = new(uint64)
			}
			if head.BlobGasUsed == nil {
				head.BlobGasUsed = new(uint64)
			}
		}
	}
	return types.NewBlock(head, &types.Body{Withdrawals: withdrawals}, nil, trie.NewStackTrie(nil))
}

// Commit writes the block and state of a genesis specification to the database.
// The block is committed as the canonical head block.
func (g *Genesis) Commit(db ethdb.Database, triedb *triedb.Database) (*types.Block, error) {
	block := g.ToBlock()
	if block.Number().Sign() != 0 {
		return nil, errors.New("can't commit genesis block with number > 0")
	}
	config := g.Config
	if config == nil {
		config = params.AllEthashProtocolChanges
	}
	if err := config.CheckConfigForkOrder(); err != nil {
		return nil, err
	}
	if config.Clique != nil && len(block.Extra()) < 32+crypto.SignatureLength {
		return nil, errors.New("can't start clique chain without signers")
	}
	// All the checks has passed, flushAlloc the states derived from the genesis
	// specification as well as the specification itself into the provided
	// database.
	if err := flushAlloc(g, db, triedb, block); err != nil {
		return nil, err
	}
	rawdb.WriteTd(db, block.Hash(), block.NumberU64(), block.Difficulty())
	rawdb.WriteBlock(db, block)
	rawdb.WriteReceipts(db, block.Hash(), block.NumberU64(), nil)
	rawdb.WriteCanonicalHash(db, block.Hash(), block.NumberU64())
	rawdb.WriteHeadBlockHash(db, block.Hash())
	rawdb.WriteHeadFastBlockHash(db, block.Hash())
	rawdb.WriteHeadHeaderHash(db, block.Hash())
	rawdb.WriteChainConfig(db, block.Hash(), config)
	return block, nil
}

// MustCommit writes the genesis block and state to db, panicking on error.
// The block is committed as the canonical head block.
func (g *Genesis) MustCommit(db ethdb.Database, triedb *triedb.Database) *types.Block {
	block, err := g.Commit(db, triedb)
	if err != nil {
		panic(err)
	}
	return block
}

// GenesisBlockForTesting creates and writes a block in which addr has the given wei balance.
func GenesisBlockForTesting(db ethdb.Database, addr common.Address, balance *big.Int) *types.Block {
	g := DefaultTestnetGenesisBlock()
	g.Alloc[addr] = types.Account{Balance: balance}
	g.BaseFee = big.NewInt(params.InitialBaseFee)
	return g.MustCommit(db, triedb.NewDatabase(db, triedb.HashDefaults))
}

// DefaultGenesisBlock returns the Ethereum main net genesis block.
func DefaultGenesisBlock() *Genesis {
	return &Genesis{
		Config:     params.MainnetChainConfig,
		Timestamp:  0x6733ec00,
		ExtraData:  hexutil.MustDecode("0x00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"),
		GasLimit:   0x3938700,
		BaseFee:    big.NewInt(1000000000),
		Difficulty: big.NewInt(1),
		Alloc:      decodePrealloc(mainnetAllocData),
		Validators: []types.ValidatorInfo{
			types.MakeValidator("0xFc67c341962B4DF4FA9c5b361E795b8f01cDa06d", "0x7fa31B94aA7F4ec0A11304edb88aC4f46740AaF9", "10", "10000000000000000000000000", true),
			types.MakeValidator("0xCB1CCfe63Bb861Ad35daEE16339998CcE1cD2818", "0x246BbE1D0F17d63BDcf4E4c281c143cA502F1626", "10", "10000000000000000000000000", true),
			types.MakeValidator("0x216188b4D0d4EcfA206Fb5694e37AE5faB58A908", "0x4Efea4F3d04119b6100843D2f606275475653430", "10", "10000000000000000000000000", true),
			types.MakeValidator("0x6aaeeA49B7F555D4fb5b06f667d0405E01994aC1", "0x6e0680C042f368b31Cb84FCFB9B59Ed6C69911aA", "5", "10000000000000000000000000", true),
			types.MakeValidator("0x54ce6d2a3e409F3FC626C8420925f42445Caef62", "0xa98d9Fd53963D8271ef73842Fd7FE066D71daA98", "5", "10000000000000000000000000", true),
			types.MakeValidator("0x546eD984ea449826eBD92012C133b2D587ECe9ec", "0xAc6E2F6a10341274a39D933628E8966BCbBa60b9", "5", "10000000000000000000000000", true),
			types.MakeValidator("0x53025D06243219be397419e6b728F95e2B983d67", "0x2245DD33ac916AA73aC721bc59241983BeDB1148", "5", "10000000000000000000000000", true),
			types.MakeValidator("0x21CbebE70cA17c36CAb35B4d114055BbcAd92E1F", "0x08277250521f59fFf9801975879688D4B2519953", "5", "10000000000000000000000000", true),
			types.MakeValidator("0x70C5f5A7D9d1f8aC14cF86b061E429A4EAB32b07", "0x9B67081D181458B799377c1Bb5c29bf26B7Cd12D", "5", "10000000000000000000000000", true),
			types.MakeValidator("0x07fd7B634A87Ae9c5905a426afB33f4B1D6F7bEd", "0x489Ee4D41819fB59dd1D26d61A8345113aE406D1", "5", "10000000000000000000000000", true),
			types.MakeValidator("0xCb75691fF7B0e819C0c85eA51f8a479C40e2D55e", "0x8d18E33aa0de1A152b3ce0A8B6510D6ca58f85be", "5", "10000000000000000000000000", true),
			types.MakeValidator("0xFF5d860F87C80D02E4F1b3847566E4d950111654", "0x1E23DdE67Dd255d226ed9D4d8Ec867dBb14fd1ab", "5", "10000000000000000000000000", true),
			types.MakeValidator("0x11c7eFc355b04292Cdc2640B01F1a1Df5D9B676f", "0x33087885ae2572C3BdF6FE854E6f4e88E9af65D8", "5", "10000000000000000000000000", true),
			types.MakeValidator("0xA516C90467204eF16bA24E155e310C4cF4f693BD", "0x1E840996ce017FEF79344bF3880642354A85cfD1", "5", "10000000000000000000000000", true),
			types.MakeValidator("0x6e4C549Ad02B14a4e130F88c75a52eDfcFf377ca", "0x906b565c67C8C9Eeee5a1B061e9610B174436ed6", "5", "10000000000000000000000000", true),
			types.MakeValidator("0x11A35C052d6F0512299e1C504F073515c52b36D5", "0x89bfF946551244972Cd06Aee95a37c450ba78740", "5", "10000000000000000000000000", true),
			types.MakeValidator("0x741cA83b3F28Dc970D280f6724457d2D0Bc931A0", "0x376Ca398a6aB68eB9cAc0079183C86db5ab506dA", "5", "10000000000000000000000000", true),
			types.MakeValidator("0x29c98adf76507D69cA0ab1609463663126589C90", "0x5E6A2162E04E8284083A7fA486748501708d4cB6", "5", "10000000000000000000000000", true),
			types.MakeValidator("0x033410651FD6B3AE18F5FB50C944ED47F8b3281a", "0xa15D9052bd9e4E9f493c0348cF46eF54E92c7F08", "5", "10000000000000000000000000", true),
			types.MakeValidator("0x93Af253665258226f4e8cAc391DDeC04BCb18c98", "0xC6f98869D067DE785ce7fdDe0Aef38D9c4aE5ee4", "5", "10000000000000000000000000", true),
			types.MakeValidator("0x9c385EB64DDAfA647163Ab1469F4a405E3404801", "0x2a47b1fbdFfa42759CE7BB0E29540F92BBcb707D", "0", "10000000000000000000000000", false),
			types.MakeValidator("0x09620b1D4fF839cb7B8cf3851E484209435330CF", "0x9a7D5e4143853c9E2C5F60558d142de2231daBE7", "0", "10000000000000000000000000", false),
			types.MakeValidator("0x7B64FB60614a23E66bEceA7Fb8f8f2Ca54768537", "0x3597E84753ACCD1c4aeBC5b8b989d158b7F5F6e1", "0", "10000000000000000000000000", false),
			types.MakeValidator("0xE7bB61b35a5F244248cBceAD8DBE0d8FC5DE1c98", "0x492B1a2E3c509c46213b5D44ecA2D51CDb450753", "0", "10000000000000000000000000", false),
			types.MakeValidator("0x6FF94CF26B09cc89CeDe893C4091A0aFB23bb6DB", "0xa06da8c5e231326e41057e1a61b530c4d7ba5a53", "0", "10000000000000000000000000", false),
		},
	}
}

func DefaultTestnetGenesisBlock() *Genesis {
	return &Genesis{
		Config:     params.TestnetChainConfig,
		Timestamp:  0x66ef5e00,
		ExtraData:  hexutil.MustDecode("0x00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"),
		GasLimit:   0x2625a00,
		BaseFee:    big.NewInt(1000000000),
		Difficulty: big.NewInt(1),
		Alloc:      decodePrealloc(testnetAllocData),
		Validators: []types.ValidatorInfo{
			types.MakeValidator("0x87392e3774B9B152948b764e3F0CB2aEdDBa1968", "0x949A2FcBE4EA880495aee6Bdd722827A4f3cdb34", "20", "200000000000000000000000000", true),
			types.MakeValidator("0xAd3dB0454B6c1Ce22A566782119463aC332eDA9B", "0x949A2FcBE4EA880495aee6Bdd722827A4f3cdb34", "20", "200000000000000000000000000", true),
			types.MakeValidator("0xcbA00A3d882497A54e4d3a0a03b7FE1d2495F295", "0x949A2FcBE4EA880495aee6Bdd722827A4f3cdb34", "20", "200000000000000000000000000", true),
			types.MakeValidator("0x8c248Fa3079A33cfCc93EF107b0C698f45B8182C", "0x949A2FcBE4EA880495aee6Bdd722827A4f3cdb34", "20", "200000000000000000000000000", true),
			types.MakeValidator("0x161c6074FE164DD60a1C149b1eA0cC641fe91662", "0x949A2FcBE4EA880495aee6Bdd722827A4f3cdb34", "20", "200000000000000000000000000", true),
		},
	}
}

// BasicTurboGenesisBlock returns a genesis containing basic allocation for Chais engine,
func BasicTurboGenesisBlock(config *params.ChainConfig, initialValidators []common.Address, faucet common.Address) *Genesis { //TODO
	extraVanity := 32
	extraData := make([]byte, extraVanity+65)
	alloc := decodePrealloc(basicAllocForTurbo)
	if (faucet != common.Address{}) {
		// 100M
		b, _ := new(big.Int).SetString("100000000000000000000000000", 10)
		alloc[faucet] = GenesisAccount{Balance: b}
	}
	validators := make([]types.ValidatorInfo, 0, len(initialValidators))
	stake, _ := new(big.Int).SetString("200000000000000000000000000", 10)
	for _, val := range initialValidators {
		validators = append(validators, types.ValidatorInfo{Address: val, Manager: faucet, Rate: big.NewInt(20), Stake: stake, AcceptDelegation: true})
	}
	alloc[system.StakingContract].Init.Admin = faucet
	return &Genesis{
		Config:     config,
		ExtraData:  extraData,
		GasLimit:   0x280de80,
		Difficulty: big.NewInt(2),
		Alloc:      alloc,
		Validators: validators,
	}
}

// DefaultGoerliGenesisBlock returns the Görli network genesis block.
func DefaultGoerliGenesisBlock() *Genesis {
	return &Genesis{
		Config:     params.GoerliChainConfig,
		Timestamp:  1548854791,
		ExtraData:  hexutil.MustDecode("0x22466c6578692069732061207468696e6722202d204166726900000000000000e0a2bd4258d2768837baa26a28fe71dc079f84c70000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"),
		GasLimit:   10485760,
		Difficulty: big.NewInt(1),
		Alloc:      decodePrealloc(goerliAllocData),
	}
}

// DefaultSepoliaGenesisBlock returns the Sepolia network genesis block.
func DefaultSepoliaGenesisBlock() *Genesis {
	return &Genesis{
		Config:     params.SepoliaChainConfig,
		Nonce:      0,
		ExtraData:  []byte("Sepolia, Athens, Attica, Greece!"),
		GasLimit:   0x1c9c380,
		Difficulty: big.NewInt(0x20000),
		Timestamp:  1633267481,
		Alloc:      decodePrealloc(sepoliaAllocData),
	}
}

// DefaultHoleskyGenesisBlock returns the Holesky network genesis block.
func DefaultHoleskyGenesisBlock() *Genesis {
	return &Genesis{
		Config:     params.HoleskyChainConfig,
		Nonce:      0x1234,
		GasLimit:   0x17d7840,
		Difficulty: big.NewInt(0x01),
		Timestamp:  1695902100,
		Alloc:      decodePrealloc(holeskyAllocData),
	}
}

// DeveloperGenesisBlock returns the 'geth --dev' genesis block.
func DeveloperGenesisBlock(gasLimit uint64, faucet *common.Address) *Genesis {
	// Override the default period to the user requested one
	config := *params.AllDevChainProtocolChanges

	// Assemble and return the genesis with the precompiles and faucet pre-funded
	genesis := &Genesis{
		Config:     &config,
		GasLimit:   gasLimit,
		BaseFee:    big.NewInt(params.InitialBaseFee),
		Difficulty: big.NewInt(0),
		Alloc: map[common.Address]types.Account{
			common.BytesToAddress([]byte{1}): {Balance: big.NewInt(1)}, // ECRecover
			common.BytesToAddress([]byte{2}): {Balance: big.NewInt(1)}, // SHA256
			common.BytesToAddress([]byte{3}): {Balance: big.NewInt(1)}, // RIPEMD
			common.BytesToAddress([]byte{4}): {Balance: big.NewInt(1)}, // Identity
			common.BytesToAddress([]byte{5}): {Balance: big.NewInt(1)}, // ModExp
			common.BytesToAddress([]byte{6}): {Balance: big.NewInt(1)}, // ECAdd
			common.BytesToAddress([]byte{7}): {Balance: big.NewInt(1)}, // ECScalarMul
			common.BytesToAddress([]byte{8}): {Balance: big.NewInt(1)}, // ECPairing
			common.BytesToAddress([]byte{9}): {Balance: big.NewInt(1)}, // BLAKE2b
			// Pre-deploy EIP-4788 system contract
			params.BeaconRootsAddress: {Nonce: 1, Code: params.BeaconRootsCode},
		},
	}
	if faucet != nil {
		genesis.Alloc[*faucet] = types.Account{Balance: new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(9))}
	}
	return genesis
}

func decodePrealloc(data string) types.GenesisAlloc {
	type locked struct {
		UserAddress  *big.Int
		TypeId       *big.Int
		LockedAmount *big.Int
		LockedTime   *big.Int
		PeriodAmount *big.Int
	}

	type initArgs struct {
		Admin           *big.Int `rlp:"optional"`
		FirstLockPeriod *big.Int `rlp:"optional"`
		ReleasePeriod   *big.Int `rlp:"optional"`
		ReleaseCnt      *big.Int `rlp:"optional"`
		TotalRewards    *big.Int `rlp:"optional"`
		RewardsPerBlock *big.Int `rlp:"optional"`
		PeriodTime      *big.Int `rlp:"optional"`
		LockedAccounts  []locked `rlp:"optional"`
	}
	var p []struct {
		Addr    *big.Int
		Balance *big.Int
		Misc    *struct {
			Nonce uint64
			Code  []byte
			Slots []struct {
				Key common.Hash
				Val common.Hash
			}
			Init *initArgs `rlp:"optional"`
		} `rlp:"optional"`
	}
	if err := rlp.NewStream(strings.NewReader(data), 0).Decode(&p); err != nil {
		panic(err)
	}
	ga := make(types.GenesisAlloc, len(p))
	for _, account := range p {
		acc := types.Account{Balance: account.Balance}
		if account.Misc != nil {
			acc.Nonce = account.Misc.Nonce
			acc.Code = account.Misc.Code

			acc.Storage = make(map[common.Hash]common.Hash)
			for _, slot := range account.Misc.Slots {
				acc.Storage[slot.Key] = slot.Val
			}

			if account.Misc.Init != nil {
				acc.Init = &types.Init{
					FirstLockPeriod: account.Misc.Init.FirstLockPeriod,
					ReleasePeriod:   account.Misc.Init.ReleasePeriod,
					ReleaseCnt:      account.Misc.Init.ReleaseCnt,
					TotalRewards:    account.Misc.Init.TotalRewards,
					RewardsPerBlock: account.Misc.Init.RewardsPerBlock,
					PeriodTime:      account.Misc.Init.PeriodTime,
				}
				if account.Misc.Init.Admin != nil {
					acc.Init.Admin = common.BigToAddress(account.Misc.Init.Admin)
				}
				if len(account.Misc.Init.LockedAccounts) > 0 {
					acc.Init.LockedAccounts = make([]types.LockedAccount, 0, len(account.Misc.Init.LockedAccounts))
					for _, locked := range account.Misc.Init.LockedAccounts {
						acc.Init.LockedAccounts = append(acc.Init.LockedAccounts,
							types.LockedAccount{
								UserAddress:  common.BigToAddress(locked.UserAddress),
								TypeId:       locked.TypeId,
								LockedAmount: locked.LockedAmount,
								LockedTime:   locked.LockedTime,
								PeriodAmount: locked.PeriodAmount,
							})
					}
				}
			}
		}
		ga[common.BigToAddress(account.Addr)] = acc
	}
	return ga
}
