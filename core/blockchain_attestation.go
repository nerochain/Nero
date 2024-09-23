package core

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"runtime"
	"sort"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
)

const (
	maxGapForOldOrFutureAttestation = 16
	maxFutureAttestations           = maxGapForOldOrFutureAttestation * 21
	attestationsCacheLimit          = 1024
	historyAttessCacheLimit         = 64
	casperFFGHistoryCacheLimit      = 21 * 3
	blockStatusCacheLimit           = 1024

	casperFFGHistoryCacheToKeep = 100

	catchUpDiffBlocks     = 2
	catchUpDiffTime       = catchUpDiffBlocks * 3
	catchUpSafetyMultiple = 2
)

const (
	diffUpperLimitWarning        = 63
	unableSureBlockStateInterval = 100
)

// HandleAttestation The attestations received from other P2P nodes are processed through a series of security checks.
// The certificates that meet the inspection will be stored according to the height of the current chain plot.
// If they are higher than the local height, they will be stored in the future cache.
func (bc *BlockChain) HandleAttestation(a *types.Attestation) error {
	currentBlockNumber := bc.CurrentBlock().Number.Uint64()
	if err := a.SanityCheck(); err != nil {
		return err
	}
	sourceNumber := a.SourceRangeEdge.Number.Uint64()
	targetNumber := a.TargetRangeEdge.Number.Uint64()
	if targetNumber-sourceNumber > unableSureBlockStateInterval {
		return errors.New("inspection interval not conforming to attestation")
	}
	if !bc.VerifyValidLimit(targetNumber, currentBlockNumber) {
		return nil
	}

	if targetNumber <= currentBlockNumber {
		isExist := bc.IsExistsRecentCache(a)
		if isExist {
			return nil
		}
	} else {
		if bc.IsExistsFutureCache(a) {
			return nil
		}
	}
	signer, err := a.RecoverSigner()
	if err != nil {
		log.Warn("RecoverSigner error:", "err", err.Error())
		return err
	}
	if !bc.VerifyLocalDataCheck(a, currentBlockNumber) {
		return errors.New("the block information in the current proof does not match the local data")
	}
	if !bc.VerifySignerInEpochValidBP(targetNumber, signer) {
		return errors.New("the signer of the current attestation is not a valid verifier in the current epoch")
	}
	if targetNumber <= currentBlockNumber {
		return bc.AddOneAttestationToRecentCache(a, signer, false)
	}
	return bc.AddOneAttestationToFutureCache(a)
}

// attestationHandleLoop process attestation trigger by NewHeadEvent
func (bc *BlockChain) AttestationHandleLoop() {
	defer bc.wg.Done()
	chainHeadCh := make(chan ChainHeadEvent)
	sub := bc.SubscribeChainHeadEvent(chainHeadCh)
	defer sub.Unsubscribe()
	for {
		select {
		case ev := <-chainHeadCh:
			bc.processAttestationOnHead(ev.Block.Header())
		case <-bc.quit:
			return
		}
	}
}

// At this stage, calculate the current best attestation
// NewAttestation = {source: A, target: information of A + 1 height block}

// Less than 1 / 3 nodes behind
// However, if some nodes lag behind other nodes due to the network or their own reasons,
// they will also update the local block status when a subsequent highest block receives
// sufficient certificates. Then the next creation of certificates can be directly followed up from this height.

