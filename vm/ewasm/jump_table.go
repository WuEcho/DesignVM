package ewasm

import (
	"github.com/CaduceusMetaverseProtocol/MetaVM/vm/ethvm"
)

type (
	executionFunc func(pc *uint64, interpreter *EWASMInterpreter, callContext *ethvm.ScopeContext) ([]byte, error)
	gasFunc       func(*ethvm.ETHVM, *Contract, *ethvm.Stack, *ethvm.Memory, uint64) (uint64, error) // last parameter is the requested memory size as a uint64
	// memorySizeFunc returns the required size, and whether the operation overflowed a uint64
	memorySizeFunc func(*ethvm.Stack) (size uint64, overflow bool)
)

type operation struct {
	// execute is the operation function
	execute     executionFunc
	constantGas uint64
	dynamicGas  gasFunc
	// minStack tells how many stack items are required
	minStack int
	// maxStack specifies the max length the stack can have for this operation
	// to not overflow the stack.
	maxStack int

	// memorySize returns the memory size required for the operation
	memorySize memorySizeFunc
}


// JumpTable contains the VM opcodes supported at a given fork.
type JumpTable [256]*operation
