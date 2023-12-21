package transcut

import (
	metatypes "github.com/CaduceusMetaverseProtocol/MetaTypes/types"
	core2 "github.com/CaduceusMetaverseProtocol/MetaVM/core"
	"github.com/CaduceusMetaverseProtocol/MetaVM/params"
	"github.com/ethereum/go-ethereum/crypto"
	"math/big"
)

/**from的标记**/
type Item struct {
	txInfo *core2.MetaTransaction
	msg    *core2.Message
	author *metatypes.Address
	mark   uint32 //所处批次
}

func NewItem(tx *core2.MetaTransaction, msg *core2.Message, author *metatypes.Address) *Item {
	return &Item{
		txInfo: tx,
		msg:    msg,
		author: author,
	}
}

func FillingTransCut(baseFee *big.Int, tx *core2.MetaTransaction, chainCnf *params.ChainConfig, cache *MarkCache) error {
	msg := tx.AsMessage(baseFee)
	item := NewItem(tx, &msg, nil)
	from := msg.From()
	cache.SetMarkCache(from, item)
	return nil
}

func (i *Item) TxInfo() *core2.MetaTransaction {
	return i.txInfo
}

func (i *Item) MessageInfo() *core2.Message  {
	return i.msg
}

type MarkCache struct {
	fromAndToMark  map[metatypes.Address]uint32
	Items          [][]*Item
	maxLevel       uint32
}

func NewMarkCache() *MarkCache {
	return &MarkCache{
		fromAndToMark: make(map[metatypes.Address]uint32,10000),
		Items: make([][]*Item,2),
	}
}

func (m *MarkCache) SetMarkCache(addr metatypes.Address, data *Item) {
	fromMark,ok := m.fromAndToMark[addr]
	if !ok {
		fromMark = 0
	}else {
		fromMark = fromMark+1
	}
	toAddr := data.TxInfo().To()
	var toMark uint32
	if toAddr != nil {
		toMark,ok := m.fromAndToMark[*toAddr]
		if !ok {
			toMark = 0
		}else {
			toMark = toMark+1
		}

		if fromMark >= toMark {
			toMark = fromMark
		}else {
			fromMark = toMark
		}

		if toMark >=m.maxLevel {
			m.maxLevel = toMark
		}
	}else {
		if fromMark >= m.maxLevel {
			m.maxLevel = fromMark
		}
	}

	m.fromAndToMark[addr] = fromMark
	m.fromAndToMark[*toAddr] = toMark


	if len(m.Items) == 0 {
		items := []*Item{data}
		m.Items = append(m.Items,items)
	}else {
		if len(m.Items) <= int(m.maxLevel) {
			for i := len(m.Items); i<=int(m.maxLevel); i++ {
				m.Items = append(m.Items,[]*Item{})
			}
		}
		
		if len(m.Items[m.maxLevel]) == 0 {
			items := []*Item{data}
			m.Items[m.maxLevel] = items
		}else {
			items := m.Items[m.maxLevel]
			items = append(items,data)
			m.Items[m.maxLevel] = items
		}
	}
}

func (m *MarkCache) GetLevelItemWithMark(index uint32) []*Item {
	return m.Items[index]
}

func (m *MarkCache) GetMaxLevel() uint32 {
	return m.maxLevel+1
}

//type FromMarkCache struct {
//	fromCache map[metatypes.Address][]*Item
//	toCache   map[metatypes.Address]uint32
//	maxLevel  uint32
//	lock      sync.RWMutex
//}
//
//func NewFromMarkCache() *FromMarkCache {
//	return &FromMarkCache{
//		fromCache: make(map[metatypes.Address][]*Item,100000),
//		toCache:   make(map[metatypes.Address]uint32,100000),
//	}
//}

/**放入到标记档中标记无需设置
	0x0001 -> 0x0002  0
	0x0001 -> 0x0003  1
	0x0004 -> 0x0001  2
-------------------------------
	0x0001 -> 0x0002  0
	0x0002 -> 0x0003  1
      0x0001 -> 0x0004  2
-------------------------------
	0x0001 -> 0x0002  0
      0x0003 -> 0x0005  0
	0x0004 -> 0x0006  0
	0x0002 -> 0x0003  1
**/

