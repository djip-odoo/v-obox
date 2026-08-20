package obox

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"epos-proxy/internal/logger"
	"epos-proxy/internal/printer"
)

func (m *Module) buildDeviceList() []map[string]string {
	discovered := printer.DiscoverAllPrinters(m.cfg, nil)
	list := make([]map[string]string, 0, len(discovered.Printers))
	for _, p := range discovered.Printers {
		list = append(list, map[string]string{
			"name":       p.Name,
			"identifier": p.Identifier,
			"type":       "printer",
		})
	}
	return list
}

func (m *Module) dispatchLocalAction(actionPayload ActionPayload) interface{} {
	fullURL := fmt.Sprintf("http://%s%s", m.localAddrFn(), actionPayload.URL)

	var req *http.Request
	var err error

	switch actionPayload.Method {
	case "POST":
		var bodyBytes []byte
		bodyBytes, err = json.Marshal(actionPayload.Payload)
		if err != nil {
			logger.Errorf("[obox queue] marshal payload error: %v", err)
			return map[string]string{"error": fmt.Sprintf("marshal payload error: %v", err)}
		}
		req, err = http.NewRequest("POST", fullURL, bytes.NewReader(bodyBytes))
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
		}
	default: // GET
		req, err = http.NewRequest("GET", fullURL, nil)
	}

	if err != nil {
		logger.Errorf("[obox queue] build request error: %v", err)
		return map[string]string{"error": fmt.Sprintf("build request error: %v", err)}
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		logger.Errorf("[obox queue] local action HTTP error url=%s: %v", fullURL, err)
		return map[string]string{"error": fmt.Sprintf("local HTTP request failed: %v", err)}
	}
	defer resp.Body.Close()

	var result interface{}
	if decodeErr := json.NewDecoder(resp.Body).Decode(&result); decodeErr != nil || result == nil {
		logger.Errorf("[obox queue] decode response error: %v", decodeErr)
		return map[string]string{"error": "empty or invalid response from local service"}
	}
	logger.Infof("[obox queue] local action result: %v", result)
	return result
}
