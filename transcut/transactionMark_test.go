package transcut

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"math/big"
	"testing"
)

/*func TestNewTransactionMarkMap(t *testing.T) {

	tsMarkMap := NewTransactionMarkMap()

	item := NewElement()
	item.address = common.Address{0x0001}
	item.txHash = common.Hash{0x0002}
	cAddr := common.Address{0x00004}
	item.UpdateRecord(cAddr.Hex())

	cAddr1 := common.Address{0x0005}
	item.UpdateRecord(cAddr1.Hex())

	cAddr2 := common.Address{0x0007}
	item.UpdateRecord(cAddr2.Hex())

	tsMarkMap.Push(item)

	v := tsMarkMap.Size()
	println("v::",v)

	tsMarkMap.Walk(func(data ContractItem) {
		v := data.GetRecordMarkValue(cAddr2.Hex())
		println("v:",v)
	})
}
*/
func TestNewFromMarkCache(t *testing.T) {
	//addr := common.HexToAddress("0x00001")
	//tx1 := types.NewTransaction(1,addr,big.NewInt(20),20,big.NewInt(20),nil)
	//
	//addr1 := common.HexToAddress("0x00002")
	//tx2 := types.NewTransaction(1,addr1,big.NewInt(20),20,big.NewInt(30),nil)
	//
	//addr3 := common.HexToAddress("0x00003")
	//tx3 := types.NewTransaction(1,addr3,big.NewInt(20),20,big.NewInt(20),nil)
	//
	//tx4 := types.NewTransaction(2,addr1,big.NewInt(20),20,big.NewInt(20),nil)
	//
	//tx5 := types.NewTransaction(3,addr3,big.NewInt(20),20,big.NewInt(30),nil)
	//
	//addr4 := common.HexToAddress("0x00004")
	//
	//tx6 := types.NewTransaction(1,addr4,big.NewInt(30),30,big.NewInt(30),nil)
	//
	//tx7 := types.NewTransaction(2,addr4,big.NewInt(25),29,big.NewInt(30),nil)
	//
	//tx8 := types.NewTransaction(2,addr,big.NewInt(30),10,big.NewInt(30),nil)

	/**
	0x001->0x002 0
	0x003->0x005 0
	0x004->0x001 1
	**/

	addr := common.HexToAddress("0x001")
	addr2 := common.HexToAddress("0x002")
	addr3 := common.HexToAddress("0x003")
	addr4 := common.HexToAddress("0x004")
	addr5 := common.HexToAddress("0x005")

	tx1 := types.NewTransaction(1,addr2,big.NewInt(20),20,big.NewInt(20),nil)
	tx2 := types.NewTransaction(1,addr5,big.NewInt(20),20,big.NewInt(20),nil)
	tx3 := types.NewTransaction(1,addr,big.NewInt(20),20,big.NewInt(20),nil)
	txs := make([]*types.Transaction,0)
	txs = append(txs,tx1,tx2,tx3)
	/**
	0x001->0x002 0
	0x003->0x005 0
	0x004->0x001 1
	**/
	cache := NewMarkCache()
	for i,tx := range txs{
		var  key common.Address
		item := &Item{
			txInfo:tx,
		}
		switch i {
		case 0:
			key = addr
		case 1:
			key = addr3
		case 2:
			key = addr4
		}
		cache.SetMarkCache(key,item)
	}

	v := cache.maxLevel
	println("v max level:",v)

	txs1 := cache.GetLevelTxsWithMark(0)
	for _,tx := range txs1 {
		println("tx info:",tx.ChainId().String())
		println("tx info:",tx.To().String())
	}
}

func TestFromMarkCache_GetLevelTxsWithMark(t *testing.T) {
	/**
	0x0001 -> 0x0002  0
	0x0002 -> 0x0003  1
	0x0001 -> 0x0004  2
	**/
	addr1 := common.HexToAddress("0x001")
	addr2 := common.HexToAddress("0x002")
	addr3 := common.HexToAddress("0x003")
	addr4 := common.HexToAddress("0x004")

	tx1 := types.NewTransaction(1,addr2,big.NewInt(20),20,big.NewInt(20),nil)
	tx2 := types.NewTransaction(1,addr3,big.NewInt(20),20,big.NewInt(20),nil)
	tx3 := types.NewTransaction(1,addr4,big.NewInt(20),20,big.NewInt(20),nil)

	txs := make([]*types.Transaction,0)
	txs = append(txs,tx1,tx2,tx3)
	cache := NewMarkCache()
	for i,tx := range txs {
		var  key common.Address
		item := &Item{
			txInfo:tx,
		}

		switch i {

		case 0:
			key = addr1
		case 1:
			key = addr2
		case 2:
			key =addr1
		}

		cache.SetMarkCache(key,item)
	}

	v := cache.GetMaxLevel()
	println("v:",v)
}