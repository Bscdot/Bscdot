// Copyright 2020 ChainSafe Systems
// SPDX-License-Identifier: LGPL-3.0-only

package substrate

import (
	"errors"
	"fmt"
	gsrpc "github.com/centrifuge/go-substrate-rpc-client/v2"
	rpcConfig "github.com/centrifuge/go-substrate-rpc-client/v2/config"
	"github.com/ethereum/go-ethereum/common"
	"math/big"
	"time"

	"github.com/ChainSafe/ChainBridge/chains"
	utils "github.com/ChainSafe/ChainBridge/shared/substrate"
	"github.com/ChainSafe/chainbridge-utils/blockstore"
	metrics "github.com/ChainSafe/chainbridge-utils/metrics/types"
	"github.com/ChainSafe/chainbridge-utils/msg"
	"github.com/ChainSafe/log15"
	"github.com/centrifuge/go-substrate-rpc-client/v2/types"
)

type listener struct {
	name          string
	chainId       msg.ChainId
	startBlock    uint64
	blockstore    blockstore.Blockstorer
	conn          *Connection
	subscriptions map[eventName]eventHandler // Handlers for specific events
	depositNonce  map[common.Address]int     //记录每个账户交易的nonce
	router        chains.Router
	log           log15.Logger
	stop          <-chan int
	sysErr        chan<- error
	latestBlock   metrics.LatestBlock
	metrics       *metrics.ChainMetrics
}

// Frequency of polling for a new block
var BlockRetryInterval = time.Second * 5
var BlockRetryLimit = 5

func NewListener(conn *Connection, name string, id msg.ChainId, startBlock uint64, log log15.Logger, bs blockstore.Blockstorer, stop <-chan int, sysErr chan<- error, m *metrics.ChainMetrics) *listener {
	return &listener{
		name:          name,
		chainId:       id,
		startBlock:    startBlock,
		blockstore:    bs,
		conn:          conn,
		subscriptions: make(map[eventName]eventHandler),
		depositNonce:  make(map[common.Address]int),
		log:           log,
		stop:          stop,
		sysErr:        sysErr,
		latestBlock:   metrics.LatestBlock{LastUpdated: time.Now()},
		metrics:       m,
	}
}

func (l *listener) setRouter(r chains.Router) {
	l.router = r
}

// start creates the initial subscription for all events
func (l *listener) start() error {
	// Check whether latest is less than starting block
	header, err := l.conn.api.RPC.Chain.GetHeaderLatest()
	if err != nil {
		return err
	}
	if uint64(header.Number) < l.startBlock {
		//return fmt.Errorf("starting block (%d) is greater than latest known block (%d)", l.startBlock, header.Number)
	}

	for _, sub := range Subscriptions {
		err := l.registerEventHandler(sub.name, sub.handler)
		if err != nil {
			return err
		}
	}

	go func() {
		err := l.pollBlocks()
		if err != nil {
			l.log.Error("Polling blocks failed", "err", err)
		}
	}()

	return nil
}

// registerEventHandler enables a handler for a given event. This cannot be used after Start is called.
func (l *listener) registerEventHandler(name eventName, handler eventHandler) error {
	if l.subscriptions[name] != nil {
		return fmt.Errorf("event %s already registered", name)
	}
	l.subscriptions[name] = handler
	return nil
}

var ErrBlockNotReady = errors.New("required result to be 32 bytes, but got 0")

// pollBlocks will poll for the latest block and proceed to parse the associated events as it sees new blocks.
// Polling begins at the block defined in `l.startBlock`. Failed attempts to fetch the latest block or parse
// a block will be retried up to BlockRetryLimit times before returning with an error.