// More than 1 / 3 backward
// That is, neither part can complete the threshold "2 / 3 + 1" confirmation.
// At this time, it will continue by default. No new certificates will be submitted.
// They are waiting for other nodes in the network to broadcast to them and reach the new justified or finalized
// block state before continuing to create subsequent certificates
// A new consensus logic is added. When the final a information obtained locally has fallen 100 blocks behind the
// height of the current block to be processed, the current node will be created
// NewAttestation = {source: block height to be processed before - 100, target: block height to be processed before}
// When enough new block certificates are not received, the node continues to create the above certificates until the
// qualified or finalized block state of the new block is received, and then the network returns to normal
func (bc *BlockChain) bestAttestationToProcessed(headNum *big.Int) (*types.Attestation, error) {
	currentNeedHandleHeight, err := bc.TurboEngine.CurrentNeedHandleHeight(headNum.Uint64())
	if err != nil {
		return nil, err
	}
	latestAttestedNum := bc.currentAttestedNumber.Load().(*big.Int).Uint64()
	if currentNeedHandleHeight <= latestAttestedNum { // Prevent multiple signups due to block rollback
		return nil, errors.New("the current block height does not reach the range")
	}
	re := bc.LastValidJustifiedOrFinalized()
	block := bc.GetBlockByNumber(currentNeedHandleHeight)
	target := &types.RangeEdge{
		Hash:   block.Hash(),
		Number: block.Number(),
	}

	// Self recovery
	if currentNeedHandleHeight > unableSureBlockStateInterval {
		diffNumber := currentNeedHandleHeight - unableSureBlockStateInterval
		if re.Number.Uint64() <= diffNumber {
			b := bc.GetBlockByNumber(diffNumber)
			source := &types.RangeEdge{Number: new(big.Int).Set(b.Number()), Hash: b.Hash()}
			return bc.TurboEngine.Attest(bc, new(big.Int).SetUint64(currentNeedHandleHeight), source, target)
		}
	}
	// Fast update
	// Try your best to submit. The maximum probability occurs when there is a block height difference
	if re.Number.Uint64() >= currentNeedHandleHeight {
		status, _ := bc.GetBlockStatusByNum(latestAttestedNum)
		if status == types.BasJustified || status == types.BasFinalized {
			b := bc.GetBlockByNumber(latestAttestedNum)
			source := &types.RangeEdge{Number: new(big.Int).Set(b.Number()), Hash: b.Hash()}
			return bc.TurboEngine.Attest(bc, new(big.Int).SetUint64(currentNeedHandleHeight), source, target)
		}
		return nil, errors.New("the current block height does not reach the range")
	}
	return bc.TurboEngine.Attest(bc, new(big.Int).SetUint64(currentNeedHandleHeight), re, target)
}

// Subscribe to the ChainHeadEvent message. After obtaining the new block event, first check whether it meets
// the processing conditions, then obtain the corresponding block, create the corresponding proof information
// according to the block information and the previous valid block status information, and finally carry out
// broadcast storage and other processes
func (bc *BlockChain) processAttestationOnHead(head *types.Header) {
	if bc.TurboEngine.AttestationStatus() == types.AttestationPending {
		// Give priority to judge whether it has caught up
		firstCatchup := bc.firstCatchUpNumber.Load().(*big.Int)
		attestationDelay := bc.TurboEngine.AttestationDelay()
		// Prevent false triggering during node initialization
		continuousBlocks := bc.chainConfig.TurboContinuousInturn(head.Number)
		if firstCatchup.Uint64() == 0 && head.Number.Uint64() > continuousBlocks*catchUpSafetyMultiple &&
			uint64(time.Now().Unix()) <= head.Time+catchUpDiffTime {
			num := head.Number.Uint64() + catchUpDiffBlocks
			bc.firstCatchUpNumber.Store(new(big.Int).SetUint64(num))
			log.Info("🎄Will StartAttestation", "Number", num+attestationDelay+unableSureBlockStateInterval)
		}
		firstCatchup = bc.firstCatchUpNumber.Load().(*big.Int)
		if firstCatchup.Uint64() > 0 && head.Number.Uint64() > firstCatchup.Uint64()+attestationDelay &&
			head.Number.Uint64()-firstCatchup.Uint64()-attestationDelay >= unableSureBlockStateInterval {
			bc.TurboEngine.StartAttestation()
			log.Info("✨StartAttestation", "firstCatchup", firstCatchup.Uint64(), "currentHeight", head.Number.Uint64())
		}
	}

	err := bc.UpdateCurrentEpochBPList(head.Hash(), head.Number.Uint64())
	if err != nil {
		log.Error(err.Error())
		return
	}
	if bc.TurboEngine.IsReadyAttest() {
		// From the perspective of the current node itself, all it can do is create
		// attestation in turn, and it cannot initiate across heights
		a, err := bc.bestAttestationToProcessed(head.Number)
		if err != nil {
			log.Warn(err.Error())
			return
		}
		if a == nil {
			return
		}
		isExist := bc.IsExistsRecentCache(a)
		if isExist {
			return
		}
		log.Debug("Create a attestation", "SourceNum", a.SourceRangeEdge.Number.Uint64(),
			"TargetNum", a.TargetRangeEdge.Number.Uint64())
		threshold, err := bc.TurboEngine.AttestationThreshold(bc, a.TargetRangeEdge.Hash, a.TargetRangeEdge.Number.Uint64())
		if err != nil {
			log.Error(err.Error())
			return
		}
		err = bc.AddOneValidAttestationToRecentCache(a, threshold, bc.TurboEngine.CurrentValidator())
		if err != nil {
			log.Error(err.Error())
			return
		}
		bc.StoreLastAttested(a.TargetRangeEdge.Number)
	}
	err = bc.MoveAttestsCacheFutureToRecent(head.Number)
	if err != nil {
		log.Error(err.Error())
	}
}

