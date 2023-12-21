package ethvm

import (
	"encoding/hex"
	"fmt"
	crypto2 "github.com/CaduceusMetaverseProtocol/MetaNebula/common/crypto"
	types3 "github.com/CaduceusMetaverseProtocol/MetaNebula/types"
	basev1 "github.com/CaduceusMetaverseProtocol/MetaProtocol/gen/proto/go/base/v1"
	types2 "github.com/CaduceusMetaverseProtocol/MetaTypes/types"
	common2 "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	"github.com/go-interpreter/wagon/exec"
	"github.com/go-interpreter/wagon/wasm"
	"golang.org/x/crypto/ed25519"
	"math"
	"math/big"
	"reflect"
)

const (
	// EEICallSuccess is the return value in case of a successful contract execution
	EEICallSuccess = 0
	// ErrEEICallFailure is the return value in case of a contract execution failture
	ErrEEICallFailure = 1
	// ErrEEICallRevert is the return value in case a contract calls `revert`
	ErrEEICallRevert = 2
)

// List of gas costs
const (
	GasCostZero           = 0
	GasCostBase           = 2
	GasCostVeryLow        = 3
	GasCostLow            = 5
	GasCostMid            = 8
	GasCostHigh           = 10
	GasCostExtCode        = 700
	GasCostBalance        = 400
	GasCostSLoad          = 200
	GasCostJumpDest       = 1
	GasCostSSet           = 20000
	GasCostSReset         = 5000
	GasRefundSClear       = 15000
	GasRefundSelfDestruct = 24000
	GasCostCreate         = 32000
	GasCostCall           = 700
	GasCostCallValue      = 9000
	GasCostCallStipend    = 2300
	GasCostNewAccount     = 25000
	GasCostLog            = 375
	GasCostLogData        = 8
	GasCostLogTopic       = 375
	GasCostCopy           = 3
	GasCostBlockHash      = 800
)

var eeiTypes = &wasm.SectionTypes{
	Entries: []wasm.FunctionSig{
		{
			ParamTypes:  []wasm.ValueType{wasm.ValueTypeI64},
			ReturnTypes: []wasm.ValueType{},
		},
		{
			ParamTypes:  []wasm.ValueType{wasm.ValueTypeI32},
			ReturnTypes: []wasm.ValueType{},
		},
		{
			ParamTypes:  []wasm.ValueType{wasm.ValueTypeI32, wasm.ValueTypeI32},
			ReturnTypes: []wasm.ValueType{},
		},
		{
			ParamTypes:  []wasm.ValueType{wasm.ValueTypeI64, wasm.ValueTypeI32},
			ReturnTypes: []wasm.ValueType{wasm.ValueTypeI32},
		},
		{
			ParamTypes:  []wasm.ValueType{wasm.ValueTypeI64, wasm.ValueTypeI32, wasm.ValueTypeI32, wasm.ValueTypeI32, wasm.ValueTypeI32},
			ReturnTypes: []wasm.ValueType{wasm.ValueTypeI32},
		},
		{
			ParamTypes:  []wasm.ValueType{wasm.ValueTypeI32},
			ReturnTypes: []wasm.ValueType{wasm.ValueTypeI32},
		},
		{
			ParamTypes:  []wasm.ValueType{wasm.ValueTypeI32, wasm.ValueTypeI32, wasm.ValueTypeI32},
			ReturnTypes: []wasm.ValueType{},
		},
		{
			ParamTypes:  []wasm.ValueType{},
			ReturnTypes: []wasm.ValueType{wasm.ValueTypeI32},
		},
		{
			ParamTypes:  []wasm.ValueType{wasm.ValueTypeI64, wasm.ValueTypeI32, wasm.ValueTypeI32, wasm.ValueTypeI32},
			ReturnTypes: []wasm.ValueType{wasm.ValueTypeI32},
		},
		{
			ParamTypes:  []wasm.ValueType{wasm.ValueTypeI64, wasm.ValueTypeI32, wasm.ValueTypeI32, wasm.ValueTypeI32, wasm.ValueTypeI32},
			ReturnTypes: []wasm.ValueType{wasm.ValueTypeI32},
		},
		{
			ParamTypes:  []wasm.ValueType{wasm.ValueTypeI32, wasm.ValueTypeI32, wasm.ValueTypeI32, wasm.ValueTypeI32},
			ReturnTypes: []wasm.ValueType{wasm.ValueTypeI32},
		},
		{
			ParamTypes:  []wasm.ValueType{wasm.ValueTypeI32, wasm.ValueTypeI32, wasm.ValueTypeI32, wasm.ValueTypeI32},
			ReturnTypes: []wasm.ValueType{},
		},
		{
			ParamTypes:  []wasm.ValueType{},
			ReturnTypes: []wasm.ValueType{wasm.ValueTypeI64},
		},
		{
			ParamTypes:  []wasm.ValueType{wasm.ValueTypeI32, wasm.ValueTypeI32, wasm.ValueTypeI32, wasm.ValueTypeI32, wasm.ValueTypeI32, wasm.ValueTypeI32, wasm.ValueTypeI32},
			ReturnTypes: []wasm.ValueType{},
		},
	},
}

func swapEndian(src []byte) []byte {
	ret := make([]byte, len(src))
	for i, v := range src {
		ret[len(src)-i-1] = v
	}
	return ret
}

func (in *EWASMInterpreter) gasAccounting(p *exec.Process, cost uint64) {
	log.Trace("gasAccounting", "contract.gas", in.contract.Gas, "cost", cost)
	if in.contract == nil {
		panic("nil contract")
	}
	if cost > in.contract.Gas {
		log.Error(fmt.Sprintf("gasAccounting : WASM-RUN : out of gas cost %d > gas %d", cost, in.contract.Gas))
		in.terminationType = TerminateInvalid
		p.Terminate()
		return
	}
	in.contract.Gas -= cost
}

func getDebugFuncs(in *EWASMInterpreter) []wasm.Function {
	return []wasm.Function{
		{
			Sig:  &eeiTypes.Entries[2],
			Host: reflect.ValueOf(func(p *exec.Process, o, l int32) { printMemHex(p, in, o, l) }),
			Body: &wasm.FunctionBody{},
		},
		{
			Sig:  &eeiTypes.Entries[1],
			Host: reflect.ValueOf(func(p *exec.Process, o int32) { printStorageHex(p, in, o) }),
			Body: &wasm.FunctionBody{},
		},
	}
}

func printMemHex(p *exec.Process, in *EWASMInterpreter, offset, length int32) {
	fmt.Println("printMemHex ------>")
	data := readSize(p, offset, int(length))
	for _, v := range data {
		fmt.Printf("%02x", v)
	}
	fmt.Printf("\r\nprintMemHex <------\r\n")
}

func printStorageHex(p *exec.Process, in *EWASMInterpreter, pathOffset int32) {
	path := types2.BytesToHash(readSize(p, pathOffset, common2.HashLength))
	val := in.StateDB.GetState(in.contract.Account(), path)
	for v := range val {
		fmt.Printf("%02x", v)
	}
	fmt.Println("")
}

