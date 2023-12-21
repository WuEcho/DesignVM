package ewasm

import (
	"github.com/CaduceusMetaverseProtocol/MetaVM/vm/ethvm"
	"github.com/ethereum/go-ethereum/crypto"
	"math/big"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/holiman/uint256"
	"golang.org/x/crypto/sha3"
)

func opAdd(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	x, y := scope.Stack.Pop(), scope.Stack.Peek()
	y.Add(&x, y)
	return nil, nil
}

func opSub(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	x, y := scope.Stack.Pop(), scope.Stack.Peek()
	y.Sub(&x, y)
	return nil, nil
}

func opMul(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	x, y := scope.Stack.Pop(), scope.Stack.Peek()
	y.Mul(&x, y)
	return nil, nil
}

func opDiv(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	x, y := scope.Stack.Pop(), scope.Stack.Peek()
	y.Div(&x, y)
	return nil, nil
}

func opSdiv(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	x, y := scope.Stack.Pop(), scope.Stack.Peek()
	y.SDiv(&x, y)
	return nil, nil
}

func opMod(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	x, y := scope.Stack.Pop(), scope.Stack.Peek()
	y.Mod(&x, y)
	return nil, nil
}

func opSmod(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	x, y := scope.Stack.Pop(), scope.Stack.Peek()
	y.SMod(&x, y)
	return nil, nil
}

func opExp(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	base, exponent := scope.Stack.Pop(), scope.Stack.Peek()
	exponent.Exp(&base, exponent)
	return nil, nil
}

func opSignExtend(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	back, num := scope.Stack.Pop(), scope.Stack.Peek()
	num.ExtendSign(num, &back)
	return nil, nil
}

func opNot(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	x := scope.Stack.Peek()
	x.Not(x)
	return nil, nil
}

func opLt(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	x, y := scope.Stack.Pop(), scope.Stack.Peek()
	if x.Lt(y) {
		y.SetOne()
	} else {
		y.Clear()
	}
	return nil, nil
}

func opGt(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	x, y := scope.Stack.Pop(), scope.Stack.Peek()
	if x.Gt(y) {
		y.SetOne()
	} else {
		y.Clear()
	}
	return nil, nil
}

func opSlt(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	x, y := scope.Stack.Pop(), scope.Stack.Peek()
	if x.Slt(y) {
		y.SetOne()
	} else {
		y.Clear()
	}
	return nil, nil
}

func opSgt(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	x, y := scope.Stack.Pop(), scope.Stack.Peek()
	if x.Sgt(y) {
		y.SetOne()
	} else {
		y.Clear()
	}
	return nil, nil
}

func opEq(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	x, y := scope.Stack.Pop(), scope.Stack.Peek()
	if x.Eq(y) {
		y.SetOne()
	} else {
		y.Clear()
	}
	return nil, nil
}

func opIszero(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	x := scope.Stack.Peek()
	if x.IsZero() {
		x.SetOne()
	} else {
		x.Clear()
	}
	return nil, nil
}

func opAnd(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	x, y := scope.Stack.Pop(), scope.Stack.Peek()
	y.And(&x, y)
	return nil, nil
}

func opOr(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	x, y := scope.Stack.Pop(), scope.Stack.Peek()
	y.Or(&x, y)
	return nil, nil
}

func opXor(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	x, y := scope.Stack.Pop(), scope.Stack.Peek()
	y.Xor(&x, y)
	return nil, nil
}

func opByte(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	th, val := scope.Stack.Pop(), scope.Stack.Peek()
	val.Byte(&th)
	return nil, nil
}

func opAddmod(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	x, y, z := scope.Stack.Pop(), scope.Stack.Pop(), scope.Stack.Peek()
	if z.IsZero() {
		z.Clear()
	} else {
		z.AddMod(&x, &y, z)
	}
	return nil, nil
}

func opMulmod(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	x, y, z := scope.Stack.Pop(), scope.Stack.Pop(), scope.Stack.Peek()
	z.MulMod(&x, &y, z)
	return nil, nil
}

