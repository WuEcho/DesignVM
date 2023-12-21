package ethvm
/**
TODO 添加说明文件
**/
import (
	"encoding/binary"
	crypto2 "github.com/CaduceusMetaverseProtocol/MetaNebula/common/crypto"
	types2 "github.com/CaduceusMetaverseProtocol/MetaNebula/types"
	basev1 "github.com/CaduceusMetaverseProtocol/MetaProtocol/gen/proto/go/base/v1"
	metatypes "github.com/CaduceusMetaverseProtocol/MetaTypes/types"
	mc "github.com/CaduceusMetaverseProtocol/MetaVM/common"
	"github.com/CaduceusMetaverseProtocol/MetaVM/params"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/holiman/uint256"
	"math/big"
	"sync"
	"sync/atomic"
	"time"
)

var NonceKey = mc.StringToHash("nonce")

var BalanceKey = mc.StringToHash("balance")

var SubBalanceKey = mc.StringToHash("subBalance")

var SetCodeKey = mc.StringToHash("setCode")

var CreatAccountKey = mc.StringToHash("newAccount")

//string to hash
func StringToHash(str string) metatypes.Hash {
	return metatypes.BytesToHash(crypto.Keccak256([]byte(str))[12:])
}

func Uint64ToByte(n uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, n)
	return b
}

func ByteToUint64(b []byte) uint64 {
	return binary.BigEndian.Uint64(b)
}

// emptyCodeHash is used by create to ensure deployment is disallowed to already
// deployed contract addresses (relevant after the account abstraction).
var emptyCodeHash = crypto2.Keccak256Hash(nil)

type (
	// CanTransferFunc is the signature of a transfer guard function
	CanTransferFunc func(StateDB, metatypes.Address, *big.Int) bool
	// TransferFunc is the signature of a transfer function
	TransferFunc func(StateDB, metatypes.Address, metatypes.Address, *big.Int)
	// GetHashFunc returns the n'th block hash in the blockchain
	// and is used by the BLOCKHASH VM op code.
	GetHashFunc func(uint64) metatypes.Hash
)

func (vm *ETHVM) precompile(addr metatypes.Address) (PrecompiledContract, bool) {
	var precompiles map[metatypes.Address]PrecompiledContract
	switch {
	default:
		precompiles = PrecompiledContractsBLS
	}
	p, ok := precompiles[addr]
	return p, ok
}

//Context provides the
type Context struct {
	BlockContext
	TxContext
}

// BlockContext provides the VM with auxiliary information. Once provided
// it shouldn't be modified.
type BlockContext struct {
	// CanTransfer returns whether the account contains
	// sufficient ether to transfer the value
	CanTransfer CanTransferFunc
	// Transfer transfers ether from one account to the other
	Transfer TransferFunc
	// GetHash returns the hash corresponding to n
	GetHash GetHashFunc

	// Block information
	Coinbase    metatypes.Address // Provides information for COINBASE
	GasLimit    uint64         // Provides information for GASLIMIT
	BlockNumber *big.Int       // Provides information for NUMBER
	Time        *big.Int       // Provides information for TIME
	Difficulty  *big.Int       // Provides information for DIFFICULTY
	BaseFee     *big.Int       // Provides information for BASEFEE
	Random      *metatypes.Hash   // Provides information for RANDOM
}

// TxContext provides the VM with information about a transaction.
// All fields can change between transactions.
type TxContext struct {
	// Message information
	Origin   metatypes.Address // Provides information for ORIGIN
	GasPrice *big.Int       // Provides information for GASPRICE
	//TODO added by echo
	ReadSet  *ReadSet  //读集
	WriteSet *WriteSet //写集
	Logs     []*basev1.MetaTxLog
	TxData   []byte  //
}

func (tc *TxContext) Copy() TxContext {
	return TxContext{
		Origin:   tc.Origin,
		GasPrice: tc.GasPrice,
		ReadSet:  NewReadSet(),
		WriteSet: NewWriteSet(),
		Logs:     make([]*basev1.MetaTxLog, 0),
	}
}

type ReadSet struct {
	cache map[metatypes.Address]map[metatypes.Hash]interface{}
	detailKeyCount uint32 //leve 2 map key count
	addressCount   uint32
	lock  *sync.RWMutex
}

func NewReadSet() *ReadSet {
	return &ReadSet{
		cache: make(map[metatypes.Address]map[metatypes.Hash]interface{},3),
		detailKeyCount: 0,
		addressCount: 0,
		lock:  new(sync.RWMutex),
	}
}

func (r *ReadSet) Set(addr metatypes.Address, cache map[metatypes.Hash]interface{}) {
	r.lock.Lock()
	defer r.lock.Unlock()
	r.cache[addr] = cache
}