func EthAddress(namespace string) metatypes.Address {
	return metatypes.BytesToAddress(crypto.Keccak256([]byte(namespace))[12:])
}

/**
 应该没有考虑到情况
	0x0001 -> 0x0002  0
      0x0001 -> 0x0003  0
	0x0004 -> 0x0006  0
	0x0004 -> 0x0001  0 -> 1
**/

//func (f *FromMarkCache) SetMarkCache(addr metatypes.Address, data *Item) {
//	f.lock.Lock()
//	defer f.lock.Unlock()
//	items, ok := f.fromCache[addr]
//	toAddr := data.txInfo.To()
//	dataTo := toAddr
//	if dataTo == nil {
//		accountAddr := EthAddress("createAccount")
//		dataTo = &accountAddr
//	}
//	if ok {
//		itemLen := len(items)
//		item := items[itemLen-1]
//		data.mark = item.mark + 1
//		//这里需要判断一下  赋予批次的那个批次数中有没有跟to相关的
//	LOOP:
//		flag := f.checkConflict(data.mark, dataTo)
//		if flag {
//			//有冲突
//			data.mark = data.mark + 1
//			goto LOOP
//		}
//
//		if f.maxLevel < data.mark {
//			f.maxLevel = data.mark
//		}
//		items = append(items, data)
//		f.toCache[*dataTo] = data.mark
//		f.fromCache[addr] = items
//	} else {
//		items = make([]*Item, 0)
//		//如果交易的from不在cache中
//		//1.需要判断to是否在以前交易的from中
//		//2.需要判断to是否在以前交易的to中
//		//3.如果都没有就则标记应为0
//		items, ok = f.fromCache[*dataTo]
//		if ok {
//			item := items[len(items)-1]
//			level := item.mark
//			data.mark = level + 1
//			//然后看同一层级的交易里面有没有同一个to的
//		LOOP2:
//			flag := f.checkConflict(data.mark, dataTo)
//			if flag {
//				//有冲突
//				data.mark = data.mark + 1
//				goto LOOP2
//			}
//			if f.maxLevel < data.mark {
//				f.maxLevel = data.mark
//			}
//			items = append(items, data)
//			f.toCache[*dataTo] = data.mark
//			f.fromCache[*dataTo] = items
//		} else {
//			mark, ok := f.toCache[*dataTo]
//			if ok {
//				data.mark = mark + 1
//			}
//			if f.maxLevel < data.mark {
//				f.maxLevel = data.mark
//			}
//			items = append(items, data)
//			f.toCache[*dataTo] = data.mark
//			f.fromCache[addr] = items
//		}
//	}
//}
//
//func (f *FromMarkCache) checkConflict(level uint32, toAddr *metatypes.Address) bool {
//	items := f.GetLevelItemWithMark(level)
//	if len(items) == 0 {
//		return false
//	} else {
//		for _, item := range items {
//			to := item.TxInfo().To()
//			if bytes.Compare(to.Bytes(),toAddr.Bytes()) == 0 {
//				return true
//			}
//
//
//
//		}
//		return false
//	}
//	return true
//}
//
///**找到某个层级的所有交易**/
//func (f *FromMarkCache) GetLevelTxsWithMark(index uint32) []*core2.MetaTransaction {
//	arr := make([]*core2.MetaTransaction, 0)
//	for _, v := range f.fromCache {
//		for _, data := range v {
//			if data.mark == index {
//				arr = append(arr, data.txInfo)
//			}
//		}
//	}
//	return arr
//}
//
///**找某个层级的所有item**/
//func (f *FromMarkCache) GetLevelItemWithMark(index uint32) []*Item {
//	arr := make([]*Item, 0)
//	for _, v := range f.fromCache {
//		for _, data := range v {
//			if data.mark == index {
//				arr = append(arr, data)
//			}
//		}
//	}
//	return arr
//}
//
//func (f *FromMarkCache) GetMaxLevel() uint32 {
//	return f.maxLevel + 1
//}
