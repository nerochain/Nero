package types

import (
	"errors"
	"math/big"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
)

// RangeEdge Range edge data structure when submitting proof based on Casper FFG
type RangeEdge struct {
	Hash   common.Hash // Block Hash
	Number *big.Int    // Block Number
}

// Attestation represents a single attestation of a block.
type Attestation struct {
	SourceRangeEdge *RangeEdge
	TargetRangeEdge *RangeEdge
	R               *big.Int
	S               *big.Int
	V               uint8

	// caches
	hash   atomic.Value
	size   atomic.Value
	signer atomic.Value
}

func NewAttestation(source *RangeEdge, target *RangeEdge, sig []byte) *Attestation {
	if len(sig) != crypto.SignatureLength {
		panic("invalid signature length")
	}
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:64])
	// If V is 27/28-form, convert it to 0/1-form for attestation
	v := sig[64]
	if v == 27 || v == 28 {
		v -= 27
	}
	return &Attestation{
		SourceRangeEdge: &RangeEdge{source.Hash, source.Number},
		TargetRangeEdge: &RangeEdge{target.Hash, target.Number},
		R:               r,
		S:               s,
		V:               v,
	}
}

func (a *Attestation) DeepCopy() *Attestation {
	return &Attestation{
		SourceRangeEdge: &RangeEdge{Hash: a.SourceRangeEdge.Hash, Number: new(big.Int).Set(a.SourceRangeEdge.Number)},
		TargetRangeEdge: &RangeEdge{Hash: a.TargetRangeEdge.Hash, Number: new(big.Int).Set(a.TargetRangeEdge.Number)},
		R:               new(big.Int).Set(a.R),
		S:               new(big.Int).Set(a.S),
		V:               a.V,
	}
}

// Hash returns the hash of the attestation
func (a *Attestation) Hash() common.Hash {
	if hash := a.hash.Load(); hash != nil {
		return hash.(common.Hash)
	}

	h := rlpHash(a)
	a.hash.Store(h)
	return h
}

// RecoverSigner recover the signer from the attestation
func (a *Attestation) RecoverSigner() (common.Address, error) {
	if signer := a.signer.Load(); signer != nil {
		return signer.(common.Address), nil
	}

	if err := a.SanityCheck(); err != nil {
		return common.Address{}, err
	}
	signer, err := recoverPlain(AttestationSignHash(a.SourceRangeEdge, a.TargetRangeEdge), a.R, a.S, big.NewInt(int64(a.V+27)), true)
	if err != nil {
		return common.Address{}, err
	}
	a.signer.Store(signer)
	return signer, nil
}

// SanityCheck makes a sanity check of the attestation
func (a *Attestation) SanityCheck() error {
	if a == nil || a.R == nil || a.S == nil || a.V > 1 || a.SourceRangeEdge.Number == nil ||
		(a.SourceRangeEdge.Hash == common.Hash{} && a.SourceRangeEdge.Number.Uint64() != 0) ||
		a.TargetRangeEdge.Number == nil || (a.TargetRangeEdge.Hash == common.Hash{}) ||
		a.SourceRangeEdge.Number.Uint64() >= a.TargetRangeEdge.Number.Uint64() {
		return errors.New("invalid attestation")
	}
	return nil
}

func (a *Attestation) SignHash() common.Hash {
	return AttestationSignHash(a.SourceRangeEdge, a.TargetRangeEdge)
}

// AttestationSignHash builds the sigHash for attestation
func AttestationSignHash(source *RangeEdge, target *RangeEdge) common.Hash {
	return crypto.Keccak256Hash(AttestationData(source, target))
}

func AttestationData(source *RangeEdge, target *RangeEdge) []byte {
	data := make([]byte, 4*common.HashLength)
	copy(data[:], source.Hash.Bytes())
	copy(data[common.HashLength:], target.Hash.Bytes())
	copy(data[common.HashLength*2:], common.BigToHash(source.Number).Bytes())
	copy(data[common.HashLength*3:], common.BigToHash(target.Number).Bytes())
	return data
}

type Signature struct {
	R *big.Int
	S *big.Int
	V uint8

	Signer common.Address // signer, duplicate for reuse with simplicity
}

// BlockNumAttestations represents all (locally collected) attestations on a block height.
type BlockNumAttestations struct {
	AttestsMap map[common.Hash]map[common.Hash]bool // (source+target)Hash -> attestation Hash -> bool
}

type storedBlockAttestations struct {
	BlockHash    common.Hash
	Attestations []*Signature
}

type storedBlockNumAttestations struct {
	BlockNumber *big.Int
	BlockAttes  []*storedBlockAttestations
}

// Block attestation status
const (
	BasUnknown   = uint8(0)
	BasJustified = uint8(1)
	BasFinalized = uint8(2)
)