// LastValidJustifiedOrFinalized Get the last valid block status information after the specified block
func (bc *BlockChain) LastValidJustifiedOrFinalized() *types.RangeEdge {
	last := bc.currentBlockStatusNumber.Load().(*big.Int)
	if last.Uint64() == 0 {
		return &types.RangeEdge{Number: new(big.Int).SetUint64(0), Hash: common.Hash{}}
	}
	block := bc.GetBlockByNumber(last.Uint64())
	return &types.RangeEdge{
		Hash:   block.Hash(),
		Number: block.Number(),
	}
}

// StoreLastAttested Stores the height of the last processed block
func (bc *BlockChain) StoreLastAttested(num *big.Int) {
	last := bc.currentAttestedNumber.Load().(*big.Int)
	if num.Cmp(last) <= 0 {
		return
	}
	rawdb.WriteLastAttestNumber(bc.db, bc.TurboEngine.CurrentValidator(), num)
	bc.currentAttestedNumber.Store(new(big.Int).Set(num))
}

// AddOneAttestationToRecentCache Trying to add a attestation to the RecentCache store requires a series of checks
func (bc *BlockChain) AddOneAttestationToRecentCache(a *types.Attestation, signer common.Address, isTest bool) error {
	bc.lockAddOneAttestationToRecentCache.Lock()
	defer bc.lockAddOneAttestationToRecentCache.Unlock()
	// check again whether it's already exists or not
	if bc.IsExistsRecentCache(a) {
		return nil
	}
	// The CasperFFG rule penalty cannot be added because regular block rollback is easy to occur
	if !isTest {
		branch, err := bc.IsFiliation(a.SourceRangeEdge, a.TargetRangeEdge)
		if err != nil || !branch {
			return errors.New("it is currently proved that the two blocks are not in the same branch")
		}
	}

	_, threshold, err := bc.TurboEngine.VerifyAttestation(bc, a)
	if err != nil && !isTest {
		return err
	}
	err = bc.VerifyCasperFFGRecentCache(a, signer)
	if err != nil {
		return err
	}
	return bc.AddOneValidAttestationToRecentCache(a, threshold, signer)
}

// Get the goroutine ID of the current execution, which is used to locate problems among multiple goroutines
func (bc *BlockChain) goID() uint64 {
	b := make([]byte, 64)
	b = b[:runtime.Stack(b, false)]
	b = bytes.TrimPrefix(b, []byte("goroutine "))
	b = b[:bytes.IndexByte(b, ' ')]
	n, _ := strconv.ParseUint(string(b), 10, 64)
	return n
}

