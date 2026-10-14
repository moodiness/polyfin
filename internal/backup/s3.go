package backup

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

// defaultRegion is the region requests are signed for when a destination
// names none, which most S3-compatible storages accept.
const defaultRegion = "us-east-1"

// s3Remote is a prefix of a bucket of S3-compatible storage, reached at
// the endpoint's address with the bucket in the path, which every such
// storage answers, and requests signed with AWS Signature Version 4.
type s3Remote struct {
	client   *http.Client
	endpoint *url.URL
	bucket   string
	prefix   string
	region   string
	access   string
	secret   string
	now      func() time.Time
}

func newS3Remote(d Destination, secret string, idle time.Duration) (*s3Remote, error) {
	endpoint, err := url.Parse(d.Address)
	if err != nil {
		return nil, ErrInvalidDestinationAddress
	}
	region := d.Region
	if region == "" {
		region = defaultRegion
	}
	return &s3Remote{client: newHTTPClient(idle), endpoint: endpoint, bucket: d.Bucket, prefix: d.Folder, region: region, access: d.Username,
		secret: secret, now: time.Now}, nil
}

func (s *s3Remote) close() error { return closeHTTP(s.client) }

// key is the object key of name.
func (s *s3Remote) key(name string) string {
	if s.prefix == "" {
		return name
	}
	return s.prefix + "/" + name
}

// url is the address of the bucket's object key, or of the bucket itself
// for an empty key, with query.
func (s *s3Remote) url(key string, query url.Values) *url.URL {
	u := *s.endpoint
	u.Path = strings.TrimSuffix(u.Path, "/") + "/" + s.bucket
	if key != "" {
		u.Path += "/" + key
	}
	// Each segment is escaped as Signature Version 4 signs it.
	u.RawPath = escapePath(u.Path)
	u.RawQuery = canonicalQuery(query)
	return &u
}

// emptyHash is the SHA-256 of an empty body.
const emptyHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// do signs and sends a request with body, whose SHA-256 is hash.
func (s *s3Remote) do(ctx context.Context, method string, u *url.URL, body io.Reader, size int64, hash string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		request.ContentLength = size
	}
	s.sign(request, hash)
	return s.client.Do(request)
}

// sign adds the date, the body's hash and the authorization of AWS
// Signature Version 4 to request.
func (s *s3Remote) sign(request *http.Request, hash string) {
	now := s.now().UTC()
	stamp := now.Format("20060102T150405Z")
	day := now.Format("20060102")
	request.Header.Set("X-Amz-Date", stamp)
	request.Header.Set("X-Amz-Content-Sha256", hash)
	const signed = "host;x-amz-content-sha256;x-amz-date"
	canonical := strings.Join([]string{
		request.Method,
		escapePath(request.URL.Path),
		request.URL.RawQuery,
		"host:" + request.URL.Host + "\n" + "x-amz-content-sha256:" + hash + "\n" + "x-amz-date:" + stamp + "\n",
		signed,
		hash,
	}, "\n")
	scope := day + "/" + s.region + "/s3/aws4_request"
	sum := sha256.Sum256([]byte(canonical))
	toSign := "AWS4-HMAC-SHA256\n" + stamp + "\n" + scope + "\n" + hex.EncodeToString(sum[:])
	key := []byte("AWS4" + s.secret)
	for _, part := range []string{day, s.region, "s3", "aws4_request"} {
		key = hmacSHA256(key, part)
	}
	request.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+s.access+"/"+scope+", SignedHeaders="+signed+
		", Signature="+hex.EncodeToString(hmacSHA256(key, toSign)))
}

func hmacSHA256(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(data))
	return mac.Sum(nil)
}

// escapePath escapes each segment of path as Signature Version 4 does:
// everything but letters, digits and -._~.
func escapePath(path string) string {
	segments := strings.Split(path, "/")
	for i, segment := range segments {
		segments[i] = escapeSigned(segment)
	}
	return strings.Join(segments, "/")
}