func (r *ReadSet) Get(addr metatypes.Address) (map[metatypes.Hash]interface{}, bool) {
	r.lock.RLock()
	defer r.lock.RUnlock()
	cache, ok := r.cache[addr]
	return cache, ok
}

func (r *ReadSet) SetCacheWithKV(addr metatypes.Address, k metatypes.Hash, v interface{}) {
	r.lock.Lock()
	defer r.lock.Unlock()
	cacheMap, ok := r.cache[addr]
	if !ok {
		cacheMap = make(map[metatypes.Hash]interface{},1)
	}
	cacheMap[k] = v
	r.cache[addr] = cacheMap
}

func (r *ReadSet) UpdateCache(addr metatypes.Address, k metatypes.Hash, v interface{}) {
	r.lock.Lock()
	defer r.lock.Unlock()
	cache, ok := r.cache[addr]
	if !ok {
		cache = make(map[metatypes.Hash]interface{},5)
		r.addressCount++
	}
	switch k {
	case SubBalanceKey:
		subVal, ok := cache[SubBalanceKey]
		if ok {
			subVal = new(big.Int).Add(subVal.(*big.Int), v.(*big.Int))
		} else {
			subVal = v
			r.detailKeyCount++
		}
		cache[SubBalanceKey] = subVal
	case BalanceKey:
		val, ok := cache[BalanceKey]
		if ok {
			val = new(big.Int).Add(val.(*big.Int), v.(*big.Int))
			cache[k] = val
		} else {
			cache[k] = v
			r.detailKeyCount++
		}
	default:
		_,ok := cache[k]
		if !ok {
			r.detailKeyCount++
		}
		cache[k] = v
	}

	r.cache[addr] = cache
}

func (r *ReadSet) GetDetailKeyCount() uint32 {
	return r.detailKeyCount
}

func (r *ReadSet) Cache() map[metatypes.Address]map[metatypes.Hash]interface{} {
	return r.cache
}

func (r *ReadSet) Copy() *ReadSet {
	r.lock.Lock()
	defer r.lock.Unlock()
	cache := make(map[metatypes.Address]map[metatypes.Hash]interface{},len(r.cache))
	valueCache := make(map[metatypes.Hash]interface{})
	for addr, cach := range r.cache {
		for k, v := range cach {
			valueCache[k] = v
		}
		cache[addr] = valueCache
	}
	return &ReadSet{
		cache: cache,
		lock:  new(sync.RWMutex),
	}
}

func (r *ReadSet) Len() uint32 {
	return r.addressCount
}

type WriteSet struct {
	cache map[metatypes.Address]map[metatypes.Hash]interface{}
	detailKeyCount uint32
	addressCount   uint32
	lock  *sync.RWMutex
}

func NewWriteSet() *WriteSet {
	return &WriteSet{
		cache: make(map[metatypes.Address]map[metatypes.Hash]interface{},3),
		detailKeyCount: 0,
		addressCount: 0,
		lock:  new(sync.RWMutex),
	}
}

func (w *WriteSet) Set(addr metatypes.Address, cacheMap map[metatypes.Hash]interface{}) {
	w.lock.Lock()
	defer w.lock.Unlock()
	w.cache[addr] = cacheMap
}

func (w *WriteSet) Get(addr metatypes.Address) (map[metatypes.Hash]interface{}, bool) {
	w.lock.RLock()
	defer w.lock.RUnlock()
	cacheMap, ok := w.cache[addr]
	return cacheMap, ok
}

func (w *WriteSet) GetAddressAndKey(addr metatypes.Address, k metatypes.Hash) (interface{}, bool) {
	w.lock.RLock()
	defer w.lock.RUnlock()
	cache, ok := w.cache[addr]
	if ok {
		val, ok := cache[k]
		if ok {
			return val, true
		} else {
			return []byte{}, false
		}
	} else {
		return []byte{}, false
	}
}

func (w *WriteSet) Copy() *WriteSet {
	w.lock.Lock()
	defer w.lock.Unlock()
	cacheMap := make(map[metatypes.Address]map[metatypes.Hash]interface{},len(w.cache))
	valCache := make(map[metatypes.Hash]interface{})
	for addr, cach := range w.cache {
		for k, v := range cach {
			valCache[k] = v
		}
		cacheMap[addr] = valCache
	}
	return &WriteSet{
		cache: cacheMap,
		lock:  new(sync.RWMutex),
	}
}

func (w *WriteSet) SetCacheWithKV(addr metatypes.Address, k metatypes.Hash, v interface{}) {
	w.lock.Lock()
	defer w.lock.Unlock()
	cacheMap, ok := w.cache[addr]
	if !ok {
		cacheMap = make(map[metatypes.Hash]interface{},5)
	}
	cacheMap[k] = v
	w.cache[addr] = cacheMap
}

