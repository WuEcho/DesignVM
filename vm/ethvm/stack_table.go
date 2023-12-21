package ethvm

import (
	"github.com/ethereum/go-ethereum/params"
)

func minSwapStack(n int) int {
	return minStack(n, n)
}

func MinSwapStack(n int) int {
	return minStack(n, n)
}

func maxSwapStack(n int) int {
	return maxStack(n, n)
}

func MaxSwapStack(n int) int {
	return maxStack(n,n)
}

func minDupStack(n int) int {
	return minStack(n, n+1)
}

func MinDupStack(n int) int {
	return minStack(n, n+1)
}

func maxDupStack(n int) int {
	return maxStack(n, n+1)
}

func MaxDupStack(n int) int {
	return maxStack(n, n+1)
}

func maxStack(pop, push int) int {
	return int(params.StackLimit) + pop - push
}

func minStack(pops, push int) int {
	return pops
}

func MinStack(pops, push int) int {
	return pops
}

func MaxStack(pop, push int) int {
	return int(params.StackLimit) + pop - push
}