package core

import (
	basetype "github.com/CaduceusMetaverseProtocol/MetaProtocol/gen/proto/go/base/v1"
	metatypes "github.com/CaduceusMetaverseProtocol/MetaTypes/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/gogo/protobuf/proto"
	"math/big"
)

type MetaTransaction struct {
	basetype.MetaProofTx
}

func NewMetaTransaction(value basetype.MetaProofTx) *MetaTransaction {
	return &MetaTransaction{
		value,
	}
}

func (tx *MetaTransaction) EthHash() common.Hash {
	if len(tx.Hash()) != 0 {
		return common.BytesToHash(tx.Hash().Bytes())
	}
	return [32]byte{}
}

func (tx *MetaTransaction) Hash() metatypes.Hash {
	if tx.Base != nil {
		h := *tx.Base.TxHash
		return h
	} else {
		return metatypes.Hash{}
	}
}

func (tx *MetaTransaction) Type() uint32 {
	return tx.Base.TxType
}

func (tx *MetaTransaction) Encode() ([]byte, error) {
	return proto.Marshal(tx)
}

func (tx *MetaTransaction) Bytes() []byte {
	d, _ := tx.Encode()
	return d
}


func (tx *MetaTransaction) SetBytes(data []byte) error {
	return proto.Unmarshal(data, tx)
}

func (tx *MetaTransaction) From() *metatypes.Address {
	return tx.Base.From
}

func (tx *MetaTransaction) EthFrom() common.Address {
	return common.BytesToAddress(tx.Base.From.Bytes())
}

// To returns the recipient address of the transaction.
// For contract-creation transactions, To returns nil.
func (tx *MetaTransaction) To() *metatypes.Address {
	return tx.Base.To
}

func (tx *MetaTransaction) EthTo() common.Address {
	return common.BytesToAddress(tx.Base.To.Bytes())
}

// Value returns the ether amount of the transaction.
func (tx *MetaTransaction) Value() *metatypes.BigInt {
	return tx.Base.Value
}

func (tx *MetaTransaction) EthValue() *big.Int {
	return new(big.Int).SetBytes(tx.Base.Value.Bytes())
}

func ToMetaTransaction(prooftx *basetype.MetaProofTx) *MetaTransaction {
	return &MetaTransaction{*prooftx}
}

// ChainId returns the EIP155 chain ID of the transaction. The return value will always be
// non-nil. For legacy transactions which are not replay-protected, the return value is
// zero.
func (tx *MetaTransaction) ChainId() *big.Int {
	return new(big.Int).SetBytes(tx.Base.ChainId.Bytes())
}

// Data returns the input data of the transaction.
func (tx *MetaTransaction) Data() []byte { return tx.Base.Data }

// Gas returns the gas limit of the transaction.
func (tx *MetaTransaction) Gas() uint64 { return tx.Base.Gas }

// GasPrice returns the gas price of the transaction.
func (tx *MetaTransaction) GasPrice() *big.Int {
	//TODO just for test
	return new(big.Int).SetUint64(1000000000)
	//return new(big.Int).SetBytes(tx.Base.GasPrice.Bytes())
}

// GasTipCap returns the gasTipCap per gas of the transaction.
func (tx *MetaTransaction) GasTipCap() *big.Int { return new(big.Int).SetBytes(tx.Base.GasPrice.Bytes()) }

// GasFeeCap returns the fee cap per gas of the transaction.
func (tx *MetaTransaction) GasFeeCap() *big.Int { return new(big.Int).SetBytes(tx.Base.GasPrice.Bytes())}

// Nonce returns the sender account nonce of the transaction.
func (tx *MetaTransaction) Nonce() uint64 { return tx.Base.Nonce }

// Cost returns gas * gasPrice + value.
func (tx *MetaTransaction) Cost() *big.Int {
	total := new(big.Int).Mul(tx.GasPrice(), new(big.Int).SetUint64(tx.Gas()))
	total.Add(total, new(big.Int).SetBytes(tx.Value().Bytes()))
	return total
}

func (tx *MetaTransaction) AsMessage(baseFee *big.Int) Message {
	// If baseFee provided, set gasPrice to effectiveGasPrice.
	price := new(big.Int)
	price.Set(tx.GasPrice())

	gasPrice := new(big.Int)
	if baseFee != nil {
		gasPrice = math.BigMin(gasPrice.Add(price, baseFee), price)
	}
	to := tx.To()
	from := tx.From()
	msg := NewMessage(*from,to,tx.Nonce(),tx.EthValue(),tx.Gas(),gasPrice,price,price,tx.Data(),nil,true)
	return msg
}

type Message struct {
	to         *metatypes.Address
	from       metatypes.Address
	nonce      uint64
	amount     *big.Int
	gasLimit   uint64
	gasPrice   *big.Int
	gasFeeCap  *big.Int
	gasTipCap  *big.Int
	data       []byte
	accessList types.AccessList
	isFake     bool
}

func NewMessage(from metatypes.Address, to *metatypes.Address, nonce uint64, amount *big.Int, gasLimit uint64, gasPrice, gasFeeCap, gasTipCap *big.Int, data []byte, accessList types.AccessList, isFake bool) Message {
	return Message{
		from:       from,
		to:         to,
		nonce:      nonce,
		amount:     amount,
		gasLimit:   gasLimit,
		gasPrice:   gasPrice,
		gasFeeCap:  gasFeeCap,
		gasTipCap:  gasTipCap,
		data:       data,
		accessList: accessList,
		isFake:     isFake,
	}
}


func (m Message) From() metatypes.Address   { return m.from }
func (m Message) To() *metatypes.Address    { return m.to }
func (m Message) GasPrice() *big.Int     { return m.gasPrice }
func (m Message) GasFeeCap() *big.Int    { return m.gasFeeCap }
func (m Message) GasTipCap() *big.Int    { return m.gasTipCap }
func (m Message) Value() *big.Int        { return m.amount }
func (m Message) Gas() uint64            { return m.gasLimit }
func (m Message) Nonce() uint64          { return m.nonce }
func (m Message) Data() []byte           { return m.data }
func (m Message) AccessList() types.AccessList { return m.accessList }
func (m Message) IsFake() bool           { return m.isFake }