// Return the list of function descriptors. This is a function instead of
// a variable in Order to avoid an initialization loop.
func eeiFuncs(in *EWASMInterpreter) []wasm.Function {
	return []wasm.Function{
		{
			Sig:  &eeiTypes.Entries[0],                                                 // TODO use constants or find the right entry in the list
			Host: reflect.ValueOf(func(p *exec.Process, a int64) { useGas(p, in, a) }), // log
			Body: &wasm.FunctionBody{},
		},
		{
			Sig:  &eeiTypes.Entries[1],
			Host: reflect.ValueOf(func(p *exec.Process, r int32) { getAddress(p, in, r) }), // log
			Body: &wasm.FunctionBody{},
		},
		{
			Sig:  &eeiTypes.Entries[2],
			Host: reflect.ValueOf(func(p *exec.Process, a, r int32) { getExternalBalance(p, in, a, r) }), // log
			Body: &wasm.FunctionBody{},
		},
		{
			Sig:  &eeiTypes.Entries[3],
			Host: reflect.ValueOf(func(p *exec.Process, n int64, r int32) int32 { return getBlockHash(p, in, n, r) }), // log
			Body: &wasm.FunctionBody{},
		},
		{
			Sig:  &eeiTypes.Entries[4],
			Host: reflect.ValueOf(func(p *exec.Process, g int64, a, v, d, l int32) int32 { return call(p, in, g, a, v, d, l) }), // log
			Body: &wasm.FunctionBody{},
		},
		{
			Sig:  &eeiTypes.Entries[6],
			Host: reflect.ValueOf(func(p *exec.Process, r, d, l int32) { callDataCopy(p, in, r, d, l) }), // log
			Body: &wasm.FunctionBody{},
		},
		{
			Sig:  &eeiTypes.Entries[7],
			Host: reflect.ValueOf(func(p *exec.Process) int32 { return getCallDataSize(p, in) }), // log
			Body: &wasm.FunctionBody{},
		},
		{
			Sig:  &eeiTypes.Entries[9],
			Host: reflect.ValueOf(func(p *exec.Process, g int64, a, v, d, l int32) int32 { return callCode(p, in, g, a, v, d, l) }), // log
			Body: &wasm.FunctionBody{},
		},
		{
			Sig:  &eeiTypes.Entries[8],
			Host: reflect.ValueOf(func(p *exec.Process, g int64, a, d, l int32) int32 { return callDelegate(p, in, g, a, d, l) }), // log
			Body: &wasm.FunctionBody{},
		},
		{
			Sig:  &eeiTypes.Entries[8],
			Host: reflect.ValueOf(func(p *exec.Process, g int64, a, d, l int32) int32 { return callStatic(p, in, g, a, d, l) }), // log
			Body: &wasm.FunctionBody{},
		},
		{
			Sig:  &eeiTypes.Entries[2],
			Host: reflect.ValueOf(func(pr *exec.Process, p, v int32) { storageStore(pr, in, p, v) }), // log
			Body: &wasm.FunctionBody{},
		},
		{
			Sig:  &eeiTypes.Entries[2],
			Host: reflect.ValueOf(func(pr *exec.Process, p, r int32) { storageLoad(pr, in, p, r) }), // log
			Body: &wasm.FunctionBody{},
		},
		{
			Sig:  &eeiTypes.Entries[1],
			Host: reflect.ValueOf(func(p *exec.Process, r int32) { getCaller(p, in, r) }), // log
			Body: &wasm.FunctionBody{},
		},
		{
			Sig:  &eeiTypes.Entries[1],
			Host: reflect.ValueOf(func(p *exec.Process, r int32) { getCallValue(p, in, r) }), // log
			Body: &wasm.FunctionBody{},
		},
		{
			Sig:  &eeiTypes.Entries[6],
			Host: reflect.ValueOf(func(p *exec.Process, r, c, l int32) { codeCopy(p, in, r, c, l) }), // log
			Body: &wasm.FunctionBody{},
		},
		{
			Sig:  &eeiTypes.Entries[7],
			Host: reflect.ValueOf(func(p *exec.Process) int32 { return getCodeSize(p, in) }), // log
			Body: &wasm.FunctionBody{},
		},
		{
			Sig:  &eeiTypes.Entries[1],
			Host: reflect.ValueOf(func(p *exec.Process, r int32) { getBlockCoinbase(p, in, r) }), // log
			Body: &wasm.FunctionBody{},
		},
		{
			Sig:  &eeiTypes.Entries[10],
			Host: reflect.ValueOf(func(p *exec.Process, v, d, l, r uint32) int32 { return create(p, in, v, d, l, r) }), //log
			Body: &wasm.FunctionBody{},
		},
		{
			Sig:  &eeiTypes.Entries[1],
			Host: reflect.ValueOf(func(p *exec.Process, r int32) { getBlockDifficulty(p, in, r) }), //log
			Body: &wasm.FunctionBody{},
		},
		{
			Sig:  &eeiTypes.Entries[11],
			Host: reflect.ValueOf(func(p *exec.Process, a, r, c, l int32) { externalCodeCopy(p, in, a, r, c, l) }),
			Body: &wasm.FunctionBody{},
		},
		{
			Sig:  &eeiTypes.Entries[5],
			Host: reflect.ValueOf(func(p *exec.Process, a int32) int32 { return getExternalCodeSize(p, in, a) }),
			Body: &wasm.FunctionBody{},
		},
		{
			Sig:  &eeiTypes.Entries[12],
			Host: reflect.ValueOf(func(p *exec.Process) int64 { return getGasLeft(p, in) }),
			Body: &wasm.FunctionBody{},
		},
		{
			Sig:  &eeiTypes.Entries[12],
			Host: reflect.ValueOf(func(p *exec.Process) int64 { return getBlockGasLimit(p, in) }),
			Body: &wasm.FunctionBody{},
		},
		{
			Sig:  &eeiTypes.Entries[1],
			Host: reflect.ValueOf(func(p *exec.Process, v int32) { getTxGasPrice(p, in, v) }),
			Body: &wasm.FunctionBody{},
		},
		{
			Sig:  &eeiTypes.Entries[13],
			Host: reflect.ValueOf(func(p *exec.Process, d, l, n, t1, t2, t3, t4 int32) { _log(p, in, d, l, n, t1, t2, t3, t4) }),
			Body: &wasm.FunctionBody{},
		},
		{
			Sig:  &eeiTypes.Entries[12],
			Host: reflect.ValueOf(func(p *exec.Process) int64 { return getBlockNumber(p, in) }),
			Body: &wasm.FunctionBody{},
		},
		{
			Sig:  &eeiTypes.Entries[1],
			Host: reflect.ValueOf(func(p *exec.Process, r int32) { getTxOrigin(p, in, r) }),
			Body: &wasm.FunctionBody{},
		},
		{
			Sig:  &eeiTypes.Entries[2],
			Host: reflect.ValueOf(func(p *exec.Process, d, l int32) { finish(p, in, d, l) }),
			Body: &wasm.FunctionBody{},
		},
		{
			Sig:  &eeiTypes.Entries[2],
			Host: reflect.ValueOf(func(p *exec.Process, d, l int32) { revert(p, in, d, l) }),
			Body: &wasm.FunctionBody{},
		},
		{
			Sig:  &eeiTypes.Entries[7],
			Host: reflect.ValueOf(func(p *exec.Process) int32 { return getReturnDataSize(p, in) }),
			Body: &wasm.FunctionBody{},
		},
		{
			Sig:  &eeiTypes.Entries[6],
			Host: reflect.ValueOf(func(p *exec.Process, r, d, l int32) { returnDataCopy(p, in, r, d, l) }),
			Body: &wasm.FunctionBody{},
		},
		{
			Sig:  &eeiTypes.Entries[1],
			Host: reflect.ValueOf(func(p *exec.Process, a int32) { selfDestruct(p, in, a) }),
			Body: &wasm.FunctionBody{},
		},
		{
			Sig:  &eeiTypes.Entries[12],
			Host: reflect.ValueOf(func(p *exec.Process) int64 { return getBlockTimestamp(p, in) }),
			Body: &wasm.FunctionBody{},
		},
		// add by echo : 扩展持久化 >>>>>>>>>>>>>>>>>>
		{
			Sig: &wasm.FunctionSig{
				ParamTypes:  []wasm.ValueType{wasm.ValueTypeI32, wasm.ValueTypeI32, wasm.ValueTypeI32, wasm.ValueTypeI32},
				ReturnTypes: []wasm.ValueType{},
			},
			Host: reflect.ValueOf(func(p *exec.Process, k, kl, v, vl int32) { storageStore2(p, in, k, kl, v, vl) }),
			Body: &wasm.FunctionBody{},
		},
		{
			Sig: &wasm.FunctionSig{
				ParamTypes:  []wasm.ValueType{wasm.ValueTypeI32, wasm.ValueTypeI32, wasm.ValueTypeI32, wasm.ValueTypeI32},
				ReturnTypes: []wasm.ValueType{},
			},
			Host: reflect.ValueOf(func(p *exec.Process, k, kl, v, vt int32) { storageLoad2(p, in, k, kl, v, vt) }),
			Body: &wasm.FunctionBody{},
		},
		// add by echo : 扩展持久化 <<<<<<<<<<<<<<<<<<
	}
}

