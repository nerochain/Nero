package cons

import (
	"errors"
	"github.com/ethereum/go-ethereum/core/types"
	"math/big"
)

// Constants to match up protocol versions and messages
const (
	cons1 = 1
)

// ProtocolName is the official short name of the `cons` protocol used during
// devp2p capability negotiation.
const ProtocolName = "cons"

// ProtocolVersions are the supported versions of the `cons` protocol (first
// is primary).
var ProtocolVersions = []uint{cons1}

// protocolLengths are the number of implemented message corresponding to
// different protocol versions.
// The length here refers to the code of the message, or the largest type, rather than the length occupied by the data of the message
// Specific view code p2p/peer.go 「msg.Code >= rw.Length」
// If you need to support new types, remember to increase this value
var protocolLengths = map[uint]uint64{cons1: 4}

// maxMessageSize is the maximum cap on the size of a protocol message.
// A single attestation packet is about 110 bytes.
const maxMessageSize = 8 * 1024

const (
	NewAttestationMsg               = 0x00 // A single attestation of a block
	NewJustifiedOrFinalizedBlockMsg = 0x01 // The current node tells other nodes that it has a block with state Justified or Finalized
	GetAttestationsMsg              = 0x02 // Request to get all attestations of a given block
	AttestationsMsg                 = 0x03 // Response of the GetAttestationsMsg
)

var (
	errMsgTooLarge    = errors.New("message too long")
	errDecode         = errors.New("invalid message")
	errInvalidMsgCode = errors.New("invalid message code")
	errBadRequest     = errors.New("bad request")
)

// Packet represents a p2p message in the `cons` protocol.
type Packet interface {
	Name() string // Name returns a string corresponding to the message type.
	Kind() byte   // Kind returns the message type.
}

// NewAttestationPacket represents a packet containing a single attestation of a block.
type NewAttestationPacket struct {
	SourceRangeEdge *types.RangeEdge
	TargetRangeEdge *types.RangeEdge
	R               *big.Int
	S               *big.Int
	V               uint8
}

func (*NewAttestationPacket) Name() string { return "NewAttestation" }
func (*NewAttestationPacket) Kind() byte   { return NewAttestationMsg }
