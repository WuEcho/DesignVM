package ethvm

import (
	"github.com/holiman/uint256"
)

// opPush0 implements the PUSH0 opcode
func opPush0(pc *uint64, interpreter *VMInterpreter, scope *ScopeContext) ([]byte, error) {
	scope.Stack.push(new(uint256.Int))
	return nil, nil
}

func opSelfBalance(pc *uint64, interpreter *VMInterpreter, scope *ScopeContext) ([]byte, error) {
	balance, _ := uint256.FromBig(interpreter.vm.StateDB.GetBalance(scope.Contract.Account()))
	scope.Stack.push(balance)
	return nil, nil
}


// opChainID implements CHAINID opcode
func opChainID(pc *uint64, interpreter *VMInterpreter, scope *ScopeContext) ([]byte, error) {
	chainId, _ := uint256.FromBig(interpreter.vm.chainConfig.ChainID)
	scope.Stack.push(chainId)
	return nil, nil
}


// opBaseFee implements BASEFEE opcode
func opBaseFee(pc *uint64, interpreter *VMInterpreter, scope *ScopeContext) ([]byte, error) {
	baseFee, _ := uint256.FromBig(interpreter.vm.Context.BaseFee)
	scope.Stack.push(baseFee)
	return nil, nil
}
