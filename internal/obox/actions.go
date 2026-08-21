package obox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"epos-proxy/internal/logger"
	"epos-proxy/internal/printer"
)

func (m *Module) buildDeviceList() []map[string]string {
	discovered := printer.DiscoverAllPrinters(m.cfg)
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

var httpClient = &http.Client{
	Timeout: 5 * time.Second,
}

func (m *Module) dispatchLocalAction(ctx context.Context, actionPayload ActionPayload) (interface{}, error) {
	fullURL := fmt.Sprintf("http://%s%s", m.localAddrFn(), actionPayload.URL)

	var body io.Reader
	if actionPayload.Method == http.MethodPost {
		bodyBytes, err := json.Marshal(actionPayload.Payload)
		if err != nil {
			return nil, fmt.Errorf("marshal payload: %w", err)
		}
		body = bytes.NewReader(bodyBytes)
	}

	method := actionPayload.Method
	if method == "" {
		method = http.MethodGet
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, body)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("local HTTP request failed (%s): %w", fullURL, err)
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, 5<<20) // 5MB cap

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(limited)
		return nil, fmt.Errorf("local service returned status %d: %s", resp.StatusCode, string(raw))
	}

	var result interface{}
	if err := json.NewDecoder(limited).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	logger.Debugf("[obox queue] local action result: %v", result)
	return result, nil
}
