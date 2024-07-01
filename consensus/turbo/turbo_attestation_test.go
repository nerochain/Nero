package turbo

import (
	"bytes"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/contracts/system"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/triedb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tests that signer voting is evaluated correctly for various simple and
// complex scenarios, as well as that a few special corner cases fail correctly.
func MakeFakeChain() (*core.BlockChain, error) {
	// Define the various voting scenarios to test
	tests := []struct {
		epoch          uint64 // default: 200
		waterdropBlock uint64 // default: nil
		chainLen       int    // default: 2*epoch
		signers        []string
		miners         []string
		batches        []int
		changes        []testerValidatorChange

		// Since the validator's state is changed by system contracts, we need to mock checkpoints manually
		// for the sake of simplicity,
		// (that is: the GenerateChain will not call the `engine.Prepare` process, so we need to mock the prepare process),
		// only need to set the first and the changed ones.
		checkpoints map[int][]string
		results     []string
		failure     error
	}{
		{
			// Single signer, no votes cast
			signers:     []string{"A"},
			results:     []string{"A"},
			miners:      []string{"A", "A", "A", "A"},
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
		}, {
			// Single signer, add one other, effective on next epoch
			signers: []string{"A"},
			changes: []testerValidatorChange{
				{account: "B", blockNum: 3, op: validatorAdd},
				{account: "B", blockNum: 3, op: validatorInc, value: 1},
			},
			miners:      []string{"A", "A", "A", "A", "B"},
			epoch:       2,
			chainLen:    5,
			results:     []string{"A", "B"},
			checkpoints: map[int][]string{2: {"A"}, 4: {"A", "B"}},
		}, {
			// Single signer, add one another, use the same validators set before the first epoch after waterdropBlock
			signers: []string{"A"},
			changes: []testerValidatorChange{
				{account: "B", blockNum: 3, op: validatorAdd},
				{account: "B", blockNum: 4, op: validatorInc, value: 1},
			},
			miners:         []string{"A", "A", "A", "A", "A"},
			epoch:          6,
			chainLen:       5,
			waterdropBlock: 4,
			results:        []string{"A"},
		},
		{
			// Single signer, add one another, use look-back validators set after the first epoch after waterdropBlock
			signers: []string{"A"},
			changes: []testerValidatorChange{
				{account: "B", blockNum: 3, op: validatorAdd},
				{account: "B", blockNum: 4, op: validatorInc, value: 1},
			},
			miners:         []string{"A", "A", "A", "A", "A", "A", "A"},
			epoch:          3,
			chainLen:       7,
			waterdropBlock: 4,
			results:        []string{"A"},
			checkpoints: map[int][]string{
				3: {"A"},
				6: {"A", "B"},
			},
		},
		{
			// Single signer, add two others, use look-back validators set after the first epoch after waterdropBlock
			signers: []string{"A"},
			changes: []testerValidatorChange{
				{account: "B", blockNum: 3, op: validatorAdd},
				{account: "B", blockNum: 4, op: validatorInc, value: 1},
				{account: "C", blockNum: 6, op: validatorAdd},
				{account: "C", blockNum: 7, op: validatorInc, value: 1},
			},
			miners:         []string{"A", "A", "A", "A", "A", "A", "A", "A", "A", "B"},
			epoch:          3,
			chainLen:       10,
			waterdropBlock: 4,
			results:        []string{"A", "B"},
			checkpoints: map[int][]string{
				3: {"A"},
				6: {"A", "B"},
				9: {"A", "B", "C"},
			},
		},
		{
			// Single signer, add two others, use look-back validators set after the first epoch after waterdropBlock
			signers: []string{"A"},
			changes: []testerValidatorChange{
				{account: "B", blockNum: 3, op: validatorAdd},
				{account: "B", blockNum: 4, op: validatorInc, value: 1},
				{account: "C", blockNum: 6, op: validatorAdd},
				{account: "C", blockNum: 7, op: validatorInc, value: 1},
			},
			miners:         []string{"A", "A", "A", "A", "A", "A", "A", "A", "A", "B", "B", "B", "C"},
			epoch:          3,
			chainLen:       13,
			waterdropBlock: 4,
			results:        []string{"A", "B", "C"},
			checkpoints: map[int][]string{
				3: {"A"},
				6: {"A", "B"},
				9: {"A", "B", "C"},
			},
		},
		{
			// two signers, add two others, use look-back validators set after the first epoch after waterdropBlock
			signers: []string{"A", "B", "C"},
			changes: []testerValidatorChange{
				{account: "D", blockNum: 3, op: validatorAdd},
				{account: "D", blockNum: 4, op: validatorInc, value: 1},
				{account: "E", blockNum: 4, op: validatorAdd},
				{account: "E", blockNum: 5, op: validatorInc, value: 1},
			},
			miners:         []string{"A", "B", "C", "A", "B", "C", "A", "B", "B", "B", "B", "C", "C", "C", "C", "D", "D"},
			epoch:          3,
			chainLen:       13,
			waterdropBlock: 8,
			results:        []string{"A", "B", "C", "D", "E"},
			checkpoints: map[int][]string{
				3: {"A", "B", "C"},
				6: {"A", "B", "C", "D", "E"},
			},
		},
	}
	// Run through the scenarios and test them
	tt := tests[0]
	{
		// Create the account pool and generate the initial set of signers
		accounts := newTesterAccountPool()

		signers := make([]common.Address, len(tt.signers))
		for j, signer := range tt.signers {
			signers[j] = accounts.address(signer)
		}
		for j := 0; j < len(signers); j++ {
			for k := j + 1; k < len(signers); k++ {
				if bytes.Compare(signers[j][:], signers[k][:]) > 0 {
					signers[j], signers[k] = signers[k], signers[j]
				}
			}
		}
		config := *params.AllTurboProtocolChanges
		epoch := tt.epoch
		if epoch == 0 {
			epoch = 200
		}
		config.Turbo = &params.TurboConfig{
			Period: 1,
			Epoch:  epoch,
		}
		// Create the genesis block with the initial set of signers
		genesis := core.BasicTurboGenesisBlock(&config, signers, accounts.adminAddr)
		// Create a pristine blockchain with the genesis injected
		db := rawdb.NewMemoryDatabase()
		genesis.Commit(db, triedb.NewDatabase(db, nil))

		// Assemble a chain of headers from the cast votes
		engine := New(&config, db)
		engine.fakeDiff = true
		// Pass all the headers through turbo and ensure tallying succeeds
		return core.NewBlockChain(db, nil, genesis, nil, engine, vm.Config{}, nil, nil)
	}
}

func TestAddOneValidAttestationToRecentCache(t *testing.T) {
	chain, err := MakeFakeChain()
	require.NoError(t, err)
	priv, err := crypto.GenerateKey()
	require.NoError(t, err)
	signer := crypto.PubkeyToAddress(priv.PublicKey)
	blockHash := common.BytesToHash([]byte{0xaa, 0xbb, 0xcc, 0x12, 0x34})

	require.NoError(t, err)
	aSignHashs := [10]common.Hash{}
	aHashs := [10]common.Hash{}
	for i := 0; i < 10; i++ {
		priv, err := crypto.GenerateKey()
		require.NoError(t, err)
		sig, err := crypto.Sign(crypto.Keccak256(types.AttestationData(&types.RangeEdge{
			Hash:   blockHash,
			Number: new(big.Int).SetUint64(uint64(i + 1)),
		}, &types.RangeEdge{
			Hash:   blockHash,
			Number: new(big.Int).SetUint64(100),
		})), priv)
		a := types.NewAttestation(&types.RangeEdge{
			Hash:   blockHash,
			Number: new(big.Int).SetUint64(uint64(i + 1)),
		}, &types.RangeEdge{
			Hash:   blockHash,
			Number: new(big.Int).SetUint64(uint64(100)),
		}, sig)
		err = chain.AddOneValidAttestationToRecentCache(a, 8, signer)
		require.NoError(t, err)
		aSignHashs[i] = a.SignHash()
		aHashs[i] = a.Hash()
	}
	cAs, found := chain.RecentAttessCache.Get(uint64(100))
	require.True(t, found)
	require.True(t, len(cAs.AttestsMap) == 10)
	for i := 0; i < 10; i++ {
		mapHash := cAs.AttestsMap[aSignHashs[i]]
		require.True(t, len(mapHash) == 1)
		require.True(t, mapHash[aHashs[i]])
	}

	hAs, found := chain.HistoryAttessCache.Get(uint64(100))
	require.True(t, found)
	require.True(t, len(hAs.Attestations[blockHash]) == 10)
}

func TestAddOneAttestationToFutureCache(t *testing.T) {
	chain, err := MakeFakeChain()
	require.NoError(t, err)

	priv, err := crypto.GenerateKey()
	require.NoError(t, err)
	//signer := crypto.PubkeyToAddress(priv.PublicKey)
	blockHash := common.BytesToHash([]byte{0xaa, 0xbb, 0xcc, 0x12, 0x34})
	sig, err := crypto.Sign(crypto.Keccak256(types.AttestationData(&types.RangeEdge{
		Hash:   blockHash,
		Number: new(big.Int).SetUint64(1),
	}, &types.RangeEdge{
		Hash:   blockHash,
		Number: new(big.Int).SetUint64(2),
	})), priv)
	require.NoError(t, err)

	for i := 1; i <= 10; i++ {
		a := types.NewAttestation(&types.RangeEdge{
			Hash:   blockHash,
			Number: new(big.Int).SetUint64(uint64(i)),
		}, &types.RangeEdge{
			Hash:   blockHash,
			Number: new(big.Int).SetUint64(uint64(1)),
		}, sig)
		err = chain.AddOneAttestationToFutureCache(a)
		require.NoError(t, err)
	}
	cAs, found := chain.FutureAttessCache.Get(uint64(1))
	require.True(t, found)
	require.True(t, len(cAs.Attestations) == 10)
}

// Shield it in advance -> bc.TurboEngine.VerifyAttestation(bc, a)
func TestAddOneAttestationToRecentCache(t *testing.T) {
	chain, err := MakeFakeChain()
	require.NoError(t, err)
	blockHash := common.BytesToHash([]byte{0xaa, 0xbb, 0xcc, 0x12, 0x34})

	aSignHashs := [10]common.Hash{}
	aHashs := [10]common.Hash{}
	for i := 0; i < 10; i++ {
		priv, err := crypto.GenerateKey()
		require.NoError(t, err)
		signer := crypto.PubkeyToAddress(priv.PublicKey)

		require.NoError(t, err)
		sig, err := crypto.Sign(crypto.Keccak256(types.AttestationData(&types.RangeEdge{
			Hash:   blockHash,
			Number: new(big.Int).SetUint64(uint64(1)),
		}, &types.RangeEdge{
			Hash:   blockHash,
			Number: new(big.Int).SetUint64(100),
		})), priv)
		a := types.NewAttestation(&types.RangeEdge{
			Hash:   blockHash,
			Number: new(big.Int).SetUint64(uint64(1)),
		}, &types.RangeEdge{
			Hash:   blockHash,
			Number: new(big.Int).SetUint64(uint64(100)),
		}, sig)
		err = chain.AddOneAttestationToRecentCache(a, signer, true)
		require.NoError(t, err)

		aSignHashs[i] = a.SignHash()
		aHashs[i] = a.Hash()
	}

	cAs, found := chain.RecentAttessCache.Get(uint64(100))
	require.True(t, found)
	mapHash := cAs.AttestsMap[aSignHashs[0]]
	require.True(t, len(mapHash) == 10)
}

func TestAddOneAttestationToRecentCacheViolationCasperFFG(t *testing.T) {
	chain, err := MakeFakeChain()
	require.NoError(t, err)
	blockHash := common.BytesToHash([]byte{0xaa, 0xbb, 0xcc, 0x12, 0x34})
	priv, err := crypto.GenerateKey()
	require.NoError(t, err)
	signer := crypto.PubkeyToAddress(priv.PublicKey)
	require.NoError(t, err)

	sig, err := crypto.Sign(crypto.Keccak256(types.AttestationData(&types.RangeEdge{
		Hash:   blockHash,
		Number: new(big.Int).SetUint64(uint64(1)),
	}, &types.RangeEdge{
		Hash:   blockHash,
		Number: new(big.Int).SetUint64(100),
	})), priv)
	a := types.NewAttestation(&types.RangeEdge{
		Hash:   blockHash,
		Number: new(big.Int).SetUint64(uint64(1)),
	}, &types.RangeEdge{
		Hash:   blockHash,
		Number: new(big.Int).SetUint64(uint64(100)),
	}, sig)
	err = chain.AddOneAttestationToRecentCache(a, signer, true)
	require.NoError(t, err)

	sig, err = crypto.Sign(crypto.Keccak256(types.AttestationData(&types.RangeEdge{
		Hash:   blockHash,
		Number: new(big.Int).SetUint64(uint64(3)),
	}, &types.RangeEdge{
		Hash:   blockHash,
		Number: new(big.Int).SetUint64(100),
	})), priv)
	b := types.NewAttestation(&types.RangeEdge{
		Hash:   blockHash,
		Number: new(big.Int).SetUint64(uint64(3)),
	}, &types.RangeEdge{
		Hash:   blockHash,
		Number: new(big.Int).SetUint64(uint64(100)),
	}, sig)
	err = chain.AddOneAttestationToRecentCache(b, signer, true)
	require.True(t, err != nil) // types.PunishMultiSig

	sig, err = crypto.Sign(crypto.Keccak256(types.AttestationData(&types.RangeEdge{
		Hash:   blockHash,
		Number: new(big.Int).SetUint64(uint64(3)),
	}, &types.RangeEdge{
		Hash:   blockHash,
		Number: new(big.Int).SetUint64(101),
	})), priv)
	c := types.NewAttestation(&types.RangeEdge{
		Hash:   blockHash,
		Number: new(big.Int).SetUint64(uint64(3)),
	}, &types.RangeEdge{
		Hash:   blockHash,
		Number: new(big.Int).SetUint64(uint64(101)),
	}, sig)
	err = chain.AddOneAttestationToRecentCache(c, signer, true)
	require.True(t, err == nil) // types.PunishNone

	sig, err = crypto.Sign(crypto.Keccak256(types.AttestationData(&types.RangeEdge{
		Hash:   blockHash,
		Number: new(big.Int).SetUint64(uint64(100)),
	}, &types.RangeEdge{
		Hash:   blockHash,
		Number: new(big.Int).SetUint64(102),
	})), priv)
	d := types.NewAttestation(&types.RangeEdge{
		Hash:   blockHash,
		Number: new(big.Int).SetUint64(uint64(100)),
	}, &types.RangeEdge{
		Hash:   blockHash,
		Number: new(big.Int).SetUint64(uint64(102)),
	}, sig)
	err = chain.AddOneAttestationToRecentCache(d, signer, true)
	require.True(t, err == nil) // types.PunishNone

	cAs, found := chain.RecentAttessCache.Get(uint64(100))
	require.True(t, found)
	require.True(t, len(cAs.AttestsMap[a.SignHash()]) == 1)

	cAs, found = chain.RecentAttessCache.Get(uint64(102))
	require.True(t, found)
	require.True(t, len(cAs.AttestsMap[d.SignHash()]) == 1)
}

func TestCalculateCurrentEpochIndex(t *testing.T) {
	chain, err := MakeFakeChain()
	require.NoError(t, err)
	index := chain.CalculateCurrentEpochIndex(1)
	require.True(t, index == 0)
	index = chain.CalculateCurrentEpochIndex(200)
	require.True(t, index == 1)
	index = chain.CalculateCurrentEpochIndex(201)
	require.True(t, index == 1)
}

func TestVerifyValidLimit(t *testing.T) {
	chain, err := MakeFakeChain()
	require.NoError(t, err)
	require.True(t, chain.VerifyLowerLimit(8, 12))
}

func TestVerifyCasperFFGRule(t *testing.T) {
	blockHash := common.BytesToHash([]byte{0xaa, 0xbb, 0xcc, 0x12, 0x34})
	priv, err := crypto.GenerateKey()
	require.NoError(t, err)
	//signer := crypto.PubkeyToAddress(priv.PublicKey)
	require.NoError(t, err)

	sig, err := crypto.Sign(crypto.Keccak256(types.AttestationData(&types.RangeEdge{
		Hash:   blockHash,
		Number: new(big.Int).SetUint64(uint64(1)),
	}, &types.RangeEdge{
		Hash:   blockHash,
		Number: new(big.Int).SetUint64(100),
	})), priv)

	tests := []struct {
		before *types.Attestation
		after  *types.Attestation
		result int
	}{
		{
			before: types.NewAttestation(&types.RangeEdge{
				Hash:   blockHash,
				Number: new(big.Int).SetUint64(uint64(1)),
			}, &types.RangeEdge{
				Hash:   blockHash,
				Number: new(big.Int).SetUint64(uint64(5)),
			}, sig),
			after: types.NewAttestation(&types.RangeEdge{
				Hash:   blockHash,
				Number: new(big.Int).SetUint64(uint64(5)),
			}, &types.RangeEdge{
				Hash:   blockHash,
				Number: new(big.Int).SetUint64(uint64(6)),
			}, sig),
			result: types.PunishNone,
		},
		{
			before: types.NewAttestation(&types.RangeEdge{
				Hash:   blockHash,
				Number: new(big.Int).SetUint64(uint64(1)),
			}, &types.RangeEdge{
				Hash:   blockHash,
				Number: new(big.Int).SetUint64(uint64(10)),
			}, sig),
			after: types.NewAttestation(&types.RangeEdge{
				Hash:   blockHash,
				Number: new(big.Int).SetUint64(uint64(5)),
			}, &types.RangeEdge{
				Hash:   blockHash,
				Number: new(big.Int).SetUint64(uint64(15)),
			}, sig),
			result: types.PunishNone,
		},
		{
			before: types.NewAttestation(&types.RangeEdge{
				Hash:   blockHash,
				Number: new(big.Int).SetUint64(uint64(1)),
			}, &types.RangeEdge{
				Hash:   blockHash,
				Number: new(big.Int).SetUint64(uint64(10)),
			}, sig),
			after: types.NewAttestation(&types.RangeEdge{
				Hash:   blockHash,
				Number: new(big.Int).SetUint64(uint64(5)),
			}, &types.RangeEdge{
				Hash:   blockHash,
				Number: new(big.Int).SetUint64(uint64(10)),
			}, sig),
			result: types.PunishMultiSig,
		},
		{
			before: types.NewAttestation(&types.RangeEdge{
				Hash:   blockHash,
				Number: new(big.Int).SetUint64(uint64(1)),
			}, &types.RangeEdge{
				Hash:   blockHash,
				Number: new(big.Int).SetUint64(uint64(10)),
			}, sig),
			after: types.NewAttestation(&types.RangeEdge{
				Hash:   blockHash,
				Number: new(big.Int).SetUint64(uint64(5)),
			}, &types.RangeEdge{
				Hash:   blockHash,
				Number: new(big.Int).SetUint64(uint64(9)),
			}, sig),
			result: types.PunishInclusive,
		},
		{
			before: types.NewAttestation(&types.RangeEdge{
				Hash:   blockHash,
				Number: new(big.Int).SetUint64(uint64(1)),
			}, &types.RangeEdge{
				Hash:   blockHash,
				Number: new(big.Int).SetUint64(uint64(10)),
			}, sig),
			after: types.NewAttestation(&types.RangeEdge{
				Hash:   blockHash,
				Number: new(big.Int).SetUint64(uint64(1)),
			}, &types.RangeEdge{
				Hash:   blockHash,
				Number: new(big.Int).SetUint64(uint64(9)),
			}, sig),
			result: types.PunishNone,
		},
	}

	chain, err := MakeFakeChain()
	require.NoError(t, err)

	engine, ok := chain.Engine().(*Turbo)
	require.True(t, ok)

	for _, tt := range tests {
		result := engine.VerifyCasperFFGRule(tt.before.SourceRangeEdge.Number.Uint64(), tt.before.TargetRangeEdge.Number.Uint64(),
			tt.after.SourceRangeEdge.Number.Uint64(), tt.after.TargetRangeEdge.Number.Uint64())
		require.True(t, result == tt.result)
	}

}
func TestIsDoubleSignPunishTransaction(t *testing.T) {
	header := &types.Header{
		ParentHash: common.Hash{},
		Number:     big.NewInt(200),
		Difficulty: common.Big2,
		Time:       uint64(time.Now().Unix()),
		Coinbase:   common.HexToAddress("0x352BbF453fFdcba6b126a73eD684260D7968dDc8"),
	}

	abi := system.ABI(system.StakingContract)

	data, err := abi.Pack("doubleSignPunish", common.BigToHash(big.NewInt(886)), header.Coinbase)
	assert.NoError(t, err)

	tx := types.NewTransaction(0, system.StakingContract, uint256Max, 0, common.Big0, data)
	check := (&Turbo{}).IsDoubleSignPunishTransaction(header.Coinbase, tx, header)
	assert.False(t, check)

	tx = types.NewTransaction(0, doubleSignIdentity, uint256Max, 0, common.Big0, data)
	check = (&Turbo{}).IsDoubleSignPunishTransaction(header.Coinbase, tx, header)
	assert.True(t, check)
}

func TestExecutedDoubleSignPunishTopic(t *testing.T) {
	keccak256Bytes := crypto.Keccak256([]byte("ExecutedDoubleSignPunish(address,address,uint8,bytes)"))
	topicStr := common.BytesToHash(keccak256Bytes).String()
	require.True(t, topicStr == "0x250969e8ccb0e19752686619d1ce1af974eeea52b88479ca3ec6cced6b7c9198")
}

func TestBuildDoubleSignPunishExecutedEventData(t *testing.T) {
	testcases := []struct {
		p      *types.ViolateCasperFFGPunish
		expect []byte
	}{
		{
			p: &types.ViolateCasperFFGPunish{
				PunishType: big.NewInt(0),
				Before:     nil,
				After:      nil,
				BlockNum:   big.NewInt(0),
				Data:       common.Hex2Bytes("00000000000000000000000088e0231aedd860cd4b241ffa83ee944a9dfcd7b9"),
			},
			expect: common.Hex2Bytes("0000000000000000000000000000000000000000000000000000000000000020000000000000000000000000000000000000000000000000000000000000002000000000000000000000000088e0231aedd860cd4b241ffa83ee944a9dfcd7b9"),
		},
		{
			p: &types.ViolateCasperFFGPunish{
				PunishType: big.NewInt(0),
				Before:     nil,
				After:      nil,
				BlockNum:   big.NewInt(0),
				Data:       common.Hex2Bytes("a9059cbb00000000000000000000000088e0231aedd860cd4b241ffa83ee944a9dfcd7b90000000000000000000000000000000000000000000000006e8ff610f77700ac"),
			},
			expect: common.Hex2Bytes("00000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000000000000000000044a9059cbb00000000000000000000000088e0231aedd860cd4b241ffa83ee944a9dfcd7b90000000000000000000000000000000000000000000000006e8ff610f77700ac00000000000000000000000000000000000000000000000000000000"),
		},
	}
	for i, tc := range testcases {
		got := buildDoubleSignPunishExecutedEventData(tc.p)
		require.True(t, bytes.Equal(tc.expect, got), i)
	}
}
