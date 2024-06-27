package system

import (
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
)

const (
	// StakingABI contains methods to interactive with Staking contract.
	StakingABI = `[
		{
			"inputs": [
				{
					"internalType": "address",
					"name": "_admin",
					"type": "address"
				},
				{
					"internalType": "uint256",
					"name": "_firstLockPeriod",
					"type": "uint256"
				},
				{
					"internalType": "uint256",
					"name": "_releasePeriod",
					"type": "uint256"
				},
				{
					"internalType": "uint256",
					"name": "_releaseCnt",
					"type": "uint256"
				},
				{
					"internalType": "uint256",
					"name": "_totalRewards",
					"type": "uint256"
				},
				{
					"internalType": "uint256",
					"name": "_rewardsPerBlock",
					"type": "uint256"
				},
				{
					"internalType": "uint256",
					"name": "_epoch",
					"type": "uint256"
				},
				{
					"internalType": "uint256",
					"name": "_ruEpoch",
					"type": "uint256"
				},
				{
					"internalType": "address payable",
					"name": "_communityPool",
					"type": "address"
				},
				{
					"internalType": "contract IBonusPool",
					"name": "_bonusPool",
					"type": "address"
				}
			],
			"name": "initialize",
			"outputs": [],
			"stateMutability": "nonpayable",
			"type": "function"
		},
		{
			"inputs": [
				{
					"internalType": "address",
					"name": "_val",
					"type": "address"
				},
				{
					"internalType": "address",
					"name": "_manager",
					"type": "address"
				},
				{
					"internalType": "uint256",
					"name": "_rate",
					"type": "uint256"
				},
				{
					"internalType": "uint256",
					"name": "_stakeEth",
					"type": "uint256"
				},
				{
					"internalType": "bool",
					"name": "_acceptDelegation",
					"type": "bool"
				}
			],
			"name": "initValidator",
			"outputs": [],
			"stateMutability": "nonpayable",
			"type": "function"
		},
		{
			"inputs": [
				{
					"internalType": "uint8",
					"name": "_count",
					"type": "uint8"
				}
			],
			"name": "getTopValidators",
			"outputs": [
				{
					"internalType": "address[]",
					"name": "",
					"type": "address[]"
				}
			],
			"stateMutability": "view",
			"type": "function"
		},
		{
			"inputs": [
				{
					"internalType": "address[]",
					"name": "newSet",
					"type": "address[]"
				}
			],
			"name": "updateActiveValidatorSet",
			"outputs": [],
			"stateMutability": "nonpayable",
			"type": "function"
		},
		{
			"inputs": [],
			"name": "decreaseMissedBlocksCounter",
			"outputs": [],
			"stateMutability": "nonpayable",
			"type": "function"
		},
		{
			"inputs": [
				{
					"internalType": "uint256",
					"name": "_rewardsPerBlock",
					"type": "uint256"
				}
			],
			"name": "updateRewardsInfo",
			"outputs": [],
			"stateMutability": "nonpayable",
			"type": "function"
		},
		{
			"inputs": [],
			"name": "distributeBlockFee",
			"outputs": [],
			"stateMutability": "payable",
			"type": "function"
		},
		{
			"inputs": [
				{
					"internalType": "bytes32",
					"name": "punishHash",
					"type": "bytes32"
				}
			],
			"name": "isDoubleSignPunished",
			"outputs": [
				{
					"internalType": "bool",
					"name": "",
					"type": "bool"
				}
			],
			"stateMutability": "view",
			"type": "function"
		},
		{
			"inputs": [
				{
					"internalType": "address",
					"name": "_val",
					"type": "address"
				}
			],
			"name": "lazyPunish",
			"outputs": [],
			"stateMutability": "nonpayable",
			"type": "function"
		},
		{
			"inputs": [
				{
					"internalType": "bytes32",
					"name": "_punishHash",
					"type": "bytes32"
				},
				{
					"internalType": "address",
					"name": "_val",
					"type": "address"
				}
			],
			"name": "doubleSignPunish",
			"outputs": [],
			"stateMutability": "nonpayable",
			"type": "function"
		},
		{
			"inputs": [],
			"name": "rewardsUpdateEpoch",
			"outputs": [
				{
					"internalType": "uint256",
					"name": "",
					"type": "uint256"
				}
			],
			"stateMutability": "view",
			"type": "function"
		}
	]`

	// CommunityPoolABI contains methods to interactive with CommunityPool contract.
	CommunityPoolABI = `[
		{
			"inputs": [
				{
					"internalType": "address",
					"name": "_admin",
					"type": "address"
				}
			],
			"name": "initialize",
			"outputs": [],
			"stateMutability": "nonpayable",
			"type": "function"
		}
	]`

	// BonusPoolABI contains methods to interactive with BonusPool contract.
	BonusPoolABI = `[
		{
			"inputs": [
				{
					"internalType": "address",
					"name": "_stakingContract",
					"type": "address"
				}
			],
			"name": "initialize",
			"outputs": [],
			"stateMutability": "payable",
			"type": "function"
		}
	]`

	// GenesisLockABI contains methods to interactive with GenesisLock contract.
	GenesisLockABI = `[
		{
			"inputs": [
				{
					"internalType": "uint256",
					"name": "_periodTime",
					"type": "uint256"
				}
			],
			"name": "initialize",
			"outputs": [],
			"stateMutability": "nonpayable",
			"type": "function"
		},
		{
			"inputs": [
				{
					"internalType": "address[]",
					"name": "userAddress",
					"type": "address[]"
				},
				{
					"internalType": "uint256[]",
					"name": "typeId",
					"type": "uint256[]"
				},
				{
					"internalType": "uint256[]",
					"name": "lockedAmount",
					"type": "uint256[]"
				},
				{
					"internalType": "uint256[]",
					"name": "lockedTime",
					"type": "uint256[]"
				},
				{
					"internalType": "uint256[]",
					"name": "periodAmount",
					"type": "uint256[]"
				}
			],
			"name": "init",
			"outputs": [],
			"stateMutability": "nonpayable",
			"type": "function"
		}
	]`
	// AddressListABI contains methods to interactive with AddressList contract.
	AddressListABI = `[
		{
			"inputs": [
				{
					"internalType": "address",
					"name": "_admin",
					"type": "address"
				}
			],
			"name": "initialize",
			"outputs": [],
			"stateMutability": "nonpayable",
			"type": "function"
		},
		{
			"inputs": [],
			"name": "getBlacksFrom",
			"outputs": [
				{
					"internalType": "address[]",
					"name": "",
					"type": "address[]"
				}
			],
			"stateMutability": "view",
			"type": "function"
		},
		{
			"inputs": [],
			"name": "getBlacksTo",
			"outputs": [
				{
					"internalType": "address[]",
					"name": "",
					"type": "address[]"
				}
			],
			"stateMutability": "view",
			"type": "function"
		},
		{
			"inputs": [
				{
					"internalType": "uint32",
					"name": "i",
					"type": "uint32"
				}
			],
			"name": "getRuleByIndex",
			"outputs": [
				{
					"internalType": "bytes32",
					"name": "",
					"type": "bytes32"
				},
				{
					"internalType": "uint128",
					"name": "",
					"type": "uint128"
				},
				{
					"internalType": "enum AddressList.CheckType",
					"name": "",
					"type": "uint8"
				}
			],
			"stateMutability": "view",
			"type": "function"
		},
		{
			"inputs": [],
			"name": "rulesLen",
			"outputs": [
				{
					"internalType": "uint32",
					"name": "",
					"type": "uint32"
				}
			],
			"stateMutability": "view",
			"type": "function"
		}
	]`

	// OnChainDaoABI contains methods to interactive with OnChainDao contract.
	OnChainDaoABI = `[
		{
			"inputs": [
				{
					"internalType": "address",
					"name": "_admin",
					"type": "address"
				}
			],
			"name": "initialize",
			"outputs": [],
			"stateMutability": "nonpayable",
			"type": "function"
		},
		{
			"inputs": [],
			"name": "getPassedProposalCount",
			"outputs": [
				{
					"internalType": "uint32",
					"name": "",
					"type": "uint32"
				}
			],
			"stateMutability": "view",
			"type": "function"
		},
		{
			"inputs": [
				{
					"internalType": "uint32",
					"name": "index",
					"type": "uint32"
				}
			],
			"name": "getPassedProposalByIndex",
			"outputs": [
				{
					"internalType": "uint256",
					"name": "id",
					"type": "uint256"
				},
				{
					"internalType": "uint256",
					"name": "action",
					"type": "uint256"
				},
				{
					"internalType": "address",
					"name": "from",
					"type": "address"
				},
				{
					"internalType": "address",
					"name": "to",
					"type": "address"
				},
				{
					"internalType": "uint256",
					"name": "value",
					"type": "uint256"
				},
				{
					"internalType": "bytes",
					"name": "data",
					"type": "bytes"
				}
			],
			"stateMutability": "view",
			"type": "function"
		},
		{
			"inputs": [
				{
					"internalType": "uint256",
					"name": "id",
					"type": "uint256"
				}
			],
			"name": "finishProposalById",
			"outputs": [],
			"stateMutability": "nonpayable",
			"type": "function"
		}
	]`
)

