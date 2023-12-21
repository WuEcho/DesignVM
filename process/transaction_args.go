package process

import (
	"bytes"
	"errors"
	"fmt"
	types2 "github.com/CaduceusMetaverseProtocol/MetaNebula/types"
	metatypes "github.com/CaduceusMetaverseProtocol/MetaTypes/types"
	"github.com/CaduceusMetaverseProtocol/MetaVM/core"
	"github.com/CaduceusMetaverseProtocol/MetaVM/interfaces"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"math"
	"math/big"
)

// TransactionArgs represents the arguments to construct a new transaction
// or a message call.
type TransactionArgs struct {
	From     *metatypes.Address `json:"from"`
	To       *metatypes.Address `json:"to"`
	Gas      *hexutil.Uint64 `json:"gas"`
	GasPrice *hexutil.Big    `json:"gasPrice"`
	Value    *hexutil.Big    `json:"value"`
	Nonce    *hexutil.Uint64 `json:"nonce"`

	Data    *hexutil.Bytes `json:"data"`
	Input   *hexutil.Bytes `json:"input"`
	ChainID *hexutil.Big   `json:"chainId,omitempty"`
}

// from retrieves the transaction sender address.
func (args *TransactionArgs) from() metatypes.Address {
	if args.From == nil {
		return metatypes.Address{}
	}
	return *args.From
}

// data retrieves the transaction calldata. Input field is preferred.
func (args *TransactionArgs) data() []byte {
	if args.Input != nil {
		return *args.Input
	}
	if args.Data != nil {
		return *args.Data
	}
	return nil
}

// setDefaults fills in default values for unspecified tx fields.
func (args *TransactionArgs) SetDefaults(statedb interfaces.StateDB, wantChainId *big.Int) error {
	if err := args.setFeeDefaults(); err != nil {
		return err
	}
	if args.Value == nil {
		args.Value = new(hexutil.Big)
	}
	if args.Nonce == nil {
		addr := args.from()
		nonce := statedb.GetNonce(types2.Account{addr})
		args.Nonce = (*hexutil.Uint64)(&nonce)
	}

	if args.Data != nil && args.Input != nil && !bytes.Equal(*args.Data, *args.Input) {
		return errors.New(`both "data" and "input" are set and not equal. Please use "input" to pass transaction call data`)
	}

	if args.To == nil && len(args.data()) == 0 {
		return errors.New(`contract creation without any data provided`)
	}
	defaultGas := uint64(10000000)
	// Estimate the gas usage if necessary.
	if args.Gas == nil {
		args.Gas = (*hexutil.Uint64)(&defaultGas)
		log.Trace("set to default gas", "gas", args.Gas)
	}
	// If chain id is provided, ensure it matches the local chain id. Otherwise, set the local
	// chain id as the default.
	if args.ChainID != nil {
		if have := (*big.Int)(args.ChainID); have.Cmp(wantChainId) != 0 {
			return fmt.Errorf("chainId does not match node's (have=%v, want=%v)", have, wantChainId)
		}
	} else {
		args.ChainID = (*hexutil.Big)(wantChainId)
	}
	return nil
}

// setFeeDefaults fills in default fee values for unspecified tx fields.
func (args *TransactionArgs) setFeeDefaults() error {
	if args.GasPrice == nil {
		price, _ := new(big.Int).SetString("1000000000", 10)
		args.GasPrice = (*hexutil.Big)(price)
	}
	return nil
}

// ToMessage converts the transaction arguments to the Message type used by the
// core evm. This method is used in calls and traces that do not require a real
// live transaction.
func (args *TransactionArgs) ToMessage(globalGasCap uint64) (Message, error) {
	// Set sender address or use zero address if none specified.
	addr := args.from()

	// Set default gas & gas price if none were set
	gas := globalGasCap
	if gas == 0 {
		gas = uint64(math.MaxUint64 / 2)
	}
	if args.Gas != nil {
		gas = uint64(*args.Gas)
	}
	if globalGasCap != 0 && globalGasCap < gas {
		log.Warn("Caller gas above allowance, capping", "requested", gas, "cap", globalGasCap)
		gas = globalGasCap
	}
	var (
		gasPrice = new(big.Int)
	)
	if args.GasPrice != nil {
		gasPrice = args.GasPrice.ToInt()
	}

	value := new(big.Int)
	if args.Value != nil {
		value = args.Value.ToInt()
	}
	data := args.data()

	msg := core.NewMessage(addr,args.To,0,value,gas,gasPrice,gasPrice,gasPrice,data,nil,true)
		//types.NewMessage(addr, args.To, 0, value, gas, gasPrice, gasPrice, gasPrice, data, nil, true)
	return msg, nil
}

// toTransaction converts the arguments to a transaction.
// This assumes that setDefaults has been called.
func (args *TransactionArgs) toTransaction() *types.Transaction {
	var data types.TxData
	to := common.BytesToAddress(args.To.Bytes())
	data = &types.LegacyTx{
		To:       &to,
		Nonce:    uint64(*args.Nonce),
		Gas:      uint64(*args.Gas),
		GasPrice: (*big.Int)(args.GasPrice),
		Value:    (*big.Int)(args.Value),
		Data:     args.data(),
	}
	return types.NewTx(data)
}

// ToTransaction converts the arguments to a transaction.
// This assumes that setDefaults has been called.
func (args *TransactionArgs) ToTransaction() *types.Transaction {
	return args.toTransaction()
}
