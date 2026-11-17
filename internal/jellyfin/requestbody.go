package jellyfin

import (
	"encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"reflect"

	"github.com/moodiness/polyfin/internal/playback"
)

// bodyOptions read a request body as Jellyfin's JSON settings do
// (Jellyfin.Extensions.Json.JsonDefaults). Over encoding/json's own rules,
// which already match property names in any letter case and keep the last
// of duplicates, a string property also takes a number or a boolean, as
// its JSON text (JsonStringConverter), and a number property a string
// holding a number (JsonNumberHandling.AllowReadingFromString), a nullable
// one an empty string as null (JsonNullableStructConverter). Booleans stay
// strict, as in Jellyfin. Apps differ in which form they send: a device
// profile's MaxAudioChannels comes as "2" from jellyfin-web and as 2 from
// an app that sends numbers. The hand-written readers, such as
// parseDisplayPreferences, follow the same rules (see jsonString). The
// lists of a device profile are read to their bounds (see boundedList).
var bodyOptions = jsonv2.JoinOptions(json.DefaultOptionsV1(), jsonv2.WithUnmarshalers(jsonv2.JoinUnmarshalers(
	jsonv2.UnmarshalFromFunc(stringFromScalar),
	numberFromString[int](), numberFromString[int8](), numberFromString[int16](), numberFromString[int32](), numberFromString[int64](),
	numberFromString[uint](), numberFromString[uint8](), numberFromString[uint16](), numberFromString[uint32](), numberFromString[uint64](),
	numberFromString[float32](), numberFromString[float64](),
	boundedList[playback.DirectPlayProfile](playback.MaxProfileEntries), boundedList[playback.TranscodingProfile](playback.MaxProfileEntries),
	boundedList[playback.ContainerProfile](playback.MaxProfileEntries), boundedList[playback.CodecProfile](playback.MaxProfileEntries),
	boundedList[playback.SubtitleProfile](playback.MaxProfileEntries), boundedList[playback.ProfileCondition](playback.MaxProfileConditions),
)))

// unmarshalBody reads a JSON request body into into with bodyOptions.
func unmarshalBody(data []byte, into any) error {
	return jsonv2.Unmarshal(data, into, bodyOptions)
}

// stringFromScalar reads a number or a boolean into a string as its JSON
// text, as JsonStringConverter does; other tokens are read as usual.
func stringFromScalar(decoder *jsontext.Decoder, into *string) error {
	switch decoder.PeekKind() {
	case jsontext.KindNumber, jsontext.KindTrue, jsontext.KindFalse:
		value, err := decoder.ReadValue()
		if err != nil {
			return err
		}
		*into = string(value)
		return nil
	}
	return errors.ErrUnsupported
}

// boundedList reads a JSON array into a list of its first bound entries
// and one more, which tells the array is longer than bound (see
// playback.DeviceProfile.Bounded): the entries past it are skipped, so
// that a long array of small values in a request body takes no more
// memory than its bound. Other tokens are read as usual.
func boundedList[T any](bound int) *jsonv2.Unmarshalers {
	return jsonv2.UnmarshalFromFunc(func(decoder *jsontext.Decoder, into *[]T) error {
		if decoder.PeekKind() != jsontext.KindBeginArray {
			return errors.ErrUnsupported
		}
		if _, err := decoder.ReadToken(); err != nil {
			return err
		}
		// An empty array stays an empty list, not none, as it would read.
		list := (*into)[:0]
		if list == nil {
			list = []T{}
		}
		for decoder.PeekKind() != jsontext.KindEndArray {
			if len(list) > bound {
				if err := decoder.SkipValue(); err != nil {
					return err
				}
				continue
			}
			var entry T
			if err := jsonv2.UnmarshalDecode(decoder, &entry); err != nil {
				return err
			}
			list = append(list, entry)
		}
		if _, err := decoder.ReadToken(); err != nil {
			return err
		}
		*into = list
		return nil
	})
}

// number is a Go number a request body may hold.
type number interface {
	int | int8 | int16 | int32 | int64 | uint | uint8 | uint16 | uint32 | uint64 | float32 | float64
}

// numberFromString reads a string holding a JSON number, without spaces,
// into a number, as AllowReadingFromString does, and an empty one into a
// pointer to a number as null; other tokens are read as usual.
func numberFromString[T number]() *jsonv2.Unmarshalers {
	return jsonv2.JoinUnmarshalers(
		jsonv2.UnmarshalFromFunc(func(decoder *jsontext.Decoder, into **T) error {
			if decoder.PeekKind() != jsontext.KindString {
				return errors.ErrUnsupported
			}
			token, err := decoder.ReadToken()
			if err != nil {
				return err
			}
			if token.String() == "" {
				*into = nil
				return nil
			}
			value := new(T)
			if err := quotedNumber(token.String(), value); err != nil {
				return err
			}
			*into = value
			return nil
		}),
		jsonv2.UnmarshalFromFunc(func(decoder *jsontext.Decoder, into *T) error {
			if decoder.PeekKind() != jsontext.KindString {
				return errors.ErrUnsupported
			}
			token, err := decoder.ReadToken()
			if err != nil {
				return err
			}
			return quotedNumber(token.String(), into)
		}),
	)
}

// quotedNumber reads text, a JSON string's content, into a number.
func quotedNumber[T number](text string, into *T) error {
	digit := func(c byte) bool { return '0' <= c && c <= '9' }
	if text == "" || text[0] != '-' && !digit(text[0]) || !digit(text[len(text)-1]) || !json.Valid([]byte(text)) {
		return &json.UnmarshalTypeError{Value: "string", Type: reflect.TypeFor[T]()}
	}
	return json.Unmarshal([]byte(text), into)
}