// AddOneValidAttestationToRecentCache Add a valid attestation to RecentCache storage, broadcast the corresponding
// data to other nodes, and store the corresponding data for historical data and CasperFFG rule verification
func (bc *BlockChain) AddOneValidAttestationToRecentCache(a *types.Attestation, threshold int, signer common.Address) error {
	bc.lockRecentAttessCache.Lock()
	defer bc.lockRecentAttessCache.Unlock()

	totalCount := 1
	dreHash := a.SignHash()
	treNumber := a.TargetRangeEdge.Number
	treHash := a.TargetRangeEdge.Hash
	treNumberUint64 := treNumber.Uint64()

	if oldBna, found := bc.RecentAttessCache.Get(treNumberUint64); found {
		oldAddrMap := oldBna.AttestsMap[dreHash]
		if oldAddrMap == nil {
			oldAddrMap = make(map[common.Hash]bool)
		}
		oldAddrMap[a.Hash()] = true
		oldBna.AttestsMap[dreHash] = oldAddrMap
		bc.RecentAttessCache.Add(treNumberUint64, oldBna)
		totalCount = len(oldBna.AttestsMap[dreHash])
	} else {
		newBna := new(types.BlockNumAttestations)
		newBna.AttestsMap = make(map[common.Hash]map[common.Hash]bool)
		newAddrMap := make(map[common.Hash]bool)
		newAddrMap[a.Hash()] = true
		newBna.AttestsMap[dreHash] = newAddrMap
		bc.RecentAttessCache.Add(treNumberUint64, newBna)
	}

	if totalCount >= threshold {
		status, _ := bc.GetBlockStatusByNum(treNumber.Uint64())
		if status == types.BasUnknown { // not found
			status, err := bc.AddBlockBasJustified(treNumber, treHash)
			if err != nil {
				log.Error(err.Error())
			}
			if status == types.BasJustified || status == types.BasFinalized {
				bc.BroadcastNewJustifiedOrFinalizedBlockToOtherNodes(
					&types.BlockStatus{BlockNumber: treNumber, Hash: treHash,
						Status: status})
			}
		}
	}
	log.Debug("🙋 Received a valid attestation", "number", treNumberUint64, "totalCount", totalCount,
		"threshold", threshold, "GoId", bc.goID())
	bc.BroadcastNewAttestationToOtherNodes(a)
	bc.addOneValidAttestationToHistoryCache(a)
	return bc.addOneValidAttestationForCasperFFG(signer, a)
}

// AddOneAttestationToFutureCache Provide storage for the blocks received by handleattesting that are higher than the local height. When the local
// block height reaches the same height, carry out corresponding inspection and merging processing
func (bc *BlockChain) AddOneAttestationToFutureCache(a *types.Attestation) error {
	bc.lockFutureAttessCache.Lock()
	defer bc.lockFutureAttessCache.Unlock()

	cAs, found := bc.FutureAttessCache.Get(a.TargetRangeEdge.Number.Uint64())
	if found {
		cAs.Attestations[a.Hash()] = a
	} else {
		cAs = new(types.FutureAttestations)
		cAs.Attestations = make(map[common.Hash]*types.Attestation)
		cAs.Attestations[a.Hash()] = a
	}

	bc.FutureAttessCache.Add(a.TargetRangeEdge.Number.Uint64(), cAs)
	return nil
}

// AddBlockBasJustified Add a block status for the corresponding block, which is currently added as a lazy setting.
// If the status of the previous block is already justified for the same branch, modify the status of
// the previous block to finalized. If the status of the latter block is judged or finalized, set the status
// of the current block to be processed to finalized, otherwise it is judged
func (bc *BlockChain) AddBlockBasJustified(num *big.Int, hash common.Hash) (uint8, error) {
	if status, hashBefore := bc.GetBlockStatusByNum(num.Uint64() - 1); status == types.BasJustified {
		branch, err := bc.IsFiliation(&types.RangeEdge{
			Hash:   hashBefore,
			Number: new(big.Int).SetUint64(num.Uint64() - 1),
		}, &types.RangeEdge{
			Hash:   hash,
			Number: num,
		})
		if err == nil && branch {
			err := bc.UpdateBlockStatus(new(big.Int).SetUint64(num.Uint64()-1), hashBefore, types.BasFinalized)
			if err != nil {
				return types.BasUnknown, err
			}
		}
	}
	currentBlockStatus := types.BasJustified
	if status, hashAfter := bc.GetBlockStatusByNum(num.Uint64() + 1); status != types.BasUnknown {
		branch, err := bc.IsFiliation(&types.RangeEdge{
			Hash:   hash,
			Number: num,
		}, &types.RangeEdge{
			Hash:   hashAfter,
			Number: new(big.Int).SetUint64(num.Uint64() + 1),
		})
		if err == nil && branch {
			currentBlockStatus = types.BasFinalized
		}
	}
	return currentBlockStatus, bc.UpdateBlockStatus(num, hash, currentBlockStatus)
}

