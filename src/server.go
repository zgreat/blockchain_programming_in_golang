package main

import (
	"bytes"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/clockworksoul/smudge"
)

type addr struct {
	AddrList []string
	AddrTo   string
}

type block struct {
	AddrFrom string
	AddrTo   string
	Block    []byte
}

type getblocks struct {
	AddrFrom string
	AddrTo   string
}

type getdata struct {
	AddrFrom string
	AddrTo   string
	Type     string
	ID       []byte
}

type inventory struct {
	AddrFrom string
	AddrTo   string
	Type     string
	Items    [][]byte
}

type tx struct {
	AddFrom     string
	AddrTo      string
	Transaction []byte
}

type version struct {
	AddrFrom    string
	AddrTo      string
	Version     int
	BlockHeight int
}

// MyStatusListener extends from smudge.StatusListener
type MyStatusListener struct {
	smudge.StatusListener
}

// MyBroadcastListener extends from smudge.MyBroadcastListener
type MyBroadcastListener struct {
	smudge.BroadcastListener
}

const (
	protocol      = "tcp"
	commandLength = 12
)

var (
	nodeVersion     int
	etherIface      string
	knownNodes      = []string{}
	baseAddress     string
	nodeAddress     string
	rewardToAddress string
)

var mempool = struct {
	sync.RWMutex
	m map[string]Transaction
}{m: make(map[string]Transaction)}

var blocksInTransit = struct {
	sync.RWMutex
	a [][]byte
}{a: [][]byte{}}

func (m MyStatusListener) OnChange(node *smudge.Node, status smudge.NodeStatus) {}

func (m MyBroadcastListener) OnBroadcast(b *smudge.Broadcast) {}

func GetIPOnInterface(i string) string {
	var err error
	ip := ""

	ifaces, err := net.Interfaces()
	if err != nil {
		return "127.0.0.1"
	}
	for _, iface := range ifaces {
		if iface.Name == i {
			addrs, err := iface.Addrs()
			if err != nil {
				return "127.0.0.1"
			}
			for _, addr := range addrs {
				var ipnet net.IP
				switch v := addr.(type) {
				case *net.IPNet:
					ipnet = v.IP
				case *net.IPAddr:
					ipnet = v.IP
				default:
					fmt.Println("Not connected no network")
				}
				ip = ipnet.String()
			}
		}
	}
	return ip
}

// ConfigServer configuration for Smudge Library
func ConfigServer(nodeID, minerAddress string) error {
	port, err := strconv.Atoi(nodeId)
	if err != nil {
		return err
	}

	baseAddress = GetIPOnInterface(etherIface)
	knownNodes = append(knownNodes, fmt.Sprintf("%s:3000", baseAddress))
	nodeAddress = fmt.Sprintf("%s:%s", baseAddress, nodeID)
	rewardToAddress = minerAddress

	// Set configuration options
	smudge.SetListenIP(net.ParseIP(baseAddress))
	smudge.SetListenPort(port)
	smudge.SetHeartbeatMillis(500)
	smudge.SetMaxBroadcastBytes(2000)
	// smudge.SetMulticastEnabled(false)
	smudge.SetClusterName("KU")

	smudge.AddStatusListener(MyStatusListener{})
	smudge.AddBroadcastListener(MyBroadcastListener{})

	// Add a new remote node. Currently, to join an existing cluster you must
	// add at least one of its healthy member nodes.

	if nodeAddress != knownNodes[0] {
		node, err := smudge.CreateNodeByAddress(knownNodes[0])
		if err == nil {
			smudge.AddNode(node)
		} else {
			return err
		}
	}

	// start the server
	go func() {
		smudge.Begin()
	}()

	// Handle SIGINT and SIGTERM.
	// quit := make(chan os.Signal, 2)
	// signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	// <-quit

	return nil
}

func StartServer(nodeID, minerAddress string) {
	var err error
	if err = ConfigServer(nodeID, minerAddress); err != nil {
		log.Panic(err)
	}

	go func() {
		ln, err := net.Listen(protocol, fmt.Sprintf(":1%s", nodeId))
		if err != nil {
			log.Panic(err)
		}
		defer ln.Close()

		for {
			conn, err2 := ln.Accept()
			if err2 != nil {
				err = err2
				return
			}
			go handleConnection(conn, Bc)
		}
	}()
	if err != nil {
		log.Panic(err)
	}

	if Bc == nil {
		Bc = NewBlockchain(nodeID)
	}

	time.Sleep(time.Second * 3)
	sendVersion("all", Bc)

	// if nodeAddress != knownNodes[0] {
	// 	sendVersion(knownNodes[0], Bc)
	// }
}