func (w *WriteSet) UpdateCache(addr metatypes.Address, k metatypes.Hash, v interface{}, value interface{}) {
	w.lock.Lock()
	defer w.lock.Unlock()

	cacheMap, ok := w.cache[addr]
	if !ok {
		cacheMap = make(map[metatypes.Hash]interface{},5)
		w.addressCount++
	}
	switch k {
	case SubBalanceKey:
		subVal, ok := cacheMap[SubBalanceKey]
		if ok {
			subVal = new(big.Int).Add(subVal.(*big.Int), v.(*big.Int))
		} else {
			subVal = v
			w.detailKeyCount++
		}
		cacheMap[SubBalanceKey] = subVal

	case BalanceKey:
		val, ok := cacheMap[BalanceKey]
		if ok {
			val = new(big.Int).Add(val.(*big.Int), v.(*big.Int))
			cacheMap[k] = val
		} else {
			val = new(big.Int).Add(value.(*big.Int), v.(*big.Int))
			cacheMap[k] = val
			w.detailKeyCount++
		}
	default:
		_,ok := cacheMap[k]
		if !ok {
			w.detailKeyCount++
		}
		cacheMap[k] = v
	}
	w.cache[addr] = cacheMap
}

func (w *WriteSet) Cache() map[metatypes.Address]map[metatypes.Hash]interface{} {
	return w.cache
}

func (w *WriteSet) Len() uint32 {
	return w.addressCount
}

func (w *WriteSet) GetDetailKeyCount() uint32{
	return w.detailKeyCount
}

func RangeWriteCache(writeCache *WriteSet, f func(addr metatypes.Address, cache map[metatypes.Hash]interface{})) {
	writeCache.lock.RLock()
	defer writeCache.lock.RUnlock()
	for addr, cache := range writeCache.Cache() {
		f(addr, cache)
	}
}

func RangeReadCache(readCache *ReadSet, f func(addr metatypes.Address, cache map[metatypes.Hash]interface{})) {
	readCache.lock.RLock()
	defer readCache.lock.RUnlock()
	for addr, cache := range readCache.Cache() {
		f(addr, cache)
	}
}

// VM is the Ethereum Virtual Machine base object and provides
// the necessary tools to run a contract on the given state with
// the provided context. It should be noted that any error
// generated through any of the calls should be considered a
// revert-state-and-consume-all-gas operation, no checks on
// specific errors should ever be performed. The interpreter makes
// sure that any errors generated are to be considered faulty code.
//
// The VM should never be reused and is not thread safe.
type ETHVM struct {
	// Context provides auxiliary blockchain related information
	Context BlockContext
	TxContext
	// StateDB gives access to the underlying state
	StateDB StateDB
	// Depth is the current call stack
	depth int

	// chainConfig contains information about the current chain
	chainConfig *params.ChainConfig
	// chain rules contains the chain rules for the current epoch
	chainRules params.Rules
	// virtual machine configuration options used to initialise the
	// vm.
	Config Config
	// global (to this context) ethereum virtual machine
	// used throughout the execution of the tx.
	interpreter Interpreter
	// abort is used to abort the VM calling operations
	// NOTE: must be set atomically
	abort int32
	// callGasTemp holds the gas available for the current call. This is needed because the
	// available gas is calculated in gasCall* according to the 63/64 rule and later
	// applied in opCall*.
	callGasTemp uint64
}

// NewVM returns a new VM. The returned VM is not thread safe and should
// only ever be used *once*.
func NewVM(blockCtx BlockContext, txCtx TxContext, statedb StateDB, chainConfig *params.ChainConfig, config Config) *ETHVM {
	vm := &ETHVM{
		Context:     blockCtx,
		TxContext:   txCtx,
		StateDB:     statedb,
		Config:      config,
		chainConfig: chainConfig,
		//chainRules:  chainConfig.Rules(blockCtx.BlockNumber, blockCtx.Random != nil),
		chainRules: chainConfig.Rules(blockCtx.BlockNumber, false),
	}
	if len(txCtx.TxData) > 0 {
		if IsWASM(txCtx.TxData) {
			vm.interpreter = NewEWASMInterpreter(vm,config)
		}else {
			vm.interpreter = NewVMInterpreter(vm, config)
		}
	}
	return vm
}

