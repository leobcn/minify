package xml // import "github.com/tdewolff/minify/xml"

import (
	"io"

	"github.com/tdewolff/minify"
	"github.com/tdewolff/parse"
	"github.com/tdewolff/parse/xml"
)

var (
	ltBytes                 = []byte("<")
	gtBytes                 = []byte(">")
	voidBytes               = []byte("/>")
	ltPIBytes               = []byte("<?")
	gtPIBytes               = []byte("?>")
	endBytes                = []byte("</")
	DOCTYPEBytes            = []byte("<!DOCTYPE")
	CDATAStartBytes         = []byte("<![CDATA[")
	CDATAEndBytes           = []byte("]]>")
	isBytes                 = []byte("=")
	spaceBytes              = []byte(" ")
	emptyBytes              = []byte("\"\"")
	ltEntityBytes           = []byte("&lt;")
	ampEntityBytes          = []byte("&amp;")
	escapedSingleQuoteBytes = []byte("&#39;")
	escapedDoubleQuoteBytes = []byte("&#34;")
)

////////////////////////////////////////////////////////////////

// Minify minifies XML files, it reads from r and writes to w.
// Removes unnecessary whitespace, tags, attributes, quotes and comments and typically saves 10% in size.
func Minify(m minify.Minifier, _ string, w io.Writer, r io.Reader) error {
	precededBySpace := true // on true the next text token must not start with a space
	escapeBuffer := make([]byte, 0, 64)

	z := xml.NewTokenizer(r)
	tb := NewTokenBuffer(z)
	for {
		t := *tb.Shift()
		if t.TokenType == xml.CDATAToken {
			var useCDATA bool
			if t.Data, useCDATA = EscapeCDATAVal(&escapeBuffer, t.Data); !useCDATA {
				t.TokenType = xml.TextToken
			}
		}
		switch t.TokenType {
		case xml.ErrorToken:
			if z.Err() == io.EOF {
				return nil
			}
			return z.Err()
		case xml.DOCTYPEToken:
			if _, err := w.Write(DOCTYPEBytes); err != nil {
				return err
			}
			if _, err := w.Write(t.Data); err != nil {
				return err
			}
			if _, err := w.Write(gtBytes); err != nil {
				return err
			}
		case xml.CDATAToken:
			if _, err := w.Write(CDATAStartBytes); err != nil {
				return err
			}
			if _, err := w.Write(t.Data); err != nil {
				return err
			}
			if _, err := w.Write(CDATAEndBytes); err != nil {
				return err
			}
		case xml.TextToken:
			if t.Data = parse.ReplaceMultiple(t.Data, parse.IsWhitespace, ' '); len(t.Data) > 0 {
				// whitespace removal; trim left
				if t.Data[0] == ' ' && precededBySpace {
					t.Data = t.Data[1:]
				}

				// whitespace removal; trim right
				precededBySpace = false
				if len(t.Data) == 0 {
					precededBySpace = true
				} else if t.Data[len(t.Data)-1] == ' ' {
					precededBySpace = true
					trim := false
					i := 0
					for {
						next := tb.Peek(i)
						// trim if EOF, text token with whitespace begin or block token
						if next.TokenType == xml.StartTagToken || next.TokenType == xml.EndTagToken || next.TokenType == xml.ErrorToken {
							trim = true
							break
						} else if next.TokenType == xml.TextToken {
							// remove if the text token starts with a whitespace
							trim = (len(next.Data) > 0 && parse.IsWhitespace(next.Data[0]))
							break
						}
						i++
					}
					if trim {
						t.Data = t.Data[:len(t.Data)-1]
						precededBySpace = false
					}
				}
				if _, err := w.Write(t.Data); err != nil {
					return err
				}
			}
		case xml.StartTagToken:
			if _, err := w.Write(ltBytes); err != nil {
				return err
			}
			if _, err := w.Write(t.Data); err != nil {
				return err
			}
		case xml.StartTagPIToken:
			if _, err := w.Write(ltPIBytes); err != nil {
				return err
			}
			if _, err := w.Write(t.Data); err != nil {
				return err
			}
		case xml.AttributeToken:
			val := t.AttrVal
			if len(val) < 2 {
				if _, err := w.Write(emptyBytes); err != nil {
					return err
				}
				continue
			}

			if _, err := w.Write(spaceBytes); err != nil {
				return err
			}
			if _, err := w.Write(t.Data); err != nil {
				return err
			}
			if _, err := w.Write(isBytes); err != nil {
				return err
			}

			// prefer single or double quotes depending on what occurs more often in value
			val = val[1 : len(val)-1]
			val = EscapeAttrVal(&escapeBuffer, val)
			if _, err := w.Write(val); err != nil {
				return err
			}
		case xml.StartTagCloseToken:
			next := tb.Peek(0)
			skipExtra := false
			if next.TokenType == xml.TextToken && parse.IsAllWhitespace(next.Data) {
				next = tb.Peek(1)
				skipExtra = true
			}
			if next.TokenType == xml.EndTagToken {
				// collapse empty tags to single void tag
				tb.Shift()
				if skipExtra {
					tb.Shift()
				}
				if _, err := w.Write(voidBytes); err != nil {
					return err
				}
			} else {
				if _, err := w.Write(gtBytes); err != nil {
					return err
				}
			}
		case xml.StartTagCloseVoidToken:
			if _, err := w.Write(voidBytes); err != nil {
				return err
			}
		case xml.StartTagClosePIToken:
			if _, err := w.Write(gtPIBytes); err != nil {
				return err
			}
		case xml.EndTagToken:
			if _, err := w.Write(endBytes); err != nil {
				return err
			}
			if _, err := w.Write(t.Data); err != nil {
				return err
			}
			if _, err := w.Write(gtBytes); err != nil {
				return err
			}
		}
	}
}

