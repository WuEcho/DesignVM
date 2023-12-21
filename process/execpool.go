package process

import (
	core2 "github.com/CaduceusMetaverseProtocol/MetaVM/core"
	"github.com/CaduceusMetaverseProtocol/MetaVM/interfaces"
	"github.com/CaduceusMetaverseProtocol/MetaVM/params"
	"github.com/CaduceusMetaverseProtocol/MetaVM/transcut"
	"github.com/CaduceusMetaverseProtocol/MetaVM/vm/ethvm"
	"github.com/ethereum/go-ethereum/core"
	"runtime"
	"sync"
)

type ExecFunc func(msg Message, tx *core2.MetaTransaction, stateDb interfaces.StateDB, vmm *ethvm.ETHVM, usedGas *uint64, gp *core.GasPool, leve int, processType ProcessType, orgRes *Result) *Result

type Task struct {
	item        *transcut.Item
	//gp          *core.GasPool
	//cfg         ethvm.Config
	//config      *params.ChainConfig
	//stateDb     interfaces.StateDB
	//usedGas     *uint64
	//level       int //批次
	//blockContext ethvm.BlockContext
	//processType ProcessType
}

func NewTask(gp *core.GasPool, cfg ethvm.Config, config *params.ChainConfig ,bcon ethvm.BlockContext ,stateDb interfaces.StateDB ,item *transcut.Item, usedGas *uint64, level int) *Task {
	return &Task{
		item:        item,
		//gp:          gp,
		//cfg:         cfg,
		//config:     config,
		//stateDb:     stateDb,
		//usedGas:     usedGas,
		//level:       level,
		//blockContext: bcon,
		//processType: ParallelProcessType,
	}
}

//func (t *Task) DoTask() *Result {
//	txContext := ethvm.TxContext{ReadSet: ethvm.NewReadSet(), WriteSet: ethvm.NewWriteSet(), TxData: t.item.TxInfo().Data()}
//	vm := ethvm.NewVM(t.blockContext, txContext, t.stateDb, t.config, t.cfg)
//	return ApplyTransactionWithFlag(t.item.MessageInfo(),t.item.TxInfo(),t.stateDb,vm,t.usedGas,t.gp,t.level,t.processType,nil)
//}

type ExecPool struct {
	wg         sync.WaitGroup
	taskNum    int
	ExecFunc   func(item *transcut.Item) *Result
	taskQueue  chan []*transcut.Item
	ResultChan chan *Result
}

func NewExecPool(routine, num int,fun func(item *transcut.Item) *Result) *ExecPool {
	pool := &ExecPool{
		taskNum: routine,
		ExecFunc: fun,
		taskQueue:  make(chan []*transcut.Item, num),
		ResultChan: make(chan *Result, num),
	}
	return pool
}

func (p *ExecPool) Run() {
	for i := 0; i < p.taskNum; i++ {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			for {
				select {
				case task, ok := <-p.taskQueue:
					if !ok {
						return
					}
					//txContext := ethvm.TxContext{ReadSet: ethvm.NewReadSet(), WriteSet: ethvm.NewWriteSet(), TxData: task.item.TxInfo().Data()}
					//vm := ethvm.NewVM(task.blockContext, txContext, task.stateDb, task.config, task.cfg)
					//res := ApplyTransactionWithFlag(task.item.MessageInfo(),task.item.TxInfo(),task.stateDb,vm,task.usedGas,task.gp,task.level,task.processType,nil)
					for _,item :=range task {
						res := p.ExecFunc(item)
						p.ResultChan <- res
					}
				}
			}
		}()
	}
}

func (p *ExecPool) Wait() {
	close(p.taskQueue)
	p.wg.Wait()
}

func (p *ExecPool) AddTask(task []*transcut.Item) {
	select {
	case p.taskQueue <- task:
	}
}

func FillUpPoolWithTask(items []*transcut.Item, gp *core.GasPool, stateDb interfaces.StateDB, usedGas *uint64, index int, blockContext ethvm.BlockContext, p *Processor, cfg ethvm.Config) []*Result {
	stateCopy := stateDb.Copy()
	handler := func(item *transcut.Item) *Result{
		txContext := ethvm.TxContext{ReadSet: ethvm.NewReadSet(), WriteSet: ethvm.NewWriteSet(), TxData: item.TxInfo().Data()}
		vm := ethvm.NewVM(blockContext, txContext, stateCopy, p.config, cfg)
		res := ApplyTransactionWithFlag(item.MessageInfo(),item.TxInfo(),stateDb,vm,usedGas,gp,index,ParallelProcessType,nil)
		return res
	}

	count := runtime.NumCPU()
	pool := NewExecPool(count, len(items),handler)
	pool.Run()

	partItem:= make([][]*transcut.Item,count)
	//for ind,item := range items{
	//	leveNum := ind%count
	//	partItem[leveNum] = append(partItem[leveNum] ,item)
	//}
	num := len(items)/count
	elsNum := len(items)%count
	for i:=0;i < count;i++ {
		if i == count - 1 {
			partItem[i] = append(partItem[i],items[i*num:num*(i+1)+elsNum]...)
		}else {
			partItem[i] = append(partItem[i],items[i*num:num*(i+1)]...)
		}
		pool.AddTask(partItem[i])
	}
	pool.Wait()
	tpResult := make([]*Result,0)
	//////需要等待第一批次的交易都执行完自开启下一批次
	for  {
		select {
		case v, ok := <-pool.ResultChan:
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
				tpResult = append(tpResult, result)
			}
		}

		if len(tpResult) == len(items) {
			close(pool.ResultChan)
			break
		}
	}

	return tpResult
}
