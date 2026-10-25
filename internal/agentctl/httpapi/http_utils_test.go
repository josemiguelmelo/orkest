package httpapi

import (
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

type failingBody struct{}

func (failingBody) Read([]byte) (int, error) { return 0, errors.New("read failed") }

func TestReadJSON(t *testing.T) {
	for _, tc := range []struct {
		name, contentType, body string
		readFailure             bool
		status                  int
	}{
		{"valid with extra field", "application/json; charset=utf-8", `{"name":"example","extra":true}`, false, 200},
		{"missing content type", "", `{}`, false, 415},
		{"unsupported content type", "text/plain", `{}`, false, 415},
		{"invalid JSON", "application/json", `{`, false, 400},
		{"wrong field type", "application/json", `{"name":123}`, false, 400},
		{"multiple values", "application/json", `{} {}`, false, 400},
		{"unreadable body", "application/json", "", true, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/example", strings.NewReader(tc.body))
			r.Header.Set("Content-Type", tc.contentType)
			if tc.readFailure {
				r.Body = io.NopCloser(failingBody{})
			}
			w := httptest.NewRecorder()
			var destination struct {
				Name string `json:"name"`
			}
			ok := readJSON(w, r, &destination)
			if ok != (tc.status == 200) || w.Code != tc.status {
				t.Fatalf("got success=%v status=%d, want status=%d", ok, w.Code, tc.status)
			}
			if ok && destination.Name != "example" {
				t.Fatalf("unexpected decoded value: %+v", destination)
			}
			if !ok && w.Header().Get("Content-Type") != "application/json" {
				t.Fatal("error is not JSON")
			}
		})
	}
	// The helper also supports destinations other than endpoint configuration structs.
	r := httptest.NewRequest("POST", "/values", strings.NewReader(`[1,2,3]`))
	r.Header.Set("Content-Type", "application/json")
	var values []int
	if !readJSON(httptest.NewRecorder(), r, &values) || len(values) != 3 {
		t.Fatalf("could not decode slice: %v", values)
	}
}
