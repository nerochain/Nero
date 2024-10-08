package turbo

import (
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/contracts"
	"github.com/ethereum/go-ethereum/core/rawdb"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/turbo/systemcontract"
	"github.com/ethereum/go-ethereum/contracts/system"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
)

const (
	attestationThresholdNumerator   = 2
	attestationThresholdDenominator = 3

	// maxOldBlockToAttest = 4
)

var (
	doubleSignIdentity = common.HexToAddress("0xfffffffffffffffffffffffffffffffffffffffe")
	uint256Max, _      = new(big.Int).SetString("0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", 0)

	// event ExecutedDoubleSignPunish(address indexed plaintiff, address indexed defendant, uint8 indexed value, bytes data);
	// event signature:  crypto.Keccak256([]byte("ExecutedDoubleSignPunish(address,address,uint8,bytes)"))
	executedDoubleSignPunishEventSig = common.HexToHash("0x250969e8ccb0e19752686619d1ce1af974eeea52b88479ca3ec6cced6b7c9198")
)

// punishDoubleSign punishes double sign attack in casper ffg
func (c *Turbo) punishDoubleSign(chain consensus.ChainHeaderReader, header *types.Header,
	state *state.StateDB, txs *[]*types.Transaction, receipts *[]*types.Receipt, punishTxs []*types.Transaction, mined bool) error {
	if !mined {
		// handle violating CasperFFG rules
		totalTxIndex := len(punishTxs)
		for i := uint32(0); i < uint32(totalTxIndex); i++ {
			log.Debug("Received a pending penalty", "Number", header.Number.Uint64())
			// execute the doubleSignPunish
			// If one transaction fails to execute, the whole block will be discarded
			tx := punishTxs[int(i)]
			receipt, err := c.replayDoubleSignPunish(chain, header, state, totalTxIndex, tx)
			if err != nil {
				return err
			}
			*txs = append(*txs, tx)
			*receipts = append(*receipts, receipt)
		}
	} else if c.signTxFn != nil {
		// Note:
		// Even if the miner is not `running`, it's still working,
		// the 'miner.worker' will try to FinalizeAndAssemble a block,
		// in this case, the signTxFn is not set. A `non-miner node` can't execute tx.

		// Add penalty transactions for violating CasperFFG rules
		punishList := rawdb.ReadAllViolateCasperFFGPunish(c.db)
		if len(punishList) > 0 {
			for _, p := range punishList {
				val, err := p.RecoverSigner()
				if err != nil {
					continue
				}
				b, err := c.IsDoubleSignPunished(chain, header, state, p.Hash())
				if err != nil {
					log.Error("IsDoubleSignPunished error", "error", err.Error())
					return err
				}
				if !b {
					// execute the Punish.sol doubleSignPunish
					tx, receipt, err := c.executeDoubleSignPunish(chain, header, state, p, len(punishList))
					if err != nil {
						log.Error("executeDoubleSignPunish error", "error", err.Error())
						return err
					}
					*txs = append(*txs, tx)
					*receipts = append(*receipts, receipt)
					log.Debug("executeDoubleSignPunish", "Violator", val, "Number", header.Number.Uint64())
				} else {
					rawdb.DeleteViolateCasperFFGPunish(c.db, p)
				}
			}
		}
	}
	return nil
}

func (c *Turbo) AttestationStatus() uint8 {
	return c.attestationStatus
}

func (c *Turbo) StartAttestation() {
	c.attestationStatus = types.AttestationStart
}

func (c *Turbo) StopAttestation() {
	c.attestationStatus = types.AttestationStop
}

func (c *Turbo) ClearAllViolateCasperFFGPunish() {
	rawdb.ClearAllViolateCasperFFGPunish(c.db)
}

// VerifyAttestation checks whether an attestation is valid,
// and if it's valid, return the signer,
// and a threshold that indicates how many attestations can justify a block.
func (c *Turbo) VerifyAttestation(chain consensus.ChainHeaderReader, a *types.Attestation) (common.Address, int, error) {
	header := chain.GetHeader(a.TargetRangeEdge.Hash, a.TargetRangeEdge.Number.Uint64())
	if header == nil {
		return common.Address{}, 0, errUnknownBlock
	}
	// verify signature
	signer, err := a.RecoverSigner()
	if err != nil {
		return common.Address{}, 0, err
	}
	// check if it is an authorized validator?
	snap, err := c.snapshot(chain, header.Number.Uint64(), header.Hash(), nil)
	if err != nil {
		return common.Address{}, 0, err
	}

	if !snap.IsAuthorized(signer) {
		return common.Address{}, 0, errIsNotValidator
	}
	return signer, attestationThreshold(snap.Len()), nil
}

