package turbo

import (
	"bytes"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"os"
	"sort"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/turbo/systemcontract"
	"github.com/ethereum/go-ethereum/contracts/system"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/triedb"
)

const (
	validatorAdd validatorOp = iota
	validatorInc
	validatorExit
)

var (
	wei = big.NewInt(1e18)
)

var (
	stakingAbi abi.ABI
)

type validatorOp byte

func init() {
	file, err := os.Open("testdata/staking_abi.json")
	if err != nil {
		panic(err)
	}
	if stakingAbi, err = abi.JSON(file); err != nil {
		panic(err)
	}
}

// testerAccountPool is a pool to maintain currently active tester accounts,
// mapped from textual names used in the tests below to actual Ethereum private
// keys capable of signing transactions.
type testerAccountPool struct {
	accounts  map[string]*ecdsa.PrivateKey
	accounts2 map[common.Address]*ecdsa.PrivateKey
	admin     *ecdsa.PrivateKey
	adminAddr common.Address
}

func newTesterAccountPool() *testerAccountPool {
	adm, _ := crypto.GenerateKey()
	addr := crypto.PubkeyToAddress(adm.PublicKey)
	return &testerAccountPool{
		accounts:  make(map[string]*ecdsa.PrivateKey),
		accounts2: make(map[common.Address]*ecdsa.PrivateKey),
		admin:     adm,
		adminAddr: addr,
	}
}

// checkpoint creates a Turbo checkpoint signer section from the provided list
// of authorized signers and embeds it into the provided header.
func (ap *testerAccountPool) checkpoint(header *types.Header, signers []string) {
	auths := make([]common.Address, len(signers))
	for i, signer := range signers {
		auths[i] = ap.address(signer)
	}
	sort.Sort(systemcontract.AddrAscend(auths))
	for i, auth := range auths {
		copy(header.Extra[extraVanity+i*common.AddressLength:], auth.Bytes())
	}
}

// address retrieves the Ethereum address of a tester account by label, creating
// a new account if no previous one exists yet.
func (ap *testerAccountPool) address(account string) common.Address {
	// Return the zero account for non-addresses
	if account == "" {
		return common.Address{}
	}
	// Ensure we have a persistent key for the account
	if ap.accounts[account] == nil {
		ap.accounts[account], _ = crypto.GenerateKey()
	}
	// Resolve and return the Ethereum address
	addr := crypto.PubkeyToAddress(ap.accounts[account].PublicKey)
	ap.accounts2[addr] = ap.accounts[account]
	return addr
}

// sign calculates a Turbo digital signature for the given block and embeds it
// back into the header.
func (ap *testerAccountPool) sign(header *types.Header) {
	priv := ap.accounts2[header.Coinbase]
	if priv == nil {
		panic(fmt.Sprintf("account not exist, %s", header.Coinbase))
	}
	// Sign the header and embed the signature in extra data
	sig, _ := crypto.Sign(SealHash(header).Bytes(), priv)
	copy(header.Extra[len(header.Extra)-extraSeal:], sig)
}

func (ap *testerAccountPool) genTx(change testerValidatorChange, nonce uint64, signer types.Signer) (*types.Transaction, error) {
	valAddr := ap.address(change.account)

	switch change.op {
	case validatorAdd:
		method := "registerValidator"
		// args: validator, manager, rate(base on 100), acceptDelegation(true/false)
		data, err := stakingAbi.Pack(method, valAddr, ap.adminAddr, big.NewInt(10), true)
		if err != nil {
			return nil, err
		}
		return types.SignTx(types.NewTransaction(nonce, system.StakingContract, toWei(50000), 3000000, big.NewInt(params.GWei), data), signer, ap.admin)
	case validatorInc:
		method := "addStake"
		data, err := stakingAbi.Pack(method, valAddr)
		if err != nil {
			return nil, err
		}
		return types.SignTx(types.NewTransaction(nonce, system.StakingContract, toWei(change.value), 1000000, big.NewInt(params.GWei), data), signer, ap.admin)
	case validatorExit:
		method := "exitStaking"
		data, err := stakingAbi.Pack(method, valAddr)
		if err != nil {
			return nil, err
		}
		return types.SignTx(types.NewTransaction(nonce, system.StakingContract, nil, 1000000, big.NewInt(params.GWei), data), signer, ap.admin)
	}
	return nil, fmt.Errorf("unsupported op: %v", change.op)
}

