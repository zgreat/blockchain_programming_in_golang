package main

import (
	"bytes"
	"crypto/elliptic"
	"encoding/gob"
	"fmt"
	"io/ioutil"
	"log"
	"os"
)

const walletFile = "wallet_%s.dat"

type Wallets struct {
	Wallets map[string]*Wallet
}

func NewWallets(nodeID string) (*Wallets, error) {
	wallets := Wallets{make(map[string]*Wallet)}
	err := wallets.LoadFromFile(nodeID)
	return &wallets, err
}

func (ws *Wallets) CreateWallet() string {
	wallet := NewWallet()
	address := fmt.Sprintf("%s", wallet.GetAddress())

	ws.Wallets[address] = wallet

	return address
}

func (ws Wallets) GetWallet(address string) Wallet {
	wallet := ws.Wallets[address]
 	if wallet == nil {
 		log.Panic("ERROR: No wallet found!")
 	}
	return *ws.Wallets[address]
}

func (ws *Wallets) GetAddresses() []string {
	var addresses []string

	for address := range ws.Wallets {
		addresses = append(addresses, address)
	}

	return addresses
}

func (ws *Wallets) LoadFromFile(nodeID string) error {
	var buf bytes.Buffer

	walletFile := fmt.Sprintf(walletFile, nodeID)
	if _, err := os.Stat(walletFile); os.IsNotExist(err) {
		return err
	}

	fileContent, err := ioutil.ReadFile(walletFile)
	if err != nil {
		log.Panic(err)
	}

	var wallets Wallets
	gob.Register(elliptic.P384())

	buf.Write(fileContent)
	decoder := gob.NewDecoder(&buf)
	err = decoder.Decode(&wallets)
	if err != nil {
		log.Panic(err)
	}

	ws.Wallets = wallets.Wallets

	return nil
}

func (ws Wallets) SaveToFile(nodeID string) {
	walletFile := fmt.Sprintf(walletFile, nodeID)
	var content bytes.Buffer

	gob.Register(elliptic.P384())

	encoder := gob.NewEncoder(&content)
	err := encoder.Encode(ws)
	if err != nil {
		log.Panic(err)
	}

	err = ioutil.WriteFile(walletFile, content.Bytes(), 0644)
	if err != nil {
		log.Panic(err)
	}
}