func attestationThreshold(valsCnt int) int {
	return valsCnt*attestationThresholdNumerator/attestationThresholdDenominator + 1
}

func (c *Turbo) CurrentValidator() common.Address {
	return c.validator
}

func (c *Turbo) MaxValidators() uint8 {
	return systemcontract.TopValidatorNum
}

func (c *Turbo) Attest(chain consensus.ChainHeaderReader, headerNum *big.Int, source, target *types.RangeEdge) (*types.Attestation, error) {
	if !c.IsReadyAttest() {
		return nil, errIsNotReadyAttest
	}
	if !c.IsAuthorizedAtHeight(chain, c.validator, target.Number.Uint64()) {
		return nil, errIsNotAuthorizedAtHeight
	}
	return c.makeNewAttestation(source, target)
}

// keccak256(abi.encode(s,t,h(s),h(t)) , where s is the hash of the last justified block,
// t is the hash of the current block to vote, and h(s) h(T) are the corresponding block numbers respectively.
func (c *Turbo) makeNewAttestation(sourceRangeEdge *types.RangeEdge, targetRangeEdge *types.RangeEdge) (*types.Attestation, error) {
	// because the sign function is `Wallet.SignData`，so we should pass the data to it, not the hash.
	sig, err := c.signFn(accounts.Account{Address: c.validator}, "", types.AttestationData(sourceRangeEdge, targetRangeEdge))
	if err != nil {
		return nil, errSignFailed
	}
	return types.NewAttestation(sourceRangeEdge, targetRangeEdge, sig), nil
}

func (c *Turbo) IsAuthorizedAtHeightAndHash(chain consensus.ChainHeaderReader, val common.Address, hash common.Hash, number uint64) bool {
	h := chain.GetHeader(hash, number)
	return c.IsAuthorizedByHeader(chain, val, h)
}

func (c *Turbo) IsAuthorizedAtHeight(chain consensus.ChainHeaderReader, val common.Address, height uint64) bool {
	h := chain.GetHeaderByNumber(height)
	return c.IsAuthorizedByHeader(chain, val, h)
}

func (c *Turbo) IsAuthorizedByHeader(chain consensus.ChainHeaderReader, val common.Address, h *types.Header) bool {
	if h == nil {
		log.Error("can not find the header when attesting")
		return false
	}
	height := h.Number.Uint64()
	snap, err := c.snapshot(chain, height, h.Hash(), nil)
	if err != nil {
		log.Error("get snapshot failed when attesting", "height", height, "err", err)
		return false
	}
	// if not an authorized validator for that block, skip it
	return snap.IsAuthorized(val)
}

func (c *Turbo) CurrentNeedHandleHeight(headerNum uint64) (uint64, error) {
	// Witness voting is postponed for two heights(config.AttestationDelay).
	if headerNum <= c.config.AttestationDelay {
		return 0, errors.New("execution height not reached")
	}
	return headerNum - c.config.AttestationDelay, nil
}

func (c *Turbo) AttestationDelay() uint64 {
	return c.config.AttestationDelay
}

func (c *Turbo) IsReadyAttest() bool {
	return c.isReady && c.attestationStatus == types.AttestationStart
}

func (c *Turbo) AttestationThreshold(chain consensus.ChainHeaderReader, hash common.Hash, number uint64) (int, error) {
	header := chain.GetHeader(hash, number)
	if header == nil {
		return 0, errUnknownBlock
	}
	snap, err := c.snapshot(chain, header.Number.Uint64(), header.Hash(), nil)
	if err != nil {
		return 0, err
	}
	return attestationThreshold(snap.Len()), nil
}

func (c *Turbo) Validators(chain consensus.ChainHeaderReader, hash common.Hash, number uint64) ([]common.Address, error) {
	header := chain.GetHeader(hash, number)
	if header == nil {
		return nil, errUnknownBlock
	}
	snap, err := c.snapshot(chain, header.Number.Uint64(), header.Hash(), nil)
	if err != nil {
		return nil, err
	}
	return snap.validators(), nil
}

