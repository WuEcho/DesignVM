package process

import (
	"github.com/CaduceusMetaverseProtocol/MetaNebula/common/log"
	basev1 "github.com/CaduceusMetaverseProtocol/MetaProtocol/gen/proto/go/base/v1"
	metatypes "github.com/CaduceusMetaverseProtocol/MetaTypes/types"
	core2 "github.com/CaduceusMetaverseProtocol/MetaVM/core"
	"github.com/CaduceusMetaverseProtocol/MetaVM/interfaces"
	"github.com/CaduceusMetaverseProtocol/MetaVM/params"
	"github.com/CaduceusMetaverseProtocol/MetaVM/transcut"
	"github.com/CaduceusMetaverseProtocol/MetaVM/vm/ethvm"
	"github.com/ethereum/go-ethereum/core"
	//"github.com/prometheus/common/log"
)

type RWRecord struct {
	Res       *Result
	level     int
	Tx        *core2.MetaTransaction
	errorInfo error
	vmError   error //虚拟机的错
	message   Message
	TxIndex   uint32
	Logs      []*basev1.MetaTxLog
}

func NewRWRecord() *RWRecord {
	return &RWRecord{
		Res:  &Result{},
		Logs: make([]*basev1.MetaTxLog, 0),
	}
}

func (rw *RWRecord) Level() int {
	return rw.level
}

func (rw *RWRecord) SetResult(res *Result) {
	rw.Res = res
}

func (rw *RWRecord) SetTxAndMsgInfo(tx *core2.MetaTransaction, message Message) {
	rw.Tx = tx
	rw.message = message
}

func (rw *RWRecord) GetTxAndMsg() (*core2.MetaTransaction, Message) {
	return rw.Tx, rw.message
}

func (rw *RWRecord) SetLogs(logs []*basev1.MetaTxLog) {
	rw.Logs = logs
}

func (rw *RWRecord) GetLogs() []*basev1.MetaTxLog {
	return rw.Logs
}

func (rw *RWRecord) SetCurrentRecordLevel(level int) {
	rw.level = level
}

func (rw *RWRecord) SetErrorInfo(err error) {
	rw.errorInfo = err
}

func (rw *RWRecord) GetError() error {
	return rw.errorInfo
}

func (rw *RWRecord) SetVmErrorInfo(err error) {
	rw.vmError = err
}

func (rw *RWRecord) GetVmError() error {
	return rw.vmError
}

func (rw *RWRecord) SetTxIndex(index uint32) {
	rw.TxIndex = index
}

func (rw *RWRecord) GetTxIndex() uint32 {
	return rw.TxIndex
}

func (rw *RWRecord) GetRRecordKeyCount() uint32 {
	return rw.Res.ReadSet.Len()
}

func (rw *RWRecord) GetWRecordKeyCount() uint32 {
	return rw.Res.WriteSet.Len()
}

func (rw *RWRecord) GetValueByKey(addr metatypes.Address, key metatypes.Hash) (interface{}, bool) {
	record, ok := rw.Res.WriteSet.Get(addr)
	if !ok {
		record, ok = rw.Res.ReadSet.Get(addr)
		if !ok {
			return []byte{}, false
		}

		res, ok := record[key]
		if !ok {
			return []byte{}, false
		}
		return res, true
	}
	//写集里面没有从读集里面检索一下
	res, ok := record[key]
	if !ok {
		return []byte{}, false
	}
	return res, true
}

type CacheSet struct {
	cache map[metatypes.Hash]*RWRecord
	Order []metatypes.Hash
}

func NewCacheSet(length int) *CacheSet {
	return &CacheSet{
		cache: make(map[metatypes.Hash]*RWRecord, length),
		Order: make([]metatypes.Hash, 0),
	}
}

func (c *CacheSet) SetCache(txHash metatypes.Hash, record *RWRecord) {
	_, ok := c.cache[txHash]
	if !ok {
		c.cache[txHash] = record
		c.Order = append(c.Order, txHash)
	}
}

func (c *CacheSet) GetRecordWithTxHash(txHash metatypes.Hash) (*RWRecord, bool) {
	v, ok := c.cache[txHash]
	return v, ok
}