// opSHL implements Shift Left
// The SHL instruction (shift left) pops 2 values from the stack, first arg1 and then arg2,
// and pushes on the stack arg2 shifted to the left by arg1 number of bits.
func opSHL(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	// Note, second operand is left in the stack; accumulate result into it, and no need to push it afterwards
	shift, value := scope.Stack.Pop(), scope.Stack.Peek()
	if shift.LtUint64(256) {
		value.Lsh(value, uint(shift.Uint64()))
	} else {
		value.Clear()
	}
	return nil, nil
}

// opSHR implements Logical Shift Right
// The SHR instruction (logical shift right) pops 2 values from the stack, first arg1 and then arg2,
// and pushes on the stack arg2 shifted to the right by arg1 number of bits with zero fill.
func opSHR(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	// Note, second operand is left in the stack; accumulate result into it, and no need to push it afterwards
	shift, value := scope.Stack.Pop(), scope.Stack.Peek()
	if shift.LtUint64(256) {
		value.Rsh(value, uint(shift.Uint64()))
	} else {
		value.Clear()
	}
	return nil, nil
}

// opSAR implements Arithmetic Shift Right
// The SAR instruction (arithmetic shift right) pops 2 values from the stack, first arg1 and then arg2,
// and pushes on the stack arg2 shifted to the right by arg1 number of bits with sign extension.
func opSAR(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	shift, value := scope.Stack.Pop(), scope.Stack.Peek()
	if shift.GtUint64(256) {
		if value.Sign() >= 0 {
			value.Clear()
		} else {
			// Max negative shift: all bits set
			value.SetAllOne()
		}
		return nil, nil
	}
	n := uint(shift.Uint64())
	value.SRsh(value, n)
	return nil, nil
}

func opKeccak256(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	offset, size := scope.Stack.Pop(), scope.Stack.Peek()
	data := scope.Memory.GetPtr(int64(offset.Uint64()), int64(size.Uint64()))

	if interpreter.ethvm.Interpreter().Hasher() == nil {
		interpreter.ethvm.Interpreter().SetHasher(sha3.NewLegacyKeccak256().(crypto.KeccakState))
	} else {
		interpreter.ethvm.Interpreter().Hasher().Reset()
	}
	interpreter.ethvm.Interpreter().Hasher().Write(data)
      buf := interpreter.ethvm.Interpreter().HasherBuf()
	interpreter.ethvm.Interpreter().Hasher().Read(buf[:])

	vm := interpreter.ethvm
	if vm.Config.EnablePreimageRecording {
		vm.StateDB.AddPreimage(buf, data)
	}

	size.SetBytes(buf[:])
	return nil, nil
}
func opAddress(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	scope.Stack.Push(new(uint256.Int).SetBytes(scope.Contract.Address().Bytes()))
	return nil, nil
}

func opBalance(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	slot := scope.Stack.Peek()
	address := common.Address(slot.Bytes20())
	slot.SetFromBig(interpreter.ethvm.StateDB.GetBalance(address))
	return nil, nil
}

func opOrigin(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	scope.Stack.Push(new(uint256.Int).SetBytes(interpreter.ethvm.Origin.Bytes()))
	return nil, nil
}
func opCaller(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	scope.Stack.Push(new(uint256.Int).SetBytes(scope.Contract.Caller().Bytes()))
	return nil, nil
}

func opCallValue(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	v, _ := uint256.FromBig(scope.Contract.Value())
	scope.Stack.Push(v)
	return nil, nil
}

func opCallDataLoad(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	x := scope.Stack.Peek()
	if offset, overflow := x.Uint64WithOverflow(); !overflow {
		data := ethvm.GetData(scope.Contract.Input, offset, 32)
		x.SetBytes(data)
	} else {
		x.Clear()
	}
	return nil, nil
}

func opCallDataSize(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	scope.Stack.Push(new(uint256.Int).SetUint64(uint64(len(scope.Contract.Input))))
	return nil, nil
}