func readSize(p *exec.Process, offset int32, size int) []byte {
	// TODO modify the process interface to find out how much memory is
	// available on the system.
	val := make([]byte, size)
	p.ReadAt(val, int64(offset))
	return val
}

func useGas(p *exec.Process, in *EWASMInterpreter, amount int64) {
	log.Trace("useGas", "amount", amount)
	in.gasAccounting(p, uint64(amount))
}

func getAddress(p *exec.Process, in *EWASMInterpreter, resultOffset int32) {
	log.Trace("getAddress", "contract", in.contract.Address().Hex(), "caller", in.contract.CallerAddress.Hex())
	in.gasAccounting(p, GasCostBase)
	contractBytes := in.contract.CodeAddr.Bytes()
	p.WriteAt(contractBytes, int64(resultOffset))
}

func getExternalBalance(p *exec.Process, in *EWASMInterpreter, addressOffset int32, resultOffset int32) {
	log.Trace("getExternalBalance", "contract", in.contract.Address().Hex(), "caller", in.contract.CallerAddress.Hex())
	in.gasAccounting(p, GasCostBalance)
	addr := types2.BytesToAddress(readSize(p, addressOffset, common2.AddressLength))
	acc := types3.Account{
		addr,
	}
	balance := in.StateDB.GetBalance(acc)
	data := swapEndian(balance.Bytes())
	p.WriteAt(data, int64(resultOffset))

	in.ethvm.ReadSet.UpdateCache(addr,BalanceKey,balance)
}

func getBlockHash(p *exec.Process, in *EWASMInterpreter, number int64, resultOffset int32) int32 {
	log.Trace("getBlockHash", "contract", in.contract.Address().Hex(), "caller", in.contract.CallerAddress.Hex())
	in.gasAccounting(p, GasCostBlockHash)
	n := big.NewInt(number)
	//fmt.Println(n)
	n.Sub(in.ethvm.Context.BlockNumber, n)
	//fmt.Println(n, n.Cmp(big.NewInt(256)), n.Cmp(big.NewInt(0)))
	if n.Cmp(big.NewInt(256)) > 0 || n.Cmp(big.NewInt(0)) <= 0 {
		return 1
	}
	h := in.ethvm.Context.GetHash(uint64(number))
	p.WriteAt(h.Bytes(), int64(resultOffset))
	return 0
}

func callCommon(sig string, p *exec.Process, in *EWASMInterpreter, contract, targetContract *Contract, input []byte, value *big.Int, snapshot int, gas int64, ro bool) int32 {
	if in.ethvm.GetDepeth() > maxCallDepth {
		return ErrEEICallFailure
	}

	if IsWASM(contract.Code) {
		savedVM := in.vm
		in.Run(targetContract, input, ro)
		in.vm = savedVM
		in.contract = contract

		if value.Cmp(big.NewInt(0)) != 0 {
			in.gasAccounting(p, uint64(gas)-targetContract.Gas-GasCostCallStipend)
		} else {
			in.gasAccounting(p, uint64(gas)-targetContract.Gas)
		}
	} else { // 非 WASM 合约，需要用 vm 去尝试执行
		addr := targetContract.Address()
		fmt.Println(sig, "----> WASM-->SOL START:", "from=", contract.Address().Hex(), ",to=", addr.Hex(), ",input=", len(input))
		ret, returnGas, err := in.ethvm.Call(contract, addr, input, uint64(gas), value)
		fmt.Println(sig, "<---- WASM-->SOL END:", "from=", contract.Address().Hex(), ",to=", addr.Hex(), "err=", err, "gas=", returnGas, ",result=", len(ret))
		if err != nil {
			in.terminationType = TerminateInvalid
		} else if err == ErrExecutionReverted {
			in.terminationType = TerminateRevert
			in.returnData = ret
		} else {
			in.terminationType = TerminateFinish
			in.returnData = ret
		}
		contract.Gas += returnGas
	}

	switch in.terminationType {
	case TerminateFinish:
		return EEICallSuccess
	case TerminateRevert:
		in.StateDB.RevertToSnapshot(snapshot)
		return ErrEEICallRevert
	default:
		in.StateDB.RevertToSnapshot(snapshot)
		contract.UseGas(targetContract.Gas)
		return ErrEEICallFailure
	}
}

