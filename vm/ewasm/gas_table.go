package ewasm

import (
	"errors"
	"github.com/CaduceusMetaverseProtocol/MetaNebula/types"
	metatypes "github.com/CaduceusMetaverseProtocol/MetaTypes/types"
	"github.com/CaduceusMetaverseProtocol/MetaVM/vm/ethvm"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/CaduceusMetaverseProtocol/MetaVM/params"
)

// memoryGasCost calculates the quadratic gas for memory expansion. It does so
// only for the memory region that is expanded, not the total memory.
func memoryGasCost(mem *ethvm.Memory, newMemSize uint64) (uint64, error) {
	if newMemSize == 0 {
		return 0, nil
	}
	// The maximum that will fit in a uint64 is max_word_count - 1. Anything above
	// that will result in an overflow. Additionally, a newMemSize which results in
	// a newMemSizeWords larger than 0xFFFFFFFF will cause the square operation to
	// overflow. The constant 0x1FFFFFFFE0 is the highest number that can be used
	// without overflowing the gas calculation.
	if newMemSize > 0x1FFFFFFFE0 {
		return 0, ethvm.ErrGasUintOverflow
	}
	newMemSizeWords := ethvm.ToWordSize(newMemSize)
	newMemSize = newMemSizeWords * 32

	if newMemSize > uint64(mem.Len()) {
		square := newMemSizeWords * newMemSizeWords
		linCoef := newMemSizeWords * params.MemoryGas
		quadCoef := square / params.QuadCoeffDiv
		newTotalFee := linCoef + quadCoef

		fee := newTotalFee - mem.GetLastGasCost()
		mem.SetLastGasCost(newTotalFee)
		return fee, nil
	}
	return 0, nil
}

// memoryCopierGas creates the gas functions for the following opcodes, and takes
// the stack position of the operand which determines the size of the data to copy
// as argument:
// CALLDATACOPY (stack position 2)
// CODECOPY (stack position 2)
// EXTCODECOPY (stack position 3)
// RETURNDATACOPY (stack position 2)
func memoryCopierGas(stackpos int) gasFunc {
	return func(vm *ethvm.ETHVM, contract *Contract, stack *ethvm.Stack, mem *ethvm.Memory, memorySize uint64) (uint64, error) {
		// Gas for expanding the memory
		gas, err := memoryGasCost(mem, memorySize)
		if err != nil {
			return 0, err
		}
		// And gas for copying data, charged per word at param.CopyGas
		words, overflow := stack.Back(stackpos).Uint64WithOverflow()
		if overflow {
			return 0, ethvm.ErrGasUintOverflow
		}

		if words, overflow = math.SafeMul(ethvm.ToWordSize(words), params.CopyGas); overflow {
			return 0, ethvm.ErrGasUintOverflow
		}

		if gas, overflow = math.SafeAdd(gas, words); overflow {
			return 0, ethvm.ErrGasUintOverflow
		}
		return gas, nil
	}
}

var (
	gasCallDataCopy   = memoryCopierGas(2)
	gasCodeCopy       = memoryCopierGas(2)
	gasExtCodeCopy    = memoryCopierGas(3)
	gasReturnDataCopy = memoryCopierGas(2)

	gasSelfdestructEIP3529 = makeSelfdestructGasFn(false)
	gasCallCodeEIP2929     = makeCallVariantGasCallEIP2929(gasCallCode)
	gasCallEIP2929         = makeCallVariantGasCallEIP2929(gasCall)
	gasDelegateCallEIP2929 = makeCallVariantGasCallEIP2929(gasDelegateCall)
	gasStaticCallEIP2929   = makeCallVariantGasCallEIP2929(gasStaticCall)
	gasSStoreEIP3529 = makeGasSStoreFunc(params.SstoreClearsScheduleRefundEIP3529)
)


