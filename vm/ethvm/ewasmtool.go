package ethvm

import (
	"github.com/ethereum/go-ethereum/ethdb"
)
// 处理 ewasm 合约部署逻辑
type EwasmTool interface {
	SplitTxdata(finalcode []byte) (code, final []byte, err error)
	JoinTxdata(code, final []byte) ([]byte, error)
	ValidateCode(code []byte) ([]byte, error)
	IsWASM(code []byte) bool
	Sentinel(code []byte) ([]byte, error)

	IsFinalcode(finalcode []byte) bool
	DelCode(key []byte)
	GetCode(key []byte) ([]byte, bool)
	GenCodekey(code []byte) []byte
	PutCode(key, val []byte)
	Init(chaindb, codedb ethdb.Database, synchronis *int32)
	//GenAuditTask(txhash common.Hash)
	Shutdown()
}

var EwasmToolImpl EwasmTool
