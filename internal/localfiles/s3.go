package localfiles

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/sigv4"
	"github.com/moodiness/polyfin/internal/source"
)

// defaultRegion is the region an S3 bucket's requests are signed for when
// its folder names none, which most S3-compatible storages accept.
const defaultRegion = "us-east-1"

// maxListPages bounds the pages of one folder's listing of a bucket: ten
// million objects at 1,000 a page.
const maxListPages = 10000

// s3Share reads a prefix of an S3 bucket, its objects listed as folders
// by "/", through connections kept alive between requests, its requests
// signed with Signature Version 4, or sent unsigned without an access key.
type s3Share struct {
	endpoint *url.URL
	bucket   string
	// prefix is the address's path, without its slashes.
	prefix string
	key    sigv4.Key
	// pathStyle tells that the bucket is named in the path of the
	// requests, not in their host (see usesPathStyle).
	pathStyle bool
	client    *http.Client
	now       func() time.Time
}

func newS3(u *url.URL, access shareAccess) (*s3Share, error) {
	endpoint, err := url.Parse(access.endpoint)
	if err != nil || endpoint.Host == "" {
		return nil, ErrInvalidEndpoint
	}
	region := access.region
	if region == "" {
		region = defaultRegion
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = 8
	transport.ResponseHeaderTimeout = 30 * time.Second
	return &s3Share{endpoint: endpoint, bucket: u.Host, prefix: strings.Trim(u.Path, "/"),
		key: sigv4.Key{Access: access.user, Secret: access.password, Region: region}, pathStyle: usesPathStyle(endpoint, u.Host),
		client: &http.Client{Transport: transport}, now: time.Now}, nil
}

// usesPathStyle tells whether a bucket is reached with its name in the
// path of the endpoint's address, which every S3-compatible storage
// answers, rather than in its host: for every endpoint but Amazon's, and
// for a bucket of Amazon's whose name is no host name of its own, with a
// dot or a capital.
func usesPathStyle(endpoint *url.URL, bucket string) bool {
	host := strings.ToLower(endpoint.Hostname())
	if net.ParseIP(host) != nil || !strings.HasSuffix(host, ".amazonaws.com") {
		return true
	}
	return strings.Contains(bucket, ".") || strings.ToLower(bucket) != bucket
}

func (c *s3Share) close() { c.client.CloseIdleConnections() }

// objectKey is the key of a file or folder of the prefix.
func (c *s3Share) objectKey(rel string) string {
	switch {
	case c.prefix == "":
		return rel
	case rel == "":
		return c.prefix
	}
	return c.prefix + "/" + rel
}

// url is the address of an object, or of the bucket for an empty key,
// with a query, escaped as Signature Version 4 signs it.
func (c *s3Share) url(key string, query url.Values) *url.URL {
	u := *c.endpoint
	base := strings.TrimSuffix(u.Path, "/")
	if c.pathStyle {
		base += "/" + c.bucket
	} else {
		u.Host = c.bucket + "." + u.Host
	}
	u.Path = base + "/" + key
	u.RawPath = sigv4.EscapePath(u.Path)
	u.RawQuery = sigv4.CanonicalQuery(query)
	return &u
}

// send signs and sends a request without a body. Without an access key,
// the bucket is read as everyone may, unsigned. A range some storages
// answer with 200 OK and its Content-Range, as rclone serve s3 does, is
// the part asked: it is answered as the 206 it is, so that neither the
// source cache nor a file read at an offset takes it for the whole object.
func (c *s3Share) send(ctx context.Context, method string, u *url.URL, header http.Header) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, method, u.String(), nil)
	if err != nil {
		return nil, err
	}
	for name, values := range header {
		request.Header[name] = values
	}
	if c.key.Access != "" {
		c.key.Sign(request, sigv4.EmptyHash, c.now())
	}
	response, err := c.client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: %v", errShareUnreachable, err)
	}
	if response.StatusCode == http.StatusOK && request.Header.Get("Range") != "" && response.Header.Get("Content-Range") != "" {
		response.StatusCode, response.Status = http.StatusPartialContent, "206 Partial Content"
	}
	return response, nil
}

// Open sends the source cache's requests of the bucket's objects, signed
// (see source.Location.Opener).
func (c *s3Share) Open(ctx context.Context, method, target string, header http.Header, _ bool) (*http.Response, error) {
	u, err := url.Parse(target)
	if err != nil {
		return nil, err
	}
	return c.send(ctx, method, u, header)
}