// addOneValidAttestationForCasperFFG Store corresponding data for casperffg rule judgment.
// The data here is stored in the cache. For punishment, only try your best
func (bc *BlockChain) addOneValidAttestationForCasperFFG(signer common.Address, a *types.Attestation) error {
	bc.lockCasperFFGHistoryCache.Lock()
	defer bc.lockCasperFFGHistoryCache.Unlock()

	newHistory := &types.CasperFFGHistory{
		TargetNum:       a.TargetRangeEdge.Number,
		SourceNum:       a.SourceRangeEdge.Number,
		TargetHash:      a.TargetRangeEdge.Hash,
		AttestationHash: a.Hash(),
	}

	cfList, found := bc.CasperFFGHistoryCache.Get(signer)
	if found {
		cfList = append(cfList, newHistory)
		sort.Sort(sort.Reverse(cfList))
		if len(cfList) > casperFFGHistoryCacheToKeep {
			cfList = cfList[:casperFFGHistoryCacheToKeep]
		}
		bc.CasperFFGHistoryCache.Add(signer, cfList)
	} else {
		var newCfList types.CasperFFGHistoryList
		newCfList = append(newCfList, newHistory)
		bc.CasperFFGHistoryCache.Add(signer, newCfList)
	}
	return nil
}

// MoveAttestsCacheFutureToRecent Review and merge future data
func (bc *BlockChain) MoveAttestsCacheFutureToRecent(num *big.Int) error {
	bc.lockFutureAttessCache.Lock()
	defer bc.lockFutureAttessCache.Unlock()

	cAs, found := bc.FutureAttessCache.Get(num.Uint64())
	if found {
		for _, a := range cAs.Attestations {
			isExist := bc.IsExistsRecentCache(a)
			if isExist {
				continue
			}
			signer, err := a.RecoverSigner()
			if err != nil {
				continue
			}
			_ = bc.AddOneAttestationToRecentCache(a, signer, false)
		}
		bc.FutureAttessCache.Remove(num.Uint64())
	}
	return nil
}

// VerifyCasperFFGRecentCache Verify whether there are multiple signatures or include
func (bc *BlockChain) VerifyCasperFFGRecentCache(a *types.Attestation, signer common.Address) error {
	cfhList, found := bc.CasperFFGHistoryCache.Get(signer)
	if found {
		for _, h := range cfhList {
			ruleType := bc.TurboEngine.VerifyCasperFFGRule(a.SourceRangeEdge.Number.Uint64(), a.TargetRangeEdge.Number.Uint64(),
				h.SourceNum.Uint64(), h.TargetNum.Uint64())
			if ruleType != types.PunishNone {
				p, err := bc.GetHistoryOneAttestation(h.TargetNum, h.TargetHash, h.AttestationHash)
				if err != nil {
					return err //Historical data is cleared relatively early
				}
				if p != nil {
					if ruleType == types.PunishMultiSig && p.Hash() != a.Hash() {
						err := bc.ViolationCasperFFGExecutePunish(p, a, types.PunishMultiSig, bc.CurrentBlock().Number)
						if err != nil {
							return err
						}
						return errors.New("multi-signature with historical attestation")
					} else if ruleType == types.PunishInclusive {
						err := bc.ViolationCasperFFGExecutePunish(p, a, types.PunishInclusive, bc.CurrentBlock().Number)
						if err != nil {
							return err
						}
						log.Debug("CasperFFG inclusive", "1TNumer", p.TargetRangeEdge.Number.Uint64(),
							"1SNumer", p.SourceRangeEdge.Number.Uint64(), "2TNumer", a.TargetRangeEdge.Number.Uint64(),
							"2SNumer", a.SourceRangeEdge.Number.Uint64())
						return errors.New("inclusive relationship with historical attestation")
					}
				}
			}
			if h.TargetNum.Uint64() <= a.SourceRangeEdge.Number.Uint64() {
				break
			}
		}
	}
	return nil
}

// ViolationCasperFFGExecutePunish The proof data to be punished will be stored persistently. When mining blocks at the current node,
// the data to be punished will be assembled into corresponding punishment transactions and placed in the new block
func (bc *BlockChain) ViolationCasperFFGExecutePunish(before *types.Attestation, after *types.Attestation, punishType int, blockNum *big.Int) error {
	return rawdb.WriteViolateCasperFFGPunish(bc.TurboEngine.GetDb(), before, after, punishType, blockNum)
}

