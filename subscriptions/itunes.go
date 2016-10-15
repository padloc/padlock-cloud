package main

import "time"
import "encoding/json"
import "strconv"
import "net/http"
import "bytes"
import "io/ioutil"
import "errors"
import "fmt"

const (
	ItunesStatusOK                   = 0
	ItunesStatusInvalidJSON          = 21000
	ItunesStatusInvalidReceipt       = 21002
	ItunesStatusNotAuthenticated     = 21003
	ItunesStatusWrongSecret          = 21004
	ItunesStatusServerUnavailable    = 21005
	ItunesStatusExpired              = 21006
	ItunesStatusWrongEnvironmentProd = 21007
	ItunesStatusWrongEnvironmentTest = 21008
)

const (
	ItunesUrlProduction = "https://buy.itunes.apple.com/verifyReceipt"
	ItunesUrlSandbox    = "https://sandbox.itunes.apple.com/verifyReceipt"
)

const ReceiptTypeItunes = "ios-appstore"

type ItunesInterface interface {
	ValidateReceipt(string) (*ItunesPlan, error)
}

type ItunesConfig struct {
	SharedSecret string `yaml:"shared_secret"`
	Environment  string `yaml:"environment"`
}

type ItunesServer struct {
	Config *ItunesConfig
}

func parseItunesResult(data []byte) (*ItunesPlan, error) {
	result := &struct {
		Status            int `json:"status"`
		LatestReceiptInfo struct {
			Expires string `json:"expires_date"`
		} `json:"latest_receipt_info"`
		LatestExpiredReceiptInfo struct {
			Expires string `json:"expires_date"`
		} `json:"latest_expired_receipt_info"`
		LatestReceipt string `json:"latest_receipt"`
	}{}

	if err := json.Unmarshal(data, result); err != nil {
		return nil, err
	}

	expiresStr := result.LatestReceiptInfo.Expires
	if expiresStr == "" {
		expiresStr = result.LatestExpiredReceiptInfo.Expires
	}

	var expiresInt int
	var err error
	if expiresStr != "" {
		if expiresInt, err = strconv.Atoi(expiresStr); err != nil {
			return nil, err
		}
	}

	expires := time.Unix(0, int64(expiresInt)*1000000)

	return &ItunesPlan{
		result.LatestReceipt,
		expires,
		result.Status,
	}, nil
}

func (itunes *ItunesServer) ValidateReceipt(receipt string) (*ItunesPlan, error) {
	fmt.Printf("validating receipt %s, environment: %s", receipt, itunes.Config.Environment)
	body, err := json.Marshal(map[string]string{
		"receipt-data": receipt,
		"password":     itunes.Config.SharedSecret,
	})
	if err != nil {
		return nil, err
	}

	var itunesUrl string
	if itunes.Config.Environment == "production" {
		itunesUrl = ItunesUrlProduction
	} else {
		itunesUrl = ItunesUrlSandbox
	}

	resp, err := http.Post(itunesUrl, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}

	respData, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	result, err := parseItunesResult(respData)

	switch result.Status {
	case ItunesStatusOK, ItunesStatusExpired:
		return result, nil
	case ItunesStatusInvalidReceipt, ItunesStatusNotAuthenticated:
		return nil, ErrInvalidReceipt
	default:
		return nil, errors.New(fmt.Sprintf("Failed to validate receipt, status: %d", result.Status))
	}
}