// s3Failure classes an answer of the storage that is not the one asked
// for, by the code its body tells (see shareFailure): a key it does not
// know or a wrong secret key is refused, a bucket or object it does not
// have missing.
func s3Failure(response *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	var answer struct {
		Code string `xml:"Code"`
	}
	_ = xml.Unmarshal(body, &answer)
	switch answer.Code {
	case "InvalidAccessKeyId", "SignatureDoesNotMatch", "AuthorizationHeaderMalformed", "InvalidToken", "ExpiredToken":
		return errShareRefused
	case "AccessDenied", "AllAccessDisabled":
		return fs.ErrPermission
	case "NoSuchBucket", "NoSuchKey":
		return fs.ErrNotExist
	}
	return statusError(response.StatusCode)
}

// s3Listing is a page of ListObjectsV2's answer.
type s3Listing struct {
	Contents []struct {
		Key          string `xml:"Key"`
		Size         int64  `xml:"Size"`
		LastModified string `xml:"LastModified"`
	} `xml:"Contents"`
	CommonPrefixes []struct {
		Prefix string `xml:"Prefix"`
	} `xml:"CommonPrefixes"`
	IsTruncated           bool   `xml:"IsTruncated"`
	NextContinuationToken string `xml:"NextContinuationToken"`
}

func (c *s3Share) list(ctx context.Context, rel string) ([]remoteEntry, error) {
	prefix := c.objectKey(rel)
	if prefix != "" {
		prefix += "/"
	}
	var entries []remoteEntry
	seen := map[string]bool{}
	tokens := map[string]bool{}
	token := ""
	for range maxListPages {
		query := url.Values{"list-type": {"2"}, "prefix": {prefix}, "delimiter": {"/"}}
		if token != "" {
			query.Set("continuation-token", token)
		}
		response, err := c.send(ctx, http.MethodGet, c.url("", query), nil)
		if err != nil {
			return nil, err
		}
		if response.StatusCode != http.StatusOK {
			err := s3Failure(response)
			response.Body.Close()
			return nil, err
		}
		var page s3Listing
		err = xml.NewDecoder(io.LimitReader(response.Body, 64<<20)).Decode(&page)
		response.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("%w: %v", errShareUnreachable, err)
		}
		// An entry listed again, as a storage may, is kept once.
		for _, folder := range page.CommonPrefixes {
			name, ok := strings.CutPrefix(folder.Prefix, prefix)
			name = strings.TrimSuffix(name, "/")
			if ok && name != "" && !strings.Contains(name, "/") && !seen[name+"/"] {
				seen[name+"/"] = true
				entries = append(entries, remoteEntry{name: name, folder: true})
			}
		}
		for _, object := range page.Contents {
			// The empty object some tools make for a folder is no file.
			name, ok := strings.CutPrefix(object.Key, prefix)
			if ok && name != "" && !strings.Contains(name, "/") && !seen[name] {
				seen[name] = true
				modified, _ := time.Parse(time.RFC3339Nano, object.LastModified)
				entries = append(entries, remoteEntry{name: name, size: object.Size, modified: modified})
			}
		}
		if !page.IsTruncated || page.NextContinuationToken == "" {
			if len(entries) == 0 && rel == "" && c.prefix != "" {
				return nil, c.notFolder(ctx)
			}
			return entries, nil
		}
		if tokens[page.NextContinuationToken] {
			return nil, fmt.Errorf("%w: the storage lists the same objects again", errShareUnreachable)
		}
		tokens[page.NextContinuationToken] = true
		token = page.NextContinuationToken
	}
	return nil, fmt.Errorf("%w: more than %d pages in one folder", errShareUnreachable, maxListPages)
}

// notFolder tells why the prefix lists nothing: errIsFile when it names
// an object, nil for an empty folder, which a bucket cannot tell from a
// folder that does not exist.
func (c *s3Share) notFolder(ctx context.Context) error {
	response, err := c.send(ctx, http.MethodHead, c.url(c.prefix, nil), nil)
	if err != nil {
		return err
	}
	response.Body.Close()
	if response.StatusCode == http.StatusOK {
		return errIsFile
	}
	return nil
}

func (c *s3Share) open(ctx context.Context, rel string) (io.ReadSeekCloser, time.Time, error) {
	u := c.url(c.objectKey(rel), nil)
	response, err := c.send(ctx, http.MethodHead, u, nil)
	if err != nil {
		return nil, time.Time{}, err
	}
	defer response.Body.Close()
	switch {
	case response.StatusCode == http.StatusNotFound:
		// A HEAD answers without a body to tell why.
		return nil, time.Time{}, fs.ErrNotExist
	case response.StatusCode != http.StatusOK:
		return nil, time.Time{}, s3Failure(response)
	case response.ContentLength < 0:
		return nil, time.Time{}, fmt.Errorf("%w: no length", errShareUnreachable)
	}
	modified, _ := http.ParseTime(response.Header.Get("Last-Modified"))
	return &httpFile{send: c.send, ctx: ctx, address: u, size: response.ContentLength}, modified, nil
}

func (c *s3Share) location(rel string) source.Location {
	return source.Location{URL: c.url(c.objectKey(rel), nil).String(), Opener: c}
}
