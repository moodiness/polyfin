package jellyfin

import (
	"encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"reflect"
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
// parseDisplayPreferences, follow the same rules (see jsonString).
var bodyOptions = jsonv2.JoinOptions(json.DefaultOptionsV1(), jsonv2.WithUnmarshalers(jsonv2.JoinUnmarshalers(
	jsonv2.UnmarshalFromFunc(stringFromScalar),
	numberFromString[int](), numberFromString[int8](), numberFromString[int16](), numberFromString[int32](), numberFromString[int64](),
	numberFromString[uint](), numberFromString[uint8](), numberFromString[uint16](), numberFromString[uint32](), numberFromString[uint64](),
	numberFromString[float32](), numberFromString[float64](),
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
