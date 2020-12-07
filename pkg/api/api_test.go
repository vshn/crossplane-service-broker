package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"code.cloudfoundry.org/lager"
	"github.com/stretchr/testify/assert"
)

func TestAPI(t *testing.T) {
	a := New(lager.NewLogger("test"))
	ts := httptest.NewServer(a)
	defer ts.Close()

	res, err := http.Get(ts.URL +  "/healthz")
	assert.NoError(t, err)
	assert.Equal(t, res.StatusCode, http.StatusOK)
}
