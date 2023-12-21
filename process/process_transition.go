package process

import (
	"fmt"
	types2 "github.com/CaduceusMetaverseProtocol/MetaNebula/types"
	metatypes "github.com/CaduceusMetaverseProtocol/MetaTypes/types"
	cm "github.com/CaduceusMetaverseProtocol/MetaVM/common"
	"github.com/CaduceusMetaverseProtocol/MetaVM/vm/ethvm"
	"github.com/ethereum/go-ethereum/common"
	cmath "github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"math"
	"math/big"
)

var NonceKey = cm.StringToHash("nonce")

var BalanceKey = cm.StringToHash("balance")

var SubBalanceKey = cm.StringToHash("subBalance")

var SetCodeKey = cm.StringToHash("setCode")

var CreatAccountKey = cm.StringToHash("newAccount")

var emptyCodeHash = crypto.Keccak256Hash(nil)

/*
The Process Transitioning Model

A state transition is a change made when a transaction is applied to the current world state
The state transitioning model does all the necessary work to work out a valid new state root.

1) Nonce handling
2) Pre pay gas
3) Create a new state object if the recipient is \0*32
4) Value transfer
== If contract creation ==

	4a) Attempt to run transaction data
	4b) If valid, use result as code for the new state object

== end ==
5) Run Script section
6) Derive new state root
*/
type ProcessTransition struct {
	gp         *core.GasPool
	msg        Message
	gas        uint64
	gasPrice   *big.Int
	gasFeeCap  *big.Int
	gasTipCap  *big.Int
	initialGas uint64
	value      *big.Int
	data       []byte
	state      ethvm.StateDB
	vm         *ethvm.ETHVM
}

// Message represents a message sent to a contract.
type Message interface {
	From() metatypes.Address
	To() *metatypes.Address

	GasPrice() *big.Int
	GasFeeCap() *big.Int
	GasTipCap() *big.Int
	Gas() uint64
	Value() *big.Int

	Nonce() uint64
	IsFake() bool
	//CheckNonce() bool
	Data() []byte
	AccessList() types.AccessList
}

// ExecutionResult includes all output after executing given vm
// message no matter the execution itself is successful or not.
type ExecutionResult struct {
	UsedGas    uint64 // Total used gas but include the refunded gas
	Err        error  // Any error encountered during the execution(listed in core/vm/errors.go)
	ReturnData []byte // Returned data from vm(function result or data supplied with revert opcode)
}

// Unwrap returns the internal vm error which allows us for further
// analysis outside.
func (result *ExecutionResult) Unwrap() error {
	return result.Err
}

// Failed returns the indicator whether the execution is successful or not
func (result *ExecutionResult) Failed() bool { return result.Err != nil }

// Return is a helper function to help caller distinguish between revert reason
// and function return. Return returns the data after execution if no error occurs.
func (result *ExecutionResult) Return() []byte {
	if result.Err != nil {
		return nil
	}
	return common.CopyBytes(result.ReturnData)
}

// Revert returns the concrete revert reason if the execution is aborted by `REVERT`
// opcode. Note the reason can be nil if no data supplied with revert opcode.
func (result *ExecutionResult) Revert() []byte {
	if result.Err != ethvm.ErrExecutionReverted {
		return nil
	}
	return common.CopyBytes(result.ReturnData)
}

// IntrinsicGas computes the 'intrinsic gas' for a message with the given data.
func IntrinsicGas(data []byte, accessList types.AccessList, isContractCreation bool) (uint64, error) {
	// Set the starting gas for the raw transaction
	var gas uint64
	if isContractCreation {
		gas = params.TxGasContractCreation
	} else {
		gas = params.TxGas
	}
	// Bump the required gas by the amount of transactional data
	if len(data) > 0 {
		// Zero and non-zero bytes are priced differently
		var nz uint64
		for _, byt := range data {
			if byt != 0 {
				nz++
			}
		}
		// Make sure we don't exceed uint64 for all data combinations
		nonZeroGas := params.TxDataNonZeroGasEIP2028

		if (math.MaxUint64-gas)/nonZeroGas < nz {
			return 0, core.ErrGasUintOverflow
		}
		gas += nz * nonZeroGas

		z := uint64(len(data)) - nz
		if (math.MaxUint64-gas)/params.TxDataZeroGas < z {
			return 0, core.ErrGasUintOverflow
		}
		gas += z * params.TxDataZeroGas
	}
	if accessList != nil {
		gas += uint64(len(accessList)) * params.TxAccessListAddressGas
		gas += uint64(accessList.StorageKeys()) * params.TxAccessListStorageKeyGas
	}
	return gas, nil
}

