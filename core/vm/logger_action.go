package vm

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
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
	log.Debug("ActionLogger.OnEnter", "depth", depth, "type", OpCode(typ), "from", from, "to", to)
	
	if depth == 0 && (typ == byte(CALL) || typ == byte(CREATE) || typ == byte(CREATE2)) {
		log.Debug("ActionLogger.OnEnter: Main call", "depth", depth, "type", OpCode(typ))
		t.callstack[0] = types.ActionFrame{
			Action: types.Action{
				OpCode:       OpCode(typ).String(),
				From:         from,
				To:           to,
				Value:        value,
				Depth:        uint64(depth), // Ensure main call depth is always 0
				Gas:          gas,
				Input:        input,
				TraceAddress: nil,
			},
			Calls: nil,
		}
	} else {
		// Ensure callstack is not empty, this is the first layer of safety check
		if len(t.callstack) == 0 {
			log.Warn("ActionLogger.OnEnter: callstack is empty, initializing")
			t.callstack = append(t.callstack, types.ActionFrame{})
		}
		// Calculate parent call depth, but ensure it's not negative
		dep := len(t.callstack) - 1
		log.Debug("ActionLogger.OnEnter: callstack length", "length", len(t.callstack), "calculated_dep", dep)
		// Second layer of safety check: Ensure dep is not negative
		if dep < 0 {
			log.Warn("ActionLogger.OnEnter: negative depth calculated", "dep", dep)
			dep = 0
		}
		parent := t.callstack[dep]
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
				Depth:        uint64(dep), // Use safely calculated depth value
				Gas:          gas,
				Input:        input,
				TraceAddress: traceAddr,
			},
		}
		log.Debug("ActionLogger.OnEnter: Adding nested call", "dep", dep, "depth", depth, "type", OpCode(typ))
		t.callstack = append(t.callstack, call)
	}
}

func (t *ActionLogger) OnExit(depth int, output []byte, gasUsed uint64, err error, reverted bool) {
	log.Debug("ActionLogger.OnExit", "depth", depth, "err", err, "reverted", reverted)
	
	// Ensure callstack is not empty, this is the first layer of safety check
	if len(t.callstack) == 0 {
		log.Warn("ActionLogger.OnExit: callstack is empty, initializing")
		t.callstack = append(t.callstack, types.ActionFrame{})
	}
	
	if depth == 0 {
		log.Debug("ActionLogger.OnExit: Main call exit", "depth", depth)
		// Handle all calls with depth 0 uniformly, including CALL, CREATE and CREATE2
		t.callstack[0].Depth = uint64(depth) // Ensure main call depth is always 0
		t.callstack[0].GasUsed = gasUsed
		if err != nil {
			log.Debug("ActionLogger.OnExit: Main call failed", "err", err)
			t.callstack[0].Output = output
			t.callstack[0].Error = err.Error()
			if err == ErrExecutionReverted && len(output) > 0 {
				// t.callstack[0].Output = output
				reason, errUnpack := abi.UnpackRevert(output)
				if errUnpack == nil {
					log.Debug("ActionLogger.OnExit: Revert reason unpacked", "reason", reason)
					t.callstack[0].Error = fmt.Sprintf("execution reverted: %v", reason)
				}
			}
		} else {
			log.Debug("ActionLogger.OnExit: Main call succeeded")
			t.callstack[0].Output = output
			t.callstack[0].Success = true
		}
		// Handle special cases for CREATE and CREATE2
		if err != nil && (t.callstack[0].OpCode == CREATE.String() || t.callstack[0].OpCode == CREATE2.String()) {
			log.Debug("ActionLogger.OnExit: CREATE/CREATE2 failed")
			t.callstack[0].To = common.Address{}
		}
	} else {
		// current depth
		size := len(t.callstack)
		log.Debug("ActionLogger.OnExit: Nested call exit", "depth", depth, "callstack_size", size)
		// Second layer of safety check: Ensure callstack is long enough
		if size <= 1 {
			log.Warn("ActionLogger.OnExit: callstack too small", "size", size)
			return
		}
		// pop call
		call := t.callstack[size-1]
		// Ensure depth is not negative
		if depth < 0 {
			log.Warn("ActionLogger.OnExit: negative depth received", "depth", depth)
			depth = 0
		}
		call.Depth = uint64(depth) // Ensure correct depth value is set
		log.Debug("ActionLogger.OnExit: Setting call depth", "call_depth", call.Depth)
		
		t.callstack = t.callstack[:size-1]
		size -= 1

		call.GasUsed = gasUsed
		call.Success = err == nil
		if err == nil {
			log.Debug("ActionLogger.OnExit: Nested call succeeded")
			call.Output = output
		} else {
			log.Debug("ActionLogger.OnExit: Nested call failed", "err", err)
			call.Output = output
			call.Error = err.Error()
			if call.OpCode == CREATE.String() || call.OpCode == CREATE2.String() {
				call.To = common.Address{}
			}
		}
		// Third layer of safety check: Ensure parent index is valid
		if size-1 >= 0 && size-1 < len(t.callstack) {
			log.Debug("ActionLogger.OnExit: Adding to parent calls", "parent_index", size-1)
			t.callstack[size-1].Calls = append(t.callstack[size-1].Calls, call)
		} else {
			log.Warn("ActionLogger.OnExit: Invalid parent index", "parent_index", size-1, "callstack_size", len(t.callstack))
		}
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