func (l *listener) pollBlocks() error {
	// assume TestKeyringPairBob.PublicKey is a multisign address

	var multiSignPk, err = types.HexDecodeString("0x96255ecf5f66b58074da258ad20e6d74fedc900798687ff86547efe30ec2e7c6")
	var multiSignAccount = types.NewAccountID(multiSignPk)
	//TestKeyringPairBob.PublicKey
	//var multiSignAddr = "16MoApHtxf63szJ2b7cQ8QzqpCSDiDJ8BKhkas2L37fkC6oU"
	//var method = "Deposit(uint8,bytes32,uint64)"
	// Query the system events and extract information from them. This example runs until exited via Ctrl-C

	// Create our API with a default connection to the local node
	api, err := gsrpc.NewSubstrateAPI(rpcConfig.Default().RPCURL)
	if err != nil {
		panic(err)
	}
	meta, err := api.RPC.State.GetMetadataLatest()
	if err != nil {
		panic(err)
	}

	// Subscribe to system events via storage
	key, err := types.CreateStorageKey(meta, "System", "Events", nil, nil)
	if err != nil {
		panic(err)
	}

	sub, err := api.RPC.State.SubscribeStorageRaw([]types.StorageKey{key})
	if err != nil {
		panic(err)
	}
	defer sub.Unsubscribe()

	// outer for loop for subscription notifications
	for {
		set := <-sub.Chan()
		// inner loop for the changes within one of those notifications
		for _, chng := range set.Changes {
			if !types.Eq(chng.StorageKey, key) || !chng.HasStorageData {
				// skip, we are only interested in events with countent
				continue
			}

			// Decode the event records
			events := types.EventRecords{}
			err = types.EventRecordsRaw(chng.StorageData).DecodeEventRecords(meta, &events)
			if err != nil {
				//panic(err)
				fmt.Printf("\terr is %v\n", err)
			}

			// Show what we are busy with
			for _, e := range events.Balances_Transfer {
				fmt.Printf("\tBalances:Transfer:: (phase=%#v)\n", e.Phase)
				fmt.Printf("\t\t%v, %v, %v\n", e.From, e.To, e.Value)
				if e.To == multiSignAccount {
					fmt.Printf("Succeed catch a tx to mulsigAddress\n")

					recipient := types.NewAccountID(common.FromHex("0xff93B45308FD417dF303D6515aB04D9e89a750Ca"))
					rId := msg.ResourceIdFromSlice(common.FromHex("0x000000000000000000000000000000c76ebe4a02bbc34786d860b355f5a5ce00"))

					//TODO: update msg.Nonce
					m := msg.NewFungibleTransfer(
						msg.ChainId(1), // Unset
						msg.ChainId(0),
						msg.Nonce(1),
						big.NewInt(123),
						rId,
						recipient[:],
					)
					l.submitMessage(m, err)

				}
			}
			for _, e := range events.Balances_Deposit {
				fmt.Printf("\tBalances:Deposit:: (phase=%#v)\n", e.Phase)
				fmt.Printf("\t\t%v, %v\n", e.Who, e.Balance)
			}
			for _, e := range events.Grandpa_NewAuthorities {
				fmt.Printf("\tGrandpa:NewAuthorities:: (phase=%#v)\n", e.Phase)
				fmt.Printf("\t\t%v\n", e.NewAuthorities)
			}
			for _, e := range events.Grandpa_Paused {
				fmt.Printf("\tGrandpa:Paused:: (phase=%#v)\n", e.Phase)
			}
			for _, e := range events.Grandpa_Resumed {
				fmt.Printf("\tGrandpa:Resumed:: (phase=%#v)\n", e.Phase)
			}
			for _, e := range events.ImOnline_HeartbeatReceived {
				fmt.Printf("\tImOnline:HeartbeatReceived:: (phase=%#v)\n", e.Phase)
				fmt.Printf("\t\t%#x\n", e.AuthorityID)
			}
			for _, e := range events.Offences_Offence {
				fmt.Printf("\tOffences:Offence:: (phase=%#v)\n", e.Phase)
				fmt.Printf("\t\t%v%v\n", e.Kind, e.OpaqueTimeSlot)
			}
			for _, e := range events.Session_NewSession {
				fmt.Printf("\tSession:NewSession:: (phase=%#v)\n", e.Phase)
				fmt.Printf("\t\t%v\n", e.SessionIndex)
			}
			for _, e := range events.Staking_OldSlashingReportDiscarded {
				fmt.Printf("\tStaking:OldSlashingReportDiscarded:: (phase=%#v)\n", e.Phase)
				fmt.Printf("\t\t%v\n", e.SessionIndex)
			}
			for _, e := range events.Staking_Slash {
				fmt.Printf("\tStaking:Slash:: (phase=%#v)\n", e.Phase)
				fmt.Printf("\t\t%#x%v\n", e.AccountID, e.Balance)
			}
			for _, e := range events.System_ExtrinsicSuccess {
				fmt.Printf("\tSystem:ExtrinsicSuccess:: (phase=%#v)\n", e.Phase)
			}
			for _, e := range events.System_ExtrinsicFailed {
				fmt.Printf("\tSystem:ErtrinsicFailed:: (phase=%#v)\n", e.Phase)
				fmt.Printf("\t\t%v\n", e.DispatchError)
			}
			for _, e := range events.Multisig_NewMultisig {
				fmt.Printf("\tSystem:detect new multisign request:: (phase=%#v)\n", e.Phase)
				fmt.Printf("\t\tFrom:%v,To: %v\n", e.Who, e.ID)
			}
			for _, e := range events.Multisig_MultisigApproval {
				fmt.Printf("\tSystem:detect new multisign approval:: (phase=%#v)\n", e.Phase)
			}
			for _, e := range events.Multisig_MultisigExecuted {
				fmt.Printf("\tSystem:detect new multisign Executed:: (phase=%#v)\n", e.Phase)
				fmt.Printf("\t\tFrom:%v,To: %v\n", e.Who, e.ID)
			}
			for _, e := range events.Multisig_MultisigCancelled {
				fmt.Printf("\tSystem:detect new multisign request:: (phase=%#v)\n", e.Phase)
			}
		}
	}
}

