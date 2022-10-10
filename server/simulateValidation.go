package main

import (
	"context"
	"fmt"
	"os"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

func (s _UserOperation) simValidation() (bool, error) {
	conn, err := ethclient.Dial(getClient())
	if err != nil {
		return false, err
	}
	EP, err := NewEntryPoint(common.HexToAddress(os.Getenv("ENTRYPOINT_CONTRACT")), conn)
	if err != nil {
		return false, err
	}
	chainID, _ := conn.ChainID(context.Background())
	rdr := string(os.Getenv("KEY_IN"))
	r, err := os.Open(rdr)
	if err != nil {
		return false, err
	}
	auth, err := bind.NewTransactorWithChainID(r, os.Getenv("PASSPHRASE"), chainID)
	if err != nil {
		return false, err
	}
	uop_array := buildUserOperationArray(s) //temporary resort
	tx, err := EP.SimulateValidation(auth, uop_array[0], false)
	if err != nil {
		return false, err
	}
	fmt.Println("Sim validation tx", tx)
	return true, nil
}
