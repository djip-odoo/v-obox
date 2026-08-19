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
	discovered := printer.DiscoverAllPrinters(m.cfg)
	list := make([]map[string]string, 0, len(discovered.Available))
	for _, p := range discovered.Available {
		list = append(list, map[string]string{
			"name":       p.Name,
			"identifier": p.Identifier,
			"type":       "printer",
		})
	}
	return list
}

func (m *Module) dispatchLocalAction(path, method string, payload interface{}) interface{} {
	fullURL := fmt.Sprintf("http://%s%s", m.localAddrFn(), path)

	var req *http.Request
	var err error

	switch method {
	case "POST":
		var bodyBytes []byte
		bodyBytes, err = json.Marshal(payload)
		if err != nil {
			logger.Errorf("[obox queue] marshal payload error: %v", err)
			return map[string]string{"status": "ok"}
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
		return map[string]string{"status": "ok"}
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		logger.Errorf("[obox queue] local action HTTP error url=%s: %v", fullURL, err)
		return map[string]string{"status": "ok"}
	}
	defer resp.Body.Close()

	var result interface{}
	if decodeErr := json.NewDecoder(resp.Body).Decode(&result); decodeErr != nil || result == nil {
		return map[string]string{"status": "ok"}
	}
	logger.Infof("[obox queue] local action result: %v", result)
	return result
}