// NewProcessTransition initialises and returns a new state transition object.
func NewProcessTransition(vm *ethvm.ETHVM, msg Message, gp *core.GasPool) *ProcessTransition {
	return &ProcessTransition{
		gp:        gp,
		vm:        vm,
		msg:       msg,
		//TODO DELETE this
		gas:        100000000,
		gasPrice:  msg.GasPrice(),
		gasFeeCap: msg.GasFeeCap(),
		gasTipCap: msg.GasTipCap(),
		value:     msg.Value(),
		data:      msg.Data(),
		state:     vm.StateDB,
	}
}

// ApplyMessage computes the new state by applying the given message
// against the old state within the environment.
//
// ApplyMessage returns the bytes returned by any EVM execution (if it took place),
// the gas used (which includes gas refunds) and an error if it failed. An error always
// indicates a core error meaning that the message would always fail for that particular
// state and would never be accepted within a block.
func ApplyMessage(vm *ethvm.ETHVM, msg Message, gp *core.GasPool) (*ExecutionResult, error) {
	return NewProcessTransition(vm, msg, gp).TransitionDb()
}

func ReApplyMessage(vm *ethvm.ETHVM, msg Message, gp *core.GasPool, gas uint64) (*ExecutionResult, error) {
	return NewProcessTransition(vm, msg, gp).ReTransitionDb()
}

// to returns the recipient of the message.
func (pt *ProcessTransition) to() metatypes.Address {
	if pt.msg == nil || pt.msg.To() == nil /* contract creation */ {
		return metatypes.Address{}
	}
	return *pt.msg.To()
}

func (pt *ProcessTransition) buyGas() error {
	mgval := new(big.Int).SetUint64(pt.msg.Gas())
	mgval = mgval.Mul(mgval, pt.gasPrice)
	balanceCheck := mgval
	if pt.gasFeeCap != nil {
		balanceCheck = new(big.Int).SetUint64(pt.msg.Gas())
		balanceCheck = balanceCheck.Mul(balanceCheck, pt.gasFeeCap)
		balanceCheck.Add(balanceCheck, pt.value)
	}

	var have = new(big.Int)
	haveByte,ok := pt.vm.WriteSet.GetAddressAndKey(pt.msg.From(),BalanceKey)
	if !ok {
		pt.state.SetBalance(types2.Account{pt.msg.From()},new(big.Int).Add(balanceCheck,balanceCheck))
		have = pt.state.GetBalance(types2.Account{pt.msg.From()})
	}else {
		have = haveByte.(*big.Int)
	}
	//log.Info("buyGas-----","have:",have.Uint64(),"want:",balanceCheck.Uint64(),"address:",pt.msg.From().String())
	if want := balanceCheck; have.Cmp(want) < 0 {
		return fmt.Errorf("%w: address %v have %v want %v", core.ErrInsufficientFunds, pt.msg.From().Hex(), have, want)
	}
	pt.vm.ReadSet.UpdateCache(pt.msg.From(), BalanceKey, have)

	if err := pt.gp.SubGas(pt.msg.Gas()); err != nil {
		return err
	}
	pt.gas += pt.msg.Gas()
	pt.initialGas = pt.msg.Gas()
	pt.vm.WriteSet.UpdateCache(pt.msg.From(), SubBalanceKey, mgval, []byte{})
	//modify by echo
	//pt.state.SubBalance(pt.msg.From(), mgval)
	return nil
}

