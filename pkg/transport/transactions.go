package transport

import (
	"ledger-sats-stack/pkg/types"
	"ledger-sats-stack/pkg/utils"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcutil"
)

// !FIXME: Move to types package
type utxoVoutMapType map[uint32]types.UTXO
type utxoMapType map[string]utxoVoutMapType

// TransactionContainer is a wrapper type to define an init method for
// Transaction
type TransactionContainer struct {
	types.Transaction
}

func (txn *TransactionContainer) init(rawTx *btcjson.TxRawResult, utxoMap utxoMapType, blockHeight int64) {
	txn.ID = rawTx.Txid
	txn.Hash = rawTx.Txid // !FIXME: Use rawTx.Hash, which can differ for witness transactions
	txn.ReceivedAt = utils.ParseUnixTimestamp(rawTx.Time)
	txn.LockTime = rawTx.LockTime

	vin := make([]types.Input, len(rawTx.Vin))
	sumVinValues := btcutil.Amount(0)
	vinHasCoinbase := false

	for idx, rawVin := range rawTx.Vin {
		inputIndex := idx
		if rawVin.IsCoinBase() {
			vin[idx] = types.Input{
				Coinbase:   rawVin.Coinbase,
				InputIndex: &inputIndex,
				Sequence:   rawVin.Sequence,
			}

			vinHasCoinbase = true
		} else {
			utxo := utxoMap[rawVin.Txid][rawVin.Vout]
			outputIndex := rawVin.Vout

			vin[idx] = types.Input{
				OutputHash:  rawVin.Txid,
				OutputIndex: &outputIndex,
				InputIndex:  &inputIndex, // TODO: Find out if the order matters
				Value:       &utxo.Value,
				Address:     utxo.Address,
				ScriptSig:   rawVin.ScriptSig.Hex,
				Sequence:    rawVin.Sequence,
			}
			if rawVin.HasWitness() {
				witness := rawVin.Witness
				vin[idx].Witness = &witness
			} else {
				vin[idx].Witness = &[]string{} // !FIXME: Coinbase txn can also have witness
			}

			sumVinValues += *vin[idx].Value
		}
	}
	txn.Inputs = vin

	vout := make([]types.Output, len(rawTx.Vout))
	sumVoutValues := btcutil.Amount(0)

	for idx, rawVout := range rawTx.Vout {
		outputValue := utils.ParseSatoshi(rawVout.Value) // !FIXME: Can panic
		outputIndex := rawVout.N
		vout[idx] = types.Output{
			OutputIndex: &outputIndex,
			Value:       &outputValue,
			ScriptHex:   rawVout.ScriptPubKey.Hex,
		}

		if len(rawVout.ScriptPubKey.Addresses) == 1 {
			vout[idx].Address = rawVout.ScriptPubKey.Addresses[0]
		} else if len(rawVout.ScriptPubKey.Addresses) > 1 {
			// TODO: Log an error / warning
		} else {
			// TODO: Document when this happens
		}

		sumVoutValues += *vout[idx].Value
	}
	txn.Outputs = vout

	txn.Block = types.Block{
		Hash:   rawTx.BlockHash,
		Height: blockHeight,
		Time:   utils.ParseUnixTimestamp(rawTx.Blocktime),
	}

	// ?XXX: Confirmations in Ledger Blockchain Explorer are always off by 1
	txn.Confirmations = rawTx.Confirmations - uint64(1)

	var fees btcutil.Amount

	if vinHasCoinbase {
		// Coinbase transaction have no fees
		fees = btcutil.Amount(0)
	} else {
		fees = sumVinValues - sumVoutValues
	}
	txn.Fees = &fees
}

// GetTransaction is a service function to query transaction details
// by hash parameter.
func (w Wire) GetTransaction(txHash string) (*TransactionContainer, error) {
	txRaw, err := w.getTransactionByHash(txHash)
	if err != nil {
		return nil, err
	}

	utxoMap, err := w.buildUtxoMap(txRaw.Vin)
	if err != nil {
		return nil, err
	}

	blockHeight := w.GetBlockHeightByHash(txRaw.BlockHash)

	transaction := new(TransactionContainer)
	transaction.init(txRaw, utxoMap, blockHeight)
	return transaction, nil
}

// GetTransactionHexByHash is a service function to get hex encoded raw
// transaction by hash.
func (w Wire) GetTransactionHexByHash(txHash string) (string, error) {
	txRaw, err := w.getTransactionByHash(txHash)
	if err != nil {
		return "", err
	}
	return txRaw.Hex, nil
}