// VerifyCasperFFGRule Judge whether there is a violation of the rules before the two proofs according to the CasperFFG rules
// Duplicate data has been filtered out before arriving here
// No matter the same branch or different branches, the same height can only be cast once
// For fully inclusive relationships, penalties will be imposed
// Other cross relationships or coupling relationships will not be violated
func (c *Turbo) VerifyCasperFFGRule(beforeSourceNum uint64, beforeTargetNum uint64, afterSourceNum uint64, afterTargetNum uint64) int {
	if beforeTargetNum == afterTargetNum {
		return types.PunishMultiSig
	} else if (beforeSourceNum < afterSourceNum && beforeTargetNum > afterTargetNum) ||
		(afterSourceNum < beforeSourceNum && afterTargetNum > beforeTargetNum) {
		return types.PunishInclusive
	}
	return types.PunishNone
}

// Assembly of penalty transactions in violation of CasperFFG rules
func (c *Turbo) executeDoubleSignPunish(chain consensus.ChainHeaderReader, header *types.Header,
	state *state.StateDB, p *types.ViolateCasperFFGPunish, totalTxIndex int) (*types.Transaction, *types.Receipt, error) {
	if c.signTxFn == nil {
		return nil, nil, errors.New("signTxFn not set")
	}

	p.PunishAddr = system.StakingContract
	p.Plaintiff = c.validator
	signer, err := p.RecoverSigner()
	if err != nil {
		return nil, nil, err
	}
	p.Defendant = signer

	pRLP, err := rlp.EncodeToBytes(p)
	if err != nil {
		return nil, nil, err
	}
	copy(p.Data, pRLP)
	//make system governance transaction
	nonce := state.GetNonce(c.validator)

	// Special to address for filtering transactions
	tx := types.NewTransaction(nonce, doubleSignIdentity, uint256Max, 0, common.Big0, pRLP)
	tx, err = c.signTxFn(accounts.Account{Address: c.validator}, tx, chain.Config().ChainID)
	if err != nil {
		return nil, nil, err
	}

	//add nonce for validator
	state.SetNonce(c.validator, nonce+1)
	receipt, err := c.executeDoubleSignPunishMsg(chain, header, state, p, totalTxIndex, tx.Hash(), common.Hash{})

	return tx, receipt, err
}

// After receiving a block containing multiple signed penalty transactions, execute the penalty transactions in it.
// If the execution fails, discard the whole block. BAD BLOCK
func (c *Turbo) replayDoubleSignPunish(chain consensus.ChainHeaderReader, header *types.Header, state *state.StateDB, totalTxIndex int, tx *types.Transaction) (*types.Receipt, error) {
	log.Debug("replayDoubleSignPunish", "Number", header.Number.Uint64())
	sender, err := types.Sender(c.signer, tx)
	if err != nil {
		return nil, err
	}
	if sender != header.Coinbase {
		return nil, errors.New("invalid sender for system transaction")
	}
	var p types.ViolateCasperFFGPunish
	if err := rlp.DecodeBytes(tx.Data(), &p); err != nil {
		return nil, err
	}
	// Clear your own records at the first time after receiving them to avoid data error accumulation
	rawdb.DeleteViolateCasperFFGPunish(c.db, &p)
	copy(p.Data, tx.Data())
	if b, err := c.IsDoubleSignPunished(chain, header, state, p.Hash()); err != nil || b {
		return nil, errors.New("is double sign punished")
	}
	// Verify that the data is valid
	signer, err := p.RecoverSigner()
	if err != nil {
		return nil, err
	}
	if signer != p.Defendant {
		return nil, errors.New("transaction signature does not match")
	}
	if types.PunishNone == c.VerifyCasperFFGRule(p.Before.SourceRangeEdge.Number.Uint64(), p.Before.TargetRangeEdge.Number.Uint64(),
		p.After.SourceRangeEdge.Number.Uint64(), p.After.TargetRangeEdge.Number.Uint64()) {
		return nil, errors.New("the transaction did not violate CasperFFG rules")
	}
	nonce := state.GetNonce(sender)
	//add nonce for validator
	state.SetNonce(sender, nonce+1)
	return c.executeDoubleSignPunishMsg(chain, header, state, &p, totalTxIndex, tx.Hash(), header.Hash())
}

