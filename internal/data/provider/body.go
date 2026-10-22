package provider

import (
	"fmt"
	"io"
	"regexp"
)

// MaxBody caps a response body. The largest real payloads (a season's
// scoreboard) are a few MB; anything past this is not a payload.
const MaxBody = 32 << 20

// controlEscape matches a JSON \u escape for a C0 control, DEL, or a C1
// control; JSON can only carry control characters in strings this way.
var controlEscape = regexp.MustCompile(`\\u00(?:[01][0-9a-fA-F]|7[fF]|[89][0-9a-fA-F])`)

// ReadBody reads at most MaxBody bytes, replacing control-character
// escapes with U+FFFD. Security: source names reach the terminal, where an
// escape sequence could rewrite the screen.
func ReadBody(r io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, MaxBody+1))
	if err != nil {
		return nil, err
	}
	if len(body) > MaxBody {
		return nil, fmt.Errorf("response larger than %d MB", MaxBody>>20)
	}
	return controlEscape.ReplaceAll(body, []byte("\\ufffd")), nil
}
