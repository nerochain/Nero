package downloader

import (
	"errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

type badBlockFn func(invalid *types.Header, origin *types.Header)

func (d *Downloader) SetBadBlockCallback(onBadBlock badBlockFn) {}

func (d *Downloader) BeaconSync(mode SyncMode, head *types.Header, final *types.Header) error {
	return errors.New("beacon sync is not supported")
}

func (d *Downloader) BeaconExtend(mode SyncMode, head *types.Header) error {
	return errors.New("beacon extend is not supported")
}

func (d *Downloader) BeaconDevSync(mode SyncMode, hash common.Hash, stop chan struct{}) error {
	return errors.New("beacon dev sync is not supported")
}