//拿一个副本
func (vm *ETHVM) Copy() *ETHVM {
	newVm := &ETHVM{
		Context:     vm.Context,
		TxContext:   vm.TxContext.Copy(),
		StateDB:     vm.StateDB,
		Config:      vm.Config,
		chainConfig: vm.chainConfig,
		//chainRules:  chainConfig.Rules(blockCtx.BlockNumber, blockCtx.Random != nil),
		chainRules: vm.chainRules,
	}
	if len(vm.TxContext.TxData) > 0 {
		if IsWASM(vm.TxContext.TxData) {
			newVm.interpreter = NewEWASMInterpreter(newVm,vm.Config)
		}else {
			newVm.interpreter = NewVMInterpreter(newVm, vm.Config)
		}
	}
	return newVm
}

func (vm *ETHVM) IncreaseDepeth() {
	vm.depth++
}

func (vm *ETHVM) ReductionDepth() {
	vm.depth--
}

func (vm *ETHVM) GetDepeth() int {
	return vm.depth
}

func (vm *ETHVM) About() int32 {
	return vm.abort
}

func (vm *ETHVM) GetChainRules() params.Rules {
	return vm.chainRules
}

func (vm *ETHVM) GetCallGasTemp() uint64 {
	return vm.callGasTemp
}

func (vm *ETHVM) GetChainConfig() *params.ChainConfig {
	return vm.chainConfig
}

func (vm *ETHVM) SetCallGasTemp(v uint64) {
	vm.callGasTemp = v
}

// Reset resets the VM with a new transaction context.Reset
// This is not threadsafe and should only be done very cautiously.
func (vm *ETHVM) Reset(txCtx TxContext, statedb StateDB) {
	vm.TxContext = txCtx
	vm.StateDB = statedb
}

// Cancel cancels any running VM operation. This may be called concurrently and
// it's safe to be called multiple times.
func (vm *ETHVM) Cancel() {
	atomic.StoreInt32(&vm.abort, 1)
}

// Cancelled returns true if Cancel has been called
func (vm *ETHVM) Cancelled() bool {
	return atomic.LoadInt32(&vm.abort) == 1
}

// Interpreter returns the current interpreter
func (vm *ETHVM) Interpreter() Interpreter {
	return vm.interpreter
}

// Call executes the contract associated with the addr with the given input as
// parameters. It also handles any necessary value transfer required and takes
// the necessary steps to create accounts and reverses the state in case of an
// execution error or failed value transfer.
func (vm *ETHVM) Call(caller ContractRef, addr metatypes.Address, input []byte, gas uint64, value *big.Int) (ret []byte, leftOverGas uint64, err error) {
	// Fail if we're trying to execute above the call depth limit
	if vm.depth > int(params.CallCreateDepth) {
		return nil, gas, ErrDepth
	}
	// Fail if we're trying to transfer more than the available balance
	var canTransferFlag bool
	dataVal, ok := vm.WriteSet.GetAddressAndKey(caller.Address(), BalanceKey)
	if ok {
		if dataVal.(*big.Int).Uint64() >= value.Uint64() {
			canTransferFlag = true
		}
	} else {
		canTransferFlag = vm.Context.CanTransfer(vm.StateDB, caller.Address(), value)
	}
	if value.Sign() != 0 && !canTransferFlag {
		return nil, gas, ErrInsufficientBalance
	}
	vm.ReadSet.UpdateCache(caller.Address(), BalanceKey, value)

	snapshot := vm.StateDB.Snapshot()
	p, isPrecompile := vm.precompile(addr)

	acc := types2.Account{
		addr,
	}
	if !vm.StateDB.Exist(acc) {
		if !isPrecompile && value.Sign() == 0 {
			// Calling a non existing account, don't do anything, but ping the tracer
			if vm.Config.Debug {
				if vm.depth == 0 {
					vm.Config.Tracer.CaptureStart(vm, caller.Address(), addr, false, input, gas, value)
					vm.Config.Tracer.CaptureEnd(ret, 0, 0, nil)
				}
			}
			return nil, gas, nil
		}
		vm.WriteSet.UpdateCache(addr, CreatAccountKey, addr.Bytes(), []byte{})
	}
	//vm.Context.Transfer(vm.StateDB, caller.Address(), addr, value)
	//modify by echo
	if value.Uint64() > 0 {
		var balanceVal = new(big.Int)
		dataVal, ok := vm.WriteSet.GetAddressAndKey(addr, BalanceKey)
		if !ok {
			balanceVal = vm.StateDB.GetBalance(acc)
		} else {
			balanceVal = dataVal.(*big.Int)
		}
		vm.ReadSet.UpdateCache(addr, BalanceKey, balanceVal)
		vm.WriteSet.UpdateCache(caller.Address(), SubBalanceKey, value, []byte{})
		vm.WriteSet.UpdateCache(addr, BalanceKey, value, balanceVal)
	}

	// Capture the tracer start/end events in debug mode
	if vm.Config.Debug {
		if vm.depth == 0 {
			vm.Config.Tracer.CaptureStart(vm, caller.Address(), addr, false, input, gas, value)
			defer func(startGas uint64, startTime time.Time) { // Lazy evaluation of the parameters
				vm.Config.Tracer.CaptureEnd(ret, startGas-gas, time.Since(startTime), err)
			}(gas, time.Now())
		}
	}

	//modify by echo
	addrCopy := acc
	// If the account has no code, we can abort here
	// The depth-check is already done, and precompiles handled above
	contract := NewContract(caller, AccountRef(addrCopy.Address), value, gas)
	var codeHash metatypes.Hash
	code, ok := vm.WriteSet.GetAddressAndKey(addr, SetCodeKey)
	if !ok {
		code = vm.StateDB.GetCode(addrCopy)
		codeHash = vm.StateDB.GetCodeHash(addrCopy)
	} else {
		codeHash = crypto2.Keccak256Hash(code.([]byte))
	}
	contract.SetCallCode(&addrCopy.Address, codeHash, code.([]byte))

	if isPrecompile {
		//ret, gas, err = RunPrecompiledContract(p, input, gas)
		ret, gas, err = RunRrecompiledContractPre(vm, p, input, contract, gas)
	} else {
		// Initialise a new contract and set the code that is to be used by the VM.
		// The contract is a scoped environment for this execution context only.
		if len(code.([]byte)) == 0 {
			ret, err = nil, nil // gas is unchanged
		} else {
			ret, err = vm.interpreter.Run(contract, input, false)
			gas = contract.Gas
		}
	}
	// When an error was returned by the VM or when setting the creation code
	// above we revert to the snapshot and consume any gas remaining. Additionally
	// when we're in homestead this also counts for code storage gas errors.
	if err != nil {
		vm.StateDB.RevertToSnapshot(snapshot)
		if err != ErrExecutionReverted {
			gas = 0
		}
	}

	return ret, gas, err
}

