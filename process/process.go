package process

import (
	"context"
	"fmt"
	"github.com/CaduceusMetaverseProtocol/MetaNebula/common/log"
	"github.com/CaduceusMetaverseProtocol/MetaNebula/core/globaldb"
	types2 "github.com/CaduceusMetaverseProtocol/MetaNebula/types"
	basev1 "github.com/CaduceusMetaverseProtocol/MetaProtocol/gen/proto/go/base/v1"
	metatypes "github.com/CaduceusMetaverseProtocol/MetaTypes/types"
	common2 "github.com/CaduceusMetaverseProtocol/MetaVM/common"
	core2 "github.com/CaduceusMetaverseProtocol/MetaVM/core"
	"github.com/CaduceusMetaverseProtocol/MetaVM/interfaces"
	"github.com/CaduceusMetaverseProtocol/MetaVM/params"
	"github.com/CaduceusMetaverseProtocol/MetaVM/transcut"
	"github.com/CaduceusMetaverseProtocol/MetaVM/vm/ethvm"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"math/big"
	"os"
	"runtime/pprof"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type ProcessType int

const (
	//并行
	ParallelProcessType ProcessType = iota
	//串行
	SerialProcessType
	RelProcessType
)

type Result struct {
	Receipt *types2.Receipt //
	Ed      ErrorDetail
	VmErr   error //虚拟机执行的报错
	level   int
	//读写集
	ReadSet  *ethvm.ReadSet
	WriteSet *ethvm.WriteSet
	Logs     []*basev1.MetaTxLog
	Tx       *core2.MetaTransaction
	Msg      Message
}

type Process interface {
	PreExecution(ctx context.Context, header *types.Header, statedb interfaces.StateDB, tx *TransactionArgs, timeout time.Duration, cfg ethvm.Config) (*ExecutionResult, error)
	//执行批次交易并返回结果以及读写集
	ExecBenchTxs(block *types.Block, statedb interfaces.StateDB, txs []*core2.MetaTransaction, cfg ethvm.Config) ([]*Result, uint64)
	//顺序执行交易
	ExecTxsInOrder(block *types.Block, statedb interfaces.StateDB, txs []*core2.MetaTransaction, cfg ethvm.Config) ([]*Result, uint64)
}

type Processor struct {
	config *params.ChainConfig          // Chain configuration options
	bc     interfaces.BlockChainContext // Canonical block chain
	engine interfaces.ConsensusEngine   // Consensus engine used for block rewards
}

type ErrorDetail struct {
	err error
	tx  *core2.MetaTransaction
}

func (e *ErrorDetail) ErrInfo() error {
	return e.err
}

func NewProcessor(config *params.ChainConfig, bc interfaces.BlockChainContext, engine interfaces.ConsensusEngine) *Processor {
	return &Processor{
		config: config,
		bc:     bc,
		engine: engine,
	}
}

func (p *Processor) PreExecution(ctx context.Context, header *types.Header, statedb interfaces.StateDB, txargs *TransactionArgs, timeout time.Duration, cfg ethvm.Config) (*ExecutionResult, error) {
	defer func(start time.Time) { log.Debug("Executing EVM call finished", "runtime", time.Since(start)) }(time.Now())
	// Execute the message.

	var cancel context.CancelFunc
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}
	// Make sure the context is cancelled when the call has completed
	// this makes sure resources are cleaned up.
	defer cancel()
	gp := new(core.GasPool).AddGas(header.GasLimit)
	msg, err := txargs.ToMessage(header.GasLimit)
	if err != nil {
		return nil, err
	}

	blockContext := NewVMBlockContext(header, p.bc, nil)
	txContext := ethvm.TxContext{Origin: msg.From(), GasPrice: new(big.Int).Set(msg.GasPrice()), ReadSet: ethvm.NewReadSet(), WriteSet: ethvm.NewWriteSet()}
	vm := ethvm.NewVM(blockContext, txContext, statedb, p.config, cfg)

	// Wait for the context to be done and cancel the evm. Even if the
	// EVM has finished, cancelling may be done (repeatedly)
	go func() {
		<-ctx.Done()
		vm.Cancel()
	}()

	result, err := ApplyMessage(vm, msg, gp)
	// If the timer caused an abort, return an appropriate error message
	if vm.Cancelled() {
		return nil, fmt.Errorf("execution aborted (timeout = %v)", timeout)
	}

	if err != nil {
		return result, fmt.Errorf("err: %w (supplied gas %d)", err, msg.Gas())
	}
	return result, nil
}

