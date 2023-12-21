package ethvm

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	crypto2 "github.com/CaduceusMetaverseProtocol/MetaNebula/common/crypto"
	basev1 "github.com/CaduceusMetaverseProtocol/MetaProtocol/gen/proto/go/base/v1"
	types2 "github.com/CaduceusMetaverseProtocol/MetaTypes/types"
	"github.com/CaduceusMetaverseProtocol/MetaVM/params"
	common2 "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/crypto"
	ethdb2 "github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/go-interpreter/wagon/exec"
	"github.com/go-interpreter/wagon/wasm"
	"golang.org/x/crypto/ed25519"
	"math/big"
	"sync"
	"sync/atomic"
	"time"
)

var SolidityCode = []byte{100,115,111,108,99,67,0,8,17,0,51}
// magic number for Wasm is "\0asm"
// See https://webassembly.github.io/spec/core/binary/modules.html#binary-module
var wasmIdent = []byte("\x00\x61\x73\x6D")
var eeiFunctionList = []string{
	"useGas",
	"getAddress",
	"getExternalBalance",
	"getBlockHash",
	"call",
	"callDataCopy",
	"getCallDataSize",
	"callCode",
	"callDelegate",
	"callStatic",
	"storageStore",
	"storageLoad",
	"getCaller",
	"getCallValue",
	"codeCopy",
	"getCodeSize",
	"getBlockCoinbase",
	"create",
	"getBlockDifficulty",
	"externalCodeCopy",
	"getExternalCodeSize",
	"getGasLeft",
	"getBlockGasLimit",
	"getTxGasPrice",
	"log",
	"getBlockNumber",
	"getTxOrigin",
	"finish",
	"revert",
	"getReturnDataSize",
	"returnDataCopy",
	"selfDestruct",
	"getBlockTimestamp",
	// add by echo : 扩展 ewasm 存储功能 >>>>
	"storageStore2",
	"storageLoad2",
	// add by echo : 扩展 ewasm 存储功能 <<<<
}

var debugFunctionList = []string{
	"printMemHex",
	"printStorageHex",
}

// ModuleResolver matches all EEI functions to native go functions
func ModuleResolver(interpreter *EWASMInterpreter, name string) (*wasm.Module, error) {
	if name == "debug" {
		debugModule := wasm.NewModule()
		debugModule.Types = eeiTypes
		debugModule.FunctionIndexSpace = getDebugFuncs(interpreter)
		entries := make(map[string]wasm.ExportEntry)
		for idx, name := range debugFunctionList {
			entries[name] = wasm.ExportEntry{
				FieldStr: name,
				Kind:     wasm.ExternalFunction,
				Index:    uint32(idx),
			}
		}
		debugModule.Export = &wasm.SectionExports{
			Entries: entries,
		}
		return debugModule, nil
	}

	if name != "ethereum" {
		return nil, fmt.Errorf("Unknown module name: %s", name)
	}

	m := wasm.NewModule()
	m.Types = eeiTypes
	m.FunctionIndexSpace = eeiFuncs(interpreter)

	entries := make(map[string]wasm.ExportEntry)

	for idx, name := range eeiFunctionList {
		entries[name] = wasm.ExportEntry{
			FieldStr: name,
			Kind:     wasm.ExternalFunction,
			Index:    uint32(idx),
		}
	}

	m.Export = &wasm.SectionExports{
		Entries: entries,
	}

	return m, nil
}

func WrappedModuleResolver(in *EWASMInterpreter) wasm.ResolveFunc {
	return func(name string) (*wasm.Module, error) {
		return ModuleResolver(in, name)
	}
}

type (
	ewasmFuns struct {
		dblock     *sync.Mutex
		spub       [32]byte
		magic      [4]byte
		chaindb    ethdb2.Database
		codedb     ethdb2.Database
		synchronis *int32
		once       *sync.Once
		stop       chan struct{}
		auditTask  chan []byte
	}
	auditTask struct {
		Key                 *big.Int
		Txhash              common2.Hash
		Status, BlockNumber *big.Int
	}
)