func (l *listener) pollBlock() error {
	var currentBlock = l.startBlock
	var retry = BlockRetryLimit
	for {
		select {
		case <-l.stop:
			return errors.New("terminated")
		default:
			// No more retries, goto next block
			if retry == 0 {
				l.sysErr <- fmt.Errorf("event polling retries exceeded (chain=%d, name=%s)", l.chainId, l.name)
				return nil
			}

			// Get finalized block hash
			finalizedHash, err := l.conn.api.RPC.Chain.GetFinalizedHead()
			if err != nil {
				l.log.Error("Failed to fetch finalized hash", "err", err)
				retry--
				time.Sleep(BlockRetryInterval)
				continue
			}

			// Get finalized block header
			finalizedHeader, err := l.conn.api.RPC.Chain.GetHeader(finalizedHash)
			if err != nil {
				l.log.Error("Failed to fetch finalized header", "err", err)
				retry--
				time.Sleep(BlockRetryInterval)
				continue
			}

			if l.metrics != nil {
				l.metrics.LatestKnownBlock.Set(float64(finalizedHeader.Number))
			}

			// Sleep if the block we want comes after the most recently finalized block
			if currentBlock > uint64(finalizedHeader.Number) {
				l.log.Trace("Block not yet finalized", "target", currentBlock, "latest", finalizedHeader.Number)
				time.Sleep(BlockRetryInterval)
				continue
			}

			// Get hash for latest block, sleep and retry if not ready
			hash, err := l.conn.api.RPC.Chain.GetBlockHash(currentBlock)
			if err != nil && err.Error() == ErrBlockNotReady.Error() {
				time.Sleep(BlockRetryInterval)
				continue
			} else if err != nil {
				l.log.Error("Failed to query latest block", "block", currentBlock, "err", err)
				retry--
				time.Sleep(BlockRetryInterval)
				continue
			}

			err = l.processEvents(hash)
			if err != nil {
				l.log.Error("Failed to process events in block", "block", currentBlock, "err", err)
				retry--
				continue
			}

			// Write to blockstore
			err = l.blockstore.StoreBlock(big.NewInt(0).SetUint64(currentBlock))
			if err != nil {
				l.log.Error("Failed to write to blockstore", "err", err)
			}

			if l.metrics != nil {
				l.metrics.BlocksProcessed.Inc()
				l.metrics.LatestProcessedBlock.Set(float64(currentBlock))
			}

			currentBlock++
			l.latestBlock.Height = big.NewInt(0).SetUint64(currentBlock)
			l.latestBlock.LastUpdated = time.Now()
			retry = BlockRetryLimit
		}
	}
}