// DevMappingPosition is the position of the state variable `devs`.
// Since the state variables are as follows:
//
//	   bool public initialized;
//	   bool public devVerifyEnabled;
//		  bool public checkInnerCreation;
//	   address public admin;
//	   address public pendingAdmin;
//
//	   mapping(address => bool) private devs;
//
//	   //NOTE: make sure this list is not too large!
//	   address[] blacksFrom;
//	   address[] blacksTo;
//	   mapping(address => uint256) blacksFromMap;      // address => index+1
//	   mapping(address => uint256) blacksToMap;        // address => index+1
//
//	   uint256 public blackLastUpdatedNumber; // last block number when the black list is updated
//	   uint256 public rulesLastUpdatedNumber;  // last block number when the rules are updated
//	   // event check rules
//	   EventCheckRule[] rules;
//	   mapping(bytes32 => mapping(uint128 => uint256)) rulesMap;   // eventSig => checkIdx => indexInArray+1
//
// according to [Layout of State Variables in Storage](https://docs.soliditylang.org/en/v0.8.4/internals/layout_in_storage.html),
// and after optimizer enabled, the `initialized`, `devVerifyEnabled`, `checkInnerCreation` and `admin` will be packed, and stores at slot 0,
// `pendingAdmin` stores at slot 1, so the position for `devs` is 2.
const DevMappingPosition = 2