func (pt *ProcessTransition) preCheck() error {
	//// Only check transactions that are not fake
	if !pt.msg.IsFake() {
		// Make sure this transaction's nonce is correct.
		stNonce := pt.state.GetNonce(types2.Account{pt.msg.From()})
		pt.vm.ReadSet.UpdateCache(pt.msg.From(), NonceKey, stNonce)

		if msgNonce := pt.msg.Nonce(); stNonce < msgNonce {
			return fmt.Errorf("%w: address %v, tx: %d state: %d", core.ErrNonceTooHigh,
				pt.msg.From().Hex(), msgNonce, stNonce)
		} else if stNonce > msgNonce {
			return fmt.Errorf("%w: address %v, tx: %d state: %d", core.ErrNonceTooLow,
				pt.msg.From().Hex(), msgNonce, stNonce)
		}
	}
	// Make sure that transaction gasFeeCap is greater than the baseFee (post london)

	// Skip the checks if gas fields are zero and baseFee was explicitly disabled (eth_call)
	if !pt.vm.Config.NoBaseFee || pt.gasFeeCap.BitLen() > 0 || pt.gasTipCap.BitLen() > 0 {
		if l := pt.gasFeeCap.BitLen(); l > 256 {
			return fmt.Errorf("%w: address %v, maxFeePerGas bit length: %d", core.ErrFeeCapVeryHigh,
				pt.msg.From().Hex(), l)
		}
		if l := pt.gasTipCap.BitLen(); l > 256 {
			return fmt.Errorf("%w: address %v, maxPriorityFeePerGas bit length: %d", core.ErrTipVeryHigh,
				pt.msg.From().Hex(), l)
		}
		if pt.gasFeeCap.Cmp(pt.gasTipCap) < 0 {
			return fmt.Errorf("%w: address %v, maxPriorityFeePerGas: %s, maxFeePerGas: %s", core.ErrTipAboveFeeCap,
				pt.msg.From().Hex(), pt.gasTipCap, pt.gasFeeCap)
		}
		// This will panic if baseFee is nil, but basefee presence is verified
		// as part of header validation.
		if pt.gasFeeCap.Cmp(pt.vm.Context.BaseFee) < 0 {
			return fmt.Errorf("%w: address %v, maxFeePerGas: %s baseFee: %s", core.ErrFeeCapTooLow,
				pt.msg.From().Hex(), pt.gasFeeCap, pt.vm.Context.BaseFee)
		}
	}

	return pt.buyGas()
}

