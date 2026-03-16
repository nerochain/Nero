package rawdb

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
)

// ReadInternalTxsRLP retrieves all the transaction receipts belonging to a block in RLP encoding.
func ReadInternalTxsRLP(db ethdb.Reader, hash common.Hash, number uint64) rlp.RawValue {
	var data []byte
	db.ReadAncients(func(reader ethdb.AncientReaderOp) error {
		// Check if the data is in ancients
		//if isCanon(reader, number, hash) {
		//	data, _ = reader.Ancient(ChainFreezerInternalTxTable, number)
		//	if data != nil {
		//			log.Debug("traceaction read from ancientdb, number", number, " len ", len(data))
		//			return nil
		//		}
		//}
		// If not, try reading from leveldb
		data, _ = db.Get(blockInternalTxsKey(number, hash))
		if data == nil {
			log.Debug("traceaction read from fastdb, number", "number", number, "len", "nil")
		} else {
			log.Debug("traceaction read from fastdb, number", "number", number, "len", len(data))
		}
		return nil
	})
	return data
}

func ReadInternalTxs(db ethdb.Reader, hash common.Hash, number uint64) []*types.InternalTx {
	data := ReadInternalTxsRLP(db, hash, number)
	if len(data) == 0 {
		return nil
	}

	result := make([]*types.InternalTx, 0)
	err := rlp.DecodeBytes(data, &result)
	if err != nil {
		log.Error("Decode data error", "err", err)
		return nil
	}

	return result
}

// WriteInternalTxs stores all the internal transactions belonging to a block.
func WriteInternalTxs(db ethdb.KeyValueWriter, hash common.Hash, number uint64, internalTxs types.InternalTxs) {
	// Convert the receipts into their storage form and serialize them
	storageITxs := make([]*types.InternalTxForStorage, len(internalTxs))
	for i, tx := range internalTxs {
		storageITxs[i] = (*types.InternalTxForStorage)(tx)
	}
	bytes, err := rlp.EncodeToBytes(storageITxs)
	if err != nil {
		log.Crit("Failed to encode block internal txs", "err", err)
	}
	log.Debug("traceaction WriteInternalTxs internal txs", "hash", hash.String(), "number", number, "lens", len(internalTxs))
	// Store the flattened receipt slice
	if err := db.Put(blockInternalTxsKey(number, hash), bytes); err != nil {
		log.Crit("Failed to encode block internal txs", "err", err)
	}
}

// DeleteInternalTxs removes all internal transactions associated with a block hash.
func DeleteInternalTxs(db ethdb.KeyValueWriter, hash common.Hash, number uint64) {
	log.Debug("traceaction DeleteInternalTxs", "hash", hash.String(), "number", number)
	if err := db.Delete(blockInternalTxsKey(number, hash)); err != nil {
		log.Crit("Failed to delete block internal txs", "err", err)
	}
}
