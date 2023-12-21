package test

import (
	"github.com/CaduceusMetaverseProtocol/MetaNebula/accounts"
	"github.com/CaduceusMetaverseProtocol/MetaNebula/accounts/keystore"
	"github.com/CaduceusMetaverseProtocol/MetaNebula/core/convert"
	"github.com/CaduceusMetaverseProtocol/MetaNebula/core/globaldb"
	types3 "github.com/CaduceusMetaverseProtocol/MetaNebula/types"
	basev1 "github.com/CaduceusMetaverseProtocol/MetaProtocol/gen/proto/go/base/v1"
	types2 "github.com/CaduceusMetaverseProtocol/MetaTypes/types"
	core2 "github.com/CaduceusMetaverseProtocol/MetaVM/core"
	params2 "github.com/CaduceusMetaverseProtocol/MetaVM/params"
	process2 "github.com/CaduceusMetaverseProtocol/MetaVM/process"
	"github.com/CaduceusMetaverseProtocol/MetaVM/vm/ethvm"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/trie"
	"golang.org/x/crypto/sha3"
	"math/big"
	"sync"
	"testing"
)

var (
	accountManger   *accounts.Manager
	ks              *keystore.KeyStore
	accountList     []accounts.Account
)

func TestNewProcessor(t *testing.T) {
	getAccount(AccountNum)
	println("account count:",len(accountList))
}

const TxNum = 10000
const AccountNum = 10000

func getAccount(num int)  {
	bk := keystore.NewKeyStore("/Users/wuxinyang/Desktop/MyGo/src/CaduceusMetaverseProtocol/MetaNebula/data/keystore",keystore.StandardScryptN, keystore.StandardScryptP)
	accountManger = accounts.NewManager(bk)
	ks = accountManger.Backends(keystore.KeyStoreType)[0].(*keystore.KeyStore)
	unLockedAccounts := make([]accounts.Account,0)
	accountList = make([]accounts.Account,0)
	for i,wallet := range accountManger.Wallets(){
		j := i
		accountList = append(accountList,wallet.Accounts()...)
		if j >= num {
			break
		}
	}

	wg := sync.WaitGroup{}
	for i,acc := range accountList {
		wg.Add(1)
		go func(ac accounts.Account,index int) {
			defer wg.Done()
			err := ks.Unlock(ac,"123456")
			if err == nil {
				unLockedAccounts = append(unLockedAccounts,ac)
			}
		}(acc,i)
	}
	wg.Wait()
	accountList = unLockedAccounts
}

func mkDynamicTx(nonce uint64, from , to accounts.Account, gasPrice, gas *types2.BigInt) (*core2.MetaTransaction) {
	otx := &basev1.MetaTxBase{
		TxType: 0,
		ChainId: types2.NewBigInt(1),
		Nonce: nonce,
		GasPrice: gasPrice,
		Value: gas,
		From: &from.Address,
		To: &to.Address,
		Data: []byte{},
	}

	wallet,_ := accountManger.Find(from)

	signTx,err := wallet.SignTx(from,otx,new(big.Int).SetUint64(1))
	if err != nil{
		println("err:",err.Error())
	}

	tx := &core2.MetaTransaction{
		MetaProofTx:basev1.MetaProofTx{
			Base: signTx,
		},
	}
	//println("txHash---:",tx.Hash().String())
	return tx
}