func escapeSigned(s string) string {
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

// canonicalQuery is query as Signature Version 4 signs it, sorted by key,
// keys and values escaped.
func canonicalQuery(query url.Values) string {
	var pairs []string
	for key, values := range query {
		for _, value := range values {
			pairs = append(pairs, escapeSigned(key)+"="+escapeSigned(value))
		}
	}
	slices.Sort(pairs)
	return strings.Join(pairs, "&")
}

// server names S3 destinations in errors.
const s3Server = "The S3 storage"

func (s *s3Remote) put(ctx context.Context, name string, body io.ReadSeeker, size int64) error {
	sum := sha256.New()
	if _, err := io.Copy(sum, body); err != nil {
		return err
	}
	if _, err := body.Seek(0, io.SeekStart); err != nil {
		return err
	}
	response, err := s.do(ctx, http.MethodPut, s.url(s.key(name), nil), io.NopCloser(body), size, hex.EncodeToString(sum.Sum(nil)))
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return httpError(s3Server, "the upload of "+name, response)
	}
	return nil
}

// listResult is the answer of ListObjectsV2.
type listResult struct {
	Contents []struct {
		Key  string `xml:"Key"`
		Size int64  `xml:"Size"`
	} `xml:"Contents"`
	IsTruncated           bool   `xml:"IsTruncated"`
	NextContinuationToken string `xml:"NextContinuationToken"`
}

// maxListPages bounds the pages of a listing: a million objects at
// 1,000 a page, far more than a prefix holding backups has.
const maxListPages = 1000

// errListing reports a storage whose listing does not end: it gives a
// continuation token it gave before.
var errListing = errors.New("the S3 storage lists the same objects again and again")

func (s *s3Remote) list(ctx context.Context) ([]remoteFile, error) {
	prefix := ""
	if s.prefix != "" {
		prefix = s.prefix + "/"
	}
	var files []remoteFile
	// A key listed again, as a storage may, is kept once. Pages may hold
	// fewer keys than asked, or none: a token given twice, or too many
	// pages, end a listing that loops.
	seen := map[string]bool{}
	tokens := map[string]bool{}
	token := ""
	for range maxListPages {
		query := url.Values{"list-type": {"2"}, "prefix": {prefix}, "delimiter": {"/"}}
		if token != "" {
			query.Set("continuation-token", token)
		}
		response, err := s.do(ctx, http.MethodGet, s.url("", query), nil, 0, emptyHash)
		if err != nil {
			return nil, err
		}
		if response.StatusCode != http.StatusOK {
			err := httpError(s3Server, "the list of the backups", response)
			response.Body.Close()
			return nil, err
		}
		var result listResult
		err = xml.NewDecoder(io.LimitReader(response.Body, 64<<20)).Decode(&result)
		response.Body.Close()
		if err != nil {
			return nil, err
		}
		for _, object := range result.Contents {
			if seen[object.Key] {
				continue
			}
			seen[object.Key] = true
			if name, ok := strings.CutPrefix(object.Key, prefix); ok && name != "" && !strings.Contains(name, "/") {
				files = append(files, remoteFile{name: name, size: object.Size})
			}
		}
		if !result.IsTruncated || result.NextContinuationToken == "" {
			return files, nil
		}
		if tokens[result.NextContinuationToken] {
			return nil, errListing
		}
		tokens[result.NextContinuationToken] = true
		token = result.NextContinuationToken
	}
	return nil, fmt.Errorf("the S3 storage lists more than %d pages under the prefix", maxListPages)
}

func (s *s3Remote) get(ctx context.Context, name string, w io.Writer) error {
	response, err := s.do(ctx, http.MethodGet, s.url(s.key(name), nil), nil, 0, emptyHash)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return errNotFound
	}
	if response.StatusCode != http.StatusOK {
		return httpError(s3Server, "the download of "+name, response)
	}
	_, err = io.Copy(w, response.Body)
	return err
}

func (s *s3Remote) remove(ctx context.Context, name string) error {
	response, err := s.do(ctx, http.MethodDelete, s.url(s.key(name), nil), nil, 0, emptyHash)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent && response.StatusCode != http.StatusOK && response.StatusCode != http.StatusNotFound {
		return httpError(s3Server, "the deletion of "+name, response)
	}
	return nil
}

// xmlErrorText reads the code and message of an S3 error answer, empty
// for another answer.
func xmlErrorText(body []byte) (code, message string) {
	var answer struct {
		XMLName xml.Name `xml:"Error"`
		Code    string   `xml:"Code"`
		Message string   `xml:"Message"`
	}
	if xml.Unmarshal(body, &answer) != nil {
		return "", ""
	}
	return answer.Code, answer.Message
}