func opCallDataCopy(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	var (
		memOffset  = scope.Stack.Pop()
		dataOffset = scope.Stack.Pop()
		length     = scope.Stack.Pop()
	)
	dataOffset64, overflow := dataOffset.Uint64WithOverflow()
	if overflow {
		dataOffset64 = 0xffffffffffffffff
	}
	// These values are checked for overflow during gas cost calculation
	memOffset64 := memOffset.Uint64()
	length64 := length.Uint64()
	scope.Memory.Set(memOffset64, length64, ethvm.GetData(scope.Contract.Input, dataOffset64, length64))

	return nil, nil
}

func opReturnDataSize(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	scope.Stack.Push(new(uint256.Int).SetUint64(uint64(len(interpreter.returnData))))
	return nil, nil
}

func opReturnDataCopy(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	var (
		memOffset  = scope.Stack.Pop()
		dataOffset = scope.Stack.Pop()
		length     = scope.Stack.Pop()
	)

	offset64, overflow := dataOffset.Uint64WithOverflow()
	if overflow {
		return nil, ethvm.ErrReturnDataOutOfBounds
	}
	// we can reuse dataOffset now (aliasing it for clarity)
	var end = dataOffset
	end.Add(&dataOffset, &length)
	end64, overflow := end.Uint64WithOverflow()
	if overflow || uint64(len(interpreter.returnData)) < end64 {
		return nil, ethvm.ErrReturnDataOutOfBounds
	}
	scope.Memory.Set(memOffset.Uint64(), length.Uint64(), interpreter.returnData[offset64:end64])
	return nil, nil
}

func opExtCodeSize(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	slot := scope.Stack.Peek()
	slot.SetUint64(uint64(interpreter.ethvm.StateDB.GetCodeSize(slot.Bytes20())))
	return nil, nil
}

func opCodeSize(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	l := new(uint256.Int)
	l.SetUint64(uint64(len(scope.Contract.Code)))
	scope.Stack.Push(l)
	return nil, nil
}

func opCodeCopy(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	var (
		memOffset  = scope.Stack.Pop()
		codeOffset = scope.Stack.Pop()
		length     = scope.Stack.Pop()
	)
	uint64CodeOffset, overflow := codeOffset.Uint64WithOverflow()
	if overflow {
		uint64CodeOffset = 0xffffffffffffffff
	}
	codeCopy := ethvm.GetData(scope.Contract.Code, uint64CodeOffset, length.Uint64())
	scope.Memory.Set(memOffset.Uint64(), length.Uint64(), codeCopy)

	return nil, nil
}

func opExtCodeCopy(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	var (
		stack      = scope.Stack
		a          = stack.Pop()
		memOffset  = stack.Pop()
		codeOffset = stack.Pop()
		length     = stack.Pop()
	)
	uint64CodeOffset, overflow := codeOffset.Uint64WithOverflow()
	if overflow {
		uint64CodeOffset = 0xffffffffffffffff
	}
	addr := common.Address(a.Bytes20())
	codeCopy := ethvm.GetData(interpreter.ethvm.StateDB.GetCode(addr), uint64CodeOffset, length.Uint64())
	scope.Memory.Set(memOffset.Uint64(), length.Uint64(), codeCopy)

	return nil, nil
}