// TransitionDb will transition the state by applying the current message and
// returning the vm execution result with following fields.
//
//   - used gas:
//     total gas used (including gas being refunded)
//   - returndata:
//     the returned data from vm
//   - concrete execution error:
//     various **EVM** error which aborts the execution,
//     e.g. ErrOutOfGas, ErrExecutionReverted
//
// However if any consensus issue encountered, return the error directly with
// nil vm execution result.
func (pt *ProcessTransition) TransitionDb() (*ExecutionResult, error) {
	// First check this message satisfies all consensus rules before
	// applying the message. The rules include these clauses
	//
	// 1. the nonce of the message caller is correct
	// 2. caller has enough balance to cover transaction fee(gaslimit * gasprice)
	// 3. the amount of gas required is available in the block
	// 4. the purchased gas is enough to cover intrinsic usage
	// 5. there is no overflow when calculating intrinsic gas
	// 6. caller has enough balance to cover asset transfer for **topmost** call

	// Check clauses 1-3, buy gas if everything is correct
	if err := pt.preCheck(); err != nil {
		return &ExecutionResult{
			UsedGas:    0,
			Err:        err,
			ReturnData: nil,
		}, err
	}
	msg := pt.msg
	sender := ethvm.AccountRef(msg.From())
	contractCreation := msg.To() == nil
	// Check clauses 4-5, subtract intrinsic gas if everything is correct
	gas, err := IntrinsicGas(pt.data, pt.msg.AccessList(), contractCreation)
	if err != nil {
		println("err:",err.Error())
		return &ExecutionResult{
			UsedGas:    0,
			Err:        err,
			ReturnData: nil,
		}, err
	}
	//TODO  fix this
	if pt.gas < gas {
		return &ExecutionResult{
			UsedGas:    0,
			Err:        fmt.Errorf("%w: have %d, want %d", core.ErrIntrinsicGas, pt.gas, gas),
			ReturnData: nil,
		}, fmt.Errorf("%w: have %d, want %d", core.ErrIntrinsicGas, pt.gas, gas)
	}
	pt.gas -= gas

	//if gas != 0 {
	//  no need subbalance because gas has cost in preCheck-> buyGas
	//	pt.vm.WriteSet.UpdateCache(msg.From(),SubBalanceKey,new(big.Int).SetUint64(gas).Bytes(),[]byte{})
	//}

	// Check clause 6
	if msg.Value().Sign() > 0 && !pt.vm.Context.CanTransfer(pt.state, msg.From(), msg.Value()) {
		return &ExecutionResult{
			UsedGas:    params.CallStipend,
			Err:        core.ErrInsufficientFundsForTransfer,
			ReturnData: nil,
		}, nil
	}

	balance := pt.state.GetBalance(types2.Account{msg.From()})
	pt.vm.ReadSet.UpdateCache(msg.From(), BalanceKey, balance)
	// Set up the initial access list.
	//pt.vm.ChainConfig().Rules(pt.vm.Context.BlockNumber, pt.vm.Context.Random != nil)

	//TODO here is state prepareAccessList should be fulled
	//rules := pt.vm.ChainConfig().Rules(pt.vm.Context.BlockNumber, false)
	//pt.state.PrepareAccessList(msg.From(), msg.To(), ethvm.ActivePrecompiles(rules), msg.AccessList())

	var (
		ret   []byte
		vmerr error // vm errors do not effect consensus and are therefore not assigned to err
	)
	if contractCreation {
		ret, _, pt.gas, vmerr = pt.vm.Create(sender, pt.data, pt.gas, pt.value)
	} else {
		// Increment the nonce for the next transaction
		val := pt.state.GetNonce(types2.Account{sender.Address()}) + 1
		pt.vm.WriteSet.UpdateCache(msg.From(), NonceKey, val, []byte{})
		ret, pt.gas, vmerr = pt.vm.Call(sender, pt.to(), pt.data, pt.gas, pt.value)
	}

	// After EIP-3529: refunds are capped to gasUsed / 5
	pt.refundGas(params.RefundQuotientEIP3529)

	effectiveTip := pt.gasPrice
	effectiveTip = cmath.BigMin(pt.gasTipCap, new(big.Int).Sub(pt.gasFeeCap, pt.vm.Context.BaseFee))

	if effectiveTip.Uint64() != 0 {
		val := pt.vm.StateDB.GetBalance(types2.Account{pt.vm.Context.Coinbase})
		v := new(big.Int).Mul(new(big.Int).SetUint64(pt.gasUsed()), effectiveTip)
		pt.vm.WriteSet.UpdateCache(pt.vm.Context.Coinbase, BalanceKey, v, val)
	}

	return &ExecutionResult{
		UsedGas: 1,
		//TODO modify this
		//UsedGas:    pt.gasUsed(),
		Err:        vmerr,
		ReturnData: ret,
	}, nil
}


