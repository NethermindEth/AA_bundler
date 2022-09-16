package main

import (
	"context"
	"encoding/json"
	"fmt"

	// "go/types"
	"math/big"
	"net/http"

	e "flashbotsAAbundler/consts"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/misc"
	ethclient "github.com/ethereum/go-ethereum/ethClient"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

var (
	safeEntryPoints = []common.Address{
		common.HexToAddress("0x98CAf6454c05885b730325aFc868C2A5919A2fda"), //goerli
	}
	zeroAddress = common.HexToAddress("0x0000000000000000000000000000000000000000")
)

type _UserOperation struct {
	Sender               common.Address `json:"sender"`
	Nonce                *big.Int       `json:"nonce"`
	InitCode             []byte         `json:"initCode"`
	CallData             []byte         `json:"callData"`
	CallGas              *big.Int       `json:"callGas"`
	VerificationGas      *big.Int       `json:"verificationGas"`
	PreVerificationGas   *big.Int       `json:"preVerificationGas"`
	MaxFeePerGas         *big.Int       `json:"maxFeePerGas"`
	MaxPriorityFeePerGas *big.Int       `json:"maxPriorityFeePerGas"`
	PaymasterAndData     []byte         `json:"paymasterAndData"` //paymasterAndData holds the paymaster address followed by the token address to use.
	Signature            []byte         `json:"signature"`
}

type UserOperationWithEntryPoint struct {
	UserOperation _UserOperation `json:"params"`
	EntryPoint    common.Address `json:"entryPoint"`
}

type Request struct { //from EIP
	Jsonrpc string        `json:"jsonrpc"`
	Id      *big.Int      `json:"id"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"` //1st User op, 2nd entry point
}
type Response struct {
	Jsonrpc string   `json:"jsonrpc"`
	Id      *big.Int `json:"id"`
	Result  Result   `json:"Result"`
}
type Result struct {
	Success bool
	TxHash  common.Hash
}

func main() {
	http.HandleFunc("/eth_sendUserOperation", handle_eth_sendUserOperation)
	http.HandleFunc("/eth_supportedEntryPoints", handle_eth_supportedEntryPoints)
	if err := http.ListenAndServe(":8080", nil); err != nil { //listens for http reqs on 8080
		log.Error("http server failed", "error", err)
	}
}

func handle_eth_sendUserOperation(respw http.ResponseWriter, req *http.Request) {
	respw.Header().Set("Content-Type", "application/json")
	//copying the params of the call to a type userOperationWithEntryPoint struct for ease in sanity checks
	var r Request
	err := json.NewDecoder(req.Body).Decode(&r)
	if err != nil {
		http.Error(respw, err.Error(), http.StatusBadRequest)
		return
	}
	sendUop := NewTypeUserOperationWithEntryPoint(r.Params)
	//Checking for safe Entry Point
	if !sendUop.checkSafeEntryPoint() {
		http.Error(respw, "Entry point not safe,", e.JsonRpcInvalidParams)
		return
	}
	//basic sanity checks
	//1. Check the length of params
	if len(r.Params) != 2 {
		http.Error(respw, "invalid number of params for eth_sendUserOperation", e.JsonRpcInvalidParams)
		return
	}
	//2. Either the sender is an existing contract, or the initCode is not empty (but not both)
	senderCheck, err := sendUop.addressHasCode()
	if err != nil {
		http.Error(respw, "error while getting code from sender address", e.JsonRpcInternalError) //error type not sure
		return
	}
	if !senderCheck && sendUop.UserOperation.InitCode == nil {
		http.Error(respw, "neither sender nor initcode available", e.JsonRpcInvalidParams)
		return
	}
	if senderCheck && sendUop.UserOperation.InitCode != nil {
		http.Error(respw, "cant take wallet as well as InitCode", e.JsonRpcInvalidParams)
	}
	//3. Verification gas is sufficiently low
	max_verification_gas := big.NewInt(100e9) //from kristof's mev searcher bot. Needs optimization
	if sendUop.UserOperation.VerificationGas.Cmp(max_verification_gas) > 0 {
		http.Error(respw, "verification gas higher than max_verification_gas", e.JsonRpcInvalidParams)
	}
	//4.preVerification gas is sufficiently high
	sum := big.NewInt(0)
	sum.Add(sendUop.UserOperation.CallGas, sendUop.UserOperation.VerificationGas)
	if sendUop.UserOperation.PreVerificationGas.Cmp(sum) < 0 {
		http.Error(respw, "PreVerificationGas is not high enough", e.JsonRpcInvalidParams)
	}
	//5. Paymaster is either zero address or contract with non zero code, registered and staked, sufficient deposit and not blacklisted
	//TODO need to have a db to handle registered paymasters and blacklisted paymasters
	//TODO check for sufficient deposit
	paymasterCheck, err := sendUop.addressHasCode()
	if err != nil {
		http.Error(respw, "error while getting code from sender address", e.JsonRpcInternalError) //error type not confirmed
		return
	}
	paymaster := getPaymaster(sendUop.UserOperation)
	if !(paymasterCheck || paymaster == zeroAddress) {
		http.Error(respw, "paymaster not contract or zero address", e.JsonRpcInvalidParams)
		return
	}
	//6. maxFeePerGas and maxPriorityFeeGas are greater or equal than block's basefee
	currBaseFee := getCurrentBlockBasefee()
	if !(sendUop.UserOperation.MaxFeePerGas.Cmp(currBaseFee) > 0 && sendUop.UserOperation.MaxPriorityFeePerGas.Cmp(currBaseFee) > 0) {
		http.Error(respw, "Max fee per gas too low ", e.JsonRpcInvalidParams)
		return
	}
	//TODO-7. Sender does not have another user op already in the pool. if that is the case the new tx should have +1 nonce

	//calling handleOps function
	success, tx, err := sendUop.CallHandleOps()
	if err != nil {
		http.Error(respw, "Handle Ops Call failed", e.JsonRpcTransactionError)
	}
	resData := r.WriteRPCResponse(success, tx.Hash())
	//need to check if this approach works
	json.NewEncoder(respw).Encode(resData)

}

func handle_eth_supportedEntryPoints(respw http.ResponseWriter, req *http.Request) {
	respw.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(respw, "0xwhatever")
}

func NewTypeUserOperationWithEntryPoint(data []interface{}) *UserOperationWithEntryPoint { //needs to be tested
	var UserOPwEP UserOperationWithEntryPoint
	json.Unmarshal([]byte(data[0].(string)), &UserOPwEP) //incorporate err handling for unmarshalling
	return &UserOPwEP
}

func (s *UserOperationWithEntryPoint) checkSafeEntryPoint() bool {
	ep := s.EntryPoint
	for _, safeAddress := range safeEntryPoints {
		if ep == safeAddress {
			return true
		}
	}
	return false
}

func (s *UserOperationWithEntryPoint) addressHasCode() (bool, error) {
	conn, err := ethclient.Dial(getClient())
	if err != nil {
		return false, err
	}
	address := s.UserOperation.Sender
	ctx := context.Background()
	code, err := conn.CodeAt(ctx, address, nil)
	if err != nil {
		return false, err
	}
	if code != nil {
		return true, nil
	} else {
		return false, nil
	}
}
func getCurrentBlockBasefee() *big.Int {
	config := params.MainnetChainConfig
	ethClient, _ := ethclient.DialContext(context.Background(), getClient())
	bn, _ := ethClient.BlockNumber(context.Background())
	bignumBn := big.NewInt(0).SetUint64(bn)
	blk, _ := ethClient.BlockByNumber(context.Background(), bignumBn)
	baseFee := misc.CalcBaseFee(config, blk.Header())
	return baseFee
}

func getClient() string {
	return "https://eth-mainnet.g.alchemy.com/v2/uj-JFn5PF9yWYFNBdz1LcIb1B_GgBZOE"
}

func getPaymaster(uop _UserOperation) common.Address {
	pda := uop.PaymasterAndData
	return common.BytesToAddress(pda[0:41])
}