func call(p *exec.Process, in *EWASMInterpreter, gas int64, addressOffset int32, valueOffset int32, dataOffset int32, dataLength int32) int32 {
	contract := in.contract

	// Get the address of the contract to call
	addr := types2.BytesToAddress(readSize(p, addressOffset, common2.AddressLength)) //这是to,要调用那个合约
	log.Warn("call", "contract", in.contract.Address().Hex(), "in.contract.Caller()", in.contract.Caller().String(), "caller", in.contract.CallerAddress.Hex(), "addr", addr.String())
	/*
		in.contract.Caller()  当前合约调用者 = 用户from
		in.contract.Address() 当前调用的合约地址 = to
		addr	              跨合约要调用的地址 = to调用的地址
	*/

	if in.ethvm.StateDB == nil {
		//获取账户state
		return ErrEEICallRevert
	}

	// Get the value. The [spec](https://github.com/ewasm/design/blob/master/eth_interface.md#call)
	// requires this operation to be U128, which is incompatible with the vm version that expects
	// a u256.
	// To be compatible with hera, one must read a u256 value, then check that this is a u128.
	v0 := readSize(p, valueOffset, u128Len)
	v1 := swapEndian(v0)
	value := big.NewInt(0).SetBytes(v1)
	check128bits := big.NewInt(1)
	check128bits.Lsh(check128bits, 128)
	if value.Cmp(check128bits) > 0 {
		log.Error(" value.Cmp(check128bits) > 0 ")
		return ErrEEICallFailure
	}

	// Fail if the account's balance is greater than 128bits as discussed
	// in https://github.com/ewasm/hera/issues/456
	balance := new(big.Int)
	data,ok := in.ethvm.WriteSet.GetAddressAndKey(contract.Address(),BalanceKey)
	if !ok {
		balance = in.StateDB.GetBalance(contract.Account())
	}else {
		balance = data.(*big.Int)
	}
	if balance.Cmp(check128bits) > 0 {
		in.gasAccounting(p, contract.Gas)
		log.Error("accountState.GetBalance(contract.Address()).Cmp(check128bits) > 0")
		return ErrEEICallRevert
	}

	if in.staticMode == true && value.Cmp(big.NewInt(0)) != 0 {
		in.gasAccounting(p, in.contract.Gas)
		log.Error("in.staticMode == true && value.Cmp(big.NewInt(0)) != 0")
		return ErrEEICallFailure
	}

	in.gasAccounting(p, GasCostCall)

	if in.ethvm.GetDepeth() > maxCallDepth {
		log.Error("in.vm.depth > maxCallDepth")
		return ErrEEICallFailure
	}

	if value.Cmp(big.NewInt(0)) != 0 {
		log.Error(" value.Cmp(big.NewInt(0)) != 0")
		in.gasAccounting(p, GasCostCallValue)
	}

	// Get the arguments.
	// TODO check the need for callvalue (seems not, a lot of that stuff is
	// already accounted for in the functions that I already called - need to
	// refactor all that)
	input := readSize(p, dataOffset, int(dataLength))

	snapshot := in.StateDB.Snapshot()

	// Check that there is enough balance to transfer the value
	if balance.Cmp(value) < 0 {
		log.Error("accountState.GetBalance(contract.Address()).Cmp(value) < 0")
		return ErrEEICallFailure
	}
	acc := types3.Account{
		addr,
	}
	// Check that the contract exists
	if !in.StateDB.Exist(acc) {
		in.gasAccounting(p, GasCostNewAccount)
		//in.StateDB.CreateAccount(addr)
		in.ethvm.WriteSet.UpdateCache(addr,CreatAccountKey,addr.Bytes(),[]byte{})
	}

	var calleeGas uint64
	if uint64(gas) > ((63 * contract.Gas) / 64) {
		calleeGas = contract.Gas - (contract.Gas / 64)
	} else {
		calleeGas = uint64(gas)
	}
	in.gasAccounting(p, calleeGas)

	if value.Cmp(big.NewInt(0)) != 0 {
		calleeGas += GasCostCallStipend
	}
	// 如果 call 的 code 是 sol 合约，这里7转到 vm 去处理，否则继续
	code,ok := in.ethvm.WriteSet.GetAddressAndKey(addr,SetCodeKey)
	if !ok {
		code = in.StateDB.GetCode(acc)
	}
	// Load the contract code in a new VM structure
	targetContract := NewContract(contract, AccountRef(addr), value, calleeGas)

	//if len(code) == 0 || IsWASM(code) {
	//	// TODO tracing
	//	// Add amount to recipient
	//	// 如果不是 绑定的账户state
	//	in.vm.Transfer(in.StateDB, contract.Address(), addr, value)
	//}

	if len(code.([]byte)) == 0 {
		log.Warn("wasm len(code) == 0")
		in.contract.Gas += calleeGas
		return EEICallSuccess
	}

	targetContract.SetCallCode(&addr, in.StateDB.GetCodeHash(acc), code.([]byte))
	//find  the relationship whether in register list
	if len(code.([]byte)) == 0 || IsWASM(code.([]byte)) {
		log.Info("call", "----> WASM----> START:")
		savedVM := in.vm
		in.Run(targetContract, input, false)
		in.vm = savedVM
		in.contract = contract
		// Add leftover gas
		in.contract.Gas += targetContract.Gas
	} else { // 非 WASM 合约，需要用 vm 去尝试执行
		log.Info("----> WASM-->SOL START:", "from=", contract.Address().Hex(), ",to=", addr.Hex(), ",input=", len(input))
		ret, returnGas, err := in.ethvm.Call(contract,addr, input, calleeGas, value)
		log.Info("<---- WASM-->SOL END:", "from=", contract.Address().Hex(), ",to=", addr.Hex(), "err=", err, "gas=", returnGas, ",result=", len(ret))
		if err != nil {
			in.terminationType = TerminateInvalid
		} else if err == ErrExecutionReverted {
			in.terminationType = TerminateRevert
			in.returnData = ret
		} else {
			in.terminationType = TerminateFinish
			in.returnData = ret
		}
		contract.Gas += returnGas
	}

	defer func() { in.terminationType = TerminateFinish }()

	switch in.terminationType {
	case TerminateFinish:
		return EEICallSuccess
	case TerminateRevert:
		in.StateDB.RevertToSnapshot(snapshot)
		return ErrEEICallRevert
	default:
		in.StateDB.RevertToSnapshot(snapshot)
		contract.UseGas(targetContract.Gas)
		return ErrEEICallFailure
	}

}

func callDataCopy(p *exec.Process, in *EWASMInterpreter, resultOffset int32, dataOffset int32, length int32) {
	log.Trace("callDataCopy", "contract", in.contract.Address().Hex(), "caller", in.contract.CallerAddress.Hex())
	in.gasAccounting(p, GasCostVeryLow+GasCostCopy*(uint64(length+31)>>5))
	data := in.contract.Input[dataOffset : dataOffset+length]
	p.WriteAt(data, int64(resultOffset))
}

func getCallDataSize(p *exec.Process, in *EWASMInterpreter) int32 {
	log.Trace("getCallDataSize", "contract", in.contract.Address().Hex(), "caller", in.contract.CallerAddress.Hex())
	in.gasAccounting(p, GasCostBase)
	return int32(len(in.contract.Input))
}

func callCode(p *exec.Process, in *EWASMInterpreter, gas int64, addressOffset int32, valueOffset int32, dataOffset int32, dataLength int32) int32 {
	log.Trace("callCode", "contract", in.contract.Address().Hex(), "caller", in.contract.CallerAddress.Hex())
	in.gasAccounting(p, GasCostCall)

	contract := in.contract

	// Get the address of the contract to call
	addr := types2.BytesToAddress(readSize(p, addressOffset, common2.AddressLength))

	// Get the value. The [spec](https://github.com/ewasm/design/blob/master/eth_interface.md#call)
	// requires this operation to be U128, which is incompatible with the vm version that expects
	// a u256.
	value := big.NewInt(0).SetBytes(readSize(p, valueOffset, u128Len))

	if value.Cmp(big.NewInt(0)) != 0 {
		in.gasAccounting(p, GasCostCallValue)
		gas += GasCostCallStipend
	}

	// Get the arguments.
	// TODO check the need for callvalue (seems not, a lot of that stuff is
	// already accounted for in the functions that I already called - need to
	// refactor all that)
	input := readSize(p, dataOffset, int(dataLength))

	snapshot := in.StateDB.Snapshot()
	acc := types3.Account{
		addr,
	}
	balance := new(big.Int)
	data,ok := in.ethvm.WriteSet.GetAddressAndKey(addr,BalanceKey)
	if !ok {
		balance = in.StateDB.GetBalance(acc)
	}else {
		balance = data.(*big.Int)
	}
	// Check that there is enough balance to transfer the value
	if balance.Cmp(value) < 0 {
		fmt.Printf("Not enough balance: wanted to use %v, got %v\n", value, in.StateDB.GetBalance(acc))
		return ErrEEICallFailure
	}

	// TODO tracing
	// TODO check that EIP-150 is respected

	// Load the contract code in a new VM structure
	targetContract := NewContract(contract, AccountRef(contract.Address()), value, uint64(gas))
	code,ok := in.ethvm.WriteSet.GetAddressAndKey(addr,SetCodeKey)
	if !ok {
		code = in.StateDB.GetCode(acc)
	}
	codeHash := crypto2.Keccak256Hash(code.([]byte))
	targetContract.SetCallCode(&addr, codeHash, code.([]byte))

	return callCommon("callCode", p, in, contract, targetContract, input, value, snapshot, gas, false)
}

