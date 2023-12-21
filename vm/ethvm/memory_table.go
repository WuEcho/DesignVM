package ethvm

func memoryKeccak256(stack *Stack) (uint64, bool) {
	return calcMemSize64(stack.Back(0), stack.Back(1))
}

func MemoryKeccak256(stack *Stack) (uint64, bool) {
	return memoryKeccak256(stack)
}

func memoryCallDataCopy(stack *Stack) (uint64, bool) {
	return calcMemSize64(stack.Back(0), stack.Back(2))
}

func MemoryCallDataCopy(stack *Stack) (uint64, bool) {
	return memoryCallDataCopy(stack)
}

func memoryReturnDataCopy(stack *Stack) (uint64, bool) {
	return calcMemSize64(stack.Back(0), stack.Back(2))
}

func MemoryReturnDataCopy(stack *Stack) (uint64, bool) {
	return memoryReturnDataCopy(stack)
}

func memoryCodeCopy(stack *Stack) (uint64, bool) {
	return calcMemSize64(stack.Back(0), stack.Back(2))
}

func MemoryCodeCopy(stack *Stack) (uint64, bool) {
	return memoryCodeCopy(stack)
}

func memoryExtCodeCopy(stack *Stack) (uint64, bool) {
	return calcMemSize64(stack.Back(1), stack.Back(3))
}

func MemoryExtCodeCopy(stack *Stack) (uint64, bool) {
	return memoryExtCodeCopy(stack)
}

func memoryMLoad(stack *Stack) (uint64, bool) {
	return calcMemSize64WithUint(stack.Back(0), 32)
}

func MemoryMLoad(stack *Stack) (uint64, bool) {
	return memoryMLoad(stack)
}

func memoryMStore8(stack *Stack) (uint64, bool) {
	return calcMemSize64WithUint(stack.Back(0), 1)
}

func MemoryMStore8(stack *Stack) (uint64, bool) {
	return memoryMStore8(stack)
}

func memoryMStore(stack *Stack) (uint64, bool) {
	return calcMemSize64WithUint(stack.Back(0), 32)
}

func MemoryMStore(stack *Stack) (uint64, bool) {
	return memoryMStore(stack)
}

func memoryCreate(stack *Stack) (uint64, bool) {
	return calcMemSize64(stack.Back(1), stack.Back(2))
}

func MemoryCreate(stack *Stack) (uint64, bool) {
	return memoryCreate(stack)
}

func memoryCreate2(stack *Stack) (uint64, bool) {
	return calcMemSize64(stack.Back(1), stack.Back(2))
}

func MemoryCreate2(stack *Stack) (uint64, bool) {
	return memoryCreate2(stack)
}

func memoryCall(stack *Stack) (uint64, bool) {
	x, overflow := calcMemSize64(stack.Back(5), stack.Back(6))
	if overflow {
		return 0, true
	}
	y, overflow := calcMemSize64(stack.Back(3), stack.Back(4))
	if overflow {
		return 0, true
	}
	if x > y {
		return x, false
	}
	return y, false
}

func MemoryCall(stack *Stack) (uint64, bool) {
	return memoryCall(stack)
}

func memoryDelegateCall(stack *Stack) (uint64, bool) {
	x, overflow := calcMemSize64(stack.Back(4), stack.Back(5))
	if overflow {
		return 0, true
	}
	y, overflow := calcMemSize64(stack.Back(2), stack.Back(3))
	if overflow {
		return 0, true
	}
	if x > y {
		return x, false
	}
	return y, false
}

func MemoryDelegateCall(stack *Stack) (uint64, bool) {
	return memoryDelegateCall(stack)
}

func memoryStaticCall(stack *Stack) (uint64, bool) {
	x, overflow := calcMemSize64(stack.Back(4), stack.Back(5))
	if overflow {
		return 0, true
	}
	y, overflow := calcMemSize64(stack.Back(2), stack.Back(3))
	if overflow {
		return 0, true
	}
	if x > y {
		return x, false
	}
	return y, false
}

func MemoryStaticCall(stack *Stack) (uint64, bool) {
	return memoryStaticCall(stack)
}

func memoryReturn(stack *Stack) (uint64, bool) {
	return calcMemSize64(stack.Back(0), stack.Back(1))
}

func MemoryReturn(stack *Stack) (uint64, bool) {
	return memoryReturn(stack)
}

func memoryRevert(stack *Stack) (uint64, bool) {
	return calcMemSize64(stack.Back(0), stack.Back(1))
}

func MemoryRevert(stack *Stack) (uint64, bool) {
	return memoryRevert(stack)
}

func memoryLog(stack *Stack) (uint64, bool) {
	return calcMemSize64(stack.Back(0), stack.Back(1))
}

func MemoryLog(stack *Stack) (uint64, bool) {
	return memoryLog(stack)
}