// makeSelfdestructGasFn can create the selfdestruct dynamic gas function for EIP-2929 and EIP-2539
func makeSelfdestructGasFn(refundsEnabled bool) gasFunc {
	gasFunc := func(vm *ethvm.ETHVM, contract *Contract, stack *ethvm.Stack, mem *ethvm.Memory, memorySize uint64) (uint64, error) {
		var (
			gas     uint64
			address = metatypes.Address(stack.Peek().Bytes20())
		)
		if !vm.StateDB.AddressInAccessList(address) {
			// If the caller cannot afford the cost, this change will be rolled back
			vm.StateDB.AddAddressToAccessList(address)
			gas = params.ColdAccountAccessCostEIP2929
		}
		// if empty and transfers value
		if vm.StateDB.Empty(types.Account{address}) && vm.StateDB.GetBalance(contract.Account()).Sign() != 0 {
			gas += params.CreateBySelfdestructGas
		}
		if refundsEnabled && !vm.StateDB.HasSuicided(contract.Account()) {
			vm.StateDB.AddRefund(params.SelfdestructRefundGas)
		}
		return gas, nil
	}
	return gasFunc
}


func gasEip2929AccountCheck(vm *ethvm.ETHVM, contract *Contract, stack *ethvm.Stack, mem *ethvm.Memory, memorySize uint64) (uint64, error) {
	addr := metatypes.Address(stack.Peek().Bytes20())
	// Check slot presence in the access list
	if !vm.StateDB.AddressInAccessList(addr) {
		// If the caller cannot afford the cost, this change will be rolled back
		vm.StateDB.AddAddressToAccessList(addr)
		// The warm storage read cost is already charged as constantGas
		return params.ColdAccountAccessCostEIP2929 - params.WarmStorageReadCostEIP2929, nil
	}
	return 0, nil
}


func makeGasSStoreFunc(clearingRefund uint64) gasFunc {
	return func(vm *ethvm.ETHVM, contract *Contract, stack *ethvm.Stack, mem *ethvm.Memory, memorySize uint64) (uint64, error) {
		// If we fail the minimum gas availability invariant, fail (0)
		if contract.Gas <= params.SstoreSentryGasEIP2200 {
			return 0, errors.New("not enough gas for reentrancy sentry")
		}
		// Gas sentry honoured, do the actual gas calculation based on the stored value
		var (
			y, x    = stack.Back(1), stack.Peek()
			slot    = metatypes.Hash(x.Bytes32())
			current = vm.StateDB.GetState(contract.Account(), slot)
			cost    = uint64(0)
		)
		// Check slot presence in the access list
		if addrPresent, slotPresent := vm.StateDB.SlotInAccessList(contract.Address(), slot); !slotPresent {
			cost = params.ColdSloadCostEIP2929
			// If the caller cannot afford the cost, this change will be rolled back
			vm.StateDB.AddSlotToAccessList(contract.Address(), slot)
			if !addrPresent {
				// Once we're done with YOLOv2 and schedule this for mainnet, might
				// be good to remove this panic here, which is just really a
				// canary to have during testing
				panic("impossible case: address was not present in access list during sstore op")
			}
		}
		value := metatypes.Hash(y.Bytes32())

		if current == value { // noop (1)
			// EIP 2200 original clause:
			//		return params.SloadGasEIP2200, nil
			return cost + params.WarmStorageReadCostEIP2929, nil // SLOAD_GAS
		}
		original := vm.StateDB.GetCommittedState(contract.Account(), x.Bytes32())
		if original == current {
			if original == (metatypes.Hash{}) { // create slot (2.1.1)
				return cost + params.SstoreSetGasEIP2200, nil
			}
			if value == (metatypes.Hash{}) { // delete slot (2.1.2b)
				vm.StateDB.AddRefund(clearingRefund)
			}
			// EIP-2200 original clause:
			//		return params.SstoreResetGasEIP2200, nil // write existing slot (2.1.2)
			return cost + (params.SstoreResetGasEIP2200 - params.ColdSloadCostEIP2929), nil // write existing slot (2.1.2)
		}
		if original != (metatypes.Hash{}) {
			if current == (metatypes.Hash{}) { // recreate slot (2.2.1.1)
				vm.StateDB.SubRefund(clearingRefund)
			} else if value == (metatypes.Hash{}) { // delete slot (2.2.1.2)
				vm.StateDB.AddRefund(clearingRefund)
			}
		}
		if original == value {
			if original == (metatypes.Hash{}) { // reset to original inexistent slot (2.2.2.1)
				// EIP 2200 Original clause:
				//vm.StateDB.AddRefund(params.SstoreSetGasEIP2200 - params.SloadGasEIP2200)
				vm.StateDB.AddRefund(params.SstoreSetGasEIP2200 - params.WarmStorageReadCostEIP2929)
			} else { // reset to original existing slot (2.2.2.2)
				// EIP 2200 Original clause:
				//	vm.StateDB.AddRefund(params.SstoreResetGasEIP2200 - params.SloadGasEIP2200)
				// - SSTORE_RESET_GAS redefined as (5000 - COLD_SLOAD_COST)
				// - SLOAD_GAS redefined as WARM_STORAGE_READ_COST
				// Final: (5000 - COLD_SLOAD_COST) - WARM_STORAGE_READ_COST
				vm.StateDB.AddRefund((params.SstoreResetGasEIP2200 - params.ColdSloadCostEIP2929) - params.WarmStorageReadCostEIP2929)
			}
		}
		// EIP-2200 original clause:
		//return params.SloadGasEIP2200, nil // dirty update (2.2)
		return cost + params.WarmStorageReadCostEIP2929, nil // dirty update (2.2)
	}
}