func (bc *BlockChain) VerifyLowerLimit(num uint64, currentNum uint64) bool {
	if currentNum <= maxGapForOldOrFutureAttestation {
		return true
	}
	return num >= (currentNum - maxGapForOldOrFutureAttestation)
}

func (bc *BlockChain) VerifyUpperLimit(num uint64, currentNum uint64) bool {
	return num <= (currentNum + maxGapForOldOrFutureAttestation)
}

func (bc *BlockChain) VerifyValidLimit(num uint64, currentNum uint64) bool {
	return bc.VerifyLowerLimit(num, currentNum) && bc.VerifyUpperLimit(num, currentNum)
}

func (bc *BlockChain) IsExistsRecentCache(a *types.Attestation) bool {
	bc.lockRecentAttessCache.Lock()
	defer bc.lockRecentAttessCache.Unlock()

	if oldBna, found := bc.RecentAttessCache.Get(a.TargetRangeEdge.Number.Uint64()); found {
		if oldAddrMap, found := oldBna.AttestsMap[a.SignHash()]; found {
			return oldAddrMap[a.Hash()]
		}
	}
	return false
}

func (bc *BlockChain) IsExistsFutureCache(a *types.Attestation) bool {
	bc.lockFutureAttessCache.Lock()
	defer bc.lockFutureAttessCache.Unlock()

	cAs, found := bc.FutureAttessCache.Get(a.TargetRangeEdge.Number.Uint64())
	if found {
		_, found := cAs.Attestations[a.Hash()]
		return found
	}
	return false
}

func (bc *BlockChain) VerifyLocalDataCheck(a *types.Attestation, number uint64) bool {
	if a.SourceRangeEdge.Number.Uint64() <= number {
		if (a.SourceRangeEdge.Number.Uint64() != 0) && (!bc.HasBlock(a.SourceRangeEdge.Hash, a.SourceRangeEdge.Number.Uint64())) {
			return false
		}
	}
	if a.TargetRangeEdge.Number.Uint64() <= number {
		if !bc.HasBlock(a.TargetRangeEdge.Hash, a.TargetRangeEdge.Number.Uint64()) {
			return false
		}
	}
	return true
}

func (bc *BlockChain) BroadcastNewAttestationToOtherNodes(a *types.Attestation) {
	bc.newAttestationFeed.Send(NewAttestationEvent{a})
}

func (bc *BlockChain) BroadcastNewJustifiedOrFinalizedBlockToOtherNodes(bs *types.BlockStatus) {
	bc.newJustifiedOrFinalizedBlockFeed.Send(NewJustifiedOrFinalizedBlockEvent{bs})
}

func (bc *BlockChain) CalculateCurrentEpochIndex(number uint64) uint64 {
	return number / bc.chainConfig.Turbo.Epoch
}

// UpdateCurrentEpochBPList Continuously update the BP list within two epoch cycles for verification when receiving the
// certificate to avoid DDoS attacks initiated by non BP nodes
func (bc *BlockChain) UpdateCurrentEpochBPList(hash common.Hash, number uint64) error {
	var last *types.EpochCheckBps
	value := bc.currentEpochCheckBps.Load()
	if value != nil {
		last = value.(*types.EpochCheckBps)
	}
	newCurrentEpochIndex := bc.CalculateCurrentEpochIndex(number)
	if last == nil || last.CurrentEpochIndex.Uint64() < newCurrentEpochIndex {
		bps, err := bc.TurboEngine.Validators(bc, hash, number)
		if err != nil {
			return err
		}
		var lastEpochBps []common.Address
		var lastEpochIndex *big.Int
		if last != nil {
			lastEpochBps = last.CurrentEpochBps
			lastEpochIndex = new(big.Int).Set(last.CurrentEpochIndex)
		} else { //Update previous cycle
			epoch := bc.chainConfig.Turbo.Epoch
			if number > epoch {
				block := bc.GetBlockByNumber(number - epoch)
				lastBps, err := bc.TurboEngine.Validators(bc, block.Hash(), block.NumberU64())
				if err != nil {
					return err
				}
				lastEpochBps = lastBps
				lastEpochIndex = new(big.Int).SetUint64(newCurrentEpochIndex - 1)
			} else {
				lastEpochBps = bps
				lastEpochIndex = new(big.Int).SetUint64(newCurrentEpochIndex)
			}
		}

		bc.currentEpochCheckBps.Store(&types.EpochCheckBps{
			CurrentEpochBps:   bps,
			LastEpochBps:      lastEpochBps,
			CurrentEpochIndex: new(big.Int).SetUint64(newCurrentEpochIndex),
			LastEpochIndex:    lastEpochIndex,
		})
	}
	return nil
}