// opExtCodeHash returns the code hash of a specified account.
// There are several cases when the function is called, while we can relay everything
// to `state.GetCodeHash` function to ensure the correctness.
//   (1) Caller tries to get the code hash of a normal contract account, state
// should return the relative code hash and set it as the result.
//
//   (2) Caller tries to get the code hash of a non-existent account, state should
// return common.Hash{} and zero will be set as the result.
//
//   (3) Caller tries to get the code hash for an account without contract code,
// state should return emptyCodeHash(0xc5d246...) as the result.
//
//   (4) Caller tries to get the code hash of a precompiled account, the result
// should be zero or emptyCodeHash.
//
// It is worth noting that in order to avoid unnecessary create and clean,
// all precompile accounts on mainnet have been transferred 1 wei, so the return
// here should be emptyCodeHash.
// If the precompile account is not transferred any amount on a private or
// customized chain, the return value will be zero.
//
//   (5) Caller tries to get the code hash for an account which is marked as suicided
// in the current transaction, the code hash of this account should be returned.
//
//   (6) Caller tries to get the code hash for an account which is marked as deleted,
// this account should be regarded as a non-existent account and zero should be returned.
func opExtCodeHash(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	slot := scope.Stack.Peek()
	address := common.Address(slot.Bytes20())
	if interpreter.ethvm.StateDB.Empty(address) {
		slot.Clear()
	} else {
		slot.SetBytes(interpreter.ethvm.StateDB.GetCodeHash(address).Bytes())
	}
	return nil, nil
}

func opGasprice(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	v, _ := uint256.FromBig(interpreter.ethvm.GasPrice)
	scope.Stack.Push(v)
	return nil, nil
}

func opBlockhash(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	num := scope.Stack.Peek()
	num64, overflow := num.Uint64WithOverflow()
	if overflow {
		num.Clear()
		return nil, nil
	}
	var upper, lower uint64
	upper = interpreter.ethvm.Context.BlockNumber.Uint64()
	if upper < 257 {
		lower = 0
	} else {
		lower = upper - 256
	}
	if num64 >= lower && num64 < upper {
		num.SetBytes(interpreter.ethvm.Context.GetHash(num64).Bytes())
	} else {
		num.Clear()
	}
	return nil, nil
}

func opCoinbase(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	scope.Stack.Push(new(uint256.Int).SetBytes(interpreter.ethvm.Context.Coinbase.Bytes()))
	return nil, nil
}

func opTimestamp(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	v, _ := uint256.FromBig(interpreter.ethvm.Context.Time)
	scope.Stack.Push(v)
	return nil, nil
}

func opNumber(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	v, _ := uint256.FromBig(interpreter.ethvm.Context.BlockNumber)
	scope.Stack.Push(v)
	return nil, nil
}

func opDifficulty(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	v, _ := uint256.FromBig(interpreter.ethvm.Context.Difficulty)
	scope.Stack.Push(v)
	return nil, nil
}

func opRandom(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	v := new(uint256.Int).SetBytes((interpreter.ethvm.Context.Random.Bytes()))
	scope.Stack.Push(v)
	return nil, nil
}

func opGasLimit(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	scope.Stack.Push(new(uint256.Int).SetUint64(interpreter.ethvm.Context.GasLimit))
	return nil, nil
}

func opPop(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	scope.Stack.Pop()
	return nil, nil
}

func opMload(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	v := scope.Stack.Peek()
	offset := int64(v.Uint64())
	v.SetBytes(scope.Memory.GetPtr(offset, 32))
	return nil, nil
}

func opMstore(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	// pop value of the stack
	mStart, val := scope.Stack.Pop(), scope.Stack.Pop()
	scope.Memory.Set32(mStart.Uint64(), &val)
	return nil, nil
}

func opMstore8(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	off, val := scope.Stack.Pop(), scope.Stack.Pop()
	scope.Memory.ChangeStore(off.Uint64(),byte(val.Uint64()))
	return nil, nil
}

func opSload(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	loc := scope.Stack.Peek()
	hash := common.Hash(loc.Bytes32())
	//modify by echo
	var val common.Hash
	////load from cache first
	cache,ok := interpreter.ethvm.WriteSet.Get(scope.Contract.Address())
	if !ok {
		val = interpreter.ethvm.StateDB.GetState(scope.Contract.Address(), hash)
	}else {
		value,ok := cache[hash]
		if !ok {
			val = interpreter.ethvm.StateDB.GetState(scope.Contract.Address(), hash)
		}else {
			val = common.BytesToHash(value)
		}
	}
	//added by echo
	interpreter.ethvm.ReadSet.UpdateCache(scope.Contract.Address(),hash,val.Bytes())
	loc.SetBytes(val.Bytes())
	return nil, nil
}