// CallCode executes the contract associated with the addr with the given input
// as parameters. It also handles any necessary value transfer required and takes
// the necessary steps to create accounts and reverses the state in case of an
// execution error or failed value transfer.
//
// CallCode differs from Call in the sense that it executes the given address'
// code with the caller as context.
func (vm *ETHVM) CallCode(caller ContractRef, addr metatypes.Address, input []byte, gas uint64, value *big.Int) (ret []byte, leftOverGas uint64, err error) {
	// Fail if we're trying to execute above the call depth limit
	if vm.depth > int(params.CallCreateDepth) {
		return nil, gas, ErrDepth
	}
	// Fail if we're trying to transfer more than the available balance
	// Note although it's noop to transfer X ether to caller itself. But
	// if caller doesn't have enough balance, it would be an error to allow
	// over-charging itself. So the check here is necessary.

	var canTransferFlag bool
	dataVal, ok := vm.WriteSet.GetAddressAndKey(caller.Address(), BalanceKey)
	if ok {
		if dataVal.(*big.Int).Uint64() >= value.Uint64() {
			canTransferFlag = true
		}
	} else {
		canTransferFlag = vm.Context.CanTransfer(vm.StateDB, caller.Address(), value)
	}

	if !canTransferFlag {
		return nil, gas, ErrInsufficientBalance
	}

	var snapshot = vm.StateDB.Snapshot()

	// Invoke tracer hooks that signal entering/exiting a call frame
	//if vm.Config.Debug {
	//	vm.Config.Tracer.CaptureEnter(CALLCODE, caller.Address(), addr, input, gas, value)
	//	defer func(startGas uint64) {
	//		vm.Config.Tracer.CaptureExit(ret, startGas-gas, err)
	//	}(gas)
	//}

	acc := types2.Account{
		addr,
	}
	//addrCopy := addr
	// Initialise a new contract and set the code that is to be used by the VM.
	// The contract is a scoped environment for this execution context only.
	contract := NewContract(caller, AccountRef(caller.Address()), value, gas)
	var codeHash metatypes.Hash
	code, ok := vm.WriteSet.GetAddressAndKey(addr, SetCodeKey)
	if !ok {
		code = vm.StateDB.GetCode(acc)
		codeHash = vm.StateDB.GetCodeHash(acc)
	} else {
		codeHash = crypto2.Keccak256Hash(code.([]byte))
	}
	contract.SetCallCode(&acc.Address, codeHash, code.([]byte))
	// It is allowed to call precompiles, even via delegatecall
	if p, isPrecompile := vm.precompile(addr); isPrecompile {
		//ret, gas, err = RunPrecompiledContract(p, input, gas)
		ret, gas, err = RunRrecompiledContractPre(vm, p, input, contract, gas)
	} else {
		//addrCopy := addr
		//// Initialise a new contract and set the code that is to be used by the VM.
		//// The contract is a scoped environment for this execution context only.
		//contract := NewContract(caller, AccountRef(caller.Address()), value, gas)
		//contract.SetCallCode(&addrCopy, vm.StateDB.GetCodeHash(addrCopy), vm.StateDB.GetCode(addrCopy))
		ret, err = vm.interpreter.Run(contract, input, false)
		gas = contract.Gas
	}
	if err != nil {
		vm.StateDB.RevertToSnapshot(snapshot)
		if err != ErrExecutionReverted {
			gas = 0
		}
	}

	return ret, gas, err
}

