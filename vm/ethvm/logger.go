package ethvm

import (
	metatypes "github.com/CaduceusMetaverseProtocol/MetaTypes/types"
	"math/big"
	"time"
)

// VMLogger is used to collect execution traces from an VM transaction
// execution. CaptureState is called for each step of the VM with the
// current VM state.
// Note that reference types are actual VM data structures; make copies
// if you need to retain them beyond the current call.
type VMLogger interface {
	CaptureStart(env *ETHVM, from metatypes.Address, to metatypes.Address, create bool, input []byte, gas uint64, value *big.Int)
	CaptureState(pc uint64, op OpCode, gas, cost uint64, scope *ScopeContext, rData []byte, depth int, err error)
	CaptureEnter(typ OpCode, from metatypes.Address, to metatypes.Address, input []byte, gas uint64, value *big.Int)
	CaptureExit(output []byte, gasUsed uint64, err error)
	CaptureFault(pc uint64, op OpCode, gas, cost uint64, scope *ScopeContext, depth int, err error)
	CaptureEnd(output []byte, gasUsed uint64, t time.Duration, err error)
}
