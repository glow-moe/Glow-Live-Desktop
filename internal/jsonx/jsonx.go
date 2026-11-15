// Package jsonx reads the games' JSON without losing a whole frame to one
// field. When a value has the wrong type for its field (Practice Tool's target
// dummy sends {"error": ...} where a player's item list goes), encoding/json
// already fills every other field and only reports the first mismatch; these
// readers keep that result instead of throwing it away. Broken JSON (bad
// syntax, a truncated body) is still an error.
package jsonx

import (
	"encoding/json"
	"errors"
	"io"
)

// Unmarshal is json.Unmarshal that keeps going past a field of the wrong type.
func Unmarshal(b []byte, v any) error {
	return keep(json.Unmarshal(b, v))
}

// Decode reads one value from r the same way.
func Decode(r io.Reader, v any) error {
	return keep(json.NewDecoder(r).Decode(v))
}

func keep(err error) error {
	var te *json.UnmarshalTypeError
	if errors.As(err, &te) {
		return nil
	}
	return err
}