func callDelegate(p *exec.Process, in *EWASMInterpreter, gas int64, addressOffset int32, dataOffset int32, dataLength int32) int32 {
	log.Trace("callDelegate", "contract", in.contract.Address().Hex(), "caller", in.contract.CallerAddress.Hex())
	in.gasAccounting(p, GasCostCall)

	contract := in.contract

	// Get the address of the contract to call
	addr := types2.BytesToAddress(readSize(p, addressOffset, common2.AddressLength))

	// Get the value. The [spec](https://github.com/ewasm/design/blob/master/eth_interface.md#call)
	// requires this operation to be U128, which is incompatible with the vm version that expects
	// a u256.
	value := contract.Value()

	if value.Cmp(big.NewInt(0)) != 0 {
		in.gasAccounting(p, GasCostCallValue)
		gas += GasCostCallStipend
	}

	// Get the arguments.
	// TODO check the need for callvalue (seems not, a lot of that stuff is
	// already accounted for in the functions that I already called - need to
	// refactor all that)
	input := readSize(p, dataOffset, int(dataLength))
	acc := types3.Account{
		addr,
	}
	snapshot := in.StateDB.Snapshot()
	balance := new(big.Int)
	data,ok := in.ethvm.WriteSet.GetAddressAndKey(addr,BalanceKey)
	if !ok {
		balance = in.StateDB.GetBalance(acc)
	}else {
		balance = data.(*big.Int)
	}
	// Check that there is enough balance to transfer the value
	if balance.Cmp(value) < 0 {
		fmt.Printf("Not enough balance: wanted to use %v, got %v\n", value, in.StateDB.GetBalance(acc))
		return ErrEEICallFailure
	}

	// TODO tracing
	// TODO check that EIP-150 is respected

	// Load the contract code in a new VM structure
	targetContract := NewContract(AccountRef(contract.Address()), AccountRef(contract.Address()), value, uint64(gas))
	code,ok := in.ethvm.WriteSet.GetAddressAndKey(addr,SetCodeKey)
	if !ok {
		code = in.StateDB.GetCode(acc)
	}
	caddr := contract.Address()
	codeHash := crypto2.Keccak256Hash(code.([]byte))
	targetContract.SetCallCode(&caddr, codeHash, code.([]byte))

	return callCommon("callDelegate", p, in, contract, targetContract, input, value, snapshot, gas, false)
}

func callStatic(p *exec.Process, in *EWASMInterpreter, gas int64, addressOffset int32, dataOffset int32, dataLength int32) int32 {
	log.Trace("callStatic", "contract", in.contract.Address().Hex(), "caller", in.contract.CallerAddress.Hex())
	contract := in.contract

	// Get the address of the contract to call
	addr := types2.BytesToAddress(readSize(p, addressOffset, common2.AddressLength))

	value := big.NewInt(0)

	// Get the arguments.
	// TODO check the need for callvalue (seems not, a lot of that stuff is
	// already accounted for in the functions that I already called - need to
	// refactor all that)
	input := readSize(p, dataOffset, int(dataLength))

	snapshot := in.StateDB.Snapshot()

	in.gasAccounting(p, GasCostCall)

	if in.ethvm.GetDepeth() > maxCallDepth {
		return ErrEEICallFailure
	}

	acc := types3.Account{
		addr,
	}
	// Check that the contract exists
	if !in.StateDB.Exist(acc) {
		in.gasAccounting(p, GasCostNewAccount)
		in.StateDB.CreateAccount(acc)
	}

	//calleeGas := uint64(gas)
	//if calleeGas > ((63 * contract.Gas) / 64) {
	//	calleeGas -= ((63 * contract.Gas) / 64)
	//}
	//in.gasAccounting(p, calleeGas)

	// Load the contract code in a new VM structure
	targetContract := NewContract(contract, AccountRef(addr), value, uint64(gas))
	code := in.StateDB.GetCode(acc)
	if len(code) == 0 {
		in.contract.Gas += uint64(gas)
		return EEICallSuccess
	}
	targetContract.SetCallCode(&addr, in.StateDB.GetCodeHash(acc), code)

	if IsWASM(code) {
		savedVM := in.vm
		saveStatic := in.staticMode
		in.staticMode = true
		defer func() { in.staticMode = saveStatic }()

		in.Run(targetContract, input, false)

		in.vm = savedVM
		in.contract = contract

		// Add leftover gas
		in.contract.Gas += targetContract.Gas
	} else { // 非 WASM 合约，需要用 vm 去尝试执行
		addr := targetContract.Address()
		fmt.Println("callStatic", "----> WASM-->SOL START:", "from=", contract.Address().Hex(), ",to=", addr.Hex(), ",input=", len(input))
		ret, returnGas, err := in.ethvm.Call(contract, addr, input, uint64(gas), value)
		fmt.Println("callStatic", "<---- WASM-->SOL END:", "from=", contract.Address().Hex(), ",to=", addr.Hex(), "err=", err, "gas=", returnGas, ",result=", len(ret))
		if err != nil {
			in.terminationType = TerminateInvalid
		} else if err == ErrExecutionReverted {
			in.terminationType = TerminateRevert
			in.returnData = ret
		} else {
			in.terminationType = TerminateFinish
			in.returnData = ret
		}
		contract.Gas += returnGas
	}

	switch in.terminationType {
	case TerminateFinish:
		return EEICallSuccess
	case TerminateRevert:
		in.StateDB.RevertToSnapshot(snapshot)
		return ErrEEICallRevert
	default:
		in.StateDB.RevertToSnapshot(snapshot)
		contract.UseGas(targetContract.Gas)
		return ErrEEICallFailure
	}
}

func storageStore(p *exec.Process, interpreter *EWASMInterpreter, pathOffset int32, valueOffset int32) {
	log.Trace("storageStore", "contract", interpreter.contract.Address().Hex(), "caller", interpreter.contract.CallerAddress.Hex())

	if interpreter.staticMode == true {
		panic("Static mode violation in storageStore")
	}
	loc := types2.BytesToHash(readSize(p, pathOffset, u256Len))
	val := types2.BytesToHash(readSize(p, valueOffset, u256Len))

	//fmt.Println(val, loc)
	nonZeroBytes := 0
	for _, b := range val.Bytes() {
		if b != 0 {
			nonZeroBytes++
		}
	}
	var oldValue types2.Hash
	data,ok := interpreter.ethvm.WriteSet.GetAddressAndKey(interpreter.contract.Address(),loc)
	if !ok {
		oldValue = interpreter.StateDB.GetState(interpreter.contract.Account(), loc)
	}else {
		oldValue = types2.BytesToHash(data.([]byte))
	}
	oldNonZeroBytes := 0
	for _, b := range oldValue.Bytes() {
		if b != 0 {
			oldNonZeroBytes++
		}
	}

	if (nonZeroBytes > 0 && oldNonZeroBytes != nonZeroBytes) || (oldNonZeroBytes != 0 && nonZeroBytes == 0) {
		interpreter.gasAccounting(p, GasCostSSet)
	} else {
		// Refund for setting one value to 0 or if the "zeroness" remains
		// unchanged.
		interpreter.gasAccounting(p, GasCostSReset)
	}

	interpreter.ethvm.WriteSet.UpdateCache(interpreter.contract.Address(),loc,val.Bytes(),[]byte{})
	//interpreter.StateDB.SetState(interpreter.contract.Address(), loc, val)
	//fmt.Println(":: PUT :: <<ewasm_storageStore>>", "key=", loc.Hex(), "val=", val.Bytes())
}

