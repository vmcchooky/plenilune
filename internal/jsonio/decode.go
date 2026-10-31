// Package jsonio decodes bounded, unambiguous JSON contracts.
package jsonio

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"unicode/utf8"
)

// ValidationError carries a bounded diagnostic supplied by a domain decoder.
// Arbitrary decoding errors may contain untrusted input and are sanitized.
type ValidationError struct{ message string }

func (e *ValidationError) Error() string { return e.message }

// Invalid is for fixed, non-sensitive domain messages, never raw input text.
func Invalid(message string) error { return &ValidationError{message: message} }

// Decode rejects duplicate keys, unknown fields, trailing values and deep input.
// Callers own their resource-specific semantic validation.
func Decode(data []byte, maxBytes int, dst any) error {
	if len(data) > maxBytes {
		return errors.New("JSON input exceeds size limit")
	}
	if !utf8.Valid(data) {
		return errors.New("JSON input must be UTF-8")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	if err := value(d, 0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("expected one JSON value")
	}
	if err := exactFields(data, reflect.TypeOf(dst)); err != nil {
		return err
	}
	d = json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		var domain *ValidationError
		if errors.As(err, &domain) {
			return domain
		}
		return errors.New("JSON does not match the expected fields or types")
	}
	return nil
}

// encoding/json accepts case-insensitive aliases for struct fields. Wire
// contracts use exact tag spelling so e.g. protocol/Protocol cannot overwrite.
func exactFields(data []byte, t reflect.Type) error {
	if t == nil {
		return errors.New("JSON destination is required")
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	trim := bytes.TrimSpace(data)
	if len(trim) == 0 || bytes.Equal(trim, []byte("null")) {
		return nil
	}
	switch t.Kind() {
	case reflect.Struct:
		if trim[0] != '{' {
			return nil
		} // TextUnmarshaler types use strings.
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(trim, &fields); err != nil {
			return errors.New("invalid JSON object")
		}
		allowed := make(map[string]reflect.Type)
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			name := strings.Split(f.Tag.Get("json"), ",")[0]
			if name == "-" {
				continue
			}
			if name == "" {
				name = f.Name
			}
			allowed[name] = f.Type
		}
		for key, raw := range fields {
			ft, ok := allowed[key]
			if !ok {
				return errors.New("unknown JSON field or incorrect field spelling")
			}
			if err := exactFields(raw, ft); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		if trim[0] != '[' {
			return nil
		}
		var items []json.RawMessage
		if err := json.Unmarshal(trim, &items); err != nil {
			return errors.New("invalid JSON array")
		}
		for _, raw := range items {
			if err := exactFields(raw, t.Elem()); err != nil {
				return err
			}
		}
	case reflect.Map:
		if trim[0] != '{' {
			return nil
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(trim, &fields); err != nil {
			return errors.New("invalid JSON map")
		}
		for _, raw := range fields {
			if err := exactFields(raw, t.Elem()); err != nil {
				return err
			}
		}
	}
	return nil
}

func value(d *json.Decoder, depth int) error {
	if depth > 64 {
		return errors.New("JSON nesting exceeds limit")
	}
	t, err := d.Token()
	if err != nil {
		return errors.New("invalid JSON")
	}
	delim, ok := t.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := make(map[string]bool)
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return errors.New("invalid JSON object")
			}
			s, ok := key.(string)
			if !ok {
				return errors.New("invalid JSON key")
			}
			if seen[s] {
				return errors.New("duplicate JSON key")
			}
			seen[s] = true
			if err := value(d, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err := value(d, depth+1); err != nil {
				return err
			}
		}
	default:
		return errors.New("unexpected JSON delimiter")
	}
	end, err := d.Token()
	if err != nil {
		return errors.New("unterminated JSON value")
	}
	if (delim == '{' && end != json.Delim('}')) || (delim == '[' && end != json.Delim(']')) {
		return fmt.Errorf("mismatched JSON delimiter")
	}
	return nil
}
