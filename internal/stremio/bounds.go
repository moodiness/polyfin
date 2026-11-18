package stremio

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"reflect"
)

// What Polyfin reads of the lists of an addon's answer.
//
// An answer is bounded in bytes (maxResponseBytes), which bounds its texts
// but not its lists: an element written in three bytes takes hundreds once
// decoded. So the lists of an answer are decoded one element at a time
// (see decodeList), each up to a number of elements, and all of them
// together up to a size in memory, whatever their elements hold (see
// room). A list is cut there, never refused: the titles, episodes or
// streams read until then are still shown. The numbers are well above what
// addons answer, a hundred titles a page or so, and no lower than what the
// bytes of an answer hold of real titles.
const (
	// MaxListed is how many streams, or subtitles, are read of an answer.
	MaxListed = 5000
	// MaxMetas is how many titles are read of a catalog page, and
	// MaxVideos how many videos of a description: a series' episodes, or
	// a channel's programmes.
	MaxMetas  = 50000
	MaxVideos = 50000
	// maxNested is how many elements are read of any other list, such as
	// a title's trailers and cast or a stream's trackers.
	maxNested = 2000
	// maxCollections is how deep collections within a collection are
	// read; the library follows them less deep.
	maxCollections = 4
	// listedRoom is the room of an answer that lists streams or subtitles,
	// and metaRoom that of a catalog page or a description, whose titles
	// and videos take more.
	listedRoom = 8 << 20
	metaRoom   = 32 << 20
)

// room is what the lists of one answer may still take in memory, in
// bytes: each element kept, of any list however deep, takes its size, and
// a list is cut once none is left. A bound on each list would not bound
// lists within lists. collections counts the collections being read
// within one another (see decodeCollection).
type room struct {
	left        int
	collections int
}

// take takes size bytes of the room, and tells whether it had them; a nil
// room has room for anything.
func (r *room) take(size int) bool {
	if r != nil && r.left < size {
		return false
	}
	if r != nil {
		r.left -= size
	}
	return true
}

