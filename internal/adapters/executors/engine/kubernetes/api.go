// Package kubernetes contains the infrastructure API client and engine
// implementation for Kubernetes-backed executor workloads.
package kubernetes

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// API is the narrow Kubernetes REST surface used by the engine.
type API interface {
	Do(context.Context, string, string, any) (map[string]any, int, error)
}

// LogAPI exposes the optional pod log stream used for worker diagnostics.
type LogAPI interface {
	PodLogs(context.Context, string) (io.ReadCloser, error)
}

type restAPI struct {
	base, tokenPath string
	client          *http.Client
}

var (
	_ API    = (*restAPI)(nil)
	_ LogAPI = (*restAPI)(nil)
)

// NewInClusterAPI constructs a client from the Kubernetes service environment
// and projected service account credentials.
func NewInClusterAPI() (API, error) {
	host := os.Getenv("KUBERNETES_SERVICE_HOST")
	port := os.Getenv("KUBERNETES_SERVICE_PORT_HTTPS")
	if host == "" {
		return nil, errors.New("kubernetes executor requires in-cluster service discovery or an injected API client")
	}
	if port == "" {
		port = "443"
	}
	const tokenPath = "/var/run/secrets/kubernetes.io/serviceaccount/token"
	tokenBytes, err := os.ReadFile(tokenPath)
	if err != nil {
		return nil, fmt.Errorf("read Kubernetes service account token: %w", err)
	}
	if strings.TrimSpace(string(tokenBytes)) == "" {
		return nil, errors.New("kubernetes service account token is empty")
	}
	ca, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/ca.crt")
	if err != nil {
		return nil, fmt.Errorf("read Kubernetes CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		return nil, errors.New("kubernetes CA bundle is invalid")
	}
	return &restAPI{base: "https://" + host + ":" + port, tokenPath: tokenPath, client: &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}}}, nil
}

func (a *restAPI) authorize(req *http.Request) error {
	token, err := os.ReadFile(a.tokenPath)
	if err != nil {
		return fmt.Errorf("read Kubernetes service account token: %w", err)
	}
	value := strings.TrimSpace(string(token))
	if value == "" {
		return errors.New("kubernetes service account token is empty")
	}
	req.Header.Set("Authorization", "Bearer "+value)
	return nil
}

func (a *restAPI) Do(ctx context.Context, method, requestPath string, body any) (map[string]any, int, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		reader = strings.NewReader(string(data))
	}
	req, err := http.NewRequestWithContext(ctx, method, a.base+requestPath, reader)
	if err != nil {
		return nil, 0, err
	}
	if err = a.authorize(req); err != nil {
		return nil, 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := a.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = res.Body.Close() }()
	var value map[string]any
	if res.StatusCode != http.StatusNotFound {
		if err = json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&value); err != nil && err != io.EOF {
			return nil, res.StatusCode, err
		}
	}
	if res.StatusCode >= 300 && res.StatusCode != http.StatusNotFound {
		return value, res.StatusCode, fmt.Errorf("kubernetes API returned HTTP %d", res.StatusCode)
	}
	return value, res.StatusCode, nil
}

func (a *restAPI) PodLogs(ctx context.Context, requestPath string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.base+requestPath, nil)
	if err != nil {
		return nil, err
	}
	if err = a.authorize(req); err != nil {
		return nil, err
	}
	streamClient := &http.Client{Transport: a.client.Transport}
	response, err := streamClient.Do(req)
	if err != nil {
		return nil, err
	}
	if response.StatusCode >= 300 {
		_ = response.Body.Close()
		return nil, fmt.Errorf("kubernetes pod logs returned HTTP %d", response.StatusCode)
	}
	return response.Body, nil
}