func makeCallVariantGasCallEIP2929(oldCalculator gasFunc) gasFunc {
	return func(vm *ethvm.ETHVM, contract *Contract, stack *ethvm.Stack, mem *ethvm.Memory, memorySize uint64) (uint64, error) {
		addr := metatypes.Address(stack.Back(1).Bytes20())
		// Check slot presence in the access list
		warmAccess := vm.StateDB.AddressInAccessList(addr)
		// The WarmStorageReadCostEIP2929 (100) is already deducted in the form of a constant cost, so
		// the cost to charge for cold access, if any, is Cold - Warm
		coldCost := params.ColdAccountAccessCostEIP2929 - params.WarmStorageReadCostEIP2929
		if !warmAccess {
			vm.StateDB.AddAddressToAccessList(addr)
			// Charge the remaining difference here already, to correctly calculate available
			// gas for call
			if !contract.UseGas(coldCost) {
				return 0, ethvm.ErrOutOfGas
			}
		}
		// Now call the old calculator, which takes into account
		// - create new account
		// - transfer value
		// - memory expansion
		// - 63/64ths rule
		gas, err := oldCalculator(vm, contract, stack, mem, memorySize)
		if warmAccess || err != nil {
			return gas, err
		}
		// In case of a cold access, we temporarily add the cold charge back, and also
		// add it to the returned gas. By adding it to the return, it will be charged
		// outside of this function, as part of the dynamic gas, and that will make it
		// also become correctly reported to tracers.
		contract.Gas += coldCost
		return gas + coldCost, nil
	}
}

// gasSLoadEIP2929 calculates dynamic gas for SLOAD according to EIP-2929
// For SLOAD, if the (address, storage_key) pair (where address is the address of the contract
// whose storage is being read) is not yet in accessed_storage_keys,
// charge 2100 gas and add the pair to accessed_storage_keys.
// If the pair is already in accessed_storage_keys, charge 100 gas.
func gasSLoadEIP2929(vm *ethvm.ETHVM, contract *Contract, stack *ethvm.Stack, mem *ethvm.Memory, memorySize uint64) (uint64, error) {
	loc := stack.Peek()
	slot := metatypes.Hash(loc.Bytes32())
	// Check slot presence in the access list
	if _, slotPresent := vm.StateDB.SlotInAccessList(contract.Address(), slot); !slotPresent {
		// If the caller cannot afford the cost, this change will be rolled back
		// If he does afford it, we can skip checking the same thing later on, during execution
		vm.StateDB.AddSlotToAccessList(contract.Address(), slot)
		return params.ColdSloadCostEIP2929, nil
	}
	return params.WarmStorageReadCostEIP2929, nil
}


