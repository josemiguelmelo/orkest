package kubernetes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestRESTAPIUsesProjectedTokenAndPreservesNotFoundSemantics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer projected-token" {
			http.Error(writer, "unauthorized", http.StatusUnauthorized)
			return
		}
		if request.URL.Path == "/missing" {
			http.NotFound(writer, request)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"metadata":{"name":"worker"}}`))
	}))
	defer server.Close()

	tokenPath := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenPath, []byte("projected-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	api := &restAPI{base: server.URL, tokenPath: tokenPath, client: server.Client()}
	resource, status, err := api.Do(context.Background(), http.MethodGet, "/worker", nil)
	if err != nil || status != http.StatusOK || nestedName(resource) != "worker" {
		t.Fatalf("Do(worker) = %#v, %d, %v", resource, status, err)
	}
	resource, status, err = api.Do(context.Background(), http.MethodGet, "/missing", nil)
	if err != nil || status != http.StatusNotFound || resource != nil {
		t.Fatalf("Do(missing) = %#v, %d, %v", resource, status, err)
	}
}

func nestedName(resource map[string]any) string {
	metadata, _ := resource["metadata"].(map[string]any)
	name, _ := metadata["name"].(string)
	return name
}
