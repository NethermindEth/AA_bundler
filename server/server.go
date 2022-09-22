package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	// "go/types"

	e "flashbotsAAbundler/consts"
	"math/big"
	"net/http"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/misc"
	ethclient "github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	"github.com/joho/godotenv"
)

var (
	safeEntryPoints = []common.Address{
		common.HexToAddress("0x98CAf6454c05885b730325aFc868C2A5919A2fda"), //goerli
	}
	zeroAddress = common.HexToAddress("0x0000000000000000000000000000000000000000")
)

type UserOperationJSON struct {
	UserOperation struct {
		Sender               common.Address `json:"sender"`
		Nonce                *big.Int       `json:"nonce"`
		CallData             []byte         `json:"callData"`
		InitCode             []byte         `json:"initCode"`
		CallGas              *big.Int       `json:"callGas"`
		VerificationGas      *big.Int       `json:"verificationGas"`
		PreVerificationGas   *big.Int       `json:"preVerificationGas"`
		MaxFeePerGas         *big.Int       `json:"maxFeePerGas"`
		MaxPriorityFeePerGas *big.Int       `json:"maxPriorityFeePerGas"`
		PaymasterAndData     []byte         `json:"paymasterAndData"`
		Signature            []byte         `json:"signature"`
	} `json:"userOperation"`
	EntryPoint common.Address `json:"entryPoint"`
}

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
	Jsonrpc string              `json:"jsonrpc"`
	Id      *big.Int            `json:"id"`
	Method  string              `json:"method"`
	Params  []UserOperationJSON `json:"params"` //1st User op, 2nd entry point
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
	envErr := godotenv.Load(".env")
	if envErr != nil {
		fmt.Printf("Error loading .env file")
		os.Exit(1)
	}
	http.HandleFunc("/eth_sendUserOperation", handle_eth_sendUserOperation)
	http.HandleFunc("/eth_supportedEntryPoints", handle_eth_supportedEntryPoints)
	if err := http.ListenAndServe(":8080", nil); err != nil { //listens for http reqs on 8080
		log.Error("http server failed", "error", err)
	}

}

