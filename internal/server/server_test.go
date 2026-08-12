package server

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"epos-proxy/internal/printer"
	"epos-proxy/internal/testutil"
)

func TestServer_Lifecycle(t *testing.T) {
	mgr := printer.NewManager()
	s := New(4548, mgr)
	defer s.Stop()

	testutil.ExpectedTrue(t, s.Running(), "Expected server to be running after New()")
	testutil.ExpectedEqual(t, s.Port, 4548)

	err := s.Stop()
	testutil.ExpectedNoError(t, err)
}

func TestPrintData_ValidXML_Success(t *testing.T) {
	// Start mock TCP listener on port 9100 for LAN printer
	_, _, err := testutil.StartMockTCPServer(t, printer.LANPort, func(conn net.Conn) {
		defer conn.Close()
		buf := make([]byte, 1024)
		_, _ = conn.Read(buf)
	})
	if err != nil {
		t.Skipf("Cannot bind port %d for printer mock: %v", printer.LANPort, err)
	}

	mgr := printer.NewManager()
	s := New(4549, mgr)
	defer s.Stop()

	printerID := printer.EncodeLANPrinterID("127.0.0.1")
	xmlPayload := `<epos-print><text align="center">ORDER #123</text><cut /></epos-print>`

	url := fmt.Sprintf("/p/%s/cgi-bin/epos/service.cgi", printerID)
	req := httptest.NewRequest("POST", url, bytes.NewReader([]byte(xmlPayload)))
	req.Header.Set("Content-Type", "text/xml")

	resp, err := s.app.Test(req)
	testutil.ExpectedNoError(t, err)
	testutil.ExpectedEqual(t, resp.StatusCode, http.StatusOK)

	body, err := io.ReadAll(resp.Body)
	testutil.ExpectedNoError(t, err)
	testutil.ExpectedContains(t, string(body), `success="true"`)
}

func TestPrintData_SchemaError(t *testing.T) {
	mgr := printer.NewManager()
	s := New(4550, mgr)
	defer s.Stop()

	invalidPayload := `<invalid>not-an-epos-print</invalid>`
	req := httptest.NewRequest("POST", "/p/any-printer/cgi-bin/epos/service.cgi", bytes.NewReader([]byte(invalidPayload)))
	req.Header.Set("Content-Type", "text/xml")

	resp, err := s.app.Test(req)
	testutil.ExpectedNoError(t, err)

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	testutil.ExpectedContains(t, bodyStr, `success="false"`)
	testutil.ExpectedContains(t, bodyStr, `code="SchemaError"`)
}

func TestPrintData_UnreachablePrinter_EX_BADPORT(t *testing.T) {
	mgr := printer.NewManager()
	s := New(4551, mgr)
	defer s.Stop()

	// Use a non-existent USB printer serial that cannot be found
	printerID := "czpOT05fRVhJU1RFTlRfU0VSSUFMCg"
	xmlPayload := `<epos-print><text>Hello</text></epos-print>`

	url := fmt.Sprintf("/p/%s/cgi-bin/epos/service.cgi", printerID)
	req := httptest.NewRequest("POST", url, bytes.NewReader([]byte(xmlPayload)))
	req.Header.Set("Content-Type", "text/xml")

	resp, err := s.app.Test(req)
	testutil.ExpectedNoError(t, err)

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	testutil.ExpectedContains(t, bodyStr, `success="false"`)
	testutil.ExpectedContains(t, bodyStr, `code="EX_BADPORT"`)
}

func TestPrintData_AutoSelectRoute(t *testing.T) {
	mgr := printer.NewManager()
	s := New(4552, mgr)
	defer s.Stop()

	// Schema error on auto-select route
	req := httptest.NewRequest("POST", "/cgi-bin/epos/service.cgi", bytes.NewReader([]byte("<bad></bad>")))
	resp, err := s.app.Test(req)
	testutil.ExpectedNoError(t, err)

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	testutil.ExpectedContains(t, bodyStr, `code="SchemaError"`)
}

func TestPrintLabel_Success(t *testing.T) {
	// Start mock TCP listener on port 9100
	_, _, err := testutil.StartMockTCPServer(t, printer.LANPort, func(conn net.Conn) {
		defer conn.Close()
		buf := make([]byte, 1024)
		_, _ = conn.Read(buf)
	})
	if err != nil {
		t.Skipf("Cannot bind port %d for label mock: %v", printer.LANPort, err)
	}

	mgr := printer.NewManager()
	s := New(4553, mgr)
	defer s.Stop()

	printerID := printer.EncodeLANPrinterID("127.0.0.1")
	labelData := []byte("^XA^FDBarcode123^FS^XZ")

	url := fmt.Sprintf("/p/%s/pstprnt", printerID)
	req := httptest.NewRequest("POST", url, bytes.NewReader(labelData))

	resp, err := s.app.Test(req)
	testutil.ExpectedNoError(t, err)
	testutil.ExpectedEqual(t, resp.StatusCode, http.StatusOK)
}

func TestPrintLabel_EmptyBody_BadRequest(t *testing.T) {
	mgr := printer.NewManager()
	s := New(4554, mgr)
	defer s.Stop()

	req := httptest.NewRequest("POST", "/p/any-printer/pstprnt", bytes.NewReader([]byte{}))
	resp, err := s.app.Test(req)
	testutil.ExpectedNoError(t, err)
	testutil.ExpectedEqual(t, resp.StatusCode, http.StatusBadRequest)
}

func TestPrintLabel_UnreachablePrinter_ServerError(t *testing.T) {
	mgr := printer.NewManager()
	s := New(4555, mgr)
	defer s.Stop()

	printerID := "czpOT05fRVhJU1RFTlRfU0VSSUFMCg"
	labelData := []byte("^XA^FDTest^FS^XZ")

	url := fmt.Sprintf("/p/%s/pstprnt", printerID)
	req := httptest.NewRequest("POST", url, bytes.NewReader(labelData))

	resp, err := s.app.Test(req)
	testutil.ExpectedNoError(t, err)
	testutil.ExpectedEqual(t, resp.StatusCode, http.StatusInternalServerError)
}

func TestCORSHeaders(t *testing.T) {
	mgr := printer.NewManager()
	s := New(4546, mgr)
	defer s.Stop()

	req := httptest.NewRequest("OPTIONS", "/cgi-bin/epos/service.cgi", nil)
	req.Header.Set("Origin", "http://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")

	resp, err := s.app.Test(req)
	testutil.ExpectedNoError(t, err)

	allowOrigin := resp.Header.Get("Access-Control-Allow-Origin")
	testutil.ExpectedEqual(t, allowOrigin, "*")
}