func opSstore(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	if interpreter.ethvm.Interpreter().GetReadOnly() {
		return nil, ethvm.ErrWriteProtection
	}
	loc := scope.Stack.Pop()
	val := scope.Stack.Pop()
	//modify by echo
	//interpreter.vm.StateDB.SetState(scope.Contract.Address(),
	//	loc.Bytes32(), val.Bytes32())
	//added by echo
	interpreter.ethvm.WriteSet.UpdateCache(scope.Contract.Address(),loc.Bytes32(),val.Bytes(),[]byte{})
	return nil, nil
}

func opJump(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	about := interpreter.ethvm.About()
	if atomic.LoadInt32(&about) != 0 {
		return nil, ethvm.ErrStopToken
	}
	pos := scope.Stack.Pop()
	if !scope.Contract.ValidJumpdest(&pos) {
		return nil, ethvm.ErrInvalidJump
	}
	*pc = pos.Uint64() - 1 // pc will be increased by the interpreter loop
	return nil, nil
}

func opJumpi(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	about := interpreter.ethvm.About()
	if atomic.LoadInt32(&about) != 0 {
		return nil, ethvm.ErrStopToken
	}
	pos, cond := scope.Stack.Pop(), scope.Stack.Pop()
	if !cond.IsZero() {
		if !scope.Contract.ValidJumpdest(&pos) {
			return nil, ethvm.ErrInvalidJump
		}
		*pc = pos.Uint64() - 1 // pc will be increased by the interpreter loop
	}
	return nil, nil
}

func opJumpdest(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	return nil, nil
}

func opPc(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	scope.Stack.Push(new(uint256.Int).SetUint64(*pc))
	return nil, nil
}

func opMsize(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	scope.Stack.Push(new(uint256.Int).SetUint64(uint64(scope.Memory.Len())))
	return nil, nil
}

func opGas(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	scope.Stack.Push(new(uint256.Int).SetUint64(scope.Contract.Gas))
	return nil, nil
}

func opCreate(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	if interpreter.ethvm.Interpreter().GetReadOnly() {
		return nil, ethvm.ErrWriteProtection
	}
	var (
		value        = scope.Stack.Pop()
		offset, size = scope.Stack.Pop(), scope.Stack.Pop()
		input        = scope.Memory.GetCopy(int64(offset.Uint64()), int64(size.Uint64()))
		gas          = scope.Contract.Gas
	)
	gas -= gas / 64
	// reuse size int for stackvalue
	stackvalue := size

	scope.Contract.UseGas(gas)
	//TODO: use uint256.Int instead of converting with toBig()
	var bigVal = big.NewInt(0)
	if !value.IsZero() {
		bigVal = value.ToBig()
	}

	res, addr, returnGas, suberr := interpreter.ethvm.Create(scope.Contract, input, gas, bigVal)
	// Push item on the stack based on the returned error. If the ruleset is
	// homestead we must check for CodeStoreOutOfGasError (homestead only
	// rule) and treat as an error, if the ruleset is frontier we must
	// ignore this error and pretend the operation was successful.
	if suberr != nil && suberr != ethvm.ErrCodeStoreOutOfGas {
		stackvalue.Clear()
	} else {
		stackvalue.SetBytes(addr.Bytes())
	}
	scope.Stack.Push(&stackvalue)
	scope.Contract.Gas += returnGas

	if suberr == ethvm.ErrExecutionReverted {
		interpreter.returnData = res // set REVERT data to return data buffer
		return res, nil
	}
	interpreter.returnData = nil // clear dirty return data buffer
	return nil, nil
}