// VerifySignerInEpochValidBP Verify whether the current BP address is a valid BP.
// The BP address in the future block will be verified in two cycles
func (bc *BlockChain) VerifySignerInEpochValidBP(number uint64, signer common.Address) bool {
	var last *types.EpochCheckBps
	value := bc.currentEpochCheckBps.Load()
	if value != nil {
		last = value.(*types.EpochCheckBps)
	}
	if last == nil {
		return false // Just started running, not ready yet
	}
	newCurrentEpochIndex := bc.CalculateCurrentEpochIndex(number)
	if last.CurrentEpochIndex.Uint64() != newCurrentEpochIndex {
		for _, bp := range last.LastEpochBps {
			if bp == signer {
				return true
			}
		}
	}
	for _, bp := range last.CurrentEpochBps {
		if bp == signer {
			return true
		}
	}
	return false
}

// addOneValidAttestationToHistoryCache Adding valid certificates to the historical data will be obtained
// by other nodes and provide corresponding original data for casperffg
func (bc *BlockChain) addOneValidAttestationToHistoryCache(a *types.Attestation) bool {
	bc.lockHistoryAttessCache.Lock()
	defer bc.lockHistoryAttessCache.Unlock()

	hAs, found := bc.HistoryAttessCache.Get(a.TargetRangeEdge.Number.Uint64())
	if found {
		oldAList := hAs.Attestations[a.TargetRangeEdge.Hash]
		oldAList = append(oldAList, a)
		hAs.Attestations[a.TargetRangeEdge.Hash] = oldAList
	} else {
		hAs = new(types.HistoryAttestations)
		hAs.Attestations = make(map[common.Hash][]*types.Attestation)
		var newAList []*types.Attestation
		newAList = append(newAList, a)
		hAs.Attestations[a.TargetRangeEdge.Hash] = newAList
	}
	return bc.HistoryAttessCache.Add(a.TargetRangeEdge.Number.Uint64(), hAs)
}

// GetHistoryAttestations Provide access interface for historical data
func (bc *BlockChain) GetHistoryAttestations(num *big.Int, hash common.Hash) ([]*types.Attestation, error) {
	bc.lockHistoryAttessCache.Lock()
	defer bc.lockHistoryAttessCache.Unlock()

	hAs, found := bc.HistoryAttessCache.Get(num.Uint64())
	if found {
		as, found := hAs.Attestations[hash]
		if found {
			return as, nil
		}
	}
	return nil, errors.New("not found")
}

// GetHistoryOneAttestation Gets the certificate specified in the history
func (bc *BlockChain) GetHistoryOneAttestation(num *big.Int, hash common.Hash, aHash common.Hash) (*types.Attestation, error) {
	aList, err := bc.GetHistoryAttestations(num, hash)
	if err != nil {
		return nil, err
	}
	for _, a := range aList {
		if aHash == a.Hash() {
			return a, nil
		}
	}
	return nil, errors.New("not found")
}

// IsFiliation Judge whether there is a parent-child relationship between the two blocks
func (bc *BlockChain) IsFiliation(parent, child *types.RangeEdge) (bool, error) {
	if parent.Number.Uint64() == 0 { // Genesis block is a valid parent of any other block
		return true, nil
	}
	if parent.Number.Uint64() >= child.Number.Uint64() {
		return false, fmt.Errorf("block height difference does not conform to parent-child relationship")
	}
	heightDiff := child.Number.Uint64() - parent.Number.Uint64()
	if heightDiff > diffUpperLimitWarning {
		log.Debug("The block gap is large, which may affect the program performance", "Height difference", heightDiff)
	}
	childBlock := bc.GetBlock(child.Hash, child.Number.Uint64())
	for ; childBlock != nil && childBlock.NumberU64() != parent.Number.Uint64(); childBlock = bc.GetBlock(childBlock.ParentHash(), childBlock.NumberU64()-1) {
	}
	if childBlock == nil {
		return false, fmt.Errorf("invalid child chain")
	}
	return childBlock.Hash() == parent.Hash, nil
}