var (
	BlackLastUpdatedNumberPosition = common.BytesToHash([]byte{0x07})
	RulesLastUpdatedNumberPosition = common.BytesToHash([]byte{0x08})
)

var (
	StakingContract       = common.HexToAddress("0x000000000000000000000000000000000000F000")
	CommunityPoolContract = common.HexToAddress("0x000000000000000000000000000000000000F001")
	BonusPoolContract     = common.HexToAddress("0x000000000000000000000000000000000000F002")
	GenesisLockContract   = common.HexToAddress("0x000000000000000000000000000000000000F003")
	AddressListContract   = common.HexToAddress("0x000000000000000000000000000000000000F004")
	OnChainDaoContract    = common.HexToAddress("0x000000000000000000000000000000000000F005")

	abiMap map[common.Address]abi.ABI
)

// init the abiMap
func init() {
	abiMap = make(map[common.Address]abi.ABI, 0)

	for addr, rawAbi := range map[common.Address]string{
		StakingContract:       StakingABI,
		CommunityPoolContract: CommunityPoolABI,
		BonusPoolContract:     BonusPoolABI,
		GenesisLockContract:   GenesisLockABI,
		AddressListContract:   AddressListABI,
		OnChainDaoContract:    OnChainDaoABI,
	} {
		if abi, err := abi.JSON(strings.NewReader(rawAbi)); err != nil {
			panic(err)
		} else {
			abiMap[addr] = abi
		}
	}
}

// ABI return abi for given contract calling
func ABI(contract common.Address) abi.ABI {
	contractABI, ok := abiMap[contract]
	if !ok {
		log.Crit("Unknown system contract: " + contract.String())
	}
	return contractABI
}

// ABIPack generates the data field for given contract calling
func ABIPack(contract common.Address, method string, args ...interface{}) ([]byte, error) {
	return ABI(contract).Pack(method, args...)
}
