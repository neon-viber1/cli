package jsonsanitize

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/cli/go-gh/v2/pkg/asciisanitizer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/transform"
)

// sanitize applies the protection followed by the go-gh JSON sanitizer, reading
// in the smallest possible chunks to exercise buffer boundaries.
func sanitize(t *testing.T, in string, oneByte bool) string {
	t.Helper()
	var r io.Reader = strings.NewReader(in)
	if oneByte {
		r = iotest.OneByteReader(r)
	}
	r = transform.NewReader(r, ProtectLiteralEscapes{})
	r = transform.NewReader(r, &asciisanitizer.Sanitizer{JSON: true})
	out, err := io.ReadAll(r)
	require.NoError(t, err)
	return string(out)
}

func decode(t *testing.T, in string) string {
	t.Helper()
	var s string
	require.NoError(t, json.Unmarshal([]byte(in), &s), in)
	return s
}

func TestProtectLiteralEscapes(t *testing.T) {
	tests := []struct {
		name string
		in   string // JSON string literal on the wire
		want string // decoded value after sanitization
	}{
		{"literal lowercase", `"\\u001b"`, `\u001b`},
		{"literal uppercase", `"\\u001B"`, `\u001B`},
		{"literal C1", `"\\u009b"`, `\u009b`},
		{"literal C1 uppercase", `"\\u009F"`, `\u009F`},
		{"real escape", `"\u001b"`, "^["},
		{"real C1 escape", `"\u009b"`, "^["},
		{"backslash then real escape", `"\\\u001b"`, `\^[`},
		{"two literal backslashes", `"\\\\u001b"`, `\\u001b`},
		{"three backslashes", `"\\\\\u001b"`, `\\^[`},
		{"four backslashes", `"\\\\\\\\u001b"`, `\\\\u001b`},
		{"literal then real", `"\\u001b \u001b"`, `\u001b ^[`},
		{"truncated", `"\\u00"`, `\u00`},
		{"non hex", `"\\u00zz"`, `\u00zz`},
		{"not control range", `"\\u0041"`, `\u0041`},
		{"safe controls literal", `"\\u000a"`, `\u000a`},
		{"plain", `"example"`, "example"},
		{"escaped backslash only", `"a\\b"`, `a\b`},
		{"trailing backslash pair", `"\\\\"`, `\\`},
	}
	for _, tt := range tests {
		for _, oneByte := range []bool{false, true} {
			name := tt.name
			if oneByte {
				name += "/one byte reader"
			}
			t.Run(name, func(t *testing.T) {
				out := sanitize(t, tt.in, oneByte)
				assert.Equal(t, tt.want, decode(t, out))
				assert.NotContains(t, out, "\x1b")
			})
		}
	}
}

func TestProtectLiteralEscapesDstBoundaries(t *testing.T) {
	// A one byte destination forces ErrShortDst handling around the rewrite.
	in := `"\\u001b \\u009B \\\u001b"`
	r := transform.NewReader(strings.NewReader(in), ProtectLiteralEscapes{})
	out, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Equal(t, `"\u005cu001b \u005cu009B \\\u001b"`, string(out))
}

func TestRoundTripper(t *testing.T) {
	const body = `{"body":"\\u001b[31m","control":"\u001b[31m"}`
	for _, ct := range []string{"application/json; charset=utf-8", "application/vnd.github+json"} {
		t.Run(ct, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", ct)
				_, _ = io.WriteString(w, body)
			}))
			defer srv.Close()
			resp, err := (&http.Client{Transport: RoundTripper(nil)}).Get(srv.URL)
			require.NoError(t, err)
			defer resp.Body.Close()
			got, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			assert.True(t, bytes.Contains(got, []byte(`\u005cu001b`)))
			var v map[string]string
			require.NoError(t, json.Unmarshal(got, &v))
			assert.Equal(t, `\u001b[31m`, v["body"])
		})
	}

	t.Run("non-JSON untouched", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = io.WriteString(w, body)
		}))
		defer srv.Close()
		resp, err := (&http.Client{Transport: RoundTripper(nil)}).Get(srv.URL)
		require.NoError(t, err)
		defer resp.Body.Close()
		got, _ := io.ReadAll(resp.Body)
		assert.Equal(t, body, string(got))
	})
}
