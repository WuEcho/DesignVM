package ethvm

import (
	"github.com/CaduceusMetaverseProtocol/MetaNebula/core/globaldb"
	types2 "github.com/CaduceusMetaverseProtocol/MetaNebula/types"
	basev1 "github.com/CaduceusMetaverseProtocol/MetaProtocol/gen/proto/go/base/v1"
	"math/big"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	metatypes "github.com/CaduceusMetaverseProtocol/MetaTypes/types"
)

// StateDB is an VM database for full state querying.
type StateDB interface {
	Prepare(thash metatypes.Hash, ti int)

	CreateAccount(addr types2.Account)

	SubBalance(types2.Account, *big.Int)
	AddBalance(types2.Account, *big.Int)
	GetBalance(types2.Account) *big.Int
	SetBalance(addr types2.Account, amount *big.Int)

	GetNonce(types2.Account) uint64
	SetNonce(types2.Account, uint64)

	GetCodeHash(types2.Account) metatypes.Hash
	GetCode(types2.Account) []byte
	SetCode(types2.Account, []byte)
	GetCodeSize(types2.Account) int

	AddRefund(uint64)
	SubRefund(uint64)
	GetRefund() uint64

	GetCommittedState(types2.Account, metatypes.Hash) metatypes.Hash
	GetState(types2.Account, metatypes.Hash) metatypes.Hash
	SetState(types2.Account, metatypes.Hash, metatypes.Hash) error

	Suicide(types2.Account) bool
	HasSuicided(types2.Account) bool

	// Exist reports whether the given account exists in state.
	// Notably this should also return true for suicided accounts.
	Exist(types2.Account) bool
	// Empty returns whether the given account is empty. Empty
	// is defined according to EIP161 (balance = nonce = code = 0).
	Empty(types2.Account) bool

	PrepareAccessList(sender metatypes.Address, dest *metatypes.Address, precompiles []metatypes.Address, txAccesses types.AccessList)
	AddressInAccessList(addr metatypes.Address) bool
	SlotInAccessList(addr metatypes.Address, slot metatypes.Hash) (addressOk bool, slotOk bool)
	// AddAddressToAccessList adds the given address to the access list. This operation is safe to perform
	// even if the feature/fork is not active yet
	AddAddressToAccessList(addr metatypes.Address)
	// AddSlotToAccessList adds the given (address,slot) to the access list. This operation is safe to perform
	// even if the feature/fork is not active yet
	AddSlotToAccessList(addr metatypes.Address, slot metatypes.Hash)

	RevertToSnapshot(int)
	Snapshot() int

	AddLog(*basev1.MetaTxLog)

	//Modify by echo this method should be achieve
	AddLogWithTxHash(log *basev1.MetaTxLog,txHash metatypes.Hash)

	GetLogs(tx metatypes.Hash, block metatypes.Hash) []*basev1.MetaTxLog

	AddPreimage(metatypes.Hash, []byte)

	ForEachStorage(metatypes.Address, func(common.Hash, common.Hash) bool) error

	GetStateObject(account types2.Account) *globaldb.StateObject

	GetDb() globaldb.Database
}

// CallContext provides a basic interface for the VM calling conventions. The VM
// depends on this context being implemented for doing subcalls and initialising new VM contracts.
type CallContext interface {
	// Call another contract
	Call(env *ETHVM, me ContractRef, addr common.Address, data []byte, gas, value *big.Int) ([]byte, error)
	// Take another's contract code and execute within our own context
	CallCode(env *ETHVM, me ContractRef, addr common.Address, data []byte, gas, value *big.Int) ([]byte, error)
	// Same as CallCode except sender and value is propagated from parent to child scope
	DelegateCall(env *ETHVM, me ContractRef, addr common.Address, data []byte, gas *big.Int) ([]byte, error)
	// Create a new contract
	Create(env *ETHVM, me ContractRef, data []byte, gas, value *big.Int) ([]byte, common.Address, error)
}


type Interpreter interface {
	// Run loops and evaluates the contract's code with the given input data and returns
	// the return byte-slice and an error if one occurred.
	Run(contract *Contract, input []byte, static bool) ([]byte, error)
	// CanRun tells if the contract, passed as an argument, can be
	// run by the current interpreter. This is meant so that the
	// caller can do something like:
	//
	// ```golang
	// for _, interpreter := range interpreters {
	//   if interpreter.CanRun(contract.code) {
	//     interpreter.Run(contract.code, input)
	//   }
	// }
	// ```
	CanRun([]byte) bool
}