// Package sigv4 signs requests to S3-compatible storage with AWS Signature
// Version 4, as backups to S3 storage and folders on S3 buckets send them.
package sigv4

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

// EmptyHash is the SHA-256 of an empty body.
const EmptyHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// Key signs requests for a region with an access key and its secret key.
type Key struct {
	Access, Secret, Region string
}

// Sign adds the date, the body's hash and the authorization of Signature
// Version 4 at now to request, its host, date and hash signed. The query
// must be canonical already (see CanonicalQuery).
func (k Key) Sign(request *http.Request, hash string, now time.Time) {
	now = now.UTC()
	stamp := now.Format("20060102T150405Z")
	day := now.Format("20060102")
	request.Header.Set("X-Amz-Date", stamp)
	request.Header.Set("X-Amz-Content-Sha256", hash)
	const signed = "host;x-amz-content-sha256;x-amz-date"
	canonical := strings.Join([]string{
		request.Method,
		EscapePath(request.URL.Path),
		request.URL.RawQuery,
		"host:" + request.URL.Host + "\n" + "x-amz-content-sha256:" + hash + "\n" + "x-amz-date:" + stamp + "\n",
		signed,
		hash,
	}, "\n")
	scope := day + "/" + k.Region + "/s3/aws4_request"
	sum := sha256.Sum256([]byte(canonical))
	toSign := "AWS4-HMAC-SHA256\n" + stamp + "\n" + scope + "\n" + hex.EncodeToString(sum[:])
	key := []byte("AWS4" + k.Secret)
	for _, part := range []string{day, k.Region, "s3", "aws4_request"} {
		key = hmacSHA256(key, part)
	}
	request.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+k.Access+"/"+scope+", SignedHeaders="+signed+
		", Signature="+hex.EncodeToString(hmacSHA256(key, toSign)))
}

func hmacSHA256(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(data))
	return mac.Sum(nil)
}

// EscapePath escapes each segment of path as Signature Version 4 does:
// everything but letters, digits and -._~.
func EscapePath(path string) string {
	segments := strings.Split(path, "/")
	for i, segment := range segments {
		segments[i] = escape(segment)
	}
	return strings.Join(segments, "/")
}

func escape(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		if 'A' <= c && c <= 'Z' || 'a' <= c && c <= 'z' || '0' <= c && c <= '9' || c == '-' || c == '.' || c == '_' || c == '~' {
			b.WriteByte(c)
			continue
		}
		b.WriteString("%" + strings.ToUpper(hex.EncodeToString([]byte{c})))
	}
	return b.String()
}

// CanonicalQuery is query as Signature Version 4 signs it, sorted by key,
// keys and values escaped.
func CanonicalQuery(query url.Values) string {
	var pairs []string
	for key, values := range query {
		for _, value := range values {
			pairs = append(pairs, escape(key)+"="+escape(value))
		}
	}
	slices.Sort(pairs)
	return strings.Join(pairs, "&")
}