var (
	EwasmFuncs = &ewasmFuns{
		dblock: new(sync.Mutex),
		spub: [32]byte{78, 213, 66, 231, 2, 216, 32, 136,
			71, 233, 64, 132, 125, 45, 77, 101,
			222, 209, 181, 20, 212, 62, 181, 43,
			190, 87, 231, 13, 194, 112, 244, 183},
		magic:  [4]byte{0, 80, 68, 88},
		codedb: rawdb.NewMemoryDatabase(),
		once:   new(sync.Once),
		stop:   make(chan struct{}),
	}
	taskNumKey = crypto.Keccak256([]byte("task-num"))
	taskPreKey = crypto.Keccak256([]byte("task-pre"))
	auditLimit = int64(20)
)

func (f *ewasmFuns) Shutdown() {
	log.Debug("ewasmFuncs-shutdown")
	if f.codedb != nil {
		f.codedb.Close()
	}
	close(f.stop)
}

func (f *ewasmFuns) DelCode(key []byte) {
	err := f.codedb.Delete(key)
	if err != nil {
		log.Error("DelCode", "key", hex.EncodeToString(key), "err", err)
	}
	log.Debug("DelCode", "key", hex.EncodeToString(key))
}

func (f *ewasmFuns) GetCode(key []byte) (val []byte, ok bool) {
	ok = true
	var v interface{}
	v, err := f.codedb.Get(key)
	if err != nil {
		return nil, false
	}
	val = v.([]byte)
	log.Debug("GetCode", "key", hex.EncodeToString(key), "ok", ok)
	return
}

// codekey -> finalkey -> final
func (f *ewasmFuns) PutCode(key, val []byte) {
	err := f.codedb.Put(key, val)
	if err != nil {
		log.Error("PutCode", "key", hex.EncodeToString(key), "val.len", len(val), "err", err)
	}
	log.Debug("PutCode", "key", hex.EncodeToString(key), "val.len", len(val))
}

func (f *ewasmFuns) IsFinalcode(finalcode []byte) bool {
	if len(finalcode) < 72 || !f.IsWASM(finalcode) {
		return false
	}
	//tail := finalcode[len(finalcode)-8:]
	//if bytes.Equal(append(f.magic[:], params.PDXTEwasm), tail[:5]) {
	tail := finalcode[len(finalcode)-8:]
	if bytes.Equal(f.magic[:], tail[:4]) {
		return true
	}
	log.Debug("IsFinalcode-error", "last7", finalcode[len(finalcode)-8:], "want-last4", f.magic[:])
	return false
}

// finalcode == {code,final,[4]byte,[4]byte}
// [4]byte 魔术字节
// [4]byte final 的字节长度
func (f *ewasmFuns) SplitTxdata(finalcode []byte) (code, final []byte, err error) {
	if len(finalcode) < 73 {
		return nil, nil, errors.New("error format of ewasm finalcode ")
	}
	//tail := finalcode[len(finalcode)-8:]
	//l := new(big.Int).SetBytes(tail[5:]).Int64()
	//size := len(finalcode)
	//code = finalcode[:size-int(l)-8]
	//final = finalcode[size-int(l)-8 : size-8]

	tail := finalcode[len(finalcode)-8:]
	l := new(big.Int).SetBytes(tail[4:]).Int64()
	size := len(finalcode)
	code = finalcode[:size-int(l)-8]
	final = finalcode[size-int(l)-8 : size-8]
	_, err = f.ValidateCode(final)
	if err != nil {
		return nil, nil, err
	}
	log.Info("SplitTxdata", "code", len(code), "final", len(final), "finalcode", len(finalcode), "last7", finalcode[len(finalcode)-7:])
	return code, final, nil
}