// IsNeedReorgByCasperFFG According to the casperffg status data, judge whether the branches of the two blocks need to be reorganized
// Check the two branches respectively. If which branch contains a block in the finalized state and the block height is higher,
// which branch will be retained, and then compare the justified state under the same logic. If both branches fail to hit,
// compare the difficulty of the two blocks according to the old logic
func (bc *BlockChain) IsNeedReorgByCasperFFG(oldBlock, newBlock *types.Block) (uint8, error) {
	if has, err := rawdb.IsReadyReadBlockStatus(bc.db); !has || err != nil {
		return types.BasUnknown, nil
	}
	oldLastJustifiedNum := uint64(0)
	newLastJustifiedNum := uint64(0)
	if oldBlock.NumberU64() > newBlock.NumberU64() {
		for ; oldBlock != nil && oldBlock.NumberU64() != newBlock.NumberU64(); oldBlock = bc.GetBlock(oldBlock.ParentHash(), oldBlock.NumberU64()-1) {
			status, hash := bc.GetBlockStatusByNum(oldBlock.Number().Uint64())
			if status != types.BasUnknown && hash == oldBlock.Hash() {
				if status == types.BasFinalized {
					// the old branch already exists with the finalized status flag
					return NoNeedReorg, nil
				} else if status == types.BasJustified && oldLastJustifiedNum == 0 {
					oldLastJustifiedNum = oldBlock.Number().Uint64()
				}
			}
		}
	} else {
		for ; newBlock != nil && newBlock.NumberU64() != oldBlock.NumberU64(); newBlock = bc.GetBlock(newBlock.ParentHash(), newBlock.NumberU64()-1) {
			status, hash := bc.GetBlockStatusByNum(newBlock.Number().Uint64())
			if status != types.BasUnknown && hash == newBlock.Hash() {
				if status == types.BasFinalized {
					return NeedReorg, nil // need to reorg
				} else if status == types.BasJustified && newLastJustifiedNum == 0 {
					newLastJustifiedNum = newBlock.Number().Uint64()
				}
			}
		}
	}
	if oldBlock == nil {
		return NotSure, fmt.Errorf("invalid old chain")
	}
	if newBlock == nil {
		return NotSure, fmt.Errorf("invalid new chain")
	}
	for {
		// If the common ancestor was found, bail out
		if oldBlock.Hash() == newBlock.Hash() {
			if oldLastJustifiedNum < newLastJustifiedNum {
				// The block height of the justified status of the new branch is higher than that of the old branch
				return NeedReorg, nil
			} else if oldLastJustifiedNum > newLastJustifiedNum {
				return NoNeedReorg, nil
			}
			return NotSure, nil // Execute old logic
		}
		status, hash := bc.GetBlockStatusByNum(oldBlock.Number().Uint64())
		if status != types.BasUnknown {
			if hash == oldBlock.Hash() {
				if status == types.BasFinalized {
					// the old branch already exists with the finalized status flag
					return NoNeedReorg, nil
				} else if status == types.BasJustified && oldLastJustifiedNum == 0 {
					oldLastJustifiedNum = oldBlock.Number().Uint64()
				}
			}
			if hash == newBlock.Hash() {
				if status == types.BasFinalized {
					return NeedReorg, nil // need to reorg
				} else if status == types.BasJustified && newLastJustifiedNum == 0 {
					newLastJustifiedNum = newBlock.Number().Uint64()
				}
			}
		}
		// Step back with both chains
		oldBlock = bc.GetBlock(oldBlock.ParentHash(), oldBlock.NumberU64()-1)
		if oldBlock == nil {
			return NotSure, fmt.Errorf("invalid old chain")
		}
		newBlock = bc.GetBlock(newBlock.ParentHash(), newBlock.NumberU64()-1)
		if newBlock == nil {
			return NotSure, fmt.Errorf("invalid new chain")
		}
	}
}

func (bc *BlockChain) MaxValidators() uint8 {
	return bc.TurboEngine.MaxValidators()
}