func (l *listener) processEvent(hash types.Hash) error {
	l.log.Trace("Fetching block for events", "hash", hash.Hex())
	meta := l.conn.getMetadata()
	// Subscribe to system events via storage
	key, err := types.CreateStorageKey(&meta, "System", "Events", nil, nil)
	if err != nil {
		return err
	}

	//var records types.EventRecordsRaw
	//_, err = l.conn.api.RPC.State.GetStorage(key, &records, hash)
	//if err != nil {
	//	return err
	//}

	//f, err := os.OpenFile("watchfile.log", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	//if err != nil {
	//	log.Fatal(err)
	//}
	////完成后，延迟关闭
	////defer f.Close()
	//// 设置日志输出到文件
	//log.SetOutput(f)

	var multiSignPk, errs = types.HexDecodeString("0x96255ecf5f66b58074da258ad20e6d74fedc900798687ff86547efe30ec2e7c6")
	if errs != nil {
		panic(err)
	}
	var multiSignAccount = types.NewAccountID(multiSignPk)

	sub, err := l.conn.api.RPC.State.SubscribeStorageRaw([]types.StorageKey{key})
	if err != nil {
		panic(err)
	}
	defer sub.Unsubscribe()

	set := <-sub.Chan()
	//log.Println("\t~_~sadsadsadad---------")

	for _, chng := range set.Changes {
		if !types.Eq(chng.StorageKey, key) || !chng.HasStorageData {
			// skip, we are only interested in events with countent
			continue
		}

		// Decode the event records
		events := types.EventRecords{}
		err = types.EventRecordsRaw(chng.StorageData).DecodeEventRecords(&meta, &events)
		if err != nil {
			//panic(err)
			fmt.Printf("\terr is %v\n", err)
		}

		// Show what we are busy with
		for _, e := range events.Balances_Transfer {
			fmt.Printf("\tBalances:Transfer:: (phase=%#v)\n", e.Phase)
			fmt.Printf("\t\t%v, %v, %v\n", e.From, e.To, e.Value)
			if e.To == multiSignAccount {
				fmt.Printf("\t~_~成功捕捉到转账到多签地址的交易---------\n")

				// 写入日志内容
				//log.Println("\t~_~成功捕捉到转账到多签地址的交易---------")

				//	var recipient, err = types.HexDecodeString("0xff93B45308FD417dF303D6515aB04D9e89a750Ca")
				//	if err != nil {
				//		panic(err)
				//	}
				//var recipientAccount = types.NewAccountID(recipient)
				//

				//msg.NewFungibleTransfer(
				//	0, // Unset
				//	msg.ChainId(evt.Destination),
				//	msg.Nonce(evt.DepositNonce),
				//	evt.Amount.Int,
				//	resourceId,
				//	evt.Recipient,
				//	)

			}
		}
		for _, e := range events.Balances_Deposit {
			fmt.Printf("\tBalances:Deposit:: (phase=%#v)\n", e.Phase)
			fmt.Printf("\t\t%v, %v\n", e.Who, e.Balance)
		}
		for _, e := range events.Grandpa_NewAuthorities {
			fmt.Printf("\tGrandpa:NewAuthorities:: (phase=%#v)\n", e.Phase)
			fmt.Printf("\t\t%v\n", e.NewAuthorities)
		}
		for _, e := range events.Grandpa_Paused {
			fmt.Printf("\tGrandpa:Paused:: (phase=%#v)\n", e.Phase)
		}
		for _, e := range events.Grandpa_Resumed {
			fmt.Printf("\tGrandpa:Resumed:: (phase=%#v)\n", e.Phase)
		}
		for _, e := range events.ImOnline_HeartbeatReceived {
			fmt.Printf("\tImOnline:HeartbeatReceived:: (phase=%#v)\n", e.Phase)
			fmt.Printf("\t\t%#x\n", e.AuthorityID)
		}
		for _, e := range events.Offences_Offence {
			fmt.Printf("\tOffences:Offence:: (phase=%#v)\n", e.Phase)
			fmt.Printf("\t\t%v%v\n", e.Kind, e.OpaqueTimeSlot)
		}
		for _, e := range events.Session_NewSession {
			fmt.Printf("\tSession:NewSession:: (phase=%#v)\n", e.Phase)
			fmt.Printf("\t\t%v\n", e.SessionIndex)
		}
		for _, e := range events.Staking_OldSlashingReportDiscarded {
			fmt.Printf("\tStaking:OldSlashingReportDiscarded:: (phase=%#v)\n", e.Phase)
			fmt.Printf("\t\t%v\n", e.SessionIndex)
		}
		for _, e := range events.Staking_Slash {
			fmt.Printf("\tStaking:Slash:: (phase=%#v)\n", e.Phase)
			fmt.Printf("\t\t%#x%v\n", e.AccountID, e.Balance)
		}
		for _, e := range events.System_ExtrinsicSuccess {
			fmt.Printf("\tSystem:ExtrinsicSuccess:: (phase=%#v)\n", e.Phase)
		}
		for _, e := range events.System_ExtrinsicFailed {
			fmt.Printf("\tSystem:ErtrinsicFailed:: (phase=%#v)\n", e.Phase)
			fmt.Printf("\t\t%v\n", e.DispatchError)
		}
		for _, e := range events.Multisig_NewMultisig {
			fmt.Printf("\tSystem:detect new multisign request:: (phase=%#v)\n", e.Phase)
			fmt.Printf("\t\tFrom:%v,To: %v\n", e.Who, e.ID)
		}
		for _, e := range events.Multisig_MultisigApproval {
			fmt.Printf("\tSystem:detect new multisign approval:: (phase=%#v)\n", e.Phase)
		}
		for _, e := range events.Multisig_MultisigExecuted {
			fmt.Printf("\tSystem:detect new multisign Executed:: (phase=%#v)\n", e.Phase)
			fmt.Printf("\t\tFrom:%v,To: %v\n", e.Who, e.ID)
		}
		for _, e := range events.Multisig_MultisigCancelled {
			fmt.Printf("\tSystem:detect new multisign request:: (phase=%#v)\n", e.Phase)
		}
	}

	//e := types.EventRecords{}
	//err = records.DecodeEventRecords(&meta, &e)
	l.log.Trace("Finished processing events", "block", hash.Hex())

	return nil
}

