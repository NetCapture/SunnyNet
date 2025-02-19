package tests

import (
	"strings"
	"testing"

	"github.com/qtgolang/SunnyNet/src/http"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/qtgolang/SunnyNet/src/tlsClient/tlsClient/profiles"
	"github.com/stretchr/testify/assert"
)

func TestWeb(t *testing.T) {
	options := []tls_client.HttpClientOption{
		tls_client.WithClientProfile(profiles.Firefox_110),
	}

	client, err := tls_client.NewHttpClient(nil, options...)
	if err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequest(http.MethodPost, "https://registrierung.web.de/account/email-registration", strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}

	req.Header = defaultHeader

	_, err = client.Do(req)
	assert.NoError(t, err)
}