////////////////////////////////////////////////////////////////

// it is assumed that b[0] equals '&'
func isAtQuoteEntity(b []byte) (quote byte, n int, ok bool) {
	if len(b) < 5 {
		return 0, 0, false
	}
	if b[1] == '#' {
		if b[2] == 'x' {
			i := 3
			for i < len(b) && b[i] == '0' {
				i++
			}
			if i+2 < len(b) && b[i] == '2' && b[i+2] == ';' {
				if b[i+1] == '2' {
					return '"', i + 3, true // &#x22;
				} else if b[i+1] == '7' {
					return '\'', i + 3, true // &#x27;
				}
			}
		} else {
			i := 2
			for i < len(b) && b[i] == '0' {
				i++
			}
			if i+2 < len(b) && b[i] == '3' && b[i+2] == ';' {
				if b[i+1] == '4' {
					return '"', i + 3, true // &#34;
				} else if b[i+1] == '9' {
					return '\'', i + 3, true // &#39;
				}
			}
		}
	} else if len(b) >= 6 && b[5] == ';' {
		if parse.EqualCaseInsensitive(b[1:5], []byte{'q', 'u', 'o', 't'}) {
			return '"', 6, true // &quot;
		} else if parse.EqualCaseInsensitive(b[1:5], []byte{'a', 'p', 'o', 's'}) {
			return '\'', 6, true // &apos;
		}
	}
	return 0, 0, false
}

// EscapeAttrVal returns the escape attribute value bytes without quotes.
func EscapeAttrVal(buf *[]byte, b []byte) []byte {
	singles := 0
	doubles := 0
	for i, c := range b {
		if c == '&' {
			if quote, _, ok := isAtQuoteEntity(b[i:]); ok {
				if quote == '"' {
					doubles++
				} else {
					singles++
				}
			}
		} else if c == '"' {
			doubles++
		} else if c == '\'' {
			singles++
		}
	}
	var quote byte
	var escapedQuote []byte
	if doubles > singles {
		quote = '\''
		escapedQuote = escapedSingleQuoteBytes
	} else {
		quote = '"'
		escapedQuote = escapedDoubleQuoteBytes
	}

	if len(b)+2 > cap(*buf) {
		*buf = make([]byte, 0, len(b)+2) // maximum size, not actual size
	}
	t := (*buf)[:len(b)+2] // maximum size, not actual size
	t[0] = quote
	j := 1
	start := 0
	for i, c := range b {
		if c == '&' {
			if entityQuote, n, ok := isAtQuoteEntity(b[i:]); ok {
				j += copy(t[j:], b[start:i])
				if entityQuote != quote {
					j += copy(t[j:], []byte{entityQuote})
				} else {
					j += copy(t[j:], escapedQuote)
				}
				start = i + n
			}
		} else if c == quote {
			j += copy(t[j:], b[start:i])
			j += copy(t[j:], escapedQuote)
			start = i + 1
		} else if c == '\t' || c == '\n' || c == '\r' {
			b[i] = ' '
		}
	}
	j += copy(t[j:], b[start:])
	t[j] = quote
	return t[:j+1]
}

// EscapeCDATAVal returns the escaped text bytes.
func EscapeCDATAVal(buf *[]byte, b []byte) ([]byte, bool) {
	n := 0
	for _, c := range b {
		if c == '<' || c == '&' {
			if c == '<' {
				n += 3 // &lt;
			} else {
				n += 4 // &amp;
			}
			if n > len("<![CDATA[]]>") {
				return b, true
			}
		}
	}
	if len(b)+n > cap(*buf) {
		*buf = make([]byte, 0, len(b)+n)
	}
	t := (*buf)[:len(b)+n]
	j := 0
	start := 0
	for i, c := range b {
		if c == '<' {
			j += copy(t[j:], b[start:i])
			j += copy(t[j:], ltEntityBytes)
			start = i + 1
		} else if c == '&' {
			j += copy(t[j:], b[start:i])
			j += copy(t[j:], ampEntityBytes)
			start = i + 1
		}
	}
	j += copy(t[j:], b[start:])
	return t[:j], false
}