func handleConnection(conn net.Conn, bc *Blockchain) {
	request, err := ioutil.ReadAll(conn)
	if err != nil {
		log.Panic(err)
	}
	command := bytesToCommand(request[:commandLength])

	logger.Logf(LogInfo, "Received %s command\n", command)

	switch command {
	// case "addr":
	// 	handleAddr(request)
	case "block":
		handleBlock(request, bc)
	case "inv":
		handleInventory(request, bc)
	case "getblocks":
		handleGetBlocks(request, bc)
	case "getdata":
		handleGetData(request, bc)
	case "tx":
		handleTx(request, bc)
	case "version":
		handleVersion(request, bc)
	default:
		logger.Logf(LogInfo, "Unknown command!")
	}
}

func handleAddr(request []byte) {
	var (
		buff    bytes.Buffer
		payload addr
	)
	logger.Logf(LogDebug, "Handle addr")

	buff.Write(request[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	if err != nil {
		log.Panic(err)
	}
	if payload.AddrTo != "all" && payload.AddrTo != nodeAddress {
		return
	}

	knownNodes = append(knownNodes, payload.AddrList...)
	logger.Logf(LogInfo, "There are %d known nodes now!\n", len(knownNodes))
	requestBlocks()
}

func handleBlock(request []byte, bc *Blockchain) {
	var (
		buff    bytes.Buffer
		payload block
	)
	logger.Logf(LogDebug, "Handle Block")

	buff.Write(request[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	if err != nil {
		log.Panic(err)
	}
	if payload.AddrTo != "all" && payload.AddrTo != nodeAddress {
		return
	}

	blockData := payload.Block
	block, err := Deserialize(blockData)
	if err != nil {
		log.Panic("ERROR:", err)
	}

	logger.Logf(LogInfo, "Recevied a new block! from", payload.AddrFrom)

	//TODO: verify block before adding
	err = bc.AddBlock(block)

	if err != nil {
		logger.Logf(LogError, err.Error())
	} else {
		logger.Logf(LogInfo, "Added block %x\n", block.Hash)
	}
	// blockHashes := bc.GetBlockHashes()

	// // TODO: use broadcasr instead
	// if nodeAddress == knownNodes[0] {
	// 	for _, node := range knownNodes {
	// 		if node != nodeAddress {
	// 			sendInventory(node, "block", blockHashes)
	// 		}
	// 	}
	// }
	// sendInventory("all", "block", blockHashes)

	blocksInTransit.Lock()
	transistNum := len(blocksInTransit.a)
	if transistNum > 0 {
		// TODO: reverse transmit
		blockHash := blocksInTransit.a[0]
		sendGetData(payload.AddrFrom, "block", blockHash)
		blocksInTransit.a = blocksInTransit.a[1:]
	} else {
		UTXOSet := UTXOSet{bc}

		// TODO: mostly cause problem
		UTXOSet.Reindex()
	}
	blocksInTransit.Unlock()
}

func handleInventory(request []byte, bc *Blockchain) {
	var (
		buff    bytes.Buffer
		payload inventory
	)
	logger.Logf(LogDebug, "Handle Inventory")

	buff.Write(request[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	if err != nil {
		log.Panic(err)
	}

	if payload.AddrTo != "all" && payload.AddrTo != nodeAddress {
		return
	}

	logger.Logf(LogInfo, "Recevied inventory with %d %s\n", len(payload.Items), payload.Type)

	if payload.Type == "block" {
		// TODO: reverse transmit
		blocksInTransit.Lock()
		blocksInTransit.a = payload.Items
		blocksInTransit.Unlock()

		blockHash := payload.Items[0]
		sendGetData(payload.AddrFrom, "block", blockHash)

		newInTransit := [][]byte{}
		blocksInTransit.RLock()
		for _, b := range blocksInTransit.a {
			if bytes.Compare(b, blockHash) != 0 {
				newInTransit = append(newInTransit, b)
			}
		}
		blocksInTransit.RUnlock()

		blocksInTransit.Lock()
		blocksInTransit.a = newInTransit
		blocksInTransit.Unlock()
	}

	if payload.Type == "tx" {
		txID := payload.Items[0]

		mempool.RLock()
		memTx := mempool.m[hex.EncodeToString(txID)]
		mempool.RUnlock()
		if memTx.ID == nil {
			sendGetData(payload.AddrFrom, "tx", txID)
		}
	}
}

func handleGetBlocks(request []byte, bc *Blockchain) {
	var (
		buff    bytes.Buffer
		payload getblocks
	)
	logger.Logf(LogDebug, "Handle Get blocks")

	buff.Write(request[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	if err != nil {
		log.Panic(err)
	}
	if payload.AddrTo != "all" && payload.AddrTo != nodeAddress {
		return
	}

	blockHashes := bc.GetBlockHashes()
	sendInventory(payload.AddrFrom, "block", blockHashes)
}

func handleGetData(request []byte, bc *Blockchain) {
	var (
		buff    bytes.Buffer
		payload getdata
	)
	logger.Logf(LogDebug, "Handle Getdata")

	buff.Write(request[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	if err != nil {
		log.Panic(err)
	}
	if payload.AddrTo != "all" && payload.AddrTo != nodeAddress {
		return
	}

	if payload.Type == "block" {
		block, err := bc.GetBlock([]byte(payload.ID))
		if err != nil {
			return
		}

		sendBlock(payload.AddrFrom, &block)
	}

	if payload.Type == "tx" {
		txID := hex.EncodeToString(payload.ID)

		mempool.RLock()
		memTx := mempool.m[txID]
		mempool.RUnlock()
		tx := memTx

		sendTx(payload.AddrFrom, &tx)
		// delete(mempool, txID)
	}

}

// TODO: if miner off then on... broadcast txs ?
func handleTx(request []byte, bc *Blockchain) {
	var (
		buff    bytes.Buffer
		payload tx
	)
	logger.Logf(LogDebug, "Handle Tx")

	buff.Write(request[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	if err != nil {
		log.Panic(err)
	}
	if payload.AddrTo != "all" && payload.AddrTo != nodeAddress {
		return
	}

	txData := payload.Transaction
	tx := DeserializeTransaction(txData)

	// spew.Dump(tx)

	mempool.Lock()
	mempool.m[hex.EncodeToString(tx.ID)] = tx
	mempool.Unlock()

	/*
		check if guaranteed broadcastTx to every node in single machine

		if nodeAddress == knownNodes[0] {
			for _, node := range knownNodes {
				if node != nodeAddress && node != payload.AddFrom {
					sendInventory(node, "tx", [][]byte{tx.ID})
				}
			}
		} else {
		}
	*/

	mempool.RLock()
	memLen := len(mempool.m)
	mempool.RUnlock()
	if memLen >= 2 && len(rewardToAddress) > 0 {
	MineTransactions:
		var txs []*Transaction
		var usedTXInput [][]byte

		mempool.Lock()
		for id := range mempool.m {
			tx := mempool.m[id]
			if hasSameTXInput(usedTXInput, tx.Vin) {
				delete(mempool.m, id)
			} else {
				verified := bc.VerifyTransaction(&tx)
				if verified {
					txs = append(txs, &tx)
					for i := range tx.Vin {
						usedTXInput = append(usedTXInput, tx.Vin[i].Txid)
					}
				}
			}
		}
		mempool.Unlock()

		if len(txs) == 0 {
			logger.Logf(LogError, "All transactions are invalid! Waiting for new ones...")
			return
		}

		cbTx := NewCoinbaseTX(rewardToAddress, "")
		txs = append(txs, cbTx)

		newBlock := bc.MineBlock(txs)
		UTXOSet := UTXOSet{bc}
		UTXOSet.Reindex()

		logger.Logf(LogWarn, "New block is mined!")

		for _, tx := range txs {
			txID := hex.EncodeToString(tx.ID)
			mempool.Lock()
			delete(mempool.m, txID)
			mempool.Unlock()
		}

		/*
			broadcast to all known nodes
			for _, node := range knownNodes {
				if node != nodeAddress {
					sendInventory(node, "block", [][]byte{newBlock.Hash})
				}
			}
		*/
		sendInventory("all", "block", [][]byte{newBlock.Hash})

		mempool.RLock()
		memLen := len(mempool.m)
		mempool.RUnlock()
		if memLen > 0 {
			goto MineTransactions
		}
	}

}

func handleVersion(request []byte, bc *Blockchain) {
	var (
		buff    bytes.Buffer
		payload version
	)
	logger.Logf(LogDebug, "Handle Version")

	buff.Write(request[commandLength:])
	decoder := gob.NewDecoder(&buff)
	err := decoder.Decode(&payload)
	if err != nil {
		log.Panic(err)
	}
	// spew.Dump(payload)

	if payload.AddrTo != "all" && payload.AddrTo != nodeAddress {
		return
	}

	myHeight := bc.GetLastBlockHeight()
	requestHeight := payload.BlockHeight

	if myHeight < requestHeight {
		sendGetBlocks(payload.AddrFrom)
	} else if myHeight > requestHeight {
		sendVersion(payload.AddrFrom, bc)
	}

	if !nodeIsKnown(payload.AddrFrom) {
		knownNodes = append(knownNodes, payload.AddrFrom)
	}
}

func sendBlock(addr string, b *Block) {
	logger.Logf(LogDebug, "Send block")
	payload := gobEncode(block{nodeAddress, addr, Serialize(b)})
	request := append(commandToBytes("block"), payload...)
	prepareData(addr, request)
}

func prepareData(address string, request []byte) {
	if address == "all" {
		for _, node := range smudge.AllNodes() {
			logger.Logf(LogDebug, node.Address())
			if strconv.Itoa(int(node.Port())) != nodeId {
				if err := sendData(node.Address(), request); err != nil {
					if strings.HasSuffix(err.Error(), "connection refused") {
						smudge.UpdateNodeStatus(node, smudge.StatusDead, nil)
						continue
					}
					log.Panic(err)
				}
			} else {
				logger.Logf(LogDebug, "same port")
			}
		}
		return
	}
	if err := sendData(address, request); err != nil {
		if strings.HasSuffix(err.Error(), "connection refused") {
			node := func() *smudge.Node {
				for _, node := range smudge.AllNodes() {
					if node.Address() == address {
						return node
					}
				}
				return nil
			}()
			smudge.UpdateNodeStatus(node, smudge.StatusDead, nil)
			return
		}
		log.Panic(err)
	}
}

func sendData(address string, request []byte) error {
	s := strings.Split(address, ":")
	to := fmt.Sprintf("%s:1%s", s[0], s[1])
	conn, err := net.DialTimeout(protocol, to, time.Second*10)
	if err != nil {
		return err
	}
	defer conn.Close()

	_, err = io.Copy(conn, bytes.NewReader(request))
	if err != nil {
		return err
	}
	return nil
}

func sendGetBlocks(addr string) {
	logger.Logf(LogDebug, "Send Get Blocks")
	encodedGetBlocks := gobEncode(getblocks{nodeAddress, addr})
	request := append(commandToBytes("getblocks"), encodedGetBlocks...)
	prepareData(addr, request)
}

func sendGetData(addr, kind string, id []byte) {
	logger.Logf(LogDebug, "Send Get Data")
	encodedGetData := gobEncode(getdata{nodeAddress, addr, kind, id})
	request := append(commandToBytes("getdata"), encodedGetData...)
	prepareData(addr, request)
}

func sendTx(addr string, tnx *Transaction) {
	logger.Logf(LogDebug, "Send Tx")
	encodedTx := gobEncode(tx{nodeAddress, addr, SerializeTransaction(*tnx)})
	request := append(commandToBytes("tx"), encodedTx...)
	prepareData(addr, request)
}

func sendInventory(addr, kind string, blockHashes [][]byte) {
	logger.Logf(LogDebug, "Send Inv")
	encodedInventory := gobEncode(inventory{nodeAddress, addr, kind, blockHashes})
	request := append(commandToBytes("inv"), encodedInventory...)
	prepareData(addr, request)
}

func sendVersion(addr string, bc *Blockchain) {
	logger.Logf(LogDebug, "Send Version")
	lastHeight := bc.GetLastBlockHeight()
	encodedLastHeight := gobEncode(version{nodeAddress, addr, nodeVersion, lastHeight})
	request := append(commandToBytes("version"), encodedLastHeight...)
	prepareData(addr, request)
}

func nodeIsKnown(address string) bool {
	for _, node := range knownNodes {
		if node == address {
			return true
		}
	}
	return false
}

func requestBlocks() {
	for _, node := range knownNodes {
		sendGetBlocks(node)
	}
}

func commandToBytes(commandString string) []byte {
	var commandBytes [12]byte
	copy(commandBytes[:], commandString)
	return commandBytes[:]
}

func bytesToCommand(commandBytes []byte) string {
	counter := 0
	for _, char := range commandBytes {
		if char == 0x0 {
			break
		}
		counter++
	}
	return string(commandBytes[:counter])
}

func gobEncode(data interface{}) []byte {
	var buff bytes.Buffer

	gobEncoder := gob.NewEncoder(&buff)
	err := gobEncoder.Encode(data)

	if err != nil {
		log.Panic(err)
	}

	return buff.Bytes()
}

func testSerialization(s1, s2 []byte) bool {
	return bytes.Equal(s1, s2)
}

func hasSameTXInput(listByte [][]byte, inputs []TXInput) bool {
	for i := range inputs {
		for _, val := range listByte {
			if bytes.Compare(val, inputs[i].Txid) == 0 {
				return true
			}
		}
	}
	return false
}