// finalcode == {code,final,[4]byte,[4]byte}
// [4]byte 魔术字节
// [4]byte final 的字节长度
func (f *ewasmFuns) JoinTxdata(code, final []byte) ([]byte, error) {
	if f.IsFinalcode(code) || f.IsFinalcode(final) {
		return nil, errors.New("reject join")
	}
	_, err := f.ValidateCode(final)
	if err != nil {
		return nil, err
	}
	finallen := int64(len(final))
	if finallen > 16777215 {
		return nil, errors.New("ewasm code too long")
	}
	i24 := big.NewInt(finallen)
	finalcode := make([]byte, 0)
	finalcode = append(finalcode, code[:]...)
	finalcode = append(finalcode, final[:]...)
	finalcode = append(finalcode, f.magic[:]...)
	//finalcode = append(finalcode, params.PDXTEwasm)
	b24 := i24.Bytes()
	bl := 4 - len(b24)
	if bl > 0 {
		r := make([]byte, bl)
		b24 = append(r[:], b24[:]...)
	}
	finalcode = append(finalcode, b24[:]...)
	log.Info("JoinTxdata", "code", len(code), "final", len(final), "finalcode", len(finalcode), "last8", finalcode[len(finalcode)-8:])
	return finalcode, nil
}

func (f *ewasmFuns) ValidateCode(code []byte) ([]byte, error) {
	if len(code) > 64 && f.IsWASM(code) {
		var (
			sig = code[len(code)-64:]
			msg = code[:len(code)-64]
		)
		ret := ed25519.Verify(f.spub[:], msg, sig)
		//fmt.Println(ret, code)
		if ret {
			return msg, nil
		}
	}
	return nil, errors.New("bad ewasm code")
}

func (f *ewasmFuns) IsWASM(code []byte) bool {
	//flag := bytes.HasSuffix(code,SolidityCode)
	//if len(code) < 41 || flag {
	//	return false
	//}
	//return true
	return bytes.Equal(code[:4], wasmIdent)
}


func (f *ewasmFuns) Sentinel(code []byte) ([]byte, error) {
	var (
		err     error
		mainSig *wasm.FunctionSig
		ethEvm     = NewVM(BlockContext{}, TxContext{ReadSet: NewReadSet(),WriteSet: NewWriteSet(),Logs: make([]*basev1.MetaTxLog,0)}, nil, params.TestChainConfig, Config{})
		in      = &EWASMInterpreter{
			StateDB:  ethEvm.StateDB,
			ethvm:    ethEvm,
		}
		meteringa = types2.HexToAddress(sentinelContractAddress)
		meteringf = AccountRef(types2.HexToAddress("0x110"))
		meteringt = AccountRef(meteringa)
	)
	in.meteringModule, err = wasm.ReadModule(bytes.NewReader(meteringCode), WrappedModuleResolver(in))
	if err != nil {
		log.Error("sentinel execute fail 0", "err", err)
		return nil, err
	}

	in.meteringStartIndex = int64(in.meteringModule.Export.Entries["main"].Index)
	mainSig = in.meteringModule.FunctionIndexSpace[in.meteringStartIndex].Sig
	if len(mainSig.ParamTypes) != 0 || len(mainSig.ReturnTypes) != 0 {
		log.Error("sentinel execute fail 1", "err", err)
		return nil, err
	}

	in.contract = NewContract(meteringf, meteringt, new(big.Int), 9999999999)
	in.contract.SetCallCode(&meteringa, crypto2.Keccak256Hash(meteringCode), meteringCode)
	vm, err := exec.NewVM(in.meteringModule)
	if err != nil {
		log.Error("sentinel execute fail 2", "err", err)
		return nil, err
	}
	vm.RecoverPanic = true
	in.vm = vm
	in.contract.Input = code
	start := time.Now()
	final, err := in.vm.ExecCode(in.meteringStartIndex)
	if err != nil {
		log.Error("sentinel execute fail 3", "err", err)
		return nil, err
	}
	if final == nil {
		final = in.returnData
	}
	_, err = f.ValidateCode(final.([]byte))
	log.Info("Sentinel", "timeused", time.Since(start), "code", len(code), "final", len(final.([]byte)))
	return final.([]byte), err
}

