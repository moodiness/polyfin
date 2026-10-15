package sigv4

import (
	"net/http"
	"net/url"
	"testing"
	"time"
)

// The requests to S3-compatible storage are signed as AWS's example of
// Signature Version 4 for a GET of a bucket's objects.
func TestSignature(t *testing.T) {
	key := Key{Access: "AKIAIOSFODNN7EXAMPLE", Secret: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", Region: "us-east-1"}
	request, err := http.NewRequest(http.MethodGet, "https://examplebucket.s3.amazonaws.com/", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.URL.RawQuery = CanonicalQuery(url.Values{"max-keys": {"2"}, "prefix": {"J"}})
	key.Sign(request, EmptyHash, time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC))
	want := "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request, " +
		"SignedHeaders=host;x-amz-content-sha256;x-amz-date, Signature=34b48302e7b5fa45bde8084f4b7868a86f0a534bc59db6670ed5711ef69dc6f7"
	if got := request.Header.Get("Authorization"); got != want {
		t.Errorf("authorization:\n%s\nwant\n%s", got, want)
	}
	if got := EscapePath("/a b/c+d=(e)!"); got != "/a%20b/c%2Bd%3D%28e%29%21" {
		t.Errorf("escaped path: %s", got)
	}
}