// gasExtCodeCopyEIP2929 implements extcodecopy according to EIP-2929
// EIP spec:
// > If the target is not in accessed_addresses,
// > charge COLD_ACCOUNT_ACCESS_COST gas, and add the address to accessed_addresses.
// > Otherwise, charge WARM_STORAGE_READ_COST gas.
func gasExtCodeCopyEIP2929(vm *ethvm.ETHVM, contract *Contract, stack *ethvm.Stack, mem *ethvm.Memory, memorySize uint64) (uint64, error) {
	// memory expansion first (dynamic part of pre-2929 implementation)
	gas, err := gasExtCodeCopy(vm, contract, stack, mem, memorySize)
	if err != nil {
		return 0, err
	}
	addr := metatypes.Address(stack.Peek().Bytes20())
	// Check slot presence in the access list
	if !vm.StateDB.AddressInAccessList(addr) {
		vm.StateDB.AddAddressToAccessList(addr)
		var overflow bool
		// We charge (cold-warm), since 'warm' is already charged as constantGas
		if gas, overflow = math.SafeAdd(gas, params.ColdAccountAccessCostEIP2929-params.WarmStorageReadCostEIP2929); overflow {
			return 0, ethvm.ErrGasUintOverflow
		}
		return gas, nil
	}
	return gas, nil
}


func gasSStore(vm *ethvm.ETHVM, contract *Contract, stack *ethvm.Stack, mem *ethvm.Memory, memorySize uint64) (uint64, error) {
	var (
		y, x    = stack.Back(1), stack.Back(0)
		current = vm.StateDB.GetState(contract.Account(), x.Bytes32())
	)
	//// The legacy gas metering only takes into consideration the current state
	//// Legacy rules should be applied if we are in Petersburg (removal of EIP-1283)
	//// OR Constantinople is not active
	//if vm.GetChainRules().IsPetersburg || !vm.GetChainRules().IsConstantinople {
	//	// This checks for 3 scenario's and calculates gas accordingly:
	//	//
	//	// 1. From a zero-value address to a non-zero value         (NEW VALUE)
	//	// 2. From a non-zero value address to a zero-value address (DELETE)
	//	// 3. From a non-zero to a non-zero                         (CHANGE)
	//	switch {
	//	case current == (common.Hash{}) && y.Sign() != 0: // 0 => non 0
	//		return params.SstoreSetGas, nil
	//	case current != (common.Hash{}) && y.Sign() == 0: // non 0 => 0
	//		vm.StateDB.AddRefund(params.SstoreRefundGas)
	//		return params.SstoreClearGas, nil
	//	default: // non 0 => non 0 (or 0 => 0)
	//		return params.SstoreResetGas, nil
	//	}
	//}
	// The new gas metering is based on net gas costs (EIP-1283):
	//
	// 1. If current value equals new value (this is a no-op), 200 gas is deducted.
	// 2. If current value does not equal new value
	//   2.1. If original value equals current value (this storage slot has not been changed by the current execution context)
	//     2.1.1. If original value is 0, 20000 gas is deducted.
	// 	   2.1.2. Otherwise, 5000 gas is deducted. If new value is 0, add 15000 gas to refund counter.
	// 	2.2. If original value does not equal current value (this storage slot is dirty), 200 gas is deducted. Apply both of the following clauses.
	// 	  2.2.1. If original value is not 0
	//       2.2.1.1. If current value is 0 (also means that new value is not 0), remove 15000 gas from refund counter. We can prove that refund counter will never go below 0.
	//       2.2.1.2. If new value is 0 (also means that current value is not 0), add 15000 gas to refund counter.
	// 	  2.2.2. If original value equals new value (this storage slot is reset)
	//       2.2.2.1. If original value is 0, add 19800 gas to refund counter.
	// 	     2.2.2.2. Otherwise, add 4800 gas to refund counter.
	value := metatypes.Hash(y.Bytes32())
	if current == value { // noop (1)
		return params.NetSstoreNoopGas, nil
	}
	original := vm.StateDB.GetCommittedState(contract.Account(), x.Bytes32())
	if original == current {
		if original == (metatypes.Hash{}) { // create slot (2.1.1)
			return params.NetSstoreInitGas, nil
		}
		if value == (metatypes.Hash{}) { // delete slot (2.1.2b)
			vm.StateDB.AddRefund(params.NetSstoreClearRefund)
		}
		return params.NetSstoreCleanGas, nil // write existing slot (2.1.2)
	}
	if original != (metatypes.Hash{}) {
		if current == (metatypes.Hash{}) { // recreate slot (2.2.1.1)
			vm.StateDB.SubRefund(params.NetSstoreClearRefund)
		} else if value == (metatypes.Hash{}) { // delete slot (2.2.1.2)
			vm.StateDB.AddRefund(params.NetSstoreClearRefund)
		}
	}
	if original == value {
		if original == (metatypes.Hash{}) { // reset to original inexistent slot (2.2.2.1)
			vm.StateDB.AddRefund(params.NetSstoreResetClearRefund)
		} else { // reset to original existing slot (2.2.2.2)
			vm.StateDB.AddRefund(params.NetSstoreResetRefund)
		}
	}
	return params.NetSstoreDirtyGas, nil
}