func opCreate2(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	if interpreter.ethvm.Interpreter().GetReadOnly() {
		return nil, ethvm.ErrWriteProtection
	}
	var (
		endowment    = scope.Stack.Pop()
		offset, size = scope.Stack.Pop(), scope.Stack.Pop()
		salt         = scope.Stack.Pop()
		input        = scope.Memory.GetCopy(int64(offset.Uint64()), int64(size.Uint64()))
		gas          = scope.Contract.Gas
	)

	// Apply EIP150
	gas -= gas / 64
	scope.Contract.UseGas(gas)
	// reuse size int for stackvalue
	stackvalue := size
	//TODO: use uint256.Int instead of converting with toBig()
	bigEndowment := big.NewInt(0)
	if !endowment.IsZero() {
		bigEndowment = endowment.ToBig()
	}
	res, addr, returnGas, suberr := interpreter.ethvm.Create2(scope.Contract, input, gas,
		bigEndowment, &salt)
	// Push item on the stack based on the returned error.
	if suberr != nil {
		stackvalue.Clear()
	} else {
		stackvalue.SetBytes(addr.Bytes())
	}
	scope.Stack.Push(&stackvalue)
	scope.Contract.Gas += returnGas

	if suberr == ethvm.ErrExecutionReverted {
		interpreter.returnData = res // set REVERT data to return data buffer
		return res, nil
	}
	interpreter.returnData = nil // clear dirty return data buffer
	return nil, nil
}

func opCall(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	stack := scope.Stack
	// Pop gas. The actual gas in interpreter.vm.callGasTemp.
	// We can use this as a temporary value
	temp := stack.Pop()
	gas := interpreter.ethvm.GetCallGasTemp()
	// Pop other call parameters.
	addr, value, inOffset, inSize, retOffset, retSize := stack.Pop(), stack.Pop(), stack.Pop(), stack.Pop(), stack.Pop(), stack.Pop()
	toAddr := common.Address(addr.Bytes20())
	// Get the arguments from the memory.
	args := scope.Memory.GetPtr(int64(inOffset.Uint64()), int64(inSize.Uint64()))

	if interpreter.ethvm.Interpreter().GetReadOnly() && !value.IsZero() {
		return nil, ethvm.ErrWriteProtection
	}
	var bigVal = big.NewInt(0)
	//TODO: use uint256.Int instead of converting with toBig()
	// By using big0 here, we save an alloc for the most common case (non-ether-transferring contract calls),
	// but it would make more sense to extend the usage of uint256.Int
	if !value.IsZero() {
		gas += params.CallStipend
		bigVal = value.ToBig()
	}

	ret, returnGas, err := interpreter.ethvm.Call(scope.Contract, toAddr, args, gas, bigVal)

	if err != nil {
		temp.Clear()
	} else {
		temp.SetOne()
	}
	stack.Push(&temp)
	if err == nil || err == ethvm.ErrExecutionReverted {
		ret = common.CopyBytes(ret)
		scope.Memory.Set(retOffset.Uint64(), retSize.Uint64(), ret)
	}
	scope.Contract.Gas += returnGas

	interpreter.returnData = ret
	return ret, nil
}

func opCallCode(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	// Pop gas. The actual gas is in interpreter.vm.callGasTemp.
	stack := scope.Stack
	// We use it as a temporary value
	temp := stack.Pop()
	gas := interpreter.ethvm.GetCallGasTemp()
	// Pop other call parameters.
	addr, value, inOffset, inSize, retOffset, retSize := stack.Pop(), stack.Pop(), stack.Pop(), stack.Pop(), stack.Pop(), stack.Pop()
	toAddr := common.Address(addr.Bytes20())
	// Get arguments from the memory.
	args := scope.Memory.GetPtr(int64(inOffset.Uint64()), int64(inSize.Uint64()))

	//TODO: use uint256.Int instead of converting with toBig()
	var bigVal = big.NewInt(0)
	if !value.IsZero() {
		gas += params.CallStipend
		bigVal = value.ToBig()
	}

	ret, returnGas, err := interpreter.ethvm.CallCode(scope.Contract, toAddr, args, gas, bigVal)
	if err != nil {
		temp.Clear()
	} else {
		temp.SetOne()
	}
	stack.Push(&temp)
	if err == nil || err == ethvm.ErrExecutionReverted {
		ret = common.CopyBytes(ret)
		scope.Memory.Set(retOffset.Uint64(), retSize.Uint64(), ret)
	}
	scope.Contract.Gas += returnGas

	interpreter.returnData = ret
	return ret, nil
}

