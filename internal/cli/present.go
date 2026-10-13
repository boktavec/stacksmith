package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/boktavec/stacksmith/core"
)

const validMessage = "spec and standards are valid"

// jsonDiagnostic fixes the JSON field names; core.Diagnostic has no tags and
// stays free of presentation concerns.
type jsonDiagnostic struct {
	Document string `json:"document"`
	Field    string `json:"field"`
	Message  string `json:"message"`
}

type jsonResult struct {
	Valid       bool             `json:"valid"`
	Diagnostics []jsonDiagnostic `json:"diagnostics"`
}

// presentText writes the success line to stdout, or one line per diagnostic to
// stderr.
func presentText(stdout, stderr io.Writer, diagnostics []core.Diagnostic) error {
	if len(diagnostics) == 0 {
		_, err := fmt.Fprintln(stdout, validMessage)
		return err
	}
	for _, d := range diagnostics {
		line := escapeControl(d.Document) + ": "
		if d.Field != "" {
			line += escapeControl(d.Field) + ": "
		}
		line += escapeControl(d.Message)
		if _, err := fmt.Fprintln(stderr, line); err != nil {
			return err
		}
	}
	return nil
}

// presentJSON writes one JSON object to stdout for both outcomes.
// encoding/json escapes control characters itself, so no extra escaping here.
func presentJSON(stdout io.Writer, diagnostics []core.Diagnostic) error {
	result := jsonResult{
		Valid:       len(diagnostics) == 0,
		Diagnostics: make([]jsonDiagnostic, 0, len(diagnostics)), // non-nil: encodes as [] not null
	}
	for _, d := range diagnostics {
		result.Diagnostics = append(result.Diagnostics, jsonDiagnostic(d))
	}
	return json.NewEncoder(stdout).Encode(result)
}

// escapeControl makes text safe to print to a terminal. Diagnostic fields can
// echo the user's own keys and parser errors, so a hostile document could
// otherwise inject ANSI escape sequences. Only non-printable runes (and
// invalid UTF-8 bytes) are rewritten; printable text, including quotes that
// core already escaped with %q, passes through byte-for-byte.
func escapeControl(text string) string {
	var out strings.Builder
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		switch {
		case r == utf8.RuneError && size == 1:
			fmt.Fprintf(&out, `\x%02x`, text[i])
		case unicode.IsPrint(r):
			out.WriteRune(r)
		default:
			// QuoteRune yields e.g. '\x1b'; drop the surrounding quotes.
			quoted := strconv.QuoteRune(r)
			out.WriteString(quoted[1 : len(quoted)-1])
		}
		i += size
	}
	return out.String()
}