// DelegateCall executes the contract associated with the addr with the given input
// as parameters. It reverses the state in case of an execution error.
//
// DelegateCall differs from CallCode in the sense that it executes the given address'
// code with the caller as context and the caller is set to the caller of the caller.
func (vm *ETHVM) DelegateCall(caller ContractRef, addr metatypes.Address, input []byte, gas uint64) (ret []byte, leftOverGas uint64, err error) {
	// Fail if we're trying to execute above the call depth limit
	if vm.depth > int(params.CallCreateDepth) {
		return nil, gas, ErrDepth
	}
	var snapshot = vm.StateDB.Snapshot()

	//// Invoke tracer hooks that signal entering/exiting a call frame
	//if vm.Config.Debug {
	//	vm.Config.Tracer.CaptureEnter(DELEGATECALL, caller.Address(), addr, input, gas, nil)
	//	defer func(startGas uint64) {
	//		vm.Config.Tracer.CaptureExit(ret, startGas-gas, err)
	//	}(gas)
	//}

	acc := types2.Account{
		addr,
	}
	//modify by echo
	//addrCopy := addr
	//// Initialise a new contract and make initialise the delegate values
	contract := NewContract(caller, AccountRef(caller.Address()), nil, gas).AsDelegate()
	var codeHash metatypes.Hash
	code, ok := vm.WriteSet.GetAddressAndKey(acc.Address, SetCodeKey)
	if !ok {
		code = vm.StateDB.GetCode(acc)
		codeHash = vm.StateDB.GetCodeHash(acc)
	} else {
		codeHash = crypto2.Keccak256Hash(code.([]byte))
	}
	contract.SetCallCode(&acc.Address, codeHash, code.([]byte))

	// It is allowed to call precompiles, even via delegatecall
	if p, isPrecompile := vm.precompile(addr); isPrecompile {
		//ret, gas, err = RunPrecompiledContract(p, input, gas)
		ret, gas, err = RunRrecompiledContractPre(vm, p, input, contract, gas)
	} else {
		//addrCopy := addr
		//// Initialise a new contract and make initialise the delegate values
		//contract := NewContract(caller, AccountRef(caller.Address()), nil, gas).AsDelegate()
		//contract.SetCallCode(&addrCopy, vm.StateDB.GetCodeHash(addrCopy), vm.StateDB.GetCode(addrCopy))
		ret, err = vm.interpreter.Run(contract, input, false)
		gas = contract.Gas
	}
	if err != nil {
		vm.StateDB.RevertToSnapshot(snapshot)
		if err != ErrExecutionReverted {
			gas = 0
		}
	}
	return ret, gas, err
}

