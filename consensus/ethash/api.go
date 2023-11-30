// Copyright 2023 Bitnet
// This file is part of the Bitnet library.
//
// This software is provided "as is", without warranty of any kind,
// express or implied, including but not limited to the warranties
// of merchantability, fitness for a particular purpose and
// noninfringement. In no even shall the authors or copyright
// holders be liable for any claim, damages, or other liability,
// whether in an action of contract, tort or otherwise, arising
// from, out of or in connection with the software or the use or
// other dealings in the software.

package ethash

import (
	"errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
)

var errEthashStopped = errors.New("ethash stopped")

// API exposes ethash related methods for the RPC interface.
type API struct {
	ethash *Ethash
}

// GetWork returns a work package for external miner.
//
// The work package consists of 3 strings:
//
//		result[0] - 32 bytes hex encoded current block header pow-hash
//		result[1] - 32 bytes hex encoded seed hash used for DAG
//		result[2] - 32 bytes hex encoded boundary condition ("target"), 2^256/difficulty
//		result[3] - hex encoded block number
//	  result[4], 32 bytes hex encoded parent block header pow-hash
//	  result[5], hex encoded gas limit
//	  result[6], hex encoded gas used
//	  result[7], hex encoded transaction count
//	  result[8], hex encoded uncle count
//	  result[9], RLP encoded header with additonal empty extra data bytes
//	  result[10], MEV Profit as float-to-string "0.124"
func (api *API) GetWork() ([11]string, error) {
	if api.ethash.remote == nil {
		return [11]string{}, errors.New("not supported")
	}

	var (
		workCh = make(chan [11]string, 1)
		errc   = make(chan error, 1)
	)
	select {
	case api.ethash.remote.fetchWorkCh <- &sealWork{errc: errc, res: workCh}:
	case <-api.ethash.remote.exitCh:
		return [11]string{}, errEthashStopped
	}
	select {
	case work := <-workCh:
		return work, nil
	case err := <-errc:
		return [11]string{}, err
	}
}

// SubmitWork can be used by external miner to submit their POW solution.
// It returns an indication if the work was accepted.
// Note either an invalid solution, a stale work a non-existent work will return false.
func (api *API) SubmitWork(nonce types.BlockNonce, hash, digest common.Hash, extraNonceStr *string) bool {
	if api.ethash.remote == nil {
		return false
	}

	var extraNonce []byte
	if extraNonceStr != nil {
		var err error
		extraNonce, err = hexutil.Decode(*extraNonceStr)
		if err != nil {
			return false
		}
	}

	var errc = make(chan error, 1)
	select {
	case api.ethash.remote.submitWorkCh <- &mineResult{
		nonce:      nonce,
		mixDigest:  digest,
		hash:       hash,
		extraNonce: extraNonce,
		errc:       errc,
	}:
	case <-api.ethash.remote.exitCh:
		return false
	}
	err := <-errc
	return err == nil
}

// SubmitHashrate can be used for remote miners to submit their hash rate.
// This enables the node to report the combined hash rate of all miners
// which submit work through this node.
//
// It accepts the miner hash rate and an identifier which must be unique
// between nodes.
func (api *API) SubmitHashrate(rate hexutil.Uint64, id common.Hash) bool {
	if api.ethash.remote == nil {
		return false
	}

	var done = make(chan struct{}, 1)
	select {
	case api.ethash.remote.submitRateCh <- &hashrate{done: done, rate: uint64(rate), id: id}:
	case <-api.ethash.remote.exitCh:
		return false
	}

	// Block until hash rate submitted successfully.
	<-done
	return true
}

// GetHashrate returns the current hashrate for local CPU miner and remote miner.
func (api *API) GetHashrate() uint64 {
	return uint64(api.ethash.Hashrate())
}