// processEvents fetches a block and parses out the events, calling Listener.handleEvents()
func (l *listener) processEvents(hash types.Hash) error {
	l.log.Trace("Fetching block for events", "hash", hash.Hex())
	meta := l.conn.getMetadata()
	key, err := types.CreateStorageKey(&meta, "System", "Events", nil, nil)
	if err != nil {
		return err
	}

	var records types.EventRecordsRaw
	_, err = l.conn.api.RPC.State.GetStorage(key, &records, hash)
	if err != nil {
		return err
	}

	e := utils.Events{}
	err = records.DecodeEventRecords(&meta, &e)
	if err != nil {
		return err
	}

	l.handleEvents(e)
	l.log.Trace("Finished processing events", "block", hash.Hex())

	return nil
}

// handleEvents calls the associated handler for all registered event types
func (l *listener) handleEvents(evts utils.Events) {
	if l.subscriptions[FungibleTransfer] != nil {
		for _, evt := range evts.ChainBridge_FungibleTransfer {
			l.log.Trace("Handling FungibleTransfer event")
			l.submitMessage(l.subscriptions[FungibleTransfer](evt, l.log))
		}
	}
	if l.subscriptions[NonFungibleTransfer] != nil {
		for _, evt := range evts.ChainBridge_NonFungibleTransfer {
			l.log.Trace("Handling NonFungibleTransfer event")
			l.submitMessage(l.subscriptions[NonFungibleTransfer](evt, l.log))
		}
	}
	if l.subscriptions[GenericTransfer] != nil {
		for _, evt := range evts.ChainBridge_GenericTransfer {
			l.log.Trace("Handling GenericTransfer event")
			l.submitMessage(l.subscriptions[GenericTransfer](evt, l.log))
		}
	}

	if len(evts.System_CodeUpdated) > 0 {
		l.log.Trace("Received CodeUpdated event")
		err := l.conn.updateMetatdata()
		if err != nil {
			l.log.Error("Unable to update Metadata", "error", err)
		}
	}
}

// submitMessage inserts the chainId into the msg and sends it to the router
func (l *listener) submitMessage(m msg.Message, err error) {
	if err != nil {
		log15.Error("Critical error processing event", "err", err)
		return
	}
	m.Source = l.chainId
	err = l.router.Send(m)
	if err != nil {
		log15.Error("failed to process event", "err", err)
	}
}
