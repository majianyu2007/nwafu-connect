package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/majianyu2007/nwafu-connect/log"
)

type QueryDeviceResult struct {
	SelfID        string `json:"selfId"`
	DeviceTrusted bool   `json:"deviceTrusted"`
}

func (s *Session) QueryDevice() (*QueryDeviceResult, error) {
	log.Println("Perform GET /passport/v1/security/queryDevice")

	u := s.baseURL + "/passport/v1/security/queryDevice"
	params := WithSharedParams(url.Values{
		"status": {"trust"}, // can be "trust" or "untrust" but no need
	})
	req, err := http.NewRequest("GET", u+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	s.setAuthJSONHeaders(req)

	resp, err := s.request(req)
	if err != nil {
		return nil, err
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	body, err := readAuthHTTPResponse(resp, "queryDevice", 8<<20)
	if err != nil {
		return nil, err
	}
	log.DebugPrintf("Received query device: %s", string(body))

	var re struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			SelfID        string `json:"selfId"`
			DeviceTrusted bool   `json:"deviceTrusted"`
		} `json:"data"`
	}
	err = json.Unmarshal(body, &re)
	if err != nil {
		return nil, err
	}
	log.DebugPrintf("Parsed query device: %+v", re)

	if re.Code != 0 {
		log.Printf("queryDevice failed with code %d: %s", re.Code, re.Message)
		return nil, fmt.Errorf("queryDevice failed with code %d: %s", re.Code, re.Message)
	}

	return &QueryDeviceResult{
		SelfID:        re.Data.SelfID,
		DeviceTrusted: re.Data.DeviceTrusted,
	}, nil
}

func (s *Session) TrustDevice(idList []string) error {
	log.Println("Perform POST /passport/v1/security/trustDevice")

	u := s.baseURL + "/passport/v1/security/trustDevice"
	payload := map[string]interface{}{
		"idList": idList,
	}
	bdy, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", u+"?"+WithSharedParams(nil).Encode(), bytes.NewReader(bdy))
	if err != nil {
		return err
	}
	s.setAuthJSONHeaders(req)

	resp, err := s.request(req)
	if err != nil {
		return err
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	body, err := readAuthHTTPResponse(resp, "trustDevice", 8<<20)
	if err != nil {
		return err
	}
	log.DebugPrintf("Received trust device: %s", string(body))

	var re struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	err = json.Unmarshal(body, &re)
	if err != nil {
		return err
	}
	log.DebugPrintf("Parsed trust device: %+v", re)

	if re.Code != 0 {
		log.Printf("trustDevice failed with code %d: %s", re.Code, re.Message)
		return fmt.Errorf("trustDevice failed with code %d: %s", re.Code, re.Message)
	}

	return nil
}

func (s *Session) UntrustDevice(idList []string) error {
	log.Println("Perform POST /passport/v1/security/untrustDevice")

	u := s.baseURL + "/passport/v1/security/untrustDevice"
	payload := map[string]interface{}{
		"idList": idList,
	}
	bdy, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", u+"?"+WithSharedParams(nil).Encode(), bytes.NewReader(bdy))
	if err != nil {
		return err
	}
	s.setAuthJSONHeaders(req)

	resp, err := s.request(req)
	if err != nil {
		return err
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	body, err := readAuthHTTPResponse(resp, "untrustDevice", 8<<20)
	if err != nil {
		return err
	}
	log.DebugPrintf("Received untrust device: %s", string(body))

	var re struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	err = json.Unmarshal(body, &re)
	if err != nil {
		return err
	}
	log.DebugPrintf("Parsed untrust device: %+v", re)

	if re.Code != 0 {
		log.Printf("untrustDevice failed with code %d: %s", re.Code, re.Message)
		return fmt.Errorf("untrustDevice failed with code %d: %s", re.Code, re.Message)
	}

	return nil
}
