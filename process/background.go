package process

import (
	"github.com/CaduceusMetaverseProtocol/MetaNebula/common"
	types2 "github.com/CaduceusMetaverseProtocol/MetaNebula/types"
	basev1 "github.com/CaduceusMetaverseProtocol/MetaProtocol/gen/proto/go/base/v1"
	metatypes "github.com/CaduceusMetaverseProtocol/MetaTypes/types"
	common2 "github.com/CaduceusMetaverseProtocol/MetaVM/common"
	"github.com/CaduceusMetaverseProtocol/MetaVM/interfaces"
	"github.com/CaduceusMetaverseProtocol/MetaVM/vm/ethvm"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/types"
	common3 "github.com/ethereum/go-ethereum/common"
	"math/big"
)

// ChainContext supports retrieving headers and consensus parameters from the
// current blockchain to be used during transaction processing.
type ChainContext interface {
	// Engine retrieves the chain's consensus engine.
	Engine() consensus.Engine

	// GetHeader returns the hash corresponding to their hash.
	GetHeader(common3.Hash, uint64) *types.Header
}

// NewVMBlockContext creates a new context for use in the VM.
func NewVMBlockContext(header *types.Header, chain ChainContext, author *metatypes.Address) ethvm.BlockContext {
	var (
		beneficiary metatypes.Address
		baseFee     *big.Int
		random      *metatypes.Hash
	)

	// If we don't have an explicit author (i.e. not mining), extract from the header
	if author == nil {
		addr,_ := chain.Engine().Author(header)
		beneficiary = metatypes.BytesToAddress(addr.Bytes()) // Ignore error, we're past header validation
	} else {
		beneficiary = *author
	}
	if header.BaseFee != nil {
		baseFee = new(big.Int).Set(header.BaseFee)
	}
	if header.Difficulty.Cmp(common.Big0) == 0 {
		hash := common2.ByteToHash(header.MixDigest.Bytes())
		random = &hash
	}
	return ethvm.BlockContext{
		CanTransfer: CanTransfer,
		Transfer:    Transfer,
		GetHash:     GetHashFn(header, chain),
		Coinbase:    beneficiary,
		BlockNumber: new(big.Int).Set(header.Number),
		Time:        new(big.Int).SetUint64(header.Time),
		Difficulty:  new(big.Int).Set(header.Difficulty),
		BaseFee:     baseFee,
		GasLimit:    header.GasLimit,
		Random:      random,
	}
}

func buildBlockContext(block *types.Block,processor *Processor) *ethvm.BlockContext {
	header := block.Header()
	blockContext := NewVMBlockContext(header, processor.bc, nil)
	return &blockContext
}

func PrepareVm(blockContext ethvm.BlockContext,processor *Processor,stateDb interfaces.StateDB,cfg ethvm.Config,txContext ethvm.TxContext) *ethvm.ETHVM {
	vm := ethvm.NewVM(blockContext,txContext,stateDb,processor.config,cfg)
	return vm
}

// NewVMTxContext creates a new transaction context for a single transaction.
func NewVMTxContext(msg Message) ethvm.TxContext {
	return ethvm.TxContext{
		Origin:   msg.From(),
		GasPrice: new(big.Int).Set(msg.GasPrice()),
		//added by echo
		ReadSet: ethvm.NewReadSet(),
		WriteSet: ethvm.NewWriteSet(),
		Logs: make([]*basev1.MetaTxLog,0),
		TxData: msg.Data(),
	}
}

// GetHashFn returns a GetHashFunc which retrieves header hashes by number
func GetHashFn(ref *types.Header, chain ChainContext) func(n uint64) metatypes.Hash {
	// Cache will initially contain [refHash.parent],
	// Then fill up with [refHash.p, refHash.pp, refHash.ppp, ...]
	var cache []metatypes.Hash

	return func(n uint64) metatypes.Hash {
		// If there's no hash cache yet, make one
		if len(cache) == 0 {
			cache = append(cache, common2.ByteToHash(ref.ParentHash.Bytes()))
		}
		if idx := ref.Number.Uint64() - n - 1; idx < uint64(len(cache)) {
			return cache[idx]
		}
		// No luck in the cache, but we can start iterating from the last element we already know
		lastKnownHash := cache[len(cache)-1]
		lastKnownNumber := ref.Number.Uint64() - uint64(len(cache))

		for {
			header := chain.GetHeader(common3.BytesToHash(lastKnownHash.Bytes()), lastKnownNumber)
			if header == nil {
				break
			}
			cache = append(cache, common2.ByteToHash(header.ParentHash.Bytes()))
			lastKnownHash = common2.ByteToHash(header.ParentHash.Bytes())
			lastKnownNumber = header.Number.Uint64() - 1
			if n == lastKnownNumber {
				return lastKnownHash
			}
		}
		return metatypes.Hash{}
	}
}

// CanTransfer checks whether there are enough funds in the address' account to make a transfer.
// This does not take the necessary gas in to account to make the transfer valid.
func CanTransfer(db ethvm.StateDB, addr metatypes.Address, amount *big.Int) bool {
	account := types2.Account{
		addr,
	}
	return db.GetBalance(account).Cmp(amount) >= 0
}

// Transfer subtracts amount from sender and adds amount to recipient using the given Db
func Transfer(db ethvm.StateDB, sender, recipient metatypes.Address, amount *big.Int) {
	db.SubBalance(types2.Account{sender}, amount)
	db.AddBalance(types2.Account{recipient}, amount)
}