// 当 save == nil 时获取最新的可用 num，否则保存 save
func (f *ewasmFuns) newTask(k []byte, save *big.Int) *big.Int {
	n := big.NewInt(0)
	if f.codedb == nil {
		fmt.Println("TEST")
		return n
	}
	if save != nil {
		if err := f.codedb.Put(k, save.Bytes()); err != nil {
			panic(err)
		}
		return save
	}
	if buf, err := f.codedb.Get(k); err == nil {
		n = new(big.Int).SetBytes(buf)
	}
	return n
}

func (f *ewasmFuns) processAuditTask(atask *auditTask) {
	tx, _, num, _ := rawdb.ReadTransaction(f.chaindb, atask.Txhash)
	if tx == nil {
		log.Error("processAuditTask ReadTransaction fail","txhash",atask.Txhash)
		return
	}
	atask.BlockNumber = big.NewInt(int64(num))
	code := tx.Data()
	//codekey := crypto.Keccak256(code)
	codekey := f.GenCodekey(code)
	final, ok := f.GetCode(codekey)
	//if ok && len(final) == 32 {
	if ok && len(final) == 31 {
		final, ok = f.GetCode(final)
	}
	log.Debug("processAuditTask-start", "num", num, "code", len(code), "final", len(final), "codekey", common2.BytesToHash(codekey).Hex())
	if atask.Key != nil && ok {
		if _final, err := f.Sentinel(code); err == nil {
			log.Debug("processAuditTask", "sentinel", len(_final))
			if _, err = f.ValidateCode(_final); err == nil {
				log.Debug("processAuditTask", "validateCode", len(_final))
				if bytes.Equal(crypto.Keccak256(final), crypto.Keccak256(_final)) {
					log.Debug("processAuditTask", "audit-result", "ok")
					atask.Status = big.NewInt(1)
				}
			}
		}
	}
	finalstatekey := crypto.Keccak256(final, []byte("status"))
	finalstateval, err := rlp.EncodeToBytes(atask)
	if err != nil {
		// 不接受异常
		panic(err)
	}
	err = f.codedb.Put(finalstatekey, finalstateval)
	log.Debug("processAuditTask-success", "finalstatekey", finalstatekey, "hex", common2.BytesToHash(finalstatekey).Hex(), "atask", atask)
	if err != nil {
		// 不接受异常
		panic(err)
	}
	if atask.Key != nil {
		f.newTask(taskPreKey, atask.Key)
	}
}

func (f *ewasmFuns) auditLoop() {
	if f.codedb == nil {
		fmt.Println("TEST")
		return
	}
	log.Info("ewasmFuns-auditLoop-start")
	preNum := f.newTask(taskPreKey, nil)
	lastNum := f.newTask(taskNumKey, nil)
	cap := lastNum.Int64() - preNum.Int64() + 100
	f.auditTask = make(chan []byte, cap)
	log.Debug("ewasmFuns-auditLoop-params", "preNum", preNum, "lastNum", lastNum, "taskCap", cap)
	for i := preNum.Int64(); i < lastNum.Int64(); i++ {
		if i == 0 {
			continue
		}
		k := big.NewInt(i)
		f.auditTask <- k.Bytes()
		log.Debug("ewasmFuncs-auditLoop-reload-task", "tk", k)
	}
	for {
		select {
		case <-f.stop:
			return
		case tk := <-f.auditTask:
			// TODO 并行处理
			tv, _ := f.codedb.Get(tk)
			task := new(auditTask)
			err := rlp.DecodeBytes(tv, task)
			if err == nil {
				log.Info("ewasmFuns-auditLoop-task", "err", err, "tk", tk, "tv", tv, "task", task)
				// 先生成任务数据，再去更新
				go f.processAuditTask(task)
			}
		}
	}
}

