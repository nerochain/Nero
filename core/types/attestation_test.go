package types

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
)

func TestAttestation_RecoverSigner(t *testing.T) {
	priv, err := crypto.GenerateKey()
	require.NoError(t, err)
	signer := crypto.PubkeyToAddress(priv.PublicKey)
	blockHash := common.BytesToHash([]byte{0xaa, 0xbb, 0xcc, 0x12, 0x34})
	sig, err := crypto.Sign(crypto.Keccak256(AttestationData(&RangeEdge{
		Hash:   blockHash,
		Number: new(big.Int).SetUint64(1),
	}, &RangeEdge{
		Hash:   blockHash,
		Number: new(big.Int).SetUint64(2),
	})), priv)
	require.NoError(t, err)

	a := NewAttestation(&RangeEdge{
		Hash:   blockHash,
		Number: new(big.Int).SetUint64(1),
	}, &RangeEdge{
		Hash:   blockHash,
		Number: new(big.Int).SetUint64(2),
	}, sig)
	recoverSigner, err := a.RecoverSigner()
	require.NoError(t, err)
	require.True(t, signer == recoverSigner)
}

func TestAttestation_RecoverSignerSpecial(t *testing.T) {
	priv, err := crypto.HexToECDSA("55120e7d4a011f2b47d5809deb718f1c58f3e2123ad38a9b779ab2ca4cbd33d4")
	require.NoError(t, err)
	signer := crypto.PubkeyToAddress(priv.PublicKey)
	blockHash := common.HexToHash("0x178d647b5f5c7edb400c26dbbcb7c190b9436e93a0ce9547e401d98874f58414")

	sig, err := crypto.Sign(crypto.Keccak256(AttestationData(&RangeEdge{
		Hash:   common.Hash{},
		Number: big.NewInt(0),
	}, &RangeEdge{
		Hash:   blockHash,
		Number: new(big.Int).SetUint64(8),
	})), priv)

	require.NoError(t, err)

	a := NewAttestation(&RangeEdge{
		Hash:   common.Hash{},
		Number: new(big.Int).SetUint64(0),
	}, &RangeEdge{
		Hash:   blockHash,
		Number: new(big.Int).SetUint64(8),
	}, sig)

	/*
		DEBUG[03-09|11:17:00.007] attestation                              TNum=8
		DEBUG[03-09|11:17:00.007] attestation                              THash=0x178d647b5f5c7edb400c26dbbcb7c190b9436e93a0ce9547e401d98874f58414
		DEBUG[03-09|11:17:00.007] attestation                              SNum=0
		DEBUG[03-09|11:17:00.007] attestation                              SHash=0x0000000000000000000000000000000000000000000000000000000000000000
		DEBUG[03-09|11:17:00.007] attestation                              S=11,924,072,049,745,746,936
		DEBUG[03-09|11:17:00.007] Received a untreated attestation
		ERROR[03-09|11:17:00.007] RecoverSigner error:                     err="invalid transaction v, r, s values"
		ERROR[03-09|11:17:00.008] Message handling failed in `cons`        peer=a99d3a88 err="invalid transaction v, r, s values"
		DEBUG[03-09|11:17:00.007] attestation                              R=4,583,883,475,535,129,780
		DEBUG[03-09|11:17:00.008] attestation                              V=1
	*/
	require.True(t, a.S.Uint64() == 11924072049745746936)
	require.True(t, a.R.Uint64() == 4583883475535129780)
	require.True(t, a.V == 1)

	recoverSigner, err := a.RecoverSigner()
	require.NoError(t, err)
	require.True(t, signer == recoverSigner)
}

func TestAttestation_Hash(t *testing.T) {
	priv, err := crypto.GenerateKey()
	require.NoError(t, err)
	signer := crypto.PubkeyToAddress(priv.PublicKey)
	blockHash := common.BytesToHash([]byte{0xaa, 0xbb, 0xcc, 0x12, 0x34})
	sig, err := crypto.Sign(crypto.Keccak256(AttestationData(&RangeEdge{
		Hash:   blockHash,
		Number: new(big.Int).SetUint64(1),
	}, &RangeEdge{
		Hash:   blockHash,
		Number: new(big.Int).SetUint64(2),
	})), priv)
	require.NoError(t, err)

	a := NewAttestation(&RangeEdge{
		Hash:   blockHash,
		Number: new(big.Int).SetUint64(1),
	}, &RangeEdge{
		Hash:   blockHash,
		Number: new(big.Int).SetUint64(2),
	}, sig)
	aHash := a.Hash()
	recoverSigner, err := a.RecoverSigner()
	require.NoError(t, err)
	require.True(t, signer == recoverSigner)
	require.True(t, aHash == a.Hash())
	require.True(t, aHash == a.DeepCopy().Hash())
	require.True(t, aHash == a.hash.Load().(common.Hash))
	require.True(t, a.TargetRangeEdge != a.DeepCopy().TargetRangeEdge)
	require.True(t, a.TargetRangeEdge.Number.Uint64() == a.DeepCopy().TargetRangeEdge.Number.Uint64())
	require.True(t, a.DeepCopy().SignHash() == a.SignHash())
}