func opDelegateCall(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	stack := scope.Stack
	// Pop gas. The actual gas is in interpreter.vm.callGasTemp.
	// We use it as a temporary value
	temp := stack.Pop()
	gas := interpreter.ethvm.GetCallGasTemp()
	// Pop other call parameters.
	addr, inOffset, inSize, retOffset, retSize := stack.Pop(), stack.Pop(), stack.Pop(), stack.Pop(), stack.Pop()
	toAddr := common.Address(addr.Bytes20())
	// Get arguments from the memory.
	args := scope.Memory.GetPtr(int64(inOffset.Uint64()), int64(inSize.Uint64()))

	ret, returnGas, err := interpreter.ethvm.DelegateCall(scope.Contract, toAddr, args, gas)
	if err != nil {
		temp.Clear()
	} else {
		temp.SetOne()
	}
	stack.Push(&temp)
	if err == nil || err == ethvm.ErrExecutionReverted {
		ret = common.CopyBytes(ret)
		scope.Memory.Set(retOffset.Uint64(), retSize.Uint64(), ret)
	}
	scope.Contract.Gas += returnGas

	interpreter.returnData = ret
	return ret, nil
}

func opStaticCall(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	// Pop gas. The actual gas is in interpreter.vm.callGasTemp.
	stack := scope.Stack
	// We use it as a temporary value
	temp := stack.Pop()
	gas := interpreter.ethvm.GetCallGasTemp()
	// Pop other call parameters.
	addr, inOffset, inSize, retOffset, retSize := stack.Pop(), stack.Pop(), stack.Pop(), stack.Pop(), stack.Pop()
	toAddr := common.Address(addr.Bytes20())
	// Get arguments from the memory.
	args := scope.Memory.GetPtr(int64(inOffset.Uint64()), int64(inSize.Uint64()))

	ret, returnGas, err := interpreter.ethvm.StaticCall(scope.Contract, toAddr, args, gas)
	if err != nil {
		temp.Clear()
	} else {
		temp.SetOne()
	}
	stack.Push(&temp)
	if err == nil || err == ethvm.ErrExecutionReverted {
		ret = common.CopyBytes(ret)
		scope.Memory.Set(retOffset.Uint64(), retSize.Uint64(), ret)
	}
	scope.Contract.Gas += returnGas

	interpreter.returnData = ret
	return ret, nil
}

func opReturn(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	offset, size := scope.Stack.Pop(), scope.Stack.Pop()
	ret := scope.Memory.GetPtr(int64(offset.Uint64()), int64(size.Uint64()))

	return ret, ethvm.ErrStopToken
}

func opRevert(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	offset, size := scope.Stack.Pop(), scope.Stack.Pop()
	ret := scope.Memory.GetPtr(int64(offset.Uint64()), int64(size.Uint64()))

	interpreter.returnData = ret
	return ret, ethvm.ErrExecutionReverted
}

func opUndefined(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	return nil, &ethvm.ErrInvalidOpCode{InValOpcode:ethvm.OpCode(scope.Contract.Code[*pc])}
}

func opStop(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	return nil, ethvm.ErrStopToken
}

func opSelfdestruct(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	if interpreter.ethvm.Interpreter().GetReadOnly() {
		return nil, ethvm.ErrWriteProtection
	}
	beneficiary := scope.Stack.Pop()
	balance := interpreter.ethvm.StateDB.GetBalance(scope.Contract.Address())
	interpreter.ethvm.StateDB.AddBalance(beneficiary.Bytes20(), balance)
	interpreter.ethvm.StateDB.Suicide(scope.Contract.Address())
	cfg := interpreter.ethvm.Interpreter().GetCfg()
	if cfg.Debug {
		cfg.Tracer.CaptureEnter(ethvm.SELFDESTRUCT, scope.Contract.Address(), beneficiary.Bytes20(), []byte{}, 0, balance)
		cfg.Tracer.CaptureExit([]byte{}, 0, nil)
	}
	return nil, ethvm.ErrStopToken
}

