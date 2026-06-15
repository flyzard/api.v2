package at

import (
	"net/http"
	"testing"
)

func TestNewClient_NoCertMeansNoClientCert(t *testing.T) {
	c, err := NewClient(Config{
		TaxpayerNIF: "123456789", Username: "u", Password: "p",
		SeriesURL: "https://example.invalid/series",
	})
	if err != nil {
		t.Fatal(err)
	}
	tr := c.httpClient.Transport.(*http.Transport)
	if n := len(tr.TLSClientConfig.Certificates); n != 0 {
		t.Fatalf("zero-value client cert installed (%d certs); empty list lets TLS answer a CertificateRequest with 'no cert'", n)
	}
	if tr.Proxy == nil {
		t.Fatal("transport lost ProxyFromEnvironment")
	}
}