// 0. If *gasleft* is less than or equal to 2300, fail the current call.
// 1. If current value equals new value (this is a no-op), SLOAD_GAS is deducted.
// 2. If current value does not equal new value:
//   2.1. If original value equals current value (this storage slot has not been changed by the current execution context):
//     2.1.1. If original value is 0, SSTORE_SET_GAS (20K) gas is deducted.
//     2.1.2. Otherwise, SSTORE_RESET_GAS gas is deducted. If new value is 0, add SSTORE_CLEARS_SCHEDULE to refund counter.
//   2.2. If original value does not equal current value (this storage slot is dirty), SLOAD_GAS gas is deducted. Apply both of the following clauses:
//     2.2.1. If original value is not 0:
//       2.2.1.1. If current value is 0 (also means that new value is not 0), subtract SSTORE_CLEARS_SCHEDULE gas from refund counter.
//       2.2.1.2. If new value is 0 (also means that current value is not 0), add SSTORE_CLEARS_SCHEDULE gas to refund counter.
//     2.2.2. If original value equals new value (this storage slot is reset):
//       2.2.2.1. If original value is 0, add SSTORE_SET_GAS - SLOAD_GAS to refund counter.
//       2.2.2.2. Otherwise, add SSTORE_RESET_GAS - SLOAD_GAS gas to refund counter.
func gasSStoreEIP2200(vm *ethvm.ETHVM, contract *Contract, stack *ethvm.Stack, mem *ethvm.Memory, memorySize uint64) (uint64, error) {
	// If we fail the minimum gas availability invariant, fail (0)
	if contract.Gas <= params.SstoreSentryGasEIP2200 {
		return 0, errors.New("not enough gas for reentrancy sentry")
	}
	// Gas sentry honoured, do the actual gas calculation based on the stored value
	var (
		y, x    = stack.Back(1), stack.Back(0)
		current = vm.StateDB.GetState(contract.Address(), x.Bytes32())
	)
	value := metatypes.Hash(y.Bytes32())

	if current == value { // noop (1)
		return params.SloadGasEIP2200, nil
	}
	original := vm.StateDB.GetCommittedState(contract.Address(), x.Bytes32())
	if original == current {
		if original == (metatypes.Hash{}) { // create slot (2.1.1)
			return params.SstoreSetGasEIP2200, nil
		}
		if value == (metatypes.Hash{}) { // delete slot (2.1.2b)
			vm.StateDB.AddRefund(params.SstoreClearsScheduleRefundEIP2200)
		}
		return params.SstoreResetGasEIP2200, nil // write existing slot (2.1.2)
	}
	if original != (metatypes.Hash{}) {
		if current == (metatypes.Hash{}) { // recreate slot (2.2.1.1)
			vm.StateDB.SubRefund(params.SstoreClearsScheduleRefundEIP2200)
		} else if value == (metatypes.Hash{}) { // delete slot (2.2.1.2)
			vm.StateDB.AddRefund(params.SstoreClearsScheduleRefundEIP2200)
		}
	}
	if original == value {
		if original == (metatypes.Hash{}) { // reset to original inexistent slot (2.2.2.1)
			vm.StateDB.AddRefund(params.SstoreSetGasEIP2200 - params.SloadGasEIP2200)
		} else { // reset to original existing slot (2.2.2.2)
			vm.StateDB.AddRefund(params.SstoreResetGasEIP2200 - params.SloadGasEIP2200)
		}
	}
	return params.SloadGasEIP2200, nil // dirty update (2.2)
}

