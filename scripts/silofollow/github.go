package main

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// maxWait is the longest the tool waits for GitHub's rate limit to reset.
const maxWait = 15 * time.Minute

// github reads GitHub's REST API, with the token of GITHUB_TOKEN or
// GH_TOKEN when one is set: without, GitHub allows 60 requests an hour,
// which a run fits in.
type github struct {
	client *http.Client
	token  string
}

func newGitHub() *github {
	return &github{client: &http.Client{Timeout: 2 * time.Minute}, token: cmp.Or(os.Getenv("GITHUB_TOKEN"), os.Getenv("GH_TOKEN"))}
}

// get reads an address of the API, waiting out GitHub's rate limits.
func (g *github) get(ctx context.Context, path, accept string) ([]byte, error) {
	for attempt := 1; ; attempt++ {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com"+path, nil)
		if err != nil {
			return nil, err
		}
		request.Header.Set("Accept", accept)
		request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		if g.token != "" {
			request.Header.Set("Authorization", "Bearer "+g.token)
		}
		response, err := g.client.Do(request)
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil {
			return nil, err
		}
		if response.StatusCode == http.StatusOK {
			return body, nil
		}
		wait, limited := rateLimited(response, time.Now())
		if !limited || attempt == 3 {
			var problem struct{ Message string }
			_ = json.Unmarshal(body, &problem)
			return nil, fmt.Errorf("GET %s: %s: %s", path, response.Status, problem.Message)
		}
		if wait > maxWait {
			return nil, fmt.Errorf("GET %s: GitHub's rate limit resets in %s; set GITHUB_TOKEN or GH_TOKEN", path, wait.Round(time.Second))
		}
		fmt.Fprintf(os.Stderr, "GitHub's rate limit: waiting %s\n", wait.Round(time.Second))
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// rateLimited reports whether an answer is GitHub's rate limit, and how
// long it asks to wait: its Retry-After, or until its limit resets.
func rateLimited(response *http.Response, now time.Time) (time.Duration, bool) {
	if response.StatusCode != http.StatusForbidden && response.StatusCode != http.StatusTooManyRequests {
		return 0, false
	}
	if seconds, err := strconv.Atoi(response.Header.Get("Retry-After")); err == nil {
		return time.Duration(seconds) * time.Second, true
	}
	if response.Header.Get("X-RateLimit-Remaining") == "0" {
		reset, err := strconv.ParseInt(response.Header.Get("X-RateLimit-Reset"), 10, 64)
		if err != nil {
			return time.Minute, true
		}
		return max(time.Unix(reset, 0).Sub(now), 0) + time.Second, true
	}
	if response.StatusCode == http.StatusTooManyRequests {
		return time.Minute, true
	}
	return 0, false
}

func escapePath(path string) string {
	segments := strings.Split(path, "/")
	for i, s := range segments {
		segments[i] = url.PathEscape(s)
	}
	return strings.Join(segments, "/")
}

// commit is the commit a reference of a repository names.
func (g *github) commit(ctx context.Context, repository, ref string) (string, error) {
	body, err := g.get(ctx, "/repos/"+repository+"/commits/"+url.PathEscape(ref), "application/vnd.github.sha")
	return strings.TrimSpace(string(body)), err
}

// file is a file of a repository at a commit.
func (g *github) file(ctx context.Context, repository, path, commit string) ([]byte, error) {
	return g.get(ctx, "/repos/"+repository+"/contents/"+escapePath(path)+"?ref="+url.QueryEscape(commit),
		"application/vnd.github.raw+json")
}

// comparison is what GitHub's compare API tells of two commits.
type comparison struct {
	HTMLURL      string         `json:"html_url"`
	TotalCommits int            `json:"total_commits"`
	Files        []comparedFile `json:"files"`
}

type comparedFile struct {
	Filename         string `json:"filename"`
	PreviousFilename string `json:"previous_filename"`
	Status           string `json:"status"`
	Additions        int    `json:"additions"`
	Deletions        int    `json:"deletions"`
}

// compareLimit is the most files the compare API lists.
const compareLimit = 300

func (g *github) compare(ctx context.Context, repository, base, head string) (comparison, error) {
	// A page of one commit: the files are listed on the first page.
	body, err := g.get(ctx, "/repos/"+repository+"/compare/"+base+"..."+head+"?per_page=1", "application/vnd.github+json")
	if err != nil {
		return comparison{}, err
	}
	var c comparison
	return c, json.Unmarshal(body, &c)
}

// tree is the files of a repository at a commit, with their blobs.
func (g *github) tree(ctx context.Context, repository, commit string) (map[string]string, error) {
	body, err := g.get(ctx, "/repos/"+repository+"/git/trees/"+commit+"?recursive=1", "application/vnd.github+json")
	if err != nil {
		return nil, err
	}
	var tree struct {
		Tree []struct {
			Path, Type, SHA string
		}
		Truncated bool
	}
	if err := json.Unmarshal(body, &tree); err != nil {
		return nil, err
	}
	if tree.Truncated {
		return nil, fmt.Errorf("the tree of %s at %s is too large for GitHub's API", repository, commit)
	}
	files := map[string]string{}
	for _, entry := range tree.Tree {
		if entry.Type == "blob" {
			files[entry.Path] = entry.SHA
		}
	}
	return files, nil
}
