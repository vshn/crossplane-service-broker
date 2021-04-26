package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"code.cloudfoundry.org/lager"
	"github.com/pascaldekloe/jwt"
	"github.com/pivotal-cf/brokerapi/v8/domain"
	"github.com/pivotal-cf/brokerapi/v8/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/vshn/crossplane-service-broker/pkg/api/auth"
)

const (
	username = "test"
	password = "TEST"

	// HMACSHA256 Token:
	//   {
	//    "alg": "HS256",
	//    "typ": "JWT"
	//   }.
	//   {
	//    "sub": "1234567890",
	//    "name": "John Doe",
	//    "iat": 1516239022,
	//    "foo": "bart"
	//   }.
	//   HMACSHA256(
	//    base64UrlEncode(header) + "." +
	//    base64UrlEncode(payload),
	//    "test"
	//   )
	// You can view and edit this token at:
	// https://jwt.io/#debugger-io?token=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyLCJmb28iOiJiYXJ0In0.stIfFmxL4y-GljRo2oJF0FWwheo6Ss-mIjJVJ-XPIY0
	token = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyLCJmb28iOiJiYXJ0In0.stIfFmxL4y-GljRo2oJF0FWwheo6Ss-mIjJVJ-XPIY0"

	// HMACSHA256 Token, but signed with a different key than the other token:
	//   {
	//    "alg": "HS256",
	//    "typ": "JWT"
	//   }.
	//   {
	//    "sub": "1234567890",
	//    "name": "John Doe",
	//    "iat": 1516239022,
	//    "foo": "bart"
	//   }.
	//   HMACSHA256(
	//    base64UrlEncode(header) + "." +
	//    base64UrlEncode(payload) +
	//    "invalid_test"
	//   )
	// You can view and edit this token at:
	// https://jwt.io/#debugger-io?token=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyLCJmb28iOiJiYXJ0In0.HCGub1H3ZaVRBBUajMYoqTCl13pyck5Mego8mWghl88
	invalidToken = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyLCJmb28iOiJiYXJ0In0.HCGub1H3ZaVRBBUajMYoqTCl13pyck5Mego8mWghl88"
)

func setupServer() (*API, *fakes.AutoFakeServiceBroker) {
	fakeServiceBroker := &fakes.AutoFakeServiceBroker{}

	a := New(fakeServiceBroker,
		auth.SingleCredential(username, password),
		&jwt.KeyRegister{Secrets: [][]byte{[]byte("test")}},
		lager.NewLogger("test"))
	return a, fakeServiceBroker
}

// Do not add references here!
type apiRequest struct {
	method      string
	path        string
	username    string
	password    string
	token       string
	apiVersion  string
	contentType string
	body        bytes.Buffer
}

func makeRequest(a *API, r apiRequest) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request, _ := http.NewRequest(r.method, r.path, &r.body)
	if r.username != "" {
		request.SetBasicAuth(r.username, r.password)
	} else if r.token != "" {
		request.Header.Set("Authorization", "Bearer "+r.token)
	}

	if r.apiVersion != "" {
		request.Header.Add("X-Broker-API-Version", r.apiVersion)
	}
	if r.contentType != "" {
		request.Header.Add("Content-Type", r.contentType)
	}
	a.ServeHTTP(recorder, request)
	return recorder
}