func gasCreateEip3860(evm *ethvm.ETHVM, contract *Contract, stack *ethvm.Stack, mem *ethvm.Memory, memorySize uint64) (uint64, error) {
	gas, err := memoryGasCost(mem, memorySize)
	if err != nil {
		return 0, err
	}
	size, overflow := stack.Back(2).Uint64WithOverflow()
	if overflow || size > params.MaxCodeSize*2 {
		return 0, ethvm.ErrGasUintOverflow
	}
	// Since size <= params.MaxInitCodeSize, these multiplication cannot overflow
	moreGas := params.InitCodeWordGas * ((size + 31) / 32)
	if gas, overflow = math.SafeAdd(gas, moreGas); overflow {
		return 0, ethvm.ErrGasUintOverflow
	}
	return gas, nil
}

func gasCreate2Eip3860(evm *ethvm.ETHVM, contract *Contract, stack *ethvm.Stack, mem *ethvm.Memory, memorySize uint64) (uint64, error) {
	gas, err := memoryGasCost(mem, memorySize)
	if err != nil {
		return 0, err
	}
	size, overflow := stack.Back(2).Uint64WithOverflow()
	if overflow || size > params.MaxCodeSize*2 {
		return 0, ethvm.ErrGasUintOverflow
	}
	// Since size <= params.MaxInitCodeSize, these multiplication cannot overflow
	moreGas := (params.InitCodeWordGas + params.Keccak256WordGas) * ((size + 31) / 32)
	if gas, overflow = math.SafeAdd(gas, moreGas); overflow {
		return 0, ethvm.ErrGasUintOverflow
	}
	return gas, nil
}

func GasCreate2Eip3860(evm *ethvm.ETHVM, contract *Contract, stack *ethvm.Stack, mem *ethvm.Memory, memorySize uint64) (uint64, error) {
	return gasCreate2Eip3860(evm,contract,stack,mem,memorySize)
}

func makeGasLog(n uint64) gasFunc {
	return func(vm *ethvm.ETHVM, contract *Contract, stack *ethvm.Stack, mem *ethvm.Memory, memorySize uint64) (uint64, error) {
		requestedSize, overflow := stack.Back(1).Uint64WithOverflow()
		if overflow {
			return 0, ethvm.ErrGasUintOverflow
		}

		gas, err := memoryGasCost(mem, memorySize)
		if err != nil {
			return 0, err
		}

		if gas, overflow = math.SafeAdd(gas, params.LogGas); overflow {
			return 0, ethvm.ErrGasUintOverflow
		}
		if gas, overflow = math.SafeAdd(gas, n*params.LogTopicGas); overflow {
			return 0, ethvm.ErrGasUintOverflow
		}

		var memorySizeGas uint64
		if memorySizeGas, overflow = math.SafeMul(requestedSize, params.LogDataGas); overflow {
			return 0, ethvm.ErrGasUintOverflow
		}
		if gas, overflow = math.SafeAdd(gas, memorySizeGas); overflow {
			return 0, ethvm.ErrGasUintOverflow
		}
		return gas, nil
	}
}

