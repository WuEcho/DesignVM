package common

import (
	"bytes"
	"encoding/binary"
	metatypes "github.com/CaduceusMetaverseProtocol/MetaTypes/types"
	"github.com/ethereum/go-ethereum/crypto"
)

func ByteToHash(value []byte) metatypes.Hash {
	return metatypes.BytesToHash(crypto.Keccak256(value))
}

func StringToHash(str string) metatypes.Hash {
	return metatypes.BytesToHash(crypto.Keccak256([]byte(str))[12:])
}

//字节转换成整形
func BytesToInt(b []byte) int64 {
	bytesBuffer := bytes.NewBuffer(b)
	var tmp int64
	binary.Read(bytesBuffer, binary.BigEndian, &tmp)
	return tmp
}

func IntToByte(num int32) []byte {
	var buffer bytes.Buffer
	binary.Write(&buffer, binary.BigEndian, num)
	return buffer.Bytes()
}

func Uint64ToByte(n uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, n)
	return b
}

func ByteToUint64(b []byte) uint64 {
	return binary.BigEndian.Uint64(b)
}

