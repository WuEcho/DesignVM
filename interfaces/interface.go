package interfaces

import (
	"github.com/CaduceusMetaverseProtocol/MetaVM/vm/ethvm"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/types"
)

type BlockChainContext interface {
	ChainContext
	CurrentBlock() *types.Block
}

type StateDB interface {
	ethvm.StateDB
	Copy() StateDB
	Finalise(bool)
	IntermediateRoot(bool) common.Hash
	TxIndex() uint
	//GetLogs(tx common.Hash, block common.Hash) []*types.Log
}

type ConsensusEngine interface {
	Author(header *types.Header) (common.Address, error)
}

// ChainContext supports retrieving headers and consensus parameters from the
// current blockchain to be used during transaction processing.
type ChainContext interface {
	// Engine retrieves the chain's consensus engine.
	Engine() consensus.Engine
	// GetHeader returns the hash corresponding to their hash.
	GetHeader(common.Hash, uint64) *types.Header
}