func (c *CacheSet) GetTxAndMsgFromCache(txHash metatypes.Hash) (*core2.MetaTransaction, Message, bool) {
	v, ok := c.cache[txHash]
	if ok {
		tx, msg := v.GetTxAndMsg()
		return tx, msg, true
	}
	return nil, nil, false
}

func (c *CacheSet) DelRecordWithTxHash(txHash metatypes.Hash) {
	delete(c.cache, txHash)
}

func (c *CacheSet) GetValueByAddressAndKey(address metatypes.Address, key metatypes.Hash) (interface{}, bool) {
	for _, record := range c.cache {
		//优先从写集里面检索
		return record.GetValueByKey(address, key)
	}
	return []byte{}, false
}

func (c *CacheSet) GetRecordByTxHash(txHash metatypes.Hash) *RWRecord {
	return c.cache[txHash]
}

type shouldRelExecTx struct {
	txHash metatypes.Hash
	index  uint64
}

func DetectConflicts(rwSet *CacheSet, cache *transcut.MarkCache, wCache *WrittenBefore, reses []*Result, statedb interfaces.StateDB, blockContext ethvm.BlockContext, cfg ethvm.Config, config *params.ChainConfig, gp *core.GasPool, usedGas *uint64, level int) ([]*Result, interfaces.StateDB) {
	var itemsCount int
	if level > 0 {
		for ; level >= 0; level-- {
			items := cache.GetLevelItemWithMark(uint32(level))
			itemsCount += len(items)
		}
	}

	//maxLeve := cache.GetMaxLevel()
	conflictTxHashes := make([]*shouldRelExecTx, 0)
	var shouldRexIndex uint32
	for txIndex, txHash := range rwSet.Order {
		shouldWrite := true
		record, ok := rwSet.GetRecordWithTxHash(txHash)
		if ok {
			for addr, rrecord := range record.Res.ReadSet.Cache() {
				keyMap, ok := wCache.Written[addr]
				if ok {
					for key := range rrecord {
						if _, ok := keyMap[key]; ok {
							log.Info("存在冲突的交易----", "  address:", addr.String(), " 冲突key:", key.String())
							//TODO 存在冲突的交易顺序需要向后调整
							conflictTxHashes = append(conflictTxHashes, &shouldRelExecTx{
								txHash: txHash,
							})
							shouldWrite = false
							break
						}
					}
					if shouldWrite == false {
						break
						shouldRexIndex++
					}
				}
			}

			if shouldWrite {
				record.TxIndex = uint32(txIndex) - shouldRexIndex
				for addr, wrecord := range record.Res.WriteSet.Cache() {
					cacheValue, ok := wCache.Written[addr]
					if !ok {
						cacheValue = make(map[metatypes.Hash]interface{}, 0)
						cacheValue = wrecord
					}else {
						for key, value := range wrecord {
							cacheValue[key] = value
						}
					}

					if record.vmError != nil || record.GetError() != nil {
						onceValue,ok := wrecord[NonceKey]
						if ok {
							nCache := make(map[metatypes.Hash]interface{},1)
							nCache[NonceKey] = onceValue
							cacheValue = nCache
						}
					}
					wCache.Written[addr] = cacheValue
					wCache.Logs[txHash] = record.Logs
					reses = append(reses, record.Res)
				}
			}
		}
	}

	statedb = UpdateWrittenCacheToDataBase(wCache, statedb)

	reses, statedb = RelExecTx(conflictTxHashes, rwSet, wCache, reses, statedb, blockContext, cfg, config, gp, usedGas, level, uint32(len(rwSet.Order))-shouldRexIndex)

	return reses, statedb
}