func TestProcessor_ExecTxsInOrder(t *testing.T) {
	db := rawdb.NewMemoryDatabase()
	conf := &params.ChainConfig{
		ChainID:             big.NewInt(1),
		Ethash:              new(params.EthashConfig),
	}
	pconf := &params2.ChainConfig{
		ChainID: big.NewInt(1),
	}
	getAccount(AccountNum)
	println("account count:",len(accountList))
	engine := ethash.NewFullFaker()
	addr1 := common.HexToAddress("0xD61F55E30fd15112180a2eD6530f5a1194393F36")
	add2 := common.HexToAddress("0xE607Cea3887f80c7427b761795BB110B71D3eecC")
	add3 := common.HexToAddress("0x5D8D4BA616B5b6aF115a2C61199Be74704512D6F")
	//add4 := common.HexToAddress("0x4d04FbF1E18307E1288a936c34adF563b8910034")
	gspec := &core.Genesis{
		Config: conf,
		Alloc: core.GenesisAlloc{
			addr1: core.GenesisAccount{
				Balance: big.NewInt(1000000000000000000), // 100 ether
				Nonce:   0,
			},
			add2: core.GenesisAccount{
				Balance: big.NewInt(1000000000000000000), // 100 ether
				Nonce:   0,
			},
			add3: core.GenesisAccount{
				Balance: big.NewInt(100000000000000000),
				Nonce: 0,
			},
		},
	}
	genesis := gspec.MustCommit(db)
	blockchain, err := core.NewBlockChain(db, nil, gspec.Config, ethash.NewFaker(), vm.Config{}, nil, nil)
	if err != nil {
		t.Fatalf("Failed to create chain: %v", err)
	}
	//stateDb,_ := state.New(genesis.Root(),state.NewDatabase(db),nil)
	process := process2.NewProcessor(pconf,blockchain,engine)

	blocks := make([]*types.Block,0)
	blocks = append(blocks,genesis)

	dateBb := globaldb.NewMemoryDataBase()
	statedb := globaldb.NewMemGDB(dateBb)
	//account need

	for i:=0;i < len(accountList);i++ {
		from := accountList[i]
		fromAccount := types3.Account{types2.HexToAddress(from.Address.Hex())}
		statedb.SetBalance(fromAccount,new(big.Int).SetUint64(100000000))
		statedb.SetNonce(fromAccount,0)
	}
	statedb.Finalise(true)

	vmconf := ethvm.Config{NoBaseFee: true}
	for i:=0;i < 100;i++ {
		block := GenerateBlock(blocks[i],nil,conf)
		txs := make([]*core2.MetaTransaction,0)
		for j:=0;j < len(accountList);j++ {
			ind := j%len(accountList)
			from := accountList[ind]
			var to types2.Address
			if j == 1 || j == 0 {
				to = types2.BytesToAddress(big.NewInt(int64(j+TxNum+3)).Bytes())
			}else {
				to = types2.BytesToAddress(big.NewInt(int64(j)).Bytes())
			}
			tx := mkDynamicTx(uint64(j+TxNum*i),from,accounts.Account{Address: to},types2.NewBigInt(21000),types2.NewBigInt(1))
			txs = append(txs,tx)
		}
		res,_ := process.ExecBenchTxs(block,convert.NewConvertedState(statedb),txs,vmconf)
		//res,_ := process.ExecTxsInOrder(block,convert.NewConvertedState(statedb),txs,vmconf)
		if len(res) != 0 {
			//for _,rs := range res{
			//	println("err:",rs.Ed.ErrInfo())
			//}
		}
		blocks = append(blocks,block)
	}
	//pprof.StopCPUProfile()
}


type fakeChainReader struct {
	config *params.ChainConfig
}

// Config returns the chain configuration.
func (cr *fakeChainReader) Config() *params.ChainConfig {
	return cr.config
}

func (cr *fakeChainReader) CurrentHeader() *types.Header                            { return nil }
func (cr *fakeChainReader) GetHeaderByNumber(number uint64) *types.Header           { return nil }
func (cr *fakeChainReader) GetHeaderByHash(hash common.Hash) *types.Header          { return nil }
func (cr *fakeChainReader) GetHeader(hash common.Hash, number uint64) *types.Header { return nil }
func (cr *fakeChainReader) GetBlock(hash common.Hash, number uint64) *types.Block   { return nil }
func (cr *fakeChainReader) GetTd(hash common.Hash, number uint64) *big.Int          { return nil }


func GenerateBlock(parent *types.Block, txs types.Transactions, conf *params.ChainConfig) *types.Block {
	header := &types.Header{
		ParentHash: parent.Hash(),
		Coinbase:   parent.Coinbase(),
		Difficulty: new(big.Int).SetUint64(1),
		GasLimit:  100000000,
		Number:    new(big.Int).Add(parent.Number(), common.Big1),
		Time:      parent.Time() + 10,
		UncleHash: types.EmptyUncleHash,
	}

	header.BaseFee = new(big.Int).SetUint64(1000)
	var receipts []*types.Receipt
	// The post-state result doesn't need to be correct (this is a bad block), but we do need something there
	// Preferably something unique. So let's use a combo of blocknum + txhash
	hasher := sha3.NewLegacyKeccak256()
	hasher.Write(header.Number.Bytes())
	header.Root = common.BytesToHash(hasher.Sum(nil))
	// Assemble and return the final block for sealing
	return types.NewBlock(header, txs, nil, receipts, trie.NewStackTrie(nil))
}