// following functions are used by the instruction jump  table

// make log instruction function
func makeLog(size int) executionFunc {
	return func(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
		if interpreter.ethvm.Interpreter().GetReadOnly() {
			return nil, ethvm.ErrWriteProtection
		}
		topics := make([]common.Hash, size)
		stack := scope.Stack
		mStart, mSize := stack.Pop(), stack.Pop()
		for i := 0; i < size; i++ {
			addr := stack.Pop()
			topics[i] = addr.Bytes32()
		}

		d := scope.Memory.GetCopy(int64(mStart.Uint64()), int64(mSize.Uint64()))
		//interpreter.ethvm.StateDB.AddLog(&types.Log{
		//	Address: scope.Contract.Address(),
		//	Topics:  topics,
		//	Data:    d,
		//	// This is a non-consensus field, but assigned here because
		//	// core/state doesn't know the current block number.
		//	BlockNumber: interpreter.ethvm.Context.BlockNumber.Uint64(),
		//})

		log := &types.Log{
			Address: scope.Contract.Address(),
			Topics:  topics,
			Data:    d,
			// This is a non-consensus field, but assigned here because
			// core/state doesn't know the current block number.
			BlockNumber: interpreter.ethvm.Context.BlockNumber.Uint64(),
		}
		interpreter.ethvm.Logs = append(interpreter.ethvm.Logs,log)
		return nil, nil
	}
}

// opPush1 is a specialized version of pushN
func opPush1(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	var (
		codeLen = uint64(len(scope.Contract.Code))
		integer = new(uint256.Int)
	)
	*pc += 1
	if *pc < codeLen {
		scope.Stack.Push(integer.SetUint64(uint64(scope.Contract.Code[*pc])))
	} else {
		scope.Stack.Push(integer.Clear())
	}
	return nil, nil
}

// make push instruction function
func makePush(size uint64, pushByteSize int) executionFunc {
	return func(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
		codeLen := len(scope.Contract.Code)

		startMin := codeLen
		if int(*pc+1) < startMin {
			startMin = int(*pc + 1)
		}

		endMin := codeLen
		if startMin+pushByteSize < endMin {
			endMin = startMin + pushByteSize
		}

		integer := new(uint256.Int)
		scope.Stack.Push(integer.SetBytes(common.RightPadBytes(
			scope.Contract.Code[startMin:endMin], pushByteSize)))

		*pc += size
		return nil, nil
	}
}


// opPush0 implements the PUSH0 opcode
func opPush0(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	scope.Stack.Push(new(uint256.Int))
	return nil, nil
}

func opSelfBalance(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	balance, _ := uint256.FromBig(interpreter.ethvm.StateDB.GetBalance(scope.Contract.Address()))
	scope.Stack.Push(balance)
	return nil, nil
}


// opChainID implements CHAINID opcode
func opChainID(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	chainId, _ := uint256.FromBig(interpreter.ethvm.GetChainConfig().ChainID)
	scope.Stack.Push(chainId)
	return nil, nil
}


// opBaseFee implements BASEFEE opcode
func opBaseFee(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
	baseFee, _ := uint256.FromBig(interpreter.ethvm.Context.BaseFee)
	scope.Stack.Push(baseFee)
	return nil, nil
}



// make dup instruction function
func makeDup(size int64) executionFunc {
	return func(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
		scope.Stack.Dup(int(size))
		return nil, nil
	}
}

// make swap instruction function
func makeSwap(size int64) executionFunc {
	// switch n + 1 otherwise n would be swapped with n
	size++
	return func(pc *uint64, interpreter *EWASMInterpreter, scope *ethvm.ScopeContext) ([]byte, error) {
		scope.Stack.Swap(int(size))
		return nil, nil
	}
}