func RelExecTx(shouldRel []*shouldRelExecTx, rwSet *CacheSet, wCache *WrittenBefore, reses []*Result, stateDb interfaces.StateDB, blockContext ethvm.BlockContext, cfg ethvm.Config, config *params.ChainConfig, gp *core.GasPool, usedGas *uint64, level int, baseIndex uint32) ([]*Result, interfaces.StateDB) {
	for index, relExecTx := range shouldRel {
		log.Info("RelExecTx----", "rel execTxHash:", relExecTx.txHash.String())
		tx, msg, ok := rwSet.GetTxAndMsgFromCache(relExecTx.txHash)
		if ok {
			vm := ethvm.NewVM(blockContext, ethvm.TxContext{ReadSet: ethvm.NewReadSet(),
				WriteSet: ethvm.NewWriteSet()}, stateDb, config, cfg)
			orgRes := reses[relExecTx.index]
			gp = gp.AddGas(orgRes.Receipt.GasUsed)
			val := *usedGas - orgRes.Receipt.GasUsed
			usedGas = &val
			result := ApplyTransactionWithFlag(msg, tx, stateDb, vm, usedGas, gp, level, RelProcessType, orgRes)
			res := &Result{
				Receipt:  result.Receipt,
				Ed:       result.Ed,
				VmErr:    result.VmErr,
				ReadSet:  result.ReadSet,
				WriteSet: result.WriteSet,
				Tx:       tx,
				Msg:      msg,
			}
			res.Receipt.GasUsed = orgRes.Receipt.GasUsed
			res.Receipt.TxIndex = baseIndex + uint32(index)
			//TODO
			//log.Info("重新被执行的交易---","tx hash：",txHash.Hex(),"  gas：",res.Receipt.GasUsed)
			reses = append(reses, res)
			if result.VmErr != nil || res.Ed.err != nil {
				for addr, cache := range result.WriteSet.Cache() {
					value,ok := cache[NonceKey]
					if ok {
						nCache := make(map[metatypes.Hash]interface{},1)
						nCache[NonceKey] = value
						result.WriteSet.Set(addr, nCache)
					}
				}
			}
			wCache.Written = result.WriteSet.Cache()
			wCache.Logs[relExecTx.txHash] = result.Logs
			stateDb = UpdateWrittenCacheToDataBase(wCache, stateDb)
		}
	}
	return reses, stateDb
}

/**
1.写集小的往前排
2.写集相同情况下读集大的往前排
**/
func (c *CacheSet) Len() int {
	return len(c.Order)
}

func (c *CacheSet) Less(i, j int) bool {
	itemIKey := c.Order[i] //itemIKey is txHash in CacheSet order slice index i

	iWkeyCount := c.cache[itemIKey].GetWRecordKeyCount()
	iRkeyCount := c.cache[itemIKey].GetRRecordKeyCount()

	itemJkey := c.Order[j]

	jWkeyCount := c.cache[itemJkey].GetWRecordKeyCount()
	jRkeyCount := c.cache[itemJkey].GetRRecordKeyCount()

	if jWkeyCount < iWkeyCount {
		return true
	} else if (jWkeyCount == iWkeyCount) && (jRkeyCount > iRkeyCount) {
		return true
	} else if (jWkeyCount == iWkeyCount) && (jRkeyCount == iRkeyCount) {
		//当修改的账户数量相同的时候就看账户里面读写的key数量
		var jrecordwKeyCount uint32
		var irecordwKeyCount uint32
		jrecord := c.cache[itemJkey]
		irecord := c.cache[itemIKey]
		//compare write key count
		jrecordwKeyCount = jrecord.Res.WriteSet.GetDetailKeyCount()
		irecordwKeyCount = irecord.Res.WriteSet.GetDetailKeyCount()

		if jrecordwKeyCount < irecordwKeyCount {
			return true
		}
		var jrecordRKeyCount uint32
		var irecordRKeyCount uint32
		//compare read key count
		jrecordRKeyCount = jrecord.Res.ReadSet.GetDetailKeyCount()
		irecordRKeyCount = irecord.Res.ReadSet.GetDetailKeyCount()

		if jrecordRKeyCount < irecordRKeyCount {
			return true
		}

		if jrecordRKeyCount == irecordRKeyCount && jrecordwKeyCount == irecordwKeyCount {
			return itemIKey.Compare(itemJkey) == 1
		}
	}
	return false
}

func (c *CacheSet) Swap(i, j int) {
	c.Order[i], c.Order[j] = c.Order[j], c.Order[i]
}