// StaticCall executes the contract associated with the addr with the given input
// as parameters while disallowing any modifications to the state during the call.
// Opcodes that attempt to perform such modifications will result in exceptions
// instead of performing the modifications.
func (vm *ETHVM) StaticCall(caller ContractRef, addr metatypes.Address, input []byte, gas uint64) (ret []byte, leftOverGas uint64, err error) {
	// Fail if we're trying to execute above the call depth limit
	if vm.depth > int(params.CallCreateDepth) {
		return nil, gas, ErrDepth
	}
	// We take a snapshot here. This is a bit counter-intuitive, and could probably be skipped.
	// However, even a staticcall is considered a 'touch'. On mainnet, static calls were introduced
	// after all empty accounts were deleted, so this is not required. However, if we omit this,
	// then certain tests start failing; stRevertTest/RevertPrecompiledTouchExactOOG.json.
	// We could change this, but for now it's left for legacy reasons
	var snapshot = vm.StateDB.Snapshot()

	// We do an AddBalance of zero here, just in order to trigger a touch.
	// This doesn't matter on Mainnet, where all empties are gone at the time of Byzantium,
	// but is the correct thing to do and matters on other networks, in tests, and potential
	// future scenarios
	acc := types2.Account{
		addr,
	}

	vm.StateDB.AddBalance(acc, big0)

	// Invoke tracer hooks that signal entering/exiting a call frame
	if vm.Config.Debug {
		vm.Config.Tracer.CaptureEnter(STATICCALL, caller.Address(), addr, input, gas, nil)
		defer func(startGas uint64) {
			vm.Config.Tracer.CaptureExit(ret, startGas-gas, err)
		}(gas)
	}

	//modify by echo
	addrCopy := addr
	// Initialise a new contract and set the code that is to be used by the VM.
	// The contract is a scoped environment for this execution context only.
	contract := NewContract(caller, AccountRef(addrCopy), new(big.Int), gas)
	contract.SetCallCode(&addrCopy, vm.StateDB.GetCodeHash(acc), vm.StateDB.GetCode(acc))

	if p, isPrecompile := vm.precompile(addr); isPrecompile {
		//ret, gas, err = RunPrecompiledContract(p, input, gas)
		ret, gas, err = RunRrecompiledContractPre(vm, p, input, contract, gas)
	} else {
		// At this point, we use a copy of address. If we don't, the go compiler will
		// leak the 'contract' to the outer scope, and make allocation for 'contract'
		// even if the actual execution ends on RunPrecompiled above.
		//addrCopy := addr
		// Initialise a new contract and set the code that is to be used by the VM.
		// The contract is a scoped environment for this execution context only.
		//contract := NewContract(caller, AccountRef(addrCopy), new(big.Int), gas)
		//contract.SetCallCode(&addrCopy, vm.StateDB.GetCodeHash(addrCopy), vm.StateDB.GetCode(addrCopy))
		// When an error was returned by the VM or when setting the creation code
		// above we revert to the snapshot and consume any gas remaining. Additionally
		// when we're in Homestead this also counts for code storage gas errors.
		ret, err = vm.interpreter.Run(contract, input, true)
		gas = contract.Gas
	}
	if err != nil {
		vm.StateDB.RevertToSnapshot(snapshot)
		if err != ErrExecutionReverted {
			gas = 0
		}
	}
	return ret, gas, err
}

type codeAndHash struct {
	code []byte
	hash metatypes.Hash
}

func (c *codeAndHash) Hash() metatypes.Hash {
	if c.hash == (metatypes.Hash{}) {
		c.hash =  crypto2.Keccak256Hash(c.code)
	}
	return c.hash
}