// IsDoubleSignPunished Execute the query of punishment contract to judge whether the punishment hash of the current query has been punished
func (c *Turbo) IsDoubleSignPunished(chain consensus.ChainHeaderReader, header *types.Header, state *state.StateDB, punishHash common.Hash) (bool, error) {
	// isDoubleSignPunished(bytes32 punishHash) public view returns (bool)
	return systemcontract.IsDoubleSignPunished(&contracts.CallContext{
		Statedb:      state,
		Header:       header,
		ChainContext: newChainContext(chain, c),
		ChainConfig:  c.chainConfig,
	}, punishHash)
}

// Execute multi sign penalty transaction in EVM
func (c *Turbo) executeDoubleSignPunishMsg(chain consensus.ChainHeaderReader, header *types.Header, state *state.StateDB, p *types.ViolateCasperFFGPunish, totalTxIndex int, txHash, bHash common.Hash) (*types.Receipt, error) {
	var receipt *types.Receipt

	state.SetTxContext(txHash, totalTxIndex)
	topics := []common.Hash{
		executedDoubleSignPunishEventSig,
		common.BytesToHash(p.Plaintiff[:]),
		common.BytesToHash(p.Defendant[:]),
		common.BigToHash(p.PunishType),
	}
	// build data
	data := buildDoubleSignPunishExecutedEventData(p)
	pLog := &types.Log{
		Address:     system.StakingContract,
		Topics:      topics,
		Data:        data,
		BlockNumber: header.Number.Uint64(),
	}
	state.AddLog(pLog)

	// must succeed
	err := systemcontract.DoubleSignPunish(&contracts.CallContext{
		Statedb:      state,
		Header:       header,
		ChainContext: newChainContext(chain, c),
		ChainConfig:  c.chainConfig,
	}, p.Hash(), p.Defendant)
	if err != nil {
		return nil, err
	}

	receipt = types.NewReceipt([]byte{}, err != nil, header.GasUsed)
	log.Info("executeDoubleSignPunishMsg", "Plaintiff", p.Plaintiff, "Defendant", p.Defendant, "pushHash", p.Hash().String(), "success", true)

	receipt.Logs = state.GetLogs(txHash, header.Number.Uint64(), bHash)
	receipt.Bloom = types.CreateBloom(types.Receipts{receipt})

	receipt.TxHash = txHash
	receipt.BlockHash = bHash
	receipt.BlockNumber = header.Number
	receipt.TransactionIndex = uint(state.TxIndex())
	return receipt, nil
}

// IsDoubleSignPunishTransaction Judge whether the transaction is a multi sign penalty transaction.
// Due to the particularity of transaction data, a special to address is used to distinguish
func (c *Turbo) IsDoubleSignPunishTransaction(sender common.Address, tx *types.Transaction, header *types.Header) bool {
	if tx.To() == nil || len(tx.Data()) < 4 {
		return false
	}
	to := tx.To()
	if sender == header.Coinbase &&
		*to == doubleSignIdentity &&
		tx.Value().Cmp(uint256Max) == 0 &&
		tx.Gas() == 0 &&
		tx.GasPrice().Sign() == 0 {
		return true
	}
	return false
}

// ApplyDoubleSignPunishTx TODO
func (c *Turbo) ApplyDoubleSignPunishTx(evm *vm.EVM, sender common.Address, tx *types.Transaction) (ret []byte, vmerr error, err error) {
	p := &types.ViolateCasperFFGPunish{}
	if err = rlp.DecodeBytes(tx.Data(), p); err != nil {
		return
	}
	nonce := evm.StateDB.GetNonce(sender)
	//add nonce for validator
	evm.StateDB.SetNonce(sender, nonce+1)
	evm.TxContext = vm.TxContext{
		Origin:   p.Plaintiff,
		GasPrice: new(big.Int).Set(big.NewInt(0)),
	}
	err = systemcontract.DoubleSignPunishWithGivenEVM(evm, p.Plaintiff, p.Hash(), p.Defendant)
	return nil, nil, err
}

func buildDoubleSignPunishExecutedEventData(p *types.ViolateCasperFFGPunish) []byte {
	doubleSignPunishDataLen := ((len(p.Data) + common.HashLength - 1) / common.HashLength) * common.HashLength
	dataLen := 2*common.HashLength + doubleSignPunishDataLen
	data := make([]byte, dataLen)
	copy(data[:common.HashLength], common.BytesToHash([]byte{0x20}).Bytes())
	copy(data[common.HashLength:2*common.HashLength], common.BigToHash(big.NewInt(int64(len(p.Data)))).Bytes())
	copy(data[2*common.HashLength:], p.Data)
	return data
}