func handle_eth_sendUserOperation(respw http.ResponseWriter, req *http.Request) {
	respw.Header().Set("Content-Type", "application/json")
	respw.WriteHeader(200)
	//copying the params of the call to a type userOperationWithEntryPoint struct for ease in sanity checks
	var r Request
	err := json.NewDecoder(req.Body).Decode(&r)
	if err != nil {
		http.Error(respw, err.Error(), http.StatusBadRequest)
		return

	}
	// fmt.Printf("t1: %s\n", reflect.TypeOf(r.Params))
	UopwithEP := NewTypeUserOperation(r.Params[0])
	fmt.Println(UopwithEP)
	// Checking for safe Entry Point
	if !checkSafeEntryPoint(UopwithEP) {
		http.Error(respw, "Entry point not safe,", e.JsonRpcInvalidParams)
		return
	}
	//basic sanity checks
	//1. Check the length of params
	if len(r.Params) != 1 {
		http.Error(respw, "invalid number of params for eth_sendUserOperation", e.JsonRpcInvalidParams)
		return
	}
	fmt.Println("check")
	//2. Either the sender is an existing contract, or the initCode is not empty (but not both)
	senderCheck, err := addressHasCode(UopwithEP.UserOperation)
	if err != nil {
		http.Error(respw, err.Error(), e.JsonRpcInternalError) //error type not sure
		return
	}
	fmt.Println("check")
	if !senderCheck && UopwithEP.UserOperation.InitCode == nil {
		http.Error(respw, "neither sender nor initcode available", e.JsonRpcInvalidParams)
		return
	}
	fmt.Println("check")
	if senderCheck && UopwithEP.UserOperation.InitCode != nil {
		http.Error(respw, "cant take wallet as well as InitCode", e.JsonRpcInvalidParams)
	}
	fmt.Println("check")
	//3. Verification gas is sufficiently low
	max_verification_gas := big.NewInt(100e9) //from kristof's mev searcher bot. Needs optimization
	if UopwithEP.UserOperation.VerificationGas.Cmp(max_verification_gas) > 0 {
		http.Error(respw, "verification gas higher than max_verification_gas", e.JsonRpcInvalidParams)
		return
	}
	fmt.Println("check")
	//4.preVerification gas is sufficiently high
	sum := big.NewInt(0)
	sum.Add(UopwithEP.UserOperation.CallGas, UopwithEP.UserOperation.VerificationGas)
	if UopwithEP.UserOperation.PreVerificationGas.Cmp(sum) < 0 {
		http.Error(respw, "PreVerificationGas is not high enough", e.JsonRpcInvalidParams)
	}
	fmt.Println("check")
	//5. Paymaster is either zero address or contract with non zero code, registered and staked, sufficient deposit and not blacklisted
	//TODO need to have a db to handle registered paymasters and blacklisted paymasters
	//TODO check for sufficient deposit
	fmt.Println(r.Params[0])
	paymasterCheck, err := addressHasCode(UopwithEP.UserOperation)
	if err != nil {
		http.Error(respw, "error while getting code from sender address", e.JsonRpcInternalError) //error type not confirmed
		return
	}
	paymaster := getPaymaster(UopwithEP.UserOperation)
	if !(paymasterCheck || paymaster == zeroAddress) {
		http.Error(respw, "paymaster not contract or zero address", e.JsonRpcInvalidParams)
		return
	}
	//6. maxFeePerGas and maxPriorityFeeGas are greater or equal than block's basefee
	currBaseFee := getCurrentBlockBasefee()
	if !(UopwithEP.UserOperation.MaxFeePerGas.Cmp(currBaseFee) > 0 && UopwithEP.UserOperation.MaxPriorityFeePerGas.Cmp(currBaseFee) > 0) {
		http.Error(respw, "Max fee per gas too low ", e.JsonRpcInvalidParams)
		return
	}
	//TODO-7. Sender does not have another user op already in the pool. if that is the case the new tx should have +1 nonce

	//calling handleOps function
	success, tx, err := UopwithEP.UserOperation.CallHandleOps()
	if err != nil {
		http.Error(respw, "Handle Ops Call failed", e.JsonRpcTransactionError)
	}
	resData := r.WriteRPCResponse(success, tx.Hash())
	//need to check if this approach works
	json.NewEncoder(respw).Encode(resData)

}

func NewTypeUserOperation(json UserOperationJSON) UserOperationWithEntryPoint {
	return UserOperationWithEntryPoint{
		UserOperation: _UserOperation{
			Sender:               json.UserOperation.Sender,
			Nonce:                json.UserOperation.Nonce,
			InitCode:             json.UserOperation.InitCode,
			CallData:             json.UserOperation.CallData,
			CallGas:              json.UserOperation.CallGas,
			VerificationGas:      json.UserOperation.VerificationGas,
			PreVerificationGas:   json.UserOperation.PreVerificationGas,
			MaxFeePerGas:         json.UserOperation.MaxFeePerGas,
			MaxPriorityFeePerGas: json.UserOperation.MaxPriorityFeePerGas,
			PaymasterAndData:     json.UserOperation.PaymasterAndData,
			Signature:            json.UserOperation.Signature,
		},
		EntryPoint: json.EntryPoint,
	}
}

func handle_eth_supportedEntryPoints(respw http.ResponseWriter, req *http.Request) {
	respw.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(respw, safeEntryPoints[0].String())
}

func checkSafeEntryPoint(s UserOperationWithEntryPoint) bool {
	ep := s.EntryPoint
	for _, safeAddress := range safeEntryPoints {
		if ep == safeAddress {
			return true
		}
	}
	return false
}

func addressHasCode(uop _UserOperation) (bool, error) {
	conn, err := ethclient.Dial(os.Getenv("CLIENT"))

	if err != nil {
		return false, err
	}

	address := uop.Sender
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
	return os.Getenv("CLIENT")
}

func getPaymaster(uop _UserOperation) common.Address {
	pda := uop.PaymasterAndData
	return common.BytesToAddress(pda[0:41])
}
