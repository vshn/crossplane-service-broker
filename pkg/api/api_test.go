package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"code.cloudfoundry.org/lager"
	"github.com/pivotal-cf/brokerapi/v8/domain"
	"github.com/pivotal-cf/brokerapi/v8/fakes"
	"github.com/stretchr/testify/assert"
)

const (
	username = "test"
	password = "TEST"
)

func setupServer() (*API, *fakes.AutoFakeServiceBroker) {
	fakeServiceBroker := &fakes.AutoFakeServiceBroker{}

	a := New(fakeServiceBroker, username, password, lager.NewLogger("test"))
	return a, fakeServiceBroker
}

func makeRequest(a *API, method, path, username, password, apiVersion, contentType string, body io.Reader) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request, _ := http.NewRequest(method, path, body)
	if username != "" {
		request.SetBasicAuth(username, password)
	}
	if apiVersion != "" {
		request.Header.Add("X-Broker-API-Version", apiVersion)
	}
	if contentType != "" {
		request.Header.Add("Content-Type", contentType)
	}
	a.ServeHTTP(recorder, request)
	return recorder
}

func assertAPIMiddlewaresWork(t *testing.T, a *API, method, path string) {
	t.Run(method+" "+path+" unauthorized", func(t *testing.T) {
		rr := makeRequest(a, method, path, "", "", "", "", nil)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})
	t.Run(method+" "+path+" authorized missing version", func(t *testing.T) {
		rr := makeRequest(a, method, path, username, password, "", "", nil)
		assert.Equal(t, http.StatusPreconditionFailed, rr.Code)
	})
}

func TestAPI_Healthz(t *testing.T) {
	a, _ := setupServer()
	rr := makeRequest(a, http.MethodGet, "/healthz", "", "", "", "", nil)
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestAPI_SimpleRequests(t *testing.T) {
	paths := []string{
		"GET /v2/catalog",
		"GET /v2/service_instances/1111-2222-3333",
		"GET /v2/service_instances/1111-2222-3333/last_operation",
		"GET /v2/service_instances/1111-2222-3333/service_bindings/3333-4444-5555",
		"GET /v2/service_instances/1111-2222-3333/service_bindings/3333-4444-5555/last_operation",
	}

	a, _ := setupServer()

	for _, p := range paths {
		parts := strings.Split(p, " ")
		method := parts[0]
		path := parts[1]

		assertAPIMiddlewaresWork(t, a, method, path)
		t.Run(p+" ok", func(t *testing.T) {
			rr := makeRequest(a, method, path, username, password, "2.14", "", nil)
			assert.Equal(t, http.StatusOK, rr.Code)
		})
	}
}

func TestAPI_Provision(t *testing.T) {
	a, fsb := setupServer()
	method := "PUT"
	path := "/v2/service_instances/1111-2222-3333"
	assertAPIMiddlewaresWork(t, a, method, path)

	fsb.ServicesReturns([]domain.Service{
		{
			ID: "1111-2222-3333",
			Plans: []domain.ServicePlan{
				{
					ID: "2222-3333-4444",
				},
			},
		},
	}, nil)

	data := domain.ProvisionDetails{
		ServiceID: "1111-2222-3333",
		PlanID:    "2222-3333-4444",
	}

	body := &bytes.Buffer{}
	assert.NoError(t, json.NewEncoder(body).Encode(data))

	rr := makeRequest(a, method, path, username, password, "2.14", "application/json", body)
	assert.Equal(t, http.StatusCreated, rr.Code)
}

func TestAPI_Deprovision(t *testing.T) {
	a, _ := setupServer()
	method := "DELETE"
	path := "/v2/service_instances/1111-2222-3333"
	assertAPIMiddlewaresWork(t, a, method, path)

	body := url.Values{
		"service_id": []string{"1111-2222-3333"},
		"plan_id":    []string{"2222-3333-4444"},
	}

	rr := makeRequest(a, method, fmt.Sprintf("%s?%s", path, body.Encode()), username, password, "2.14", "", nil)
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestAPI_Update(t *testing.T) {
	a, _ := setupServer()
	method := "PATCH"
	path := "/v2/service_instances/1111-2222-3333"
	assertAPIMiddlewaresWork(t, a, method, path)

	data := domain.UpdateDetails{
		ServiceID: "1111-2222-3333",
	}

	body := &bytes.Buffer{}
	assert.NoError(t, json.NewEncoder(body).Encode(data))

	rr := makeRequest(a, method, path, username, password, "2.14", "application/json", body)
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestAPI_Bind(t *testing.T) {
	a, _ := setupServer()
	method := "PUT"
	path := "/v2/service_instances/1111-2222-3333/service_bindings/3333-4444-5555"
	assertAPIMiddlewaresWork(t, a, method, path)

	data := domain.UpdateDetails{
		ServiceID: "1111-2222-3333",
		PlanID:    "2222-3333-4444",
	}

	body := &bytes.Buffer{}
	assert.NoError(t, json.NewEncoder(body).Encode(data))

	rr := makeRequest(a, method, path, username, password, "2.14", "application/json", body)
	assert.Equal(t, http.StatusCreated, rr.Code)
}

func TestAPI_Unbind(t *testing.T) {
	a, _ := setupServer()
	method := "DELETE"
	path := "/v2/service_instances/1111-2222-3333/service_bindings/3333-4444-5555"
	assertAPIMiddlewaresWork(t, a, method, path)

	body := url.Values{
		"service_id": []string{"1111-2222-3333"},
		"plan_id":    []string{"2222-3333-4444"},
	}

	rr := makeRequest(a, method, fmt.Sprintf("%s?%s", path, body.Encode()), username, password, "2.14", "", nil)
	assert.Equal(t, http.StatusOK, rr.Code)
}