type BlockStatus struct {
	BlockNumber *big.Int    // Block Number
	Hash        common.Hash // Block Hash
	Status      uint8       // BasJustified/BasFinalized

	cacheHash atomic.Value
}

// CacheHash returns the hash of the blockStatus
func (bs *BlockStatus) CacheHash() common.Hash {
	if hash := bs.cacheHash.Load(); hash != nil {
		return hash.(common.Hash)
	}

	h := rlpHash(bs)
	bs.cacheHash.Store(h)
	return h
}

func (bs *BlockStatus) DeepCopy() *BlockStatus {
	return &BlockStatus{
		BlockNumber: new(big.Int).Set(bs.BlockNumber),
		Hash:        bs.Hash,
		Status:      bs.Status,
	}
}

type BlockStatusList []*BlockStatus

func (b BlockStatusList) Len() int { return len(b) }
func (b BlockStatusList) Less(i, j int) bool {
	return b[i].BlockNumber.Uint64() < b[j].BlockNumber.Uint64()
}
func (b BlockStatusList) Swap(i, j int) { b[i], b[j] = b[j], b[i] }

type AttestationsList []*Attestation

func (p AttestationsList) Len() int { return len(p) }
func (p AttestationsList) Less(i, j int) bool {
	return p[i].TargetRangeEdge.Number.Uint64() < p[j].TargetRangeEdge.Number.Uint64()
}
func (p AttestationsList) Swap(i, j int) { p[i], p[j] = p[j], p[i] }

const (
	PunishNone      = 0
	PunishMultiSig  = 1
	PunishInclusive = 2
)

type EpochCheckBps struct {
	CurrentEpochBps   []common.Address
	LastEpochBps      []common.Address
	CurrentEpochIndex *big.Int
	LastEpochIndex    *big.Int
}

type FutureAttestations struct {
	Attestations map[common.Hash]*Attestation
}

type RequestAttestation struct {
	BlockNumber *big.Int    // Block Number
	Hash        common.Hash // Block Hash
}

type HistoryAttestations struct {
	Attestations map[common.Hash][]*Attestation
}

type ViolateCasperFFGPunish struct {
	PunishType *big.Int
	Before     *Attestation
	After      *Attestation
	BlockNum   *big.Int
	// caches
	hash       atomic.Value
	PunishAddr common.Address
	Plaintiff  common.Address
	Defendant  common.Address
	Data       []byte
}

func (v *ViolateCasperFFGPunish) Hash() common.Hash {
	if hash := v.hash.Load(); hash != nil {
		return hash.(common.Hash)
	}
	data := make([]byte, 3*common.HashLength)
	copy(data[common.HashLength:], v.Before.Hash().Bytes())
	copy(data[:common.HashLength], v.After.Hash().Bytes())
	copy(data[common.HashLength:], common.BigToHash(v.PunishType).Bytes())
	h := crypto.Keccak256Hash(data)
	v.hash.Store(h)
	return h
}

func (v *ViolateCasperFFGPunish) RecoverSigner() (common.Address, error) {
	signerBefore, err := v.Before.RecoverSigner()
	if err != nil {
		return common.Address{}, err
	}
	signerAfter, err := v.After.RecoverSigner()
	if err != nil {
		return common.Address{}, err
	}
	if signerBefore != signerAfter {
		return common.Address{}, errors.New("transaction signature does not match")
	}
	return signerBefore, nil
}

type ViolateCasperFFGPunishList []*ViolateCasperFFGPunish

func (v ViolateCasperFFGPunishList) Len() int { return len(v) }
func (v ViolateCasperFFGPunishList) Less(i, j int) bool {
	return v[i].BlockNum.Uint64() < v[j].BlockNum.Uint64()
}
func (v ViolateCasperFFGPunishList) Swap(i, j int) { v[i], v[j] = v[j], v[i] }

// TargetNum ->Used to determine whether to sign more than one signature
// SourceNum + TargetNum ->Used to determine whether it contains
// TargetNum + TargetHash + AttestationHash ->Used to query the corresponding attestation data from the history cache

type CasperFFGHistory struct {
	TargetNum       *big.Int
	SourceNum       *big.Int
	TargetHash      common.Hash
	AttestationHash common.Hash
}

type CasperFFGHistoryList []*CasperFFGHistory

func (cf CasperFFGHistoryList) Len() int { return len(cf) }
func (cf CasperFFGHistoryList) Less(i, j int) bool {
	return cf[i].TargetNum.Uint64() < cf[j].TargetNum.Uint64()
}
func (cf CasperFFGHistoryList) Swap(i, j int) { cf[i], cf[j] = cf[j], cf[i] }

const (
	AttestationPending = uint8(0)
	AttestationStart   = uint8(1)
	AttestationStop    = uint8(2)
)

type StatusBlockInfo struct {
	Number hexutil.Uint64
	Hash   common.Hash
}
