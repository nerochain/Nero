package types

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

type Data []byte

// MarshalText encodes b as a hex string with 0x prefix.
func (d Data) MarshalText() ([]byte, error) {
	return hexutil.Bytes(d[:]).MarshalText()
}

type Action struct {
	From         common.Address `gencodec:"required" json:"from"`
	To           common.Address `gencodec:"optional" json:"to,omitempty"`
	Value        *big.Int       `gencodec:"optional" json:"value,omitempty"`
	Success      bool           `gencodec:"required" json:"success"`
	OpCode       string         `gencodec:"required" json:"opcode"`
	Depth        uint64         `gencodec:"required" json:"depth"`
	Gas          uint64         `gencodec:"required" json:"gas"`
	GasUsed      uint64         `gencodec:"required" json:"gas_used"`
	Input        Data           `gencodec:"required" json:"input"`
	Output       Data           `gencodec:"optional" json:"output,omitempty"`
	TraceAddress []uint64       `gencodec:"required" json:"trace_address"`
	Error        string         `gencodec:"optional" json:"error,omitempty"`
}

type ActionConfig struct {
	From     *common.Address
	To       *common.Address
	OpCode   *string
	MinValue *big.Int
}

type ActionFrame struct {
	Action
	Calls []ActionFrame
}

type InternalTx struct {
	TxHash      common.Hash `json:"transactionHash" gencodec:"required"`
	BlockHash   common.Hash `json:"blockHash,omitempty"`
	BlockNumber *big.Int    `json:"blockNumber,omitempty"`
	Actions     []*Action   `json:"logs" gencodec:"required"`
}

type InternalTxForStorage InternalTx

type InternalTxs []*InternalTx