func (p *Processor) ExecBenchTxs(block *types.Block, statedb interfaces.StateDB, txs []*core2.MetaTransaction, cfg ethvm.Config) ([]*Result, uint64) {
	var (
		results []*Result
		usedGas = new(uint64)
		gp      = new(core.GasPool).AddGas(block.GasLimit())
	)

	cpuProfilePath := "/Users/wuxinyang/Desktop/cpufile"
	cpuProfile := fmt.Sprintf(cpuProfilePath+"/pprof.samples.cpu.0%d.pb", block.NumberU64())
	if cpuProfile != "" {
		f, err := os.Create(cpuProfile)
		if err != nil {
			log.Infof("ExecBenchTxs create cpu profile failed err %s", err.Error())
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}
	timeStart := time.Now()

	cache := transcut.NewMarkCache()
	fee := block.Header().BaseFee
	for _, tx := range txs {
		transcut.FillingTransCut(fee, tx, p.config, cache)
	}
	timeUsed := time.Since(timeStart)
	log.Info("ExecBenchTxs---FillingTransCut", "--time used:", timeUsed.String())

	//TODO modify by echo
	level := cache.GetMaxLevel()
	blockContext := buildBlockContext(block, p)
	//执行池可以一个批次一个
	for i := 0; i < int(level); i++ {
		timeStart1 := time.Now()
		items := cache.GetLevelItemWithMark(uint32(i))
		var wg sync.WaitGroup
		resChan := make(chan *Result, len(items))
		tepRes := make([]*Result, 0)
		statedbCp := statedb.Copy()
		for _, item := range items {
			wg.Add(1)
			currentItem := item
			go func(item *transcut.Item, stateDb interfaces.StateDB, conf *params.ChainConfig, cf ethvm.Config) {
				defer func() {
					wg.Done()
				}()
				gp = new(core.GasPool).AddGas(block.GasLimit())
				txContext := ethvm.TxContext{ReadSet: ethvm.NewReadSet(), WriteSet: ethvm.NewWriteSet(), TxData: item.TxInfo().Data()}
				vm := ethvm.NewVM(*blockContext, txContext, stateDb, conf, cf)
				res := ApplyTransactionWithFlag(item.MessageInfo(), item.TxInfo(), stateDb, vm, usedGas, gp, i, ParallelProcessType, nil)
				resChan <- res
			}(currentItem, statedbCp, p.config, cfg)
		}
		wg.Wait()
		for {
			select {
			case v, ok := <-resChan:
				if ok {
					ed := ErrorDetail{
						err: v.Ed.err,
						tx:  v.Tx,
					}
					result := &Result{
						Receipt:  v.Receipt,
						Ed:       ed,
						VmErr:    v.VmErr,
						level:    v.level,
						ReadSet:  v.ReadSet,
						WriteSet: v.WriteSet,
						Logs:     v.Logs,
						Tx:       v.Tx,
						Msg:      v.Msg,
					}
					tepRes = append(tepRes, result)
				}
			}
			if len(tepRes) == len(items) {
				break
			}
		}
		timeUsed1 := time.Since(timeStart1)
		log.Info("ExecBenchTxs----FillUpPoolWithTask", "time used:", timeUsed1.String())

		timeStart2 := time.Now()
		rwSet := NewCacheSet(len(tepRes))
		for _, result := range tepRes {
			rwcod := NewRWRecord()
			rwcod.SetCurrentRecordLevel(result.level)
			rwcod.SetResult(result)
			rwcod.SetTxAndMsgInfo(result.Tx, result.Msg)
			rwcod.SetErrorInfo(result.Ed.err)
			rwcod.SetVmErrorInfo(result.VmErr)
			rwcod.SetLogs(result.Logs)
			rwSet.SetCache(result.Ed.tx.Hash(), rwcod)
		}

		sort.Sort(rwSet)
		timeUsed2 := time.Since(timeStart2)
		log.Info("ExecBenchTxs----set cache", "time used:", timeUsed2.String())

		timeStart3 := time.Now()
		wCache := NewWrittenBefore(1000)
		reses, _ := DetectConflicts(rwSet, cache, wCache, results, statedb, *blockContext, cfg, p.config, gp, usedGas, i)
		timeUsed3 := time.Since(timeStart3)
		log.Info("ExecBenchTxs----FindClashWithReadAndWriteCache", "time used:", timeUsed3.String())
		results = reses
	}

	timeUsedFl := time.Since(timeStart)
	log.Infof("block %d ExecBenchTxs------exec tx time used：%s", block.NumberU64(), timeUsedFl.String(), "tx len", len(txs))
	return results, *usedGas
}

func (p *Processor) ExecTxsInOrder(block *types.Block, statedb interfaces.StateDB, txs []*core2.MetaTransaction, cfg ethvm.Config) ([]*Result, uint64) {
	var (
		header  = block.Header()
		usedGas = new(uint64)
		gp      = new(core.GasPool).AddGas(block.GasLimit())
	)
	results := make([]*Result, len(txs), len(txs))
	// Iterate over and process the individual transactions
	//cpuProfile := fmt.Sprintf("/Users/wuxinyang/pprof/pprof.samples.cpu.0%d.pb",block.NumberU64())
	//if cpuProfile != "" {
	//	f,err := os.Create(cpuProfile)
	//	if err != nil{
	//		log.Infof("ExecTxsInOrder create cpu profile failed err %s",err.Error())
	//	}
	//	pprof.StartCPUProfile(f)
	//	defer pprof.StopCPUProfile()
	//}

	timeStart := time.Now()
	blockContext := NewVMBlockContext(header, p.bc, nil)
	for i, tx := range txs {
		msg := tx.AsMessage(header.BaseFee)
		txContext := ethvm.TxContext{ReadSet: ethvm.NewReadSet(), WriteSet: ethvm.NewWriteSet()}
		txContext.TxData = msg.Data()
		statedb.Prepare(tx.Hash(), i)
		vmenv := PrepareVm(blockContext, p, statedb, cfg, txContext)
		res := ApplyTransactionWithFlag(msg, tx, statedb, vmenv, usedGas, gp, 0, SerialProcessType, nil)
		if res.Ed.err != nil || res.VmErr != nil {
			for addr, cache := range res.WriteSet.Cache() {
				newCache := cache
				for k, _ := range newCache {
					if k != NonceKey {
						delete(cache, k)
					}
				}
				res.WriteSet.Set(addr, newCache)
			}
		}
		//对交易的结果进行校验 有错误的话需要只更新nonce
		results[i] = res
		statedb = UpdateWrittenCacheToDataBaseInOrder(res, statedb)
	}
	timeStemp1 := time.Now()
	statedb.Finalise(true)
	timeUsed1 := time.Since(timeStemp1)
	log.Infof("block %d ExecTxsInOrder------exec Finalise time used：%s", block.NumberU64(), timeUsed1.String())
	timeUsed := time.Since(timeStart)
	log.Infof("block %d ExecTxsInOrder------exec tx time used：%s", block.NumberU64(), timeUsed.String(), "tx len", len(txs))
	return results, *usedGas
}

func ApplyTransactionWithFlag(msg Message, tx *core2.MetaTransaction, statedb interfaces.StateDB, vmm *ethvm.ETHVM, usedGas *uint64, gp *core.GasPool, leve int, processType ProcessType, orgRes *Result) *Result {
	txContext := NewVMTxContext(msg)
	txContext.WriteSet = vmm.WriteSet
	txContext.ReadSet = vmm.ReadSet
	vmm.Reset(txContext, statedb)
	var (
		err    error
		result *ExecutionResult
	)

	switch processType {
	case ParallelProcessType:
		fallthrough
	case SerialProcessType:
		result, err = ApplyMessage(vmm, msg, gp)
	case RelProcessType:
		result, err = ReApplyMessage(vmm, msg, gp, orgRes.Receipt.CumulativeGasUsed)
	}
	if err != nil {
		log.Error("apply message error", "errinfo", err.Error())
		return &Result{
			Receipt: nil,
			Ed: ErrorDetail{
				err: err,
				tx:  tx,
			},
			VmErr:    nil,
			level:    leve,
			ReadSet:  ethvm.NewReadSet(),
			WriteSet: ethvm.NewWriteSet(),
			Tx:       tx,
			Msg:      msg,
		}
	}

	var root []byte
	//TODO Modify by echo
	//statedb.Finalise(true)
	// Create a new receipt for the transaction, storing the intermediate root and gas used
	// by the tx.
	receipt := types2.NewReceipt()
	receipt.Type = tx.Type()
	receipt.Root = root
	receipt.CumulativeGasUsed = *usedGas
	if result.Failed() {
		receipt.Status = types.ReceiptStatusFailed
	} else {
		receipt.Status = types.ReceiptStatusSuccessful
	}
	atomic.AddUint64(usedGas, result.UsedGas)
	//*usedGas += result.UsedGas
	receipt.SetTxHash(tx.Hash())
	receipt.GasUsed = result.UsedGas

	// If the transaction created a contract, store the creation address in the receipt.
	if msg.To() == nil {
		receipt.ContractAddress.SetBytes(vmm.TxContext.Origin.Bytes())
	}

	//TODO modify by echo
	// Set the receipt logs and create the bloom filter.
	receipt.Logs = vmm.Logs
	receipt.Bloom.SetReceiptLog(vmm.Logs)
	//receipt.BlockHash = block.Hash()
	receipt.BlockNumber.Set(vmm.Context.BlockNumber)
	//receipt.TransactionIndex = uint(statedb.TxIndex())
	res := &Result{
		Receipt: receipt,
		Ed: ErrorDetail{
			err: err,
			tx:  tx,
		},
		VmErr:    result.Err,
		level:    leve,
		ReadSet:  vmm.ReadSet,
		WriteSet: vmm.WriteSet,
		Logs:     vmm.Logs,
		Tx:       tx,
		Msg:      msg,
	}

	return res
}

type WrittenPool struct {
	pool  *sync.Pool
	lock  sync.Mutex
	count int //记录对象池中的对象数量
}

func NewWrittenPool() *WrittenPool {
	return &WrittenPool{
		pool: &sync.Pool{
			New: func() interface{} {
				return &WrittenBefore{
					Written: make(map[metatypes.Address]map[metatypes.Hash]interface{}, 1),
					Logs:    make(map[metatypes.Hash][]*basev1.MetaTxLog, 1),
				}
			},
		},
	}
}

func (wp *WrittenPool) Get() *WrittenBefore {
	wp.lock.Lock()
	defer wp.lock.Unlock()

	obj := wp.pool.Get()
	if obj == nil {
		return &WrittenBefore{
			Written: make(map[metatypes.Address]map[metatypes.Hash]interface{}, 1),
			Logs:    make(map[metatypes.Hash][]*basev1.MetaTxLog, 1),
		}
	}
	return obj.(*WrittenBefore)
}

func (wp *WrittenPool) Put(written *WrittenBefore) {
	wp.lock.Lock()
	defer wp.lock.Unlock()
	if wp.count >= 1000 {
		return
	}

	if len(written.Written) > 0 {
		written = &WrittenBefore{
			Written: make(map[metatypes.Address]map[metatypes.Hash]interface{}, 1),
			Logs:    make(map[metatypes.Hash][]*basev1.MetaTxLog, 1),
		}
	}
	wp.pool.Put(written)
	wp.count++
}

var (
	WrittenCache *WrittenBefore
)

type WrittenBefore struct {
	Written map[metatypes.Address]map[metatypes.Hash]interface{}
	Logs    map[metatypes.Hash][]*basev1.MetaTxLog
}

func NewWrittenBefore(length int) *WrittenBefore {
	WrittenCache = &WrittenBefore{
		Written: make(map[metatypes.Address]map[metatypes.Hash]interface{}, length*2),
		Logs:    make(map[metatypes.Hash][]*basev1.MetaTxLog, length),
	}
	return WrittenCache
}

func UpdateWrittenCacheToDataBaseInOrder(res *Result, statedb interfaces.StateDB) interfaces.StateDB {
	for addr, cache := range res.WriteSet.Cache() {
		account := types2.Account{
			addr,
		}
		//log.Info("UpdateWrittenCacheToDataBase---","addr:",account.Address.String())
		_, ok := cache[ethvm.CreatAccountKey]
		if ok {
			statedb.CreateAccount(account)
			statedb.SetNonce(account, 1)
		}

		code, ok := cache[ethvm.SetCodeKey]
		if ok {
			statedb.SetCode(account, code.([]byte))
		}

		v, ok := cache[ethvm.BalanceKey]
		if ok {
			statedb.SetBalance(account, v.(*big.Int))
		}

		for k, v := range cache {
			switch k {
			case ethvm.BalanceKey:
			case ethvm.CreatAccountKey:
			case ethvm.SetCodeKey:
			case ethvm.NonceKey:
				statedb.SetNonce(account, v.(uint64))
				//log.Infof("UpdateWrittenCacheToDataBase---","addr",account.String(),"Nonce",common2.ByteToUint64(v))
			case ethvm.SubBalanceKey:
				statedb.SubBalance(account, v.(*big.Int))
			default:
				statedb.SetState(account, k, common2.ByteToHash(v.([]byte)))
			}
		}
	}

	for _, log := range res.Logs {
		statedb.AddLogWithTxHash(log, res.Tx.Hash())
	}
	return statedb
}

type objectChan struct {
	acc     types2.Account
	object  *globaldb.StateObject
	cache   map[metatypes.Hash]interface{}
}

func UpdateWrittenCacheToDataBase(w *WrittenBefore, statedb interfaces.StateDB) interfaces.StateDB {
	var wg sync.WaitGroup
	mapLegth := len(w.Written)
	objChan := make(chan *objectChan,mapLegth)
	for addr, cache := range w.Written {
		wg.Add(1)
		go func(innerAddr metatypes.Address, innerCache map[metatypes.Hash]interface{},stateDb interfaces.StateDB) {
			defer wg.Done()
			account := types2.Account{
				innerAddr,
			}
			obj := statedb.GetStateObject(account)
			objChan <- &objectChan{
				acc: account,
				object: obj,
				cache: innerCache,
			}
		}(addr, cache, statedb)
	}
	wg.Wait()
	counter := 0
	for  {
		select {
		case obj,ok := <- objChan:
			if ok{
				valueCache := obj.cache
				account := obj.acc


				_,ok := valueCache[ethvm.CreatAccountKey]
				if ok {
					statedb.CreateAccount(account)
					statedb.SetNonce(account,1)
				}

				bal,ok := valueCache[ethvm.BalanceKey]
				if ok {
					obj.object.SetBalance(bal.(*big.Int))
				}

				for k,v := range valueCache{
					switch k {
					case ethvm.BalanceKey:
					case ethvm.CreatAccountKey:
					case ethvm.SetCodeKey:
					case ethvm.NonceKey:
						obj.object.SetNonce(v.(uint64))
					case ethvm.SubBalanceKey:
						balance := obj.object.GetBalance()
						r := new(big.Int).Sub(balance, v.(*big.Int))
						obj.object.SetBalance(r)
					default:
						obj.object.SetState(statedb.GetDb(),k,common2.ByteToHash(v.([]byte)))
					}
				}
				counter++
			}
		}

		if counter == mapLegth {
			break
		}
	}

	//log.Info("UpdateWrittenCacheToDataBase----","accountNum：",accountNum,"creatAccount:",creatAccount,"setBalanceCount：",setBalanceCount,"setNonceCount:",setNonceCount,"subBalanceCount:",subBalanceCount)
	for txHash, logs := range w.Logs {
			for _, log := range logs {
				statedb.AddLogWithTxHash(log, txHash)
			}
	}
	return statedb
}