// decodeList decodes the JSON array data one element at a time with
// decode, as encoding/json decodes a list: null is no list, and an
// element of another type than read is kept as far as it was decoded, the
// first such error being returned with the list. Unlike it, only the
// first most elements are read, the elements holds tells empty are left
// out, unless holds is nil, and the list is cut where within has no room
// left for an element: an element takes its room before its own lists do.
func decodeList[T any](data []byte, most int, within *room, decode func(*json.Decoder, *T, *room) error, holds func(*T) bool) ([]T, error) {
	if len(data) == 0 {
		return nil, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	switch opening, err := decoder.Token(); {
	case err != nil:
		return nil, err
	case opening == nil:
		return nil, nil
	case opening != json.Delim('['):
		// As encoding/json names the value it found.
		found := "number"
		switch opening.(type) {
		case json.Delim:
			found = "object"
		case string:
			found = "string"
		case bool:
			found = "bool"
		}
		return nil, &json.UnmarshalTypeError{Value: found, Type: reflect.TypeFor[[]T]()}
	}
	list := []T{}
	size := int(reflect.TypeFor[T]().Size())
	var mismatch error
	for read := 0; read < most && decoder.More() && within.take(size); read++ {
		var element T
		if err := decode(decoder, &element, within); err != nil {
			if _, mismatched := errors.AsType[*json.UnmarshalTypeError](err); !mismatched {
				return list, err
			}
			mismatch = cmp.Or(mismatch, err)
		}
		if holds != nil && !holds(&element) {
			// Its room is given back.
			within.take(-size)
			continue
		}
		list = append(list, element)
	}
	return list, mismatch
}

// whole decodes an element that has no list of its own to bound.
func whole[T any](decoder *json.Decoder, element *T, _ *room) error {
	return decoder.Decode(element)
}

// nested decodes a list of an element whose elements have no list of
// their own, up to maxNested of them, and keeps in err the first error of
// the element's lists.
func nested[T any](data json.RawMessage, within *room, err *error) []T {
	list, failed := decodeList(data, maxNested, within, whole[T], nil)
	*err = cmp.Or(*err, failed)
	return list
}

// listedStream is a stream as an addon writes it. Its lists are read
// apart, within the bounds of its answer (see decodeStream): each field
// here is shallower than Stream's of the same name, so it takes the value.
type listedStream struct {
	Stream
	Sources   json.RawMessage `json:"sources"`
	Subtitles json.RawMessage `json:"subtitles"`
	RARs      json.RawMessage `json:"rarUrls"`
	Zips      json.RawMessage `json:"zipUrls"`
	SevenZip  json.RawMessage `json:"7zipUrls"`
	TGZs      json.RawMessage `json:"tgzUrls"`
	Tars      json.RawMessage `json:"tarUrls"`
}

// decodeStream decodes a stream an addon lists, its own lists within the
// bounds of the answer.
func decodeStream(decoder *json.Decoder, stream *Stream, within *room) error {
	var listed listedStream
	err := decoder.Decode(&listed)
	*stream = listed.Stream
	stream.Sources = nested[string](listed.Sources, within, &err)
	stream.Subtitles = nested[Subtitle](listed.Subtitles, within, &err)
	stream.RARs = nested[json.RawMessage](listed.RARs, within, &err)
	stream.Zips = nested[json.RawMessage](listed.Zips, within, &err)
	stream.SevenZip = nested[json.RawMessage](listed.SevenZip, within, &err)
	stream.TGZs = nested[json.RawMessage](listed.TGZs, within, &err)
	stream.Tars = nested[json.RawMessage](listed.Tars, within, &err)
	return err
}

// tells reports whether a stream an addon lists tells anything: something
// to play, or what Polyfin reads of a stream it does not play, a notice's
// text, a link or a file counted by its shape, a file's name or size. One
// that tells nothing, as an empty object, is left out of its list.
func tells(s *Stream) bool {
	return s.URL != "" || s.InfoHash != "" || s.Name != "" || s.Description != "" || s.Title != "" || s.ExternalURL != "" ||
		s.YouTubeID != "" || s.Usenet != "" || s.Archive() || s.BehaviorHints.Filename != "" || s.BehaviorHints.VideoSize != 0
}

// downloadable reports a subtitle that can be downloaded: no other is
// ever listed, so no other is kept.
func downloadable(s *Subtitle) bool { return s.Valid() }

// decodeVideo decodes a video of a description, its own streams within
// the bounds of the answer.
func decodeVideo(decoder *json.Decoder, video *Video, within *room) error {
	var listed struct {
		Video
		// Shallower than Video's field of the same name.
		Streams json.RawMessage `json:"streams"`
	}
	err := decoder.Decode(&listed)
	*video = listed.Video
	video.Streams, _ = decodeList(listed.Streams, maxNested, within, decodeStream, tells)
	return err
}

// UnmarshalJSON decodes a meta that is an answer by itself, as the meta
// resource gives it, within the bounds of an answer (see decode).
func (d *described) UnmarshalJSON(data []byte) error {
	return d.decode(json.NewDecoder(bytes.NewReader(data)), &room{left: metaRoom})
}

// decode decodes a meta an addon sends, as unmarshal does, then its
// videos, trailers, collection and extras within the bounds of the answer,
// of which within is the room left. A list of another type than read, or
// with such an element, never loses the title.
func (d *described) decode(decoder *json.Decoder, within *room) error {
	// Without described's methods, lest UnmarshalJSON be called again.
	type plain described
	err := decoder.Decode((*plain)(d))
	if _, mismatched := errors.AsType[*json.UnmarshalTypeError](err); mismatched {
		err = nil
	}
	d.Meta.Videos, _ = decodeList(d.Videos, MaxVideos, within, decodeVideo, nil)
	d.Meta.Trailers, _ = decodeList(d.Trailers, maxNested, within, whole[Trailer], nil)
	d.Meta.Collection = decodeCollection(d.Collection, within)
	if present(d.Extras) {
		d.Meta.Extras = &Extras{}
		_ = d.Meta.Extras.decode(d.Extras, within)
	}
	d.Videos, d.Trailers, d.Collection, d.Extras = nil, nil, nil, nil
	return err
}

// present reports whether an element gives a value, other than null.
func present(data json.RawMessage) bool {
	return len(data) > 0 && string(data) != "null"
}

// decodeCollection decodes what a collection groups, within the bounds of
// the answer; nil for none, or for one deeper than maxCollections within
// others.
func decodeCollection(data json.RawMessage, within *room) *Collection {
	if !present(data) || within.collections >= maxCollections {
		return nil
	}
	var listed struct {
		Items   json.RawMessage `json:"items"`
		Sources json.RawMessage `json:"sources"`
	}
	_ = json.Unmarshal(data, &listed)
	within.collections++
	defer func() { within.collections-- }()
	collection := &Collection{}
	collection.Items, _ = decodeList(listed.Items, maxNested, within, decodeItem, nil)
	collection.Sources, _ = decodeList(listed.Sources, maxNested, within, whole[CollectionSource], nil)
	return collection
}

// decodeItem decodes a collection another one groups: a preview, as its
// addon writes it.
func decodeItem(decoder *json.Decoder, item *Meta, within *room) error {
	var decoded described
	err := decoded.decode(decoder, within)
	*item = decoded.Meta
	return err
}

// pageMeta decodes a meta of a catalog page, as decodeMeta does, its
// lists within the bounds of the page.
func pageMeta(decoder *json.Decoder, meta *Meta, within *room) error {
	var decoded described
	err := decoded.decode(decoder, within)
	*meta = decoded.meta()
	return err
}
