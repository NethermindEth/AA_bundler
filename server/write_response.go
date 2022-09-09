package main

import (
	"github.com/ethereum/go-ethereum/common"
)

func (r *Request) WriteRPCResponse(success bool, txHash common.Hash) (res *Response) {
	return &Response{
		Jsonrpc: "2.0",
		Id:      r.Id,
		Result: Result{
			Success: success,
			TxHash:  txHash,
		},
	}
}
