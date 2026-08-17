package token

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

func encodeRawURL(raw []byte) string {
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeRawURL(seg string, max int) ([]byte, error) {
	if max <= 0 || seg == "" || len(seg) > base64.RawURLEncoding.EncodedLen(max) {
		return nil, denyInvalid()
	}
	if strings.ContainsAny(seg, "+/=") {
		return nil, denyInvalid()
	}
	decoded, err := base64.RawURLEncoding.DecodeString(seg)
	if err != nil || len(decoded) == 0 || len(decoded) > max {
		return nil, denyInvalid()
	}
	if encodeRawURL(decoded) != seg {
		return nil, denyInvalid()
	}
	return decoded, nil
}

func splitCompact(raw string) (header, payload, signature string, err error) {
	if raw == "" || len(raw) > MaxTokenBytes {
		return "", "", "", denyInvalid()
	}
	if strings.Count(raw, ".") != 2 {
		return "", "", "", denyInvalid()
	}
	header, rest, _ := strings.Cut(raw, ".")
	payload, signature, ok := strings.Cut(rest, ".")
	if !ok || header == "" || payload == "" || signature == "" {
		return "", "", "", denyInvalid()
	}
	if len(header) > maxSegmentBytes || len(payload) > maxSegmentBytes || len(signature) > maxSegmentBytes {
		return "", "", "", denyInvalid()
	}
	return header, payload, signature, nil
}

func decodeStrictJSONObject(raw []byte, allowed map[string]struct{}, maxBytes int) (map[string]json.RawMessage, error) {
	if len(raw) == 0 || len(raw) > maxBytes {
		return nil, denyInvalid()
	}
	if err := prevalidateJSON(raw); err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	tok, err := dec.Token()
	if err != nil {
		return nil, denyInvalid()
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		return nil, denyInvalid()
	}
	out := make(map[string]json.RawMessage, len(allowed))
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, denyInvalid()
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, denyInvalid()
		}
		if _, known := allowed[key]; !known {
			return nil, denyInvalid()
		}
		if _, dup := out[key]; dup {
			return nil, denyInvalid()
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, denyInvalid()
		}
		if err := prevalidateJSON(value); err != nil {
			return nil, err
		}
		out[key] = value
	}
	end, err := dec.Token()
	if err != nil {
		return nil, denyInvalid()
	}
	if endDelim, ok := end.(json.Delim); !ok || endDelim != '}' {
		return nil, denyInvalid()
	}
	if err := requireExactEOF(dec, raw); err != nil {
		return nil, err
	}
	return out, nil
}

func decodeJSONString(raw json.RawMessage) (string, error) {
	if err := prevalidateJSON(raw); err != nil {
		return "", err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return "", denyInvalid()
	}
	s, ok := tok.(string)
	if !ok {
		return "", denyInvalid()
	}
	if err := requireExactEOF(dec, raw); err != nil {
		return "", err
	}
	if !utf8.ValidString(s) || strings.ContainsRune(s, '\uFFFD') {
		return "", denyInvalid()
	}
	return s, nil
}

func decodeJSONInt(raw json.RawMessage) (int64, error) {
	if err := prevalidateJSON(raw); err != nil {
		return 0, denyInvalid()
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	tok, err := dec.Token()
	if err != nil {
		return 0, denyInvalid()
	}
	num, ok := tok.(json.Number)
	if !ok {
		return 0, denyInvalid()
	}
	text := num.String()
	if text == "" || strings.ContainsAny(text, ".eE+") {
		return 0, denyInvalid()
	}
	n, err := num.Int64()
	if err != nil {
		return 0, denyInvalid()
	}
	if err := requireExactEOF(dec, raw); err != nil {
		return 0, denyInvalid()
	}
	return n, nil
}

func requireExactEOF(dec *json.Decoder, raw []byte) error {
	if dec == nil || len(raw) == 0 {
		return denyInvalid()
	}
	if dec.More() {
		return denyInvalid()
	}
	if int(dec.InputOffset()) != len(raw) {
		return denyInvalid()
	}
	if _, err := dec.Token(); err != io.EOF {
		return denyInvalid()
	}
	return nil
}

func prevalidateJSON(raw []byte) error {
	if len(raw) == 0 || !utf8.Valid(raw) {
		return denyInvalid()
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] != '\\' {
			continue
		}
		if i+1 >= len(raw) {
			return denyInvalid()
		}
		if raw[i+1] != 'u' && raw[i+1] != 'U' {
			i++
			continue
		}
		if i+5 >= len(raw) {
			return denyInvalid()
		}
		unit, ok := parseJSONHexUnit(raw[i+2 : i+6])
		if !ok {
			return denyInvalid()
		}
		i += 5
		switch {
		case utf16.IsSurrogate(rune(unit)):
			if unit >= 0xDC00 {
				return denyInvalid()
			}
			if i+6 >= len(raw) || raw[i+1] != '\\' || (raw[i+2] != 'u' && raw[i+2] != 'U') {
				return denyInvalid()
			}
			low, ok := parseJSONHexUnit(raw[i+3 : i+7])
			if !ok || low < 0xDC00 || low > 0xDFFF {
				return denyInvalid()
			}
			if r := utf16.DecodeRune(rune(unit), rune(low)); r == utf8.RuneError {
				return denyInvalid()
			}
			i += 6
		case unit == 0xFFFD:
			return denyInvalid()
		}
	}
	return nil
}

func parseJSONHexUnit(raw []byte) (uint16, bool) {
	if len(raw) != 4 {
		return 0, false
	}
	var unit uint16
	for _, b := range raw {
		unit <<= 4
		switch {
		case b >= '0' && b <= '9':
			unit |= uint16(b - '0')
		case b >= 'a' && b <= 'f':
			unit |= uint16(b - 'a' + 10)
		case b >= 'A' && b <= 'F':
			unit |= uint16(b - 'A' + 10)
		default:
			return 0, false
		}
	}
	return unit, true
}

func writeJSONString(buf *bytes.Buffer, s string) error {
	encoded, err := json.Marshal(s)
	if err != nil {
		return denyInvalid()
	}
	buf.Write(encoded)
	return nil
}

func boundedUTF8(field, value string, maxBytes int) error {
	if value == "" || !utf8.ValidString(value) || len(value) > maxBytes {
		return denyInvalid()
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f || r == utf8.RuneError {
			return denyInvalid()
		}
	}
	_ = field
	return nil
}

func fmtInt(n int64) string {
	return fmt.Sprintf("%d", n)
}