func gasKeccak256(vm *ethvm.ETHVM, contract *Contract, stack *ethvm.Stack, mem *ethvm.Memory, memorySize uint64) (uint64, error) {
	gas, err := memoryGasCost(mem, memorySize)
	if err != nil {
		return 0, err
	}
	wordGas, overflow := stack.Back(1).Uint64WithOverflow()
	if overflow {
		return 0, ethvm.ErrGasUintOverflow
	}
	if wordGas, overflow = math.SafeMul(ethvm.ToWordSize(wordGas), params.Keccak256WordGas); overflow {
		return 0, ethvm.ErrGasUintOverflow
	}
	if gas, overflow = math.SafeAdd(gas, wordGas); overflow {
		return 0, ethvm.ErrGasUintOverflow
	}
	return gas, nil
}

// pureMemoryGascost is used by several operations, which aside from their
// static cost have a dynamic cost which is solely based on the memory
// expansion
func pureMemoryGascost(vm *ethvm.ETHVM, contract *Contract, stack *ethvm.Stack, mem *ethvm.Memory, memorySize uint64) (uint64, error) {
	return memoryGasCost(mem, memorySize)
}

var (
	gasReturn  = pureMemoryGascost
	gasRevert  = pureMemoryGascost
	gasMLoad   = pureMemoryGascost
	gasMStore8 = pureMemoryGascost
	gasMStore  = pureMemoryGascost
	gasCreate  = pureMemoryGascost
)

func gasCreate2(vm *ethvm.ETHVM, contract *Contract, stack *ethvm.Stack, mem *ethvm.Memory, memorySize uint64) (uint64, error) {
	gas, err := memoryGasCost(mem, memorySize)
	if err != nil {
		return 0, err
	}
	wordGas, overflow := stack.Back(2).Uint64WithOverflow()
	if overflow {
		return 0, ethvm.ErrGasUintOverflow
	}
	if wordGas, overflow = math.SafeMul(ethvm.ToWordSize(wordGas), params.Keccak256WordGas); overflow {
		return 0, ethvm.ErrGasUintOverflow
	}
	if gas, overflow = math.SafeAdd(gas, wordGas); overflow {
		return 0, ethvm.ErrGasUintOverflow
	}
	return gas, nil
}

func gasExpFrontier(vm *ethvm.ETHVM, contract *Contract, stack *ethvm.Stack, mem *ethvm.Memory, memorySize uint64) (uint64, error) {
	expByteLen := uint64((stack.Data()[stack.Len()-2].BitLen() + 7) / 8)

	var (
		gas      = expByteLen * params.ExpByteFrontier // no overflow check required. Max is 256 * ExpByte gas
		overflow bool
	)
	if gas, overflow = math.SafeAdd(gas, params.ExpGas); overflow {
		return 0, ethvm.ErrGasUintOverflow
	}
	return gas, nil
}

func gasExpEIP158(vm *ethvm.ETHVM, contract *Contract, stack *ethvm.Stack, mem *ethvm.Memory, memorySize uint64) (uint64, error) {
	expByteLen := uint64((stack.Data()[stack.Len()-2].BitLen() + 7) / 8)

	var (
		gas      = expByteLen * params.ExpByteEIP158 // no overflow check required. Max is 256 * ExpByte gas
		overflow bool
	)
	if gas, overflow = math.SafeAdd(gas, params.ExpGas); overflow {
		return 0, ethvm.ErrGasUintOverflow
	}
	return gas, nil
}

