package tests

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestHTTPMaskProxy_AutoPathRoot(t *testing.T) {
	runHTTPMaskAutoPathRootProxyCase(t, "httpmask-auto-pathroot-ok")
}

func TestMatrixSmoke_HTTPMaskAutoPathRoot(t *testing.T) {
	runHTTPMaskAutoPathRootProxyCase(t, "matrix-smoke-app-httpmask-ok")
}

func runHTTPMaskAutoPathRootProxyCase(t *testing.T, wantBody string) {
	t.Helper()

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, wantBody)
	}))
	defer origin.Close()

	serverKey, clientKey := newTestKeys(t)

	ports, err := getFreePorts(2)
	if err != nil {
		t.Fatalf("ports: %v", err)
	}
	serverPort := ports[0]
	clientPort := ports[1]

	serverCfg := newTestServerConfig(serverPort, serverKey)
	serverCfg.HTTPMask.Disable = false
	serverCfg.HTTPMask.Mode = "auto"
	serverCfg.HTTPMask.PathRoot = "aabbcc"

	startSudokuServer(t, serverCfg)

	clientCfg := newTestClientConfig(clientPort, localServerAddr(serverPort), clientKey)
	clientCfg.ProxyMode = "global"
	clientCfg.HTTPMask.Disable = false
	clientCfg.HTTPMask.Mode = "auto"
	clientCfg.HTTPMask.PathRoot = "aabbcc"

	startSudokuClient(t, clientCfg)

	proxyURL, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", clientPort))
	if err != nil {
		t.Fatalf("proxy url: %v", err)
	}

	httpClient := &http.Client{
		Transport: &http.Transport{
			Proxy:             http.ProxyURL(proxyURL),
			DisableKeepAlives: true,
		},
		Timeout: 10 * time.Second,
	}

	resp, err := httpClient.Get(origin.URL)
	if err != nil {
		t.Fatalf("proxy get: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("bad status: %s body=%q", resp.Status, string(body))
	}
	if string(body) != wantBody {
		t.Fatalf("unexpected body: %q", string(body))
	}
}
