package invocation

import (
	"regexp"
	"strings"
)

// RedactedValue replaces sensitive values in stored command lines.
const RedactedValue = "[REDACTED]"

// Matches the userinfo component of URLs, e.g. "https://user:pass@host".
var urlUserinfoPattern = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.-]*://)[^/?#@\s]+@`)

// isSensitiveOptionName reports whether an option's value must not be
// stored. Bazel does not mark header flags (--remote_header, --bes_header,
// --remote_downloader_header, ...) as HIDDEN, so their values, which
// typically carry bearer tokens or API keys, would otherwise be persisted
// verbatim.
func isSensitiveOptionName(name string) bool {
	return strings.HasSuffix(name, "_header")
}

// redactHeaderValue keeps the header name of a "Name=Value" header flag
// value for debuggability and drops the value.
func redactHeaderValue(value string) string {
	if name, _, found := strings.Cut(value, "="); found {
		return name + "=" + RedactedValue
	}
	return RedactedValue
}

// RedactOptionValue returns the value to store for the option with the
// given name (without leading dashes).
func RedactOptionValue(name, value string) string {
	if isSensitiveOptionName(name) {
		return redactHeaderValue(value)
	}
	return urlUserinfoPattern.ReplaceAllString(value, "${1}"+RedactedValue+"@")
}

// RedactRawOption redacts a raw command line argument such as
// "--remote_header=Authorization=Bearer abc", as found in the
// OptionsParsed event and residual arguments.
func RedactRawOption(arg string) string {
	if strings.HasPrefix(arg, "-") {
		if flag, value, found := strings.Cut(arg, "="); found {
			return flag + "=" + RedactOptionValue(strings.TrimLeft(flag, "-"), value)
		}
	}
	return urlUserinfoPattern.ReplaceAllString(arg, "${1}"+RedactedValue+"@")
}

// RedactRawOptions applies RedactRawOption to each argument. Bazel also
// accepts the space-separated form ("--remote_header" "value"), so an
// argument following a bare sensitive flag is redacted as a value.
func RedactRawOptions(args []string) []string {
	if args == nil {
		return nil
	}
	result := make([]string, len(args))
	for i, arg := range args {
		if i > 0 && isBareSensitiveFlag(args[i-1]) {
			result[i] = redactHeaderValue(arg)
			continue
		}
		result[i] = RedactRawOption(arg)
	}
	return result
}

func isBareSensitiveFlag(arg string) bool {
	return strings.HasPrefix(arg, "-") &&
		!strings.Contains(arg, "=") &&
		isSensitiveOptionName(strings.TrimLeft(arg, "-"))
}