// testerValidatorChange represents a single transaction that changes the validators status.
type testerValidatorChange struct {
	account  string
	blockNum int
	op       validatorOp
	value    uint64
}

func toWei(cube uint64) *big.Int {
	v := new(big.Int).SetUint64(cube)
	return v.Mul(v, wei)
}

type testcase struct {
	epoch    uint64 // default: 2
	chainLen int    // default: 2*epoch
	signers  []string
	miners   []string
	batches  []int
	changes  []testerValidatorChange

	// Since the validator's state is changed by system contracts, we need to mock checkpoints manually
	// for the sake of simplicity,
	// (that is: the GenerateChain will not call the `engine.Prepare` process, so we need to mock the prepare process),
	// only need to set the first and the changed ones.
	checkpoints map[int][]string
	results     []string
	failure     error
}

// Tests that Turbo signer voting is evaluated correctly for various simple and
// complex scenarios, as well as that a few special corner cases fail correctly.
func TestTurbo(t *testing.T) {
	testcases := []testcase{
		{
			// Single signer, no votes cast
			signers:     []string{"A"},
			results:     []string{"A"},
			miners:      []string{"A", "A", "A", "A"},
			epoch:       2,
			checkpoints: map[int][]string{2: {"A"}},
		}, {
			// Single signer, add one other, not effective until next epoch
			signers: []string{"A"},
			changes: []testerValidatorChange{
				{account: "B", blockNum: 3, op: validatorAdd},
				{account: "B", blockNum: 4, op: validatorInc, value: 1},
			},
			miners:   []string{"A", "A", "A", "A", "A"},
			epoch:    6,
			chainLen: 5,
			results:  []string{"A"},
		},
		// {
		// 	// Single signer, add one other, effective on next epoch
		// 	signers: []string{"A"},
		// 	changes: []testerValidatorChange{
		// 		{account: "B", blockNum: 3, op: validatorAdd},
		// 		{account: "B", blockNum: 3, op: validatorInc, value: 1},
		// 	},
		// 	miners:      []string{"A", "A", "A", "A", "A", "A", "B"},
		// 	epoch:       2,
		// 	chainLen:    7,
		// 	results:     []string{"A", "B"},
		// 	checkpoints: map[int][]string{2: {"A"}, 4: {"A", "B"}},
		// },
		{
			signers: []string{"A"},
			changes: []testerValidatorChange{
				{account: "B", blockNum: 3, op: validatorAdd},
				{account: "B", blockNum: 4, op: validatorInc, value: 1},
			},
			miners:   []string{"A", "A", "A", "A", "A"},
			epoch:    6,
			chainLen: 5,
			results:  []string{"A"},
		},
		// {
		// 	signers: []string{"A"},
		// 	changes: []testerValidatorChange{
		// 		{account: "B", blockNum: 3, op: validatorAdd},
		// 		{account: "B", blockNum: 4, op: validatorInc, value: 1},
		// 	},
		// 	miners:   []string{"A", "A", "A", "A", "A", "A", "A"},
		// 	epoch:    3,
		// 	chainLen: 7,
		// 	results:  []string{"A"},
		// 	checkpoints: map[int][]string{
		// 		3: {"A"},
		// 		6: {"A", "B"},
		// 	},
		// }, {
		// 	signers: []string{"A"},
		// 	changes: []testerValidatorChange{
		// 		{account: "B", blockNum: 3, op: validatorAdd},
		// 		{account: "B", blockNum: 4, op: validatorInc, value: 1},
		// 		{account: "C", blockNum: 6, op: validatorAdd},
		// 		{account: "C", blockNum: 7, op: validatorInc, value: 1},
		// 	},
		// 	miners:   []string{"A", "A", "A", "A", "A", "A", "A", "A", "A", "B"},
		// 	epoch:    3,
		// 	chainLen: 10,
		// 	results:  []string{"A", "B"},
		// 	checkpoints: map[int][]string{
		// 		3: {"A"},
		// 		6: {"A", "B"},
		// 		9: {"A", "B", "C"},
		// 	},
		// }, {
		// 	signers: []string{"A"},
		// 	changes: []testerValidatorChange{
		// 		{account: "B", blockNum: 3, op: validatorAdd},
		// 		{account: "B", blockNum: 4, op: validatorInc, value: 1},
		// 		{account: "C", blockNum: 6, op: validatorAdd},
		// 		{account: "C", blockNum: 7, op: validatorInc, value: 1},
		// 	},
		// 	miners:   []string{"A", "A", "A", "A", "A", "A", "A", "A", "A", "B", "B", "B", "C"},
		// 	epoch:    3,
		// 	chainLen: 13,
		// 	results:  []string{"A", "B", "C"},
		// 	checkpoints: map[int][]string{
		// 		3: {"A"},
		// 		6: {"A", "B"},
		// 		9: {"A", "B", "C"},
		// 	},
		// }, {
		// 	signers: []string{"A", "B", "C"},
		// 	changes: []testerValidatorChange{
		// 		{account: "D", blockNum: 3, op: validatorAdd},
		// 		{account: "D", blockNum: 4, op: validatorInc, value: 1},
		// 		{account: "E", blockNum: 4, op: validatorAdd},
		// 		{account: "E", blockNum: 5, op: validatorInc, value: 1},
		// 	},
		// 	miners:   []string{"A", "B", "C", "A", "B", "C", "A", "B", "B", "A", "C", "C", "A", "C", "C", "D", "D"},
		// 	epoch:    3,
		// 	chainLen: 13,
		// 	results:  []string{"A", "B", "C", "D", "E"},
		// 	checkpoints: map[int][]string{
		// 		3: {"A", "B", "C"},
		// 		6: {"A", "B", "C", "D", "E"},
		// 	},
		// },
	}
	// Run through the scenarios and test them
	for i, tc := range testcases {
		runTurboTest(t, i, &tc)
	}
}