func storageLoad(p *exec.Process, in *EWASMInterpreter, pathOffset int32, resultOffset int32) {
	log.Trace("storageLoad", "contract", in.contract.Address().Hex(), "caller", in.contract.CallerAddress.Hex())
	in.gasAccounting(p, 200)
	loc := types2.BytesToHash(readSize(p, pathOffset, u256Len))
	valBytes,ok := in.ethvm.WriteSet.GetAddressAndKey(in.contract.Address(),loc)
	if !ok {
		valBytes = in.StateDB.GetState(in.contract.Account(), loc).Bytes()
	}
	//valBytes := in.StateDB.GetState(in.contract.Address(), loc).Bytes()
	p.WriteAt(valBytes.([]byte), int64(resultOffset))
	in.ethvm.ReadSet.UpdateCache(in.contract.Address(),loc,valBytes)
	//fmt.Println(":: GET :: <<ewasm_storageLoad>>", "key=", loc.Hex(), "val=", valBytes)
}

func getCaller(p *exec.Process, in *EWASMInterpreter, resultOffset int32) {
	log.Trace("getCaller", "contract", in.contract.Address().Hex(), "caller", in.contract.CallerAddress.Hex())
	callerAddress := in.contract.CallerAddress
	in.gasAccounting(p, GasCostBase)
	p.WriteAt(callerAddress.Bytes(), int64(resultOffset))
}

func getCallValue(p *exec.Process, in *EWASMInterpreter, resultOffset int32) {
	log.Trace("getCallValue", "contract", in.contract.Address().Hex(), "caller", in.contract.CallerAddress.Hex())
	in.gasAccounting(p, GasCostBase)
	p.WriteAt(swapEndian(in.contract.Value().Bytes()), int64(resultOffset))
}

func codeCopy(p *exec.Process, in *EWASMInterpreter, resultOffset int32, codeOffset int32, length int32) {
	log.Trace("codeCopy", "contract", in.contract.Address().Hex(), "caller", in.contract.CallerAddress.Hex())
	in.gasAccounting(p, GasCostVeryLow+GasCostCopy*(uint64(length+31)>>5))
	code := in.contract.Code
	p.WriteAt(code[codeOffset:codeOffset+length], int64(resultOffset))
}

func getCodeSize(p *exec.Process, in *EWASMInterpreter) int32 {
	log.Trace("getCodeSize", "contract", in.contract.Address().Hex(), "caller", in.contract.CallerAddress.Hex())
	in.gasAccounting(p, GasCostBase)
	acc := types3.Account{
		*in.contract.CodeAddr,
	}
	code := in.StateDB.GetCode(acc)
	return int32(len(code))
}

func getBlockCoinbase(p *exec.Process, in *EWASMInterpreter, resultOffset int32) {
	log.Trace("getBlockCoinbase", "contract", in.contract.Address().Hex(), "caller", in.contract.CallerAddress.Hex())
	in.gasAccounting(p, GasCostBase)
	p.WriteAt(in.ethvm.Context.Coinbase.Bytes(), int64(resultOffset))
}

func sentinel(in *EWASMInterpreter, input []byte) ([]byte, uint64, error) {
	savedContract := in.contract
	savedVM := in.vm
	defer func() {
		in.contract = savedContract
		in.vm = savedVM
	}()

	meteringContractAddress := types2.BytesToAddress([]byte(sentinelContractAddress))
	//meteringCode := in.StateDB.GetCode(meteringContractAddress)
	if meteringCode == nil || len(meteringCode) == 0 {
		meteringCode, _ = hex.DecodeString(sentinelContractCode)
	}
	in.contract = NewContract(in.contract, AccountRef(meteringContractAddress), &big.Int{}, in.contract.Gas)
	in.contract.SetCallCode(&meteringContractAddress, crypto2.Keccak256Hash(meteringCode), meteringCode)
	vm, err := exec.NewVM(in.meteringModule)
	fmt.Println("sentinel.in.vm.NewVM", "err=", err)
	if err != nil {
		panic(fmt.Sprintf("Error allocating metering VM: %v", err))
	}
	vm.RecoverPanic = true
	in.vm = vm
	in.contract.Input = input
	meteredCode, err := in.vm.ExecCode(in.meteringStartIndex)
	if len(in.returnData) > 64 {
		var (
			sig = in.returnData[len(in.returnData)-64:]
			msg = in.returnData[:len(in.returnData)-64]
			p   = hexutil.MustDecode("0x4ed542e702d8208847e940847d2d4d65ded1b514d43eb52bbe57e70dc270f4b7")
		)
		fmt.Println("sentinel.in.vm.ExecCode", "err=", err, "return.len=", len(in.returnData), "sig=", in.returnData[len(in.returnData)-64:])
		fmt.Println("--->", "msg.len", len(msg))
		ret := ed25519.Verify(p, msg, sig)
		fmt.Println("result -->", ret)
	}
	if meteredCode == nil {
		meteredCode = in.returnData
	}

	var asBytes []byte
	if err == nil {
		asBytes = meteredCode.([]byte)
	}

	return asBytes, savedContract.Gas - in.contract.Gas, err
}

func create(p *exec.Process, in *EWASMInterpreter, valueOffset uint32, codeOffset uint32, length uint32, resultOffset uint32) int32 {
	log.Trace("create", "contract", in.contract.Address().Hex(), "caller", in.contract.CallerAddress.Hex())
	in.gasAccounting(p, GasCostCreate)
	savedVM := in.vm
	savedContract := in.contract
	defer func() {
		in.vm = savedVM
		in.contract = savedContract
	}()
	in.terminationType = TerminateInvalid

	if int(codeOffset)+int(length) > len(in.vm.Memory()) {
		return ErrEEICallFailure
	}
	input := readSize(p, int32(codeOffset), int(length))

	if (int(valueOffset) + u128Len) > len(in.vm.Memory()) {
		return ErrEEICallFailure
	}
	value := swapEndian(readSize(p, int32(valueOffset), u128Len))

	in.terminationType = TerminateFinish

	// EIP150 says that the calling contract should keep 1/64th of the
	// leftover gas.
	gas := in.contract.Gas - in.contract.Gas/64
	in.gasAccounting(p, gas)

	// TODO : 合约创建合约也要等待100块吗？那等待的过程中逻辑应该怎么写？
	input, err := EwasmFuncs.Sentinel(input)
	if err != nil {
		log.Error("sentinel fail in eei.create", "err", err)
		return ErrEEICallFailure
	}
	if len(input) < 5 {
		return ErrEEICallFailure
	}

	_, addr, gasLeft, _ := in.ethvm.Create(in.contract, input, gas, big.NewInt(0).SetBytes(value))

	switch in.terminationType {
	case TerminateFinish:
		savedContract.Gas += gasLeft
		p.WriteAt(addr.Bytes(), int64(resultOffset))
		return EEICallSuccess
	case TerminateRevert:
		savedContract.Gas += gas
		return ErrEEICallRevert
	default:
		savedContract.Gas += gasLeft
		return ErrEEICallFailure
	}
}

func getBlockDifficulty(p *exec.Process, in *EWASMInterpreter, resultOffset int32) {
	log.Trace("getBlockDifficulty", "contract", in.contract.Address().Hex(), "caller", in.contract.CallerAddress.Hex())
	in.gasAccounting(p, GasCostBase)
	p.WriteAt(swapEndian(in.ethvm.Context.Difficulty.Bytes()), int64(resultOffset))
}

