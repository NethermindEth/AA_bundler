package main

import (
	"github.com/joho/godotenv"
	"io"
	"net/http/httptest"
	"os"
	"testing"
)

func TestSupportedEntryPoints(t *testing.T) {
	godotenv.Load(".env")

	req := httptest.NewRequest("GET", "/eth_supportedEntryPoints", nil)
	respw := httptest.NewRecorder()
	handle_eth_supportedEntryPoints(respw, req)
	res := respw.Result()
	defer res.Body.Close()

	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Errorf("Error: %v", err)
	}
	if string(data) != os.Getenv("ENTRYPOINT_CONTRACT") {
		t.Errorf("Unexpected body %v", string(data))
	}
}
