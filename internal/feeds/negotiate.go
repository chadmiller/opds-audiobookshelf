package feeds

import (
	"strconv"
	"strings"
)

// Negotiate picks between jsonBuilder and atomBuilder (whichever the
// request's Accept header selects) and returns the winner. Clients that
// don't send a meaningful Accept header (many minimal/embedded OPDS clients
// send none, or "*/*") get atomBuilder, since Atom/XML is the older, more
// broadly supported format; jsonBuilder is only returned when a client
// explicitly asks for JSON with a higher or equal preference.
func Negotiate(acceptHeader string, jsonBuilder, atomBuilder Builder) Builder {
	if wantsJSON(acceptHeader) {
		return jsonBuilder
	}
	return atomBuilder
}

func wantsJSON(acceptHeader string) bool {
	if strings.TrimSpace(acceptHeader) == "" {
		return false
	}
	jsonQ := bestQ(acceptHeader, "application/opds+json", "application/json")
	atomQ := bestQ(acceptHeader, "application/atom+xml", "application/opds+xml", "application/xml", "text/xml")
	return jsonQ > atomQ
}

// bestQ returns the highest q-value (RFC 7231 §5.3.1) among the given exact
// media types as they appear in an Accept header, falling back to any
// matching wildcard range ("*/*" or "type/*"). Returns -1 if nothing in the
// header could match any of the given types.
func bestQ(acceptHeader string, mediaTypes ...string) float64 {
	best := -1.0
	for _, part := range strings.Split(acceptHeader, ",") {
		rangeType, q := parseAcceptPart(part)
		for _, mt := range mediaTypes {
			if rangeMatches(rangeType, mt) && q > best {
				best = q
			}
		}
	}
	return best
}

func parseAcceptPart(part string) (rangeType string, q float64) {
	q = 1.0
	fields := strings.Split(part, ";")
	rangeType = strings.ToLower(strings.TrimSpace(fields[0]))
	for _, param := range fields[1:] {
		param = strings.TrimSpace(param)
		if v, ok := strings.CutPrefix(param, "q="); ok {
			if parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
				q = parsed
			}
		}
	}
	return rangeType, q
}

// rangeMatches reports whether an Accept media-range (e.g. "*/*",
// "application/*", "application/json") matches an exact media type.
func rangeMatches(mediaRange, exact string) bool {
	if mediaRange == "*/*" || mediaRange == exact {
		return true
	}
	rangeType, rangeSub, ok := strings.Cut(mediaRange, "/")
	if !ok || rangeSub != "*" {
		return false
	}
	exactType, _, ok := strings.Cut(exact, "/")
	return ok && exactType == rangeType
}