func externalCodeCopy(p *exec.Process, in *EWASMInterpreter, addressOffset int32, resultOffset int32, codeOffset int32, length int32) {
	log.Trace("externalCodeCopy", "contract", in.contract.Address().Hex(), "caller", in.contract.CallerAddress.Hex())
	in.gasAccounting(p, params.ExtcodeCopyBaseEIP150+GasCostCopy*(uint64(length+31)>>5))
	addr := types2.BytesToAddress(readSize(p, addressOffset, common2.AddressLength))
	acc := types3.Account{
		addr,
	}
	code := in.StateDB.GetCode(acc)
	p.WriteAt(code[codeOffset:codeOffset+length], int64(resultOffset))
}

func getExternalCodeSize(p *exec.Process, in *EWASMInterpreter, addressOffset int32) int32 {
	log.Trace("getExternalCodeSize", "contract", in.contract.Address().Hex(), "caller", in.contract.CallerAddress.Hex())
	in.gasAccounting(p, params.ExtcodeSizeGasEIP150)
	addr := types2.BytesToAddress(readSize(p, addressOffset, common2.AddressLength))
	acc := types3.Account{
		addr,
	}
	code := in.StateDB.GetCode(acc)
	return int32(len(code))
}

func getGasLeft(p *exec.Process, in *EWASMInterpreter) int64 {
	log.Trace("getGasLeft", "contract", in.contract.Address().Hex(), "caller", in.contract.CallerAddress.Hex())
	in.gasAccounting(p, GasCostBase)
	return int64(in.contract.Gas)
}

func getBlockGasLimit(p *exec.Process, in *EWASMInterpreter) int64 {
	log.Trace("getBlockGasLimit", "contract", in.contract.Address().Hex(), "caller", in.contract.CallerAddress.Hex())
	in.gasAccounting(p, GasCostBase)
	return int64(in.ethvm.Context.GasLimit)
}

func getTxGasPrice(p *exec.Process, in *EWASMInterpreter, valueOffset int32) {
	log.Trace("getTxGasPrice", "contract", in.contract.Address().Hex(), "caller", in.contract.CallerAddress.Hex())
	in.gasAccounting(p, GasCostBase)
	p.WriteAt(in.ethvm.GasPrice.Bytes(), int64(valueOffset))
}

// It would be nice to be able to use variadic functions to pass the number of topics,
// however this imposes a change in wagon because the number of arguments is being
// checked when calling a function.
func _log(p *exec.Process, in *EWASMInterpreter, dataOffset int32, length int32, numberOfTopics int32, topic1 int32, topic2 int32, topic3 int32, topic4 int32) {
	log.Trace("_log", "contract", in.contract.Address().Hex(), "caller", in.contract.CallerAddress.Hex())
	in.gasAccounting(p, GasCostLog+GasCostLogData*uint64(length)+uint64(numberOfTopics)*GasCostLogTopic)

	// TODO need to add some info about the memory boundary on wagon
	if uint64(len(in.vm.Memory())) <= uint64(length)+uint64(dataOffset) {
		panic("out of memory")
	}
	data := readSize(p, dataOffset, int(uint32(length)))
	topics := make([]common2.Hash, numberOfTopics)

	if numberOfTopics > 4 || numberOfTopics < 0 {
		in.terminationType = TerminateInvalid
		p.Terminate()
	}

	// Variadic functions FTW
	if numberOfTopics > 0 {
		if uint64(len(in.vm.Memory())) <= uint64(topic1) {
			panic("out of memory")
		}
		topics[0] = common2.BigToHash(big.NewInt(0).SetBytes(readSize(p, topic1, u256Len)))
	}
	if numberOfTopics > 1 {
		if uint64(len(in.vm.Memory())) <= uint64(topic2) {
			panic("out of memory")
		}
		topics[1] = common2.BigToHash(big.NewInt(0).SetBytes(readSize(p, topic2, u256Len)))
	}
	if numberOfTopics > 2 {
		if uint64(len(in.vm.Memory())) <= uint64(topic3) {
			panic("out of memory")
		}
		topics[2] = common2.BigToHash(big.NewInt(0).SetBytes(readSize(p, topic3, u256Len)))
	}
	if numberOfTopics > 3 {
		if uint64(len(in.vm.Memory())) <= uint64(topic3) {
			panic("out of memory")
		}
		topics[3] = common2.BigToHash(big.NewInt(0).SetBytes(readSize(p, topic4, u256Len)))
	}

	metaLog := types3.NewMetaTxLog()
	metaLog.Address.SetBytes(in.contract.Address().Bytes())
	for _, topic := range topics {
		data := types2.BytesToHash(topic.Bytes())
		metaLog.Topics = append(metaLog.Topics, &basev1.MetaTopic{Topic: &data})
	}
	metaLog.Data = common2.CopyBytes(data)
	metaLog.BlockNumber = in.ethvm.Context.BlockNumber.Uint64()

	in.StateDB.AddLog(metaLog)
}

func getBlockNumber(p *exec.Process, in *EWASMInterpreter) int64 {
	log.Trace("getBlockNumber", "contract", in.contract.Address().Hex(), "caller", in.contract.CallerAddress.Hex())
	in.gasAccounting(p, GasCostBase)
	return in.ethvm.Context.BlockNumber.Int64()
}

func getTxOrigin(p *exec.Process, in *EWASMInterpreter, resultOffset int32) {
	log.Trace("getTxOrigin", "contract", in.contract.Address().Hex(), "caller", in.contract.CallerAddress.Hex())
	in.gasAccounting(p, GasCostBase)
	p.WriteAt(in.ethvm.Origin.Bytes(), int64(resultOffset))
}

func unWindContract(p *exec.Process, in *EWASMInterpreter, dataOffset int32, length int32) {
	log.Trace("unWindContract", "contract", in.contract.Address().Hex(), "caller", in.contract.CallerAddress.Hex())
	in.returnData = make([]byte, length)
	p.ReadAt(in.returnData, int64(dataOffset))
	//fmt.Println("<<ewasm_contract_finish>>", "addr=", in.contract.Address().Hex(), "input.len=", len(in.contract.Input), "output.len=", len(in.returnData), "input=", string(in.contract.Input), "output=", in.returnData)
}

func finish(p *exec.Process, in *EWASMInterpreter, dataOffset int32, length int32) {
	log.Trace("finish", "contract", in.contract.Address().Hex(), "caller", in.contract.CallerAddress.Hex())
	unWindContract(p, in, dataOffset, length)

	in.terminationType = TerminateFinish
	p.Terminate()
}

func revert(p *exec.Process, in *EWASMInterpreter, dataOffset int32, length int32) {
	log.Trace("revert", "contract", in.contract.Address().Hex(), "caller", in.contract.CallerAddress.Hex())
	unWindContract(p, in, dataOffset, length)

	in.terminationType = TerminateRevert
	p.Terminate()
}

func getReturnDataSize(p *exec.Process, in *EWASMInterpreter) int32 {
	log.Trace("getReturnDataSize", "contract", in.contract.Address().Hex(), "caller", in.contract.CallerAddress.Hex())
	in.gasAccounting(p, GasCostBase)
	return int32(len(in.returnData))
}

func returnDataCopy(p *exec.Process, in *EWASMInterpreter, resultOffset int32, dataOffset int32, length int32) {
	log.Trace("returnDataCopy", "contract", in.contract.Address().Hex(), "caller", in.contract.CallerAddress.Hex())
	in.gasAccounting(p, GasCostVeryLow+GasCostCopy*(uint64(length+31)>>5))
	p.WriteAt(in.returnData[dataOffset:dataOffset+length], int64(resultOffset))
}

