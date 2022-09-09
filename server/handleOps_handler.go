package main

import (
	"context"
	"strings"

	c "flashbotsAAbundler/consts"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	typ "github.com/ethereum/go-ethereum/core/types"
	ethclient "github.com/ethereum/go-ethereum/ethClient"
)

func (s *UserOperationWithEntryPoint) CallHandleOps() (bool, *typ.Transaction, error) {
	conn, err := ethclient.Dial(getClient())
	if err != nil {
		return false, nil, err
	}
	EP, err := NewEntryPoint(common.HexToAddress(c.ENTRYPOINT_CONTRACT), conn)
	if err != nil {
		return false, nil, err
	}
	chainID, _ := conn.ChainID(context.Background())
	auth, err := bind.NewTransactorWithChainID(strings.NewReader(c.Key), c.Passphrase, chainID)
	if err != nil {
		return false, nil, err
	}
	uop_array := buildUserOperationArray(s.UserOperation)
	tx, err := EP.HandleOps(auth, uop_array, common.HexToAddress(c.TempBeneficiary))
	if err != nil {
		return false, nil, err
	}
	return true, tx, nil

}

func buildUserOperationArray(uop _UserOperation) []UserOperation {
	var ops = []UserOperation{
		UserOperation{
			Sender:               uop.Sender,
			Nonce:                uop.Nonce,
			InitCode:             uop.InitCode,
			CallData:             uop.CallData,
			CallGasLimit:         uop.CallGas,
			VerificationGasLimit: uop.VerificationGas,
			PreVerificationGas:   uop.PreVerificationGas,
			MaxFeePerGas:         uop.MaxFeePerGas,
			MaxPriorityFeePerGas: uop.MaxPriorityFeePerGas,
			PaymasterAndData:     uop.PaymasterAndData,
			Signature:            uop.Signature,
		},
	}
	return ops
}