func (pt *ProcessTransition) ReTransitionDb() (*ExecutionResult, error) {
	err := pt.buyGas()
	if err != nil {
		return &ExecutionResult{
			UsedGas:    pt.gasUsed(),
			Err:        err,
			ReturnData: []byte{},
		}, err
	}
	msg := pt.msg
	sender := ethvm.AccountRef(msg.From())
	contractCreation := msg.To() == nil

	// Check clauses 4-5, subtract intrinsic gas if everything is correct
	gas, err := IntrinsicGas(pt.data, pt.msg.AccessList(), contractCreation)
	if err != nil {
		return &ExecutionResult{
			UsedGas:    0,
			Err:        err,
			ReturnData: nil,
		}, err
	}
	if pt.gas < gas {
		return &ExecutionResult{
			UsedGas:    0,
			Err:       fmt.Errorf("%w: have %d, want %d", core.ErrIntrinsicGas, pt.gas, gas),
			ReturnData: nil,
		}, fmt.Errorf("%w: have %d, want %d", core.ErrIntrinsicGas, pt.gas, gas)
	}
	pt.gas -= gas
	var canTransferFlag bool
	dataVal, ok := pt.vm.WriteSet.GetAddressAndKey(msg.From(), BalanceKey)
	if ok {
		if dataVal.(*big.Int).Uint64() >= msg.Value().Uint64() {
			canTransferFlag = true
		}
	} else {
		canTransferFlag = pt.vm.Context.CanTransfer(pt.state, msg.From(), msg.Value())
	}
	if msg.Value().Sign() > 0 && !canTransferFlag {
		return &ExecutionResult{
			UsedGas:    0,
			Err:        fmt.Errorf("%w: address %v", core.ErrInsufficientFundsForTransfer, msg.From().Hex()),
			ReturnData: nil,
		}, fmt.Errorf("%w: address %v", core.ErrInsufficientFundsForTransfer, msg.From().Hex())
	}

	pt.vm.WriteSet.UpdateCache(msg.From(),SubBalanceKey,msg.Value(),[]byte{})

	var (
		ret   []byte
		vmerr error // vm errors do not effect consensus and are therefore not assigned to err
	)
	if contractCreation {
		ret, _, pt.gas, vmerr = pt.vm.Create(sender, pt.data, pt.gas, pt.value)
	} else {
		// Increment the nonce for the next transaction
		var senderNonce uint64
		val, ok := pt.vm.WriteSet.GetAddressAndKey(msg.From(), NonceKey)
		if !ok {
			senderNonce = pt.state.GetNonce(types2.Account{sender.Address()}) + 1
		} else {
			senderNonce = val.(uint64)
		}
		pt.vm.WriteSet.UpdateCache(msg.From(), NonceKey, senderNonce, []byte{})
		//modify by echo
		//pt.state.SetNonce(msg.From(), pt.state.GetNonce(sender.Address())+1)
		ret, pt.gas, vmerr = pt.vm.Call(sender, pt.to(), pt.data, pt.gas, pt.value)
	}

	// After EIP-3529: refunds are capped to gasUsed / 5
	pt.refundGas(params.RefundQuotientEIP3529)

	effectiveTip := pt.gasPrice
	effectiveTip = cmath.BigMin(pt.gasTipCap, new(big.Int).Sub(pt.gasFeeCap, pt.vm.Context.BaseFee))

	if effectiveTip.Uint64() != 0 {
		var balanceVal = new(big.Int)
		dataVal, ok := pt.vm.WriteSet.GetAddressAndKey(pt.vm.Context.Coinbase, BalanceKey)
		if !ok {
			balanceVal = pt.vm.StateDB.GetBalance(types2.Account{pt.vm.Context.Coinbase})
		} else {
			balanceVal = dataVal.(*big.Int)
		}
		pt.vm.WriteSet.UpdateCache(pt.vm.Context.Coinbase,BalanceKey,new(big.Int).Mul(new(big.Int).SetUint64(pt.gasUsed()), effectiveTip),balanceVal)
	}

	return &ExecutionResult{
		UsedGas:    pt.gasUsed(),
		Err:        vmerr,
		ReturnData: ret,
	}, nil
}

func (pt *ProcessTransition) refundGas(refundQuotient uint64) {
	// Apply refund counter, capped to a refund quotient
	refund := pt.gasUsed() / refundQuotient
	if refund > pt.state.GetRefund() {
		refund = pt.state.GetRefund()
	}
	pt.gas += refund

	// Return ETH for remaining gas, exchanged at the original rate.
	remaining := new(big.Int).Mul(new(big.Int).SetUint64(pt.gas), pt.gasPrice)
	if remaining.Uint64() != 0 {
		balanVal, ok := pt.vm.WriteSet.GetAddressAndKey(pt.msg.From(), BalanceKey)
		if !ok {
			balanVal = pt.state.GetBalance(types2.Account{pt.msg.From()})
		}
		pt.vm.WriteSet.UpdateCache(pt.msg.From(), BalanceKey, remaining, balanVal)
	}
	//modify by echo
	//pt.state.AddBalance(pt.msg.From(), remaining)

	// Also return remaining gas to the block gas counter so it is
	// available for the next transaction.
	pt.gp.AddGas(pt.gas)
}

// gasUsed returns the amount of gas used up by the state transition.
func (pt *ProcessTransition) gasUsed() uint64 {
	return pt.initialGas - pt.gas
}