// runTurboTest is the real test logic
func runTurboTest(t *testing.T, testID int, tc *testcase) {
	// Create the account pool and generate the initial set of signers
	accounts := newTesterAccountPool()
	signers := make([]common.Address, len(tc.signers))
	for i, signer := range tc.signers {
		signers[i] = accounts.address(signer)
	}

	sort.Slice(signers, func(i, j int) bool {
		return bytes.Compare(signers[i][:], signers[j][:]) < 0
	})

	config := *params.AllTurboProtocolChanges
	config.Turbo = &params.TurboConfig{
		Period: 1,
		Epoch:  tc.epoch,
	}

	// Create the genesis block with the initial set of signers
	genesis := core.BasicTurboGenesisBlock(&config, signers, accounts.adminAddr)
	// Create a pristine blockchain with the genesis injected
	db := rawdb.NewMemoryDatabase()
	triedb := triedb.NewDatabase(db, nil)
	genesisBlock, _ := genesis.Commit(db, triedb)

	// Assemble a chain of headers from the cast votes
	engine := New(&config, db)
	engine.fakeDiff = true
	// Pass all the headers through turbo and ensure tallying succeeds
	chain, err := core.NewBlockChain(db, nil, genesis, nil, engine, vm.Config{}, nil, nil)
	// chain, err := core.NewBlockChain(db, nil, genspec, nil, engine, vm.Config{}, nil, nil)

	if err != nil {
		t.Errorf("test %d: failed to create test chain: %v", testID, err)
		return
	}
	// do some extra work for turbo.
	// set state fn
	engine.SetStateFn(func(hash common.Hash) (*state.StateDB, error) {
		statedb, err := state.New(hash, state.NewDatabaseWithNodeDB(db, triedb), nil)
		if err != nil {
			panic(fmt.Sprintf("can't get statedb for %s : %v", hash.String(), err))
		}
		return statedb, nil
	})
	// turbo need the chain for extra_validate feature
	engine.SetChain(chain)

	chainLen := tc.chainLen
	if chainLen == 0 {
		chainLen = int(2 * tc.epoch)
	}
	//tx signer
	signer := types.LatestSigner(&config)

	blocks, _ := core.GenerateChain(&config, genesisBlock, engine, db, chainLen, func(idx int, gen *core.BlockGen) {
		// j is not block number, but index which starts from 0.
		// Cast the vote contained in this block
		gen.SetCoinbase(accounts.address(tc.miners[idx]))
		// Since the `validator` field is empty in engine, so the difficulty from chainMaker is not correct.
		gen.SetDifficulty(diffInTurn)

		for _, change := range tc.changes {
			if change.blockNum == (idx + 1) {
				tx, err := accounts.genTx(change, gen.TxNonce(accounts.adminAddr), signer)
				if err != nil {
					panic("genTx: " + err.Error())
				}
				receipt := gen.AddTxWithChain(chain, tx)
				if change.op == validatorAdd {
					if receipt.Status == 0 {
						panic("add validator failed")
					}
				}
				//t.Logf("info: op=%v, gasUesd=%d\n", change.op, receipt.GasUsed)
			}
		}
	})
	// Iterate through the blocks and seal them individually
	var lastCheckpointExtra []byte
	for i, block := range blocks {
		// Get the header and prepare it for signing
		header := block.Header()
		if i > 0 {
			header.ParentHash = blocks[i-1].Hash()
		}
		header.Extra = make([]byte, extraVanity+extraSeal)
		if uint64(i+1)%tc.epoch == 0 {
			if tc.checkpoints != nil {
				auths, exist := tc.checkpoints[i+1]
				if exist {
					header.Extra = make([]byte, extraVanity+len(auths)*common.AddressLength+extraSeal)
					accounts.checkpoint(header, auths)
					lastCheckpointExtra = make([]byte, len(header.Extra))
					copy(lastCheckpointExtra, header.Extra)
				} else if len(lastCheckpointExtra) > 0 {
					header.Extra = make([]byte, len(lastCheckpointExtra))
					copy(header.Extra, lastCheckpointExtra)
				} else {
					t.Errorf("need to set checkpoints correctly")
					return
				}
			}
		}

		header.Difficulty = diffInTurn // Ignored, we just need a valid number
		// Generate the signature, embed it into the header and the block
		accounts.sign(header)
		blocks[i] = block.WithSeal(header)
	}
	// Split the blocks up into individual import batches (corner case testing)
	batches := [][]*types.Block{nil}
	idx := 0
	for _, batch := range tc.batches {
		if idx >= chainLen {
			break
		}
		batches = append(batches, nil)
		n := idx + batch
		if n > chainLen {
			n = chainLen
		}
		for ; idx < n; idx++ {
			batches[len(batches)-1] = append(batches[len(batches)-1], blocks[idx])
		}
	}
	if idx < chainLen {
		batches = append(batches, nil)
		n := chainLen
		for ; idx < n; idx++ {
			batches[len(batches)-1] = append(batches[len(batches)-1], blocks[idx])
		}
	}

	for i := 0; i < len(batches)-1; i++ {
		if idx, err := chain.InsertChain(batches[i]); err != nil {
			t.Errorf("test %d: failed to import batch %d, block %d: %v", testID, i, idx, err)
			return
		}
	}
	if _, err = chain.InsertChain(batches[len(batches)-1]); err != tc.failure {
		t.Errorf("test %d: failure mismatch: have %v, want %v", testID, err, tc.failure)
	}
	if tc.failure != nil {
		return
	}
	// No failure was produced or requested, generate the final voting snapshot
	head := blocks[len(blocks)-1]

	snap, err := engine.snapshot(chain, head.NumberU64(), head.Hash(), nil)
	if err != nil {
		t.Errorf("test %d: failed to retrieve voting snapshot: %v", testID, err)
		return
	}
	// Verify the final list of signers against the expected ones
	signers = make([]common.Address, len(tc.results))
	for i, signer := range tc.results {
		signers[i] = accounts.address(signer)
	}
	sort.Slice(signers, func(i, j int) bool {
		return bytes.Compare(signers[i][:], signers[j][:]) < 0
	})
	result := snap.validators()
	if len(result) != len(signers) {
		t.Errorf("test %d: signers mismatch: have %x, want %x", testID, result, signers)
		return
	}
	for i := 0; i < len(result); i++ {
		if !bytes.Equal(result[i][:], signers[i][:]) {
			t.Errorf("test %d, signer %d: signer mismatch: have %x, want %x", testID, i, result[i], signers[i])
		}
	}
}