// create creates a new contract using code as deployment code.
func (vm *ETHVM) create(caller ContractRef, codeAndHash *codeAndHash, gas uint64, value *big.Int, address metatypes.Address, typ OpCode) ([]byte, metatypes.Address, uint64, error) {
	// Depth check execution. Fail if we're trying to execute above the
	// limit.
	if vm.depth > int(params.CallCreateDepth) {
		return nil, metatypes.Address{}, gas, ErrDepth
	}
	//modify by echo
	var canTranserFlag bool
	val, ok := vm.WriteSet.GetAddressAndKey(caller.Address(), BalanceKey)
	if ok {
		if val.(*big.Int).Uint64() >= value.Uint64() {
			canTranserFlag = true
		}
	} else {
		canTranserFlag = vm.Context.CanTransfer(vm.StateDB, caller.Address(), value)
	}
	if !canTranserFlag {
		return nil, metatypes.Address{}, gas, ErrInsufficientBalance
	}

	var nonce uint64
	data, ok := vm.WriteSet.GetAddressAndKey(caller.Address(), NonceKey)
	if !ok {
		acc := types2.Account{
			caller.Address(),
		}
		nonce = vm.StateDB.GetNonce(acc)
	} else {
		nonce = data.(uint64)
	}
	//modify by echo
	vm.ReadSet.UpdateCache(caller.Address(), NonceKey, nonce)

	if nonce+1 < nonce {
		return nil, metatypes.Address{}, gas, ErrNonceUintOverflow
	}
	//modify by echo
	//vm.StateDB.SetNonce(caller.Address(), nonce+1)
	vm.WriteSet.UpdateCache(caller.Address(), NonceKey, nonce+1, []byte{})
	// We add this to the access list _before_ taking a snapshot. Even if the creation fails,
	// the access-list change should not be rolled back
	vm.StateDB.AddAddressToAccessList(address)

	acc := types2.Account{
		address,
	}
	// Ensure there's no existing contract already at the designated address
	contractHash := vm.StateDB.GetCodeHash(acc)
	if vm.StateDB.GetNonce(acc) != 0 || (contractHash != (metatypes.Hash{}) && contractHash != emptyCodeHash) {
		return nil, metatypes.Address{}, 0, ErrContractAddressCollision
	}
	// Create a new account on the state
	snapshot := vm.StateDB.Snapshot()
	//vm.StateDB.CreateAccount(address)
	//modify by echo
	vm.WriteSet.UpdateCache(address, CreatAccountKey, address.Bytes(), []byte{})
	//vm.Context.Transfer(vm.StateDB, caller.Address(), address, value)
	//modify by echo
	if value.Uint64() > 0 {
		var balanceVal = new(big.Int)
		dataVal, ok := vm.WriteSet.GetAddressAndKey(address, BalanceKey)
		if !ok {
			balanceVal = vm.StateDB.GetBalance(acc)
		} else {
			balanceVal = dataVal.(*big.Int)
		}
		vm.WriteSet.UpdateCache(caller.Address(), SubBalanceKey, value, []byte{})
		vm.WriteSet.UpdateCache(address, BalanceKey, value, balanceVal)
	}

	// Initialise a new contract and set the code that is to be used by the VM.
	// The contract is a scoped environment for this execution context only.
	contract := NewContract(caller, AccountRef(address), value, gas)
	contract.SetCodeOptionalHash(&address, codeAndHash)

	if vm.Config.Debug {
		if vm.depth == 0 {
			vm.Config.Tracer.CaptureStart(vm, caller.Address(), address, true, codeAndHash.code, gas, value)
		} else {
			vm.Config.Tracer.CaptureEnter(typ, caller.Address(), address, codeAndHash.code, gas, value)
		}
	}

	start := time.Now()

	ret, err := vm.interpreter.Run(contract, nil, false)

	// Check whether the max code size has been exceeded, assign err if the case.
	if err == nil && len(ret) > params.MaxCodeSize {
		err = ErrMaxCodeSizeExceeded
	}

	// Reject code starting with 0xEF if EIP-3541 is enabled.
	if err == nil && len(ret) >= 1 && ret[0] == 0xEF {
		err = ErrInvalidCode
	}

	// if the contract creation ran successfully and no errors were returned
	// calculate the gas required to store the code. If the code could not
	// be stored due to not enough gas set an error and let it be handled
	// by the error checking condition below.
	if err == nil {
		createDataGas := uint64(len(ret)) * params.CreateDataGas
		if contract.UseGas(createDataGas) {
			//vm.StateDB.SetCode(address, ret)
			vm.WriteSet.UpdateCache(address, SetCodeKey, ret, []byte{})
		} else {
			err = ErrCodeStoreOutOfGas
		}
	}

	// When an error was returned by the VM or when setting the creation code
	// above we revert to the snapshot and consume any gas remaining. Additionally
	// when we're in homestead this also counts for code storage gas errors.
	if err != nil && err != ErrCodeStoreOutOfGas {
		vm.StateDB.RevertToSnapshot(snapshot)
		if err != ErrExecutionReverted {
			contract.UseGas(contract.Gas)
		}
	}

	if vm.Config.Debug {
		if vm.depth == 0 {
			vm.Config.Tracer.CaptureEnd(ret, gas-contract.Gas, time.Since(start), err)
		} else {
			vm.Config.Tracer.CaptureExit(ret, gas-contract.Gas, err)
		}
	}
	return ret, address, contract.Gas, err
}

// Create creates a new contract using code as deployment code.
func (vm *ETHVM) Create(caller ContractRef, code []byte, gas uint64, value *big.Int) (ret []byte, contractAddr metatypes.Address, leftOverGas uint64, err error) {
	var nonce uint64
	val, ok := vm.WriteSet.GetAddressAndKey(caller.Address(), NonceKey)
	if !ok {
		acc := types2.Account{
			caller.Address(),
		}
		nonce = vm.StateDB.GetNonce(acc)
	} else {
		nonce = val.(uint64)
	}
	contractAddr = crypto2.CreateAddress(caller.Address(), nonce)
	return vm.create(caller, &codeAndHash{code: code}, gas, value, caller.Address(), CREATE)
}

// Create2 creates a new contract using code as deployment code.
//
// The different between Create2 with Create is Create2 uses keccak256(0xff ++ msg.sender ++ salt ++ keccak256(init_code))[12:]
// instead of the usual sender-and-nonce-hash as the address where the contract is initialized at.
func (vm *ETHVM) Create2(caller ContractRef, code []byte, gas uint64, endowment *big.Int, salt *uint256.Int) (ret []byte, contractAddr metatypes.Address, leftOverGas uint64, err error) {
	codeAndHash := &codeAndHash{code: code}
	contractAddr = crypto2.CreateAddress2(caller.Address(), salt.Bytes32(), codeAndHash.Hash().Bytes())
	return vm.create(caller, codeAndHash, gas, endowment, contractAddr, CREATE2)
}

// ChainConfig returns the environment's chain configuration
func (vm *ETHVM) ChainConfig() *params.ChainConfig { return vm.chainConfig }