func selfDestruct(p *exec.Process, in *EWASMInterpreter, addressOffset int32) {
	log.Trace("selfDestruct", "contract", in.contract.Address().Hex(), "caller", in.contract.CallerAddress.Hex())
	contract := in.contract
	mem := in.vm.Memory()

	balance := new(big.Int)
	data,ok := in.ethvm.WriteSet.GetAddressAndKey(contract.Address(),BalanceKey)
	if !ok {
		balance = in.StateDB.GetBalance(contract.Account())
	}else {
		balance = data.(*big.Int)
	}
	addr := types2.BytesToAddress(mem[addressOffset : addressOffset+common2.AddressLength])
	acc := types3.Account{
		addr,
	}
	totalGas := params.SelfdestructGasEIP150
	// If the destination address doesn't exist, add the account creation costs
	if in.StateDB.Empty(acc) && balance.Sign() != 0 {
		totalGas += params.CreateBySelfdestructGas
	}
	in.gasAccounting(p, totalGas)

	orgBalance := new(big.Int)
	value,ok := in.ethvm.WriteSet.GetAddressAndKey(addr,BalanceKey)
	if !ok {
		orgBalance = in.StateDB.GetBalance(acc)
	}else {
		orgBalance = value.(*big.Int)
	}

	in.ethvm.WriteSet.UpdateCache(addr,BalanceKey,balance,orgBalance)
	//in.StateDB.AddBalance(addr, balance)
	in.StateDB.Suicide(contract.Account())

	// Same as for `revert` and `return`, I need to forcefully terminate
	// the execution of the contract.
	in.terminationType = TerminateSuicide
	p.Terminate()
}

func getBlockTimestamp(p *exec.Process, in *EWASMInterpreter) int64 {
	log.Trace("getBlockTimestamp", "contract", in.contract.Address().Hex(), "caller", in.contract.CallerAddress.Hex())
	in.gasAccounting(p, GasCostBase)
	return in.ethvm.Context.Time.Int64()
}

// add by echo : 扩展 eei 持久化接口，提供大于 32byte 的 key / value 存储能力 >>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>

/*
pub fn ethereum_storageLoad2(
   keyOffset: *const u32, keyLength: u32,
   resultOffset: *const u32
);

key --stateDB--> key --> val
*/
func storageLoad2(p *exec.Process, interpreter *EWASMInterpreter, pathOffset, pathLen, resultOffset, resultType int32) {
	log.Trace("storageLoad2", "contract", interpreter.contract.Address().Hex(), "caller", interpreter.contract.CallerAddress.Hex())
	loc := readSize(p, pathOffset, int(pathLen))
	key := types2.BytesToHash(crypto.Keccak256(loc))

	var value types2.Hash
	data,ok := interpreter.ethvm.WriteSet.GetAddressAndKey(interpreter.contract.Address(),key)
	if !ok {
		value = interpreter.StateDB.GetState(interpreter.contract.Account(), key)
	}else {
		value = types2.BytesToHash(data.([]byte))
	}
	//valBytes := interpreter.StateDB.GetState(interpreter.contract.Address(), key)
	//valBytes := interpreter.StateDB.GetState(interpreter.contract.Address(), valkey)
	log.Debug("storageLoad2-params", "key", key.Hex(), "loc", loc, "val", value)
	//fmt.Println("load-loc", loc)
	//fmt.Println("storageLoad2-params", "key", key.Hex(), "loc", string(loc), "val", string(valBytes))
	b := int(math.Ceil(float64(len(value)) / 32.0))
	if b == 0 {
		//interpreter.gasTable.SLoad
		interpreter.gasAccounting(p, params.SloadGasEIP2200)
		log.Info("storageLoad2-gas", "b", b, "gas", params.SloadGasEIP2200)
	} else {
		interpreter.gasAccounting(p, params.SloadGasEIP2200*uint64(b))
		log.Info("storageLoad2-gas", "b", b, "gas", params.SloadGasEIP2200*uint64(b))
	}
	if resultType == 1 {
		// 取长度 : 转换为 32byte 右对齐 []byte
		l := big.NewInt(int64(len(value)))
		bb := make([]byte, 32, 32)
		if len(l.Bytes()) < 32 {
			ll := len(l.Bytes())
			copy(bb[32-ll:], l.Bytes()[:])
		}

		interpreter.ethvm.ReadSet.UpdateCache(interpreter.contract.Address(),key,bb)
		p.WriteAt(bb, int64(resultOffset))
		log.Info("storageLoad2-ret", "take_len", l, "bytes", bb)
	} else {
		// 取值
		interpreter.ethvm.ReadSet.UpdateCache(interpreter.contract.Address(),key,value.Bytes())
		p.WriteAt(value.Bytes(), int64(resultOffset))
		log.Info("storageLoad2-ret", "take_val=", value)
	}
}

/*
pub fn ethereum_storageStore2(
	keyOffset: *const u32, keyLength: u32,
	valueOffset: *const u32, valueLength: u32,
)

key --stateDB--> key --> val
*/
func storageStore2(p *exec.Process, interpreter *EWASMInterpreter, pathOffset, pathLen, valueOffset, valueLen int32) {
	log.Trace("storageStore2", "contract", interpreter.contract.Address().Hex(), "caller", interpreter.contract.CallerAddress.Hex())
	if interpreter.staticMode == true {
		panic("Static mode violation in storageStore")
	}
	loc := readSize(p, pathOffset, int(pathLen))
	key := types2.BytesToHash(crypto.Keccak256(loc))
	val := readSize(p, valueOffset, int(valueLen))
	//valkey := common2.BytesToHash(crypto.Keccak256(val))
	//log.Info("storageStore2-params", "key", key.Hex(), "loc", string(loc), "val", string(val))
	//fmt.Println("save-loc", loc)
	//fmt.Println("storageStore2-params", "key", key.Hex(), "loc", loc, "val", val)
	nonZeroBytes := 0
	for _, b := range val {
		if b != 0 {
			nonZeroBytes++
		}
	}

	var oldValue types2.Hash
	data,ok := interpreter.ethvm.WriteSet.GetAddressAndKey(interpreter.contract.Address(),key)
	if !ok {
		oldValue = interpreter.StateDB.GetState(interpreter.contract.Account(), key)
	}else {
		oldValue = types2.BytesToHash(data.([]byte))
	}
	oldNonZeroBytes := 0
	for _, b := range oldValue {
		if b != 0 {
			oldNonZeroBytes++
		}
	}

	b := int(math.Ceil(float64(len(val)) / 32.0))
	if (nonZeroBytes > 0 && oldNonZeroBytes != nonZeroBytes) || (oldNonZeroBytes != 0 && nonZeroBytes == 0) {
		interpreter.gasAccounting(p, uint64(GasCostSSet*b))
		//log.Info("storageStore2-sset", "b", b, "gas", GasCostSSet*b)
	} else {
		interpreter.gasAccounting(p, uint64(GasCostSReset*b))
		log.Info("storageStore2-sreset", "b", b, "gas", GasCostSReset*b)
	}
	interpreter.ethvm.WriteSet.UpdateCache(interpreter.contract.Address(),key,val,[]byte{})
	//interpreter.StateDB.SetState(interpreter.contract.Address(), key, types2.BytesToHash(val))
	//interpreter.StateDB.SetState(interpreter.contract.Address(),valkey,common2.BytesToHash(val))
}

// add by echo : 扩展 eei 持久化接口，提供大于 32byte 的 key / value 存储能力 <<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<
