package corwhisk

import (
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"testing"
)

//TODO:!!
func Test_PlatformValidation(t *testing.T) {
	addr := "foo"
	conf := WhiskClientConfig{
		RequestPerMinute:       40,
		ConcurrencyLimit:       20,
		BatchRequestFeature:    true,
		MultiDeploymentFeature: false,
		DataPreloadingFeature:  false,
		Address:                &addr,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("Expected Accept: application/json header, got: %s", r.Header.Get("Accept"))
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"features":{"batch":true,"feedback":true,"lch":false},"limits":{"actions_per_minute":120,"concurrent_actions":30}}`))
	}))

	err := ensurePlatformConfig(server.URL, conf)
	assert.Nil(t, err)

	conf.DataPreloadingFeature = true
	err = ensurePlatformConfig(server.URL, conf)
	assert.NotNil(t, err)
}
