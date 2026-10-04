package iptv

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

const (
	// maxListSize bounds a download of a list, and maxChannels the
	// channels kept from it.
	maxListSize = 100 << 20
	maxChannels = 100_000
	// A list download must answer within listAnswer, send something at
	// least every listStall and end within listTimeout.
	listAnswer  = 30 * time.Second
	listStall   = time.Minute
	listTimeout = 5 * time.Minute
)

// fetch downloads an account's channel list. confined keeps the requests
// on public addresses. Errors never contain an address: they embed
// credentials.
func fetch(ctx context.Context, client *stremio.Client, account Account, confined bool) ([]Entry, error) {
	var entries []Entry
	add := func(e Entry) error {
		if len(entries) >= maxChannels {
			return ErrTooLarge
		}
		entries = append(entries, e)
		return nil
	}
	var err error
	switch account.Kind {
	case addons.KindM3U:
		err = download(ctx, client, account.URL, confined, func(body io.Reader) error { return ParseM3U(body, add) })
	case addons.KindXtream:
		err = fetchXtream(ctx, client, account, confined, add)
	default:
		err = ErrInvalidAddress
	}
	return entries, err
}

// download requests address and hands its body to read, bounded in size
// and time (see maxListSize and listTimeout).
func download(ctx context.Context, client *stremio.Client, address string, confined bool, read func(io.Reader) error) error {
	ctx, cancel := context.WithTimeout(ctx, listTimeout)
	defer cancel()
	answered := time.AfterFunc(listAnswer, cancel)
	response, err := client.Open(ctx, http.MethodGet, address, nil, confined)
	answered.Stop()
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: HTTP %d", stremio.ErrUnreachable, response.StatusCode)
	}
	stall := time.AfterFunc(listStall, cancel)
	defer stall.Stop()
	err = read(&bounded{r: response.Body, left: maxListSize, stall: stall})
	if ctx.Err() != nil && !errors.Is(err, ErrTooLarge) && !errors.Is(err, ErrInvalidList) {
		return fmt.Errorf("%w: %v", stremio.ErrUnreachable, ctx.Err())
	}
	return err
}

// bounded reads at most left bytes, then fails with ErrTooLarge; each read
// that brings something pushes the stall timer back.
type bounded struct {
	r     io.Reader
	left  int64
	stall *time.Timer
}

func (b *bounded) Read(p []byte) (int, error) {
	if b.left <= 0 {
		var probe [1]byte
		if n, _ := b.r.Read(probe[:]); n > 0 {
			return 0, ErrTooLarge
		}
		return 0, io.EOF
	}
	if int64(len(p)) > b.left {
		p = p[:b.left]
	}
	n, err := b.r.Read(p)
	b.left -= int64(n)
	if n > 0 {
		b.stall.Reset(listStall)
	}
	if err != nil && !errors.Is(err, io.EOF) {
		err = fmt.Errorf("%w: %v", stremio.ErrUnreachable, err)
	}
	return n, err
}

// text reads a JSON value Xtream servers send as a string or a number.
type text string

func (t *text) UnmarshalJSON(data []byte) error {
	var s string
	if json.Unmarshal(data, &s) == nil {
		*t = text(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(data, &n); err != nil {
		*t = ""
		return nil
	}
	*t = text(n.String())
	return nil
}

// fetchXtream lists an Xtream Codes account's live channels: the account's
// allowed formats, then the live categories and streams of the player API.
func fetchXtream(ctx context.Context, client *stremio.Client, account Account, confined bool, add func(Entry) error) error {
	api := func(action string) string {
		address, _ := account.address()
		if action != "" {
			address += "&action=" + action
		}
		return address
	}
	var login struct {
		UserInfo struct {
			Auth    text     `json:"auth"`
			Formats []string `json:"allowed_output_formats"`
		} `json:"user_info"`
	}
	if err := download(ctx, client, api(""), confined, decodeJSON(&login)); err != nil {
		return err
	}
	if login.UserInfo.Auth != "1" {
		return ErrLoginRefused
	}
	// MPEG-TS, unless the account only allows HLS.
	hls := len(login.UserInfo.Formats) > 0 && !slices.Contains(login.UserInfo.Formats, "ts") && slices.Contains(login.UserInfo.Formats, "m3u8")
	var categories []struct {
		ID   text   `json:"category_id"`
		Name string `json:"category_name"`
	}
	if err := download(ctx, client, api("get_live_categories"), confined, decodeJSON(&categories)); err != nil {
		return err
	}
	names := make(map[text]string, len(categories))
	for _, category := range categories {
		names[category.ID] = strings.TrimSpace(category.Name)
	}
	return download(ctx, client, api("get_live_streams"), confined, func(body io.Reader) error {
		decoder := json.NewDecoder(body)
		if token, err := decoder.Token(); err != nil || token != json.Delim('[') {
			return ErrInvalidList
		}
		for decoder.More() {
			var stream struct {
				Number   text   `json:"num"`
				Name     string `json:"name"`
				ID       text   `json:"stream_id"`
				Icon     string `json:"stream_icon"`
				Guide    string `json:"epg_channel_id"`
				Category text   `json:"category_id"`
			}
			if err := decoder.Decode(&stream); err != nil {
				return jsonError(err)
			}
			name := strings.TrimSpace(stream.Name)
			if stream.ID == "" || heading(name) {
				continue
			}
			number, _ := strconv.Atoi(string(stream.Number))
			if err := add(Entry{ID: string(stream.ID), Name: name, Number: max(number, 0), Logo: strings.TrimSpace(stream.Icon),
				Group: names[stream.Category], GuideID: strings.TrimSpace(stream.Guide), URL: account.streamURL(string(stream.ID), hls)}); err != nil {
				return err
			}
		}
		return nil
	})
}

func decodeJSON(into any) func(io.Reader) error {
	return func(body io.Reader) error { return jsonError(json.NewDecoder(body).Decode(into)) }
}

// jsonError reports a response that is not the JSON expected as an invalid
// list.
func jsonError(err error) error {
	var syntax *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &syntax) || errors.As(err, &typeErr) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		return ErrInvalidList
	}
	return err
}