func (f *ewasmFuns) GenAuditTask(txhash common2.Hash) {
	if f.synchronis != nil && atomic.LoadInt32(f.synchronis) == 1 {
		// 正在同步，跳过验证，没有任务，直接生成结果
		log.Debug("ewasmFuns-GenAuditTask-skip : default success", "tx", txhash.Hex())
		atask := &auditTask{
			Status: big.NewInt(1),
			Txhash: txhash,
		}
		f.processAuditTask(atask)
		return
	}
	var (
		err        error
		tk         *big.Int
		atask      auditTask
		ataskBytes []byte
	)
	f.dblock.Lock()
	defer func() {
		if err == nil {
			log.Debug("GenAuditTask-success", "key", tk, "txhash", txhash.Hex())
			// commit task
			f.auditTask <- f.newTask(taskNumKey, tk).Bytes()
		}
		f.dblock.Unlock()
	}()
	// task-num
	tk = new(big.Int).Add(f.newTask(taskNumKey, nil), big.NewInt(1))
	atask = auditTask{tk, txhash, big.NewInt(0), nil}
	// 这个方法出现的异常不可恢复,panic
	ataskBytes, err = rlp.EncodeToBytes(&atask)
	log.Debug("Ataskbytes", "err", err, "task", ataskBytes)
	if err != nil {
		panic(err)
	}
	err = f.codedb.Put(tk.Bytes(), ataskBytes)
	if err != nil {
		panic(err)
	}
}

func (f *ewasmFuns) getFinalStatus(final []byte) (*auditTask, error) {
	finalstatekey := crypto.Keccak256(final, []byte("status"))
	buf, err := f.codedb.Get(finalstatekey)
	//log.Debug("ewasmFuns-getFinalStatus", "err", err, "finalstatekey", finalstatekey, "hex", common.BytesToHash(finalstatekey).Hex())
	if err != nil {
		return nil, err
	}
	atask := new(auditTask)
	err = rlp.DecodeBytes(buf, atask)
	if err != nil {
		return nil, err
	}
	return atask, nil
}

func (f *ewasmFuns) VerifyStatus(currentBlockNumber *big.Int, final []byte) error {

	atask, err := f.getFinalStatus(final)
	if err != nil {
		log.Debug("VerifyStatus-getFinalStatus-error", "err", err)
		return err
	}
	/* 同步时默认审核通过，不影响验证逻辑
	if atomic.LoadInt32(f.synchronis) == 1 {
		// 正在同步，跳过验证
		log.Debug("ewasmFuns-verifyStatus-skip", "synchronis", atomic.LoadInt32(f.synchronis))
		return nil
	}*/
	//log.Debug("VerifyStatus-ready", "status", atask.Status, "num", atask.BlockNumber, "current", currentBlockNumber)
	if atask.Status != nil && atask.Status.Int64() == 1 &&
		atask.BlockNumber != nil && (new(big.Int).Add(atask.BlockNumber, big.NewInt(auditLimit))).Cmp(currentBlockNumber) < 0 {
		//log.Debug("VerifyStatus-success")
		return nil
	}
	log.Debug("VerifyStatus-fail")
	return fmt.Errorf("VerifyStatus-fail : status=%d , num=%d , current=%d", atask.Status, atask.BlockNumber, currentBlockNumber)
}

func (f *ewasmFuns) GenCodekey(code []byte) []byte {
	//code, _, err := utopia.ParseData(code)
	//if err != nil {
	//	panic(err)
	//}
	return crypto.Keccak256(code)
}

func (f *ewasmFuns) Init(chaindb, codedb ethdb2.Database, synchronis *int32) {
	f.once.Do(func() {
		log.Debug("ewasmFuncs-init")
		f.chaindb = chaindb
		f.codedb = codedb
		f.synchronis = synchronis
		go EwasmFuncs.auditLoop()
	})
}

func init() {
	EwasmToolImpl = EwasmFuncs
}
