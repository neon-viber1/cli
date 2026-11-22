// Package jsonsanitize keeps literal escape text in JSON responses intact when
// the response is later passed through the go-gh JSON sanitizer.
//
// The go-gh sanitizer rewrites any six byte `\u00XX` sequence that maps to a C0
// or C1 control character, even when the backslash is itself escaped. The wire
// text `\\u001b` decodes to the printable characters `\u001b`, yet the
// sanitizer turns it into `\\^[`, changing the string value. This package
// re-encodes the escaped backslash of such literal text as `\u005c`, which
// decodes to the same value but is not matched by the sanitizer. Real escapes
// (an odd number of backslashes before `u`) and raw control characters are left
// for the go-gh sanitizer to neutralize, so terminal protection is unchanged.
package jsonsanitize

import (
	"io"
	"net/http"
	"regexp"

	"golang.org/x/text/transform"
)

// literalWindow is the length of `\\u00XX`, the longest sequence inspected.
const literalWindow = 7

// ProtectLiteralEscapes is a transform.Transformer that rewrites a literal
// (escaped-backslash) control escape such as `\\u001b` to `\u005cu001b`.
type ProtectLiteralEscapes struct{}

// Reset implements transform.Transformer. The transformer keeps no state.
func (ProtectLiteralEscapes) Reset() {}

// Transform implements transform.Transformer. It walks escape pairs so that
// backslash parity is decided without state carried between calls.
func (ProtectLiteralEscapes) Transform(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	for nSrc < len(src) {
		b := src[nSrc]
		if b != '\\' {
			if nDst >= len(dst) {
				return nDst, nSrc, transform.ErrShortDst
			}
			dst[nDst] = b
			nDst++
			nSrc++
			continue
		}
		rest := src[nSrc:]
		if len(rest) < literalWindow && !atEOF {
			return nDst, nSrc, transform.ErrShortSrc
		}
		if len(rest) >= 2 && rest[1] == '\\' {
			// An escaped backslash pair. If it is followed by u00XX naming a
			// control character, the text is literal and must be protected.
			if len(rest) >= literalWindow && isControlEscape(rest[2:literalWindow]) {
				const repl = `\u005c`
				if len(dst)-nDst < len(repl) {
					return nDst, nSrc, transform.ErrShortDst
				}
				nDst += copy(dst[nDst:], repl)
				nSrc += 2
				continue
			}
		}
		// Any other escape is copied as a pair. A real `\u` escape is left to
		// the go-gh sanitizer.
		n := 2
		if len(rest) < 2 {
			n = 1
		}
		if len(dst)-nDst < n {
			return nDst, nSrc, transform.ErrShortDst
		}
		nDst += copy(dst[nDst:], rest[:n])
		nSrc += n
	}
	return nDst, nSrc, nil
}

// isControlEscape reports whether b is `u00XX` where XX names a C0 or C1
// control character that the go-gh sanitizer replaces (tab, newline, vertical
// tab and carriage return are not replaced).
func isControlEscape(b []byte) bool {
	if len(b) != 5 {
		return false
	}
	if b[0] != 'u' || b[1] != '0' || b[2] != '0' {
		return false
	}
	hi, ok1 := hexVal(b[3])
	lo, ok2 := hexVal(b[4])
	if !ok1 || !ok2 {
		return false
	}
	v := hi<<4 | lo
	switch {
	case v <= 0x1f:
		return v != 0x09 && v != 0x0a && v != 0x0b && v != 0x0d
	case v >= 0x80 && v <= 0x9f:
		return true
	}
	return false
}

func hexVal(c byte) (int, bool) {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0'), true
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10, true
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10, true
	}
	return 0, false
}

var jsonTypeRE = regexp.MustCompile(`[/+]json($|;)`)

// RoundTripper wraps base so that JSON response bodies pass through
// ProtectLiteralEscapes. It must sit below the go-gh HTTP client, whose own
// sanitizer then sees only text it can handle correctly. A nil base uses
// http.DefaultTransport at request time.
func RoundTripper(base http.RoundTripper) http.RoundTripper {
	return roundTripper{base: base}
}

type roundTripper struct {
	base http.RoundTripper
}

func (rt roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	base := rt.base
	if base == nil {
		base = http.DefaultTransport
	}
	resp, err := base.RoundTrip(req)
	if err != nil || resp == nil || resp.Body == nil || !isJSON(resp.Header.Get("Content-Type")) {
		return resp, err
	}
	resp.Body = struct {
		io.Reader
		io.Closer
	}{
		Reader: transform.NewReader(resp.Body, ProtectLiteralEscapes{}),
		Closer: resp.Body,
	}
	return resp, nil
}

// isJSON mirrors the content type test used by the go-gh sanitizer so that
// exactly the responses it rewrites are protected.
func isJSON(ct string) bool {
	return jsonTypeRE.MatchString(ct)
}
