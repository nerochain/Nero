package vm

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/ethereum/go-ethereum/common"
)

type ActionLogger struct {
	callstack []types.ActionFrame
	reason    error // Textual reason for the interruption
}

// NewActionLogger returns a native go tracer which tracks
// call frames of a tx, and implements vm.EVMLogger.
func NewActionLogger() *ActionLogger {
	// First callframe contains tx context info
	// and is populated on start and end.
	return &ActionLogger{callstack: make([]types.ActionFrame, 1)}
}

func (t *ActionLogger) Hooks() *tracing.Hooks {
	return &tracing.Hooks{
		OnEnter: t.OnEnter,
		OnExit:  t.OnExit,
	}
}

func (t *ActionLogger) OnEnter(depth int, typ byte, from common.Address, to common.Address, input []byte, gas uint64, value *big.Int) {
	if depth == 0 && (typ == byte(CALL) || typ == byte(CREATE) || typ == byte(CREATE2)) {
		t.callstack[0] = types.ActionFrame{
			Action: types.Action{
				OpCode:       OpCode(typ).String(),
				From:         from,
				To:           to,
				Value:        value,
				Depth:        ^uint64(0),
				Gas:          gas,
				Input:        input,
				TraceAddress: nil,
			},
			Calls: nil,
		}
	} else {
		// inherit trace address from parent
		parent := t.callstack[depth]
		traceAddr := make([]uint64, len(parent.TraceAddress), len(parent.TraceAddress)+1)
		copy(traceAddr, parent.TraceAddress)

		// get index in its depth
		traceAddr = append(traceAddr, uint64(len(parent.Calls)))

		call := types.ActionFrame{
			Action: types.Action{
				OpCode:       OpCode(typ).String(),
				From:         from,
				To:           to,
				Value:        value,
				Depth:        uint64(depth),
				Gas:          gas,
				Input:        input,
				TraceAddress: traceAddr,
			},
		}
		t.callstack = append(t.callstack, call)
	}
}

func (t *ActionLogger) OnExit(depth int, output []byte, gasUsed uint64, err error, reverted bool) {
	if depth == 0 && (t.callstack[0].OpCode == CREATE.String() || t.callstack[0].OpCode == CREATE2.String()) {
		t.callstack[0].GasUsed = gasUsed
		if err != nil {
			t.callstack[0].Output = output
			t.callstack[0].Error = err.Error()
			if err == ErrExecutionReverted && len(output) > 0 {
				// t.callstack[0].Output = output
				reason, errUnpack := abi.UnpackRevert(output)
				if errUnpack == nil {
					t.callstack[0].Error = fmt.Sprintf("execution reverted: %v", reason)
				}
			}
		} else {
			t.callstack[0].Output = output
			t.callstack[0].Success = true
		}
	} else {
		// current depth
		size := len(t.callstack)
		if size <= 1 {
			return
		}
		// pop call
		call := t.callstack[size-1]
		t.callstack = t.callstack[:size-1]
		size -= 1

		call.GasUsed = gasUsed
		call.Success = err == nil
		if err == nil {
			call.Output = output
		} else {
			call.Output = output
			call.Error = err.Error()
			if call.OpCode == CREATE.String() || call.OpCode == CREATE2.String() {
				call.To = common.Address{}
			}
		}
		t.callstack[size-1].Calls = append(t.callstack[size-1].Calls, call)
	}
}

// GetResult returns the json-encoded nested list of call traces, and any
// error arising from the encoding or forceful termination (via `Stop`).
func (t *ActionLogger) GetResult() ([]*types.Action, error) {
	if len(t.callstack) != 1 {
		return nil, errors.New("incorrect number of top-level calls")
	}

	actions := make([]*types.Action, 0)
	actions = append(actions, &t.callstack[0].Action)
	// DFS
	var addAction func(actionFrame *types.ActionFrame)
	addAction = func(actionFrame *types.ActionFrame) {
		for i := 0; i < len(actionFrame.Calls); i++ {
			actions = append(actions, &actionFrame.Calls[i].Action)
			addAction(&actionFrame.Calls[i])
		}
	}
	addAction(&t.callstack[0])

	return actions, t.reason
}

func (t *ActionLogger) Clear() {
	t.callstack = make([]types.ActionFrame, 1)
}
