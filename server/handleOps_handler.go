package main

import (
	"context"
	"fmt"
	"os"
	"reflect"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	typ "github.com/ethereum/go-ethereum/core/types"
	ethclient "github.com/ethereum/go-ethereum/ethclient"
)

func (s _UserOperation) CallHandleOps() (bool, *typ.Transaction, error) {
	conn, err := ethclient.Dial(getClient())
	if err != nil {
		return false, nil, err
	}
	EP, err := NewEntryPoint(common.HexToAddress(os.Getenv("ENTRYPOINT_CONTRACT")), conn)
	if err != nil {
		return false, nil, err
	}
	chainID, _ := conn.ChainID(context.Background())
	fmt.Println("chain id", chainID)
	rdr := string(os.Getenv("KEY_IN"))
	r, err := os.Open(rdr)
	if err != nil {
		return false, nil, err
	}
	auth, err := bind.NewTransactorWithChainID(r, os.Getenv("PASSPHRASE"), chainID)
	if err != nil {
		return false, nil, err
	}

	uop_array := buildUserOperationArray(s)
	fmt.Println(reflect.TypeOf(uop_array[0].Nonce))
	tx, err := EP.HandleOps(auth, uop_array, common.HexToAddress(os.Getenv("TEMP_BENEFICIARY")))
	if err != nil {
		return false, nil, err
	}
	return true, tx, nil

}

func buildUserOperationArray(uop _UserOperation) []UserOperation {
	var ops = []UserOperation{
		{
			Sender:               uop.Sender,
			Nonce:                uop.Nonce,
			InitCode:             []byte(uop.InitCode),
			CallData:             []byte(uop.CallData),
			CallGasLimit:         uop.CallGasLimit,
			VerificationGasLimit: uop.VerificationGasLimit,
			PreVerificationGas:   uop.PreVerificationGas,
			MaxFeePerGas:         uop.MaxFeePerGas,
			MaxPriorityFeePerGas: uop.MaxPriorityFeePerGas,
			PaymasterAndData:     []byte(uop.PaymasterAndData),
			Signature:            []byte(uop.Signature),
		},
	}
	return ops
}