func assertAPIMiddlewaresWork(t *testing.T, a *API, method, path string) {
	t.Run(fmt.Sprintf("given '%s' on '%s' and no credentials expect unauthorized", method, path), func(t *testing.T) {
		rr := makeRequest(a, apiRequest{
			method: method,
			path:   path,
		})
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})
	t.Run(fmt.Sprintf("given '%s' on '%s' and invalid username expect unauthorized", method, path), func(t *testing.T) {
		rr := makeRequest(a, apiRequest{
			method:   method,
			username: fmt.Sprintf("invalid_%s", username),
			password: password,
			path:     path,
		})
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})
	t.Run(fmt.Sprintf("given '%s' on '%s' and invalid password expect unauthorized", method, path), func(t *testing.T) {
		rr := makeRequest(a, apiRequest{
			method:   method,
			username: username,
			password: fmt.Sprintf("invalid_%s", password),
			path:     path,
		})
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})
	t.Run(fmt.Sprintf("given '%s' on '%s' and invalid bearer token expect unauthorized", method, path), func(t *testing.T) {
		rr := makeRequest(a, apiRequest{
			method: method,
			token:  "garbage",
			path:   path,
		})
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})
	t.Run(fmt.Sprintf("given '%s' on '%s' and invalid bearer token signature expect unauthorized", method, path), func(t *testing.T) {
		rr := makeRequest(a, apiRequest{
			method: method,
			token:  invalidToken,
			path:   path,
		})
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})
	t.Run(fmt.Sprintf("given '%s' on '%s' and missing api version expect unauthorized", method, path), func(t *testing.T) {
		_, _ = assertAuthenticatedRequest(t, a, http.StatusPreconditionFailed, apiRequest{
			method: method,
			path:   path,
		})
	})
}

func TestAPI_Healthz(t *testing.T) {
	a, _ := setupServer()
	rr := makeRequest(a, apiRequest{method: http.MethodGet, path: "/healthz"})
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestAPI_Metrics(t *testing.T) {
	a, _ := setupServer()
	rr := makeRequest(a, apiRequest{method: http.MethodGet, path: "/metrics"})
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
			_, _ = assertAuthenticatedRequest(t, a, http.StatusOK, apiRequest{
				method:     method,
				path:       path,
				apiVersion: "2.14",
			})
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

	body := bytes.Buffer{}
	assert.NoError(t, json.NewEncoder(&body).Encode(data))

	_, _ = assertAuthenticatedRequest(t, a, http.StatusCreated, apiRequest{
		method:      method,
		path:        path,
		apiVersion:  "2.14",
		contentType: "application/json",
		body:        body,
	})
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

	_, _ = assertAuthenticatedRequest(t, a, http.StatusOK, apiRequest{
		method:     method,
		path:       fmt.Sprintf("%s?%s", path, body.Encode()),
		apiVersion: "2.14",
	})
}

func TestAPI_Update(t *testing.T) {
	a, _ := setupServer()
	method := "PATCH"
	path := "/v2/service_instances/1111-2222-3333"
	assertAPIMiddlewaresWork(t, a, method, path)

	data := domain.UpdateDetails{
		ServiceID: "1111-2222-3333",
	}

	body := bytes.Buffer{}
	assert.NoError(t, json.NewEncoder(&body).Encode(data))

	_, _ = assertAuthenticatedRequest(t, a, http.StatusOK, apiRequest{
		method:      method,
		path:        path,
		apiVersion:  "2.14",
		contentType: "application/json",
		body:        body,
	})
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

	body := bytes.Buffer{}
	assert.NoError(t, json.NewEncoder(&body).Encode(data))

	_, _ = assertAuthenticatedRequest(t, a, http.StatusCreated, apiRequest{
		method:      method,
		path:        path,
		apiVersion:  "2.14",
		contentType: "application/json",
		body:        body,
	})
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

	_, _ = assertAuthenticatedRequest(t, a, http.StatusOK, apiRequest{
		method:     method,
		path:       fmt.Sprintf("%s?%s", path, body.Encode()),
		apiVersion: "2.14",
	})
}

func assertAuthenticatedRequest(t *testing.T, a *API, expectedStatus int, r apiRequest) (rBasic, rBearer *httptest.ResponseRecorder) {
	basicAuthRequest := r
	basicAuthRequest.username = username
	basicAuthRequest.password = password
	basicAuthRequest.token = ""
	rBasic = makeRequest(a, basicAuthRequest)
	assert.Equalf(t, expectedStatus, rBasic.Code, "Error when authenticating to '%s' using valid Basic credentials: Unexpected status code on response", r.path)

	bearerAuthRequest := r
	bearerAuthRequest.username = ""
	bearerAuthRequest.password = ""
	bearerAuthRequest.token = token
	rBearer = makeRequest(a, bearerAuthRequest)
	assert.Equalf(t, expectedStatus, rBearer.Code, "Error when authenticating to '%s' using a valid Bearer token: Unexpected status code on response", r.path)
	return
}