func gasCall(vm *ethvm.ETHVM, contract *Contract, stack *ethvm.Stack, mem *ethvm.Memory, memorySize uint64) (uint64, error) {
	var (
		gas            uint64
		transfersValue = !stack.Back(2).IsZero()
		address        = metatypes.Address(stack.Back(1).Bytes20())
	)

	if transfersValue && vm.StateDB.Empty(types.Account{address}) {
		gas += params.CallNewAccountGas
	}

	if transfersValue {
		gas += params.CallValueTransferGas
	}
	memoryGas, err := memoryGasCost(mem, memorySize)
	if err != nil {
		return 0, err
	}
	var overflow bool
	if gas, overflow = math.SafeAdd(gas, memoryGas); overflow {
		return 0, ethvm.ErrGasUintOverflow
	}

	callGasTemp, err := ethvm.CallGas(contract.Gas, gas, stack.Back(0))
	if err != nil {
		return 0, err
	}
	vm.SetCallGasTemp(callGasTemp)

	if gas, overflow = math.SafeAdd(gas, callGasTemp); overflow {
		return 0, ethvm.ErrGasUintOverflow
	}
	return gas, nil
}

func gasCallCode(vm *ethvm.ETHVM, contract *Contract, stack *ethvm.Stack, mem *ethvm.Memory, memorySize uint64) (uint64, error) {
	memoryGas, err := memoryGasCost(mem, memorySize)
	if err != nil {
		return 0, err
	}
	var (
		gas      uint64
		overflow bool
	)
	if stack.Back(2).Sign() != 0 {
		gas += params.CallValueTransferGas
	}
	if gas, overflow = math.SafeAdd(gas, memoryGas); overflow {
		return 0, ethvm.ErrGasUintOverflow
	}
	callGasTemp, err := ethvm.CallGas(contract.Gas, gas, stack.Back(0))
	if err != nil {
		return 0, err
	}
	vm.SetCallGasTemp(callGasTemp)
	if gas, overflow = math.SafeAdd(gas, callGasTemp); overflow {
		return 0, ethvm.ErrGasUintOverflow
	}
	return gas, nil
}

func gasDelegateCall(vm *ethvm.ETHVM, contract *Contract, stack *ethvm.Stack, mem *ethvm.Memory, memorySize uint64) (uint64, error) {
	gas, err := memoryGasCost(mem, memorySize)
	if err != nil {
		return 0, err
	}
	callGasTemp, err := ethvm.CallGas(contract.Gas, gas, stack.Back(0))
	if err != nil {
		return 0, err
	}
	vm.SetCallGasTemp(callGasTemp)
	var overflow bool
	if gas, overflow = math.SafeAdd(gas, callGasTemp); overflow {
		return 0, ethvm.ErrGasUintOverflow
	}
	return gas, nil
}

func gasStaticCall(vm *ethvm.ETHVM, contract *Contract, stack *ethvm.Stack, mem *ethvm.Memory, memorySize uint64) (uint64, error) {
	gas, err := memoryGasCost(mem, memorySize)
	if err != nil {
		return 0, err
	}
	callGasTemp, err := ethvm.CallGas(contract.Gas, gas, stack.Back(0))
	if err != nil {
		return 0, err
	}
	vm.SetCallGasTemp(callGasTemp)
	var overflow bool
	if gas, overflow = math.SafeAdd(gas, callGasTemp); overflow {
		return 0, ethvm.ErrGasUintOverflow
	}
	return gas, nil
}

func gasSelfdestruct(vm *ethvm.ETHVM, contract *Contract, stack *ethvm.Stack, mem *ethvm.Memory, memorySize uint64) (uint64, error) {
	gas := params.SelfdestructGasEIP150
	var address = metatypes.Address(stack.Back(0).Bytes20())

	// if empty and transfers value
	if vm.StateDB.Empty(types.Account{address,}) && vm.StateDB.GetBalance(contract.Address()).Sign() != 0 {
		gas += params.CreateBySelfdestructGas
	}

	if !vm.StateDB.HasSuicided(contract.Account()) {
		vm.StateDB.AddRefund(params.SelfdestructRefundGas)
	}
	return gas, nil
}
