// Package envconf reads configuration values from environment variables.
//
// Every service in this repository is configured the same way: a setting has a
// built-in default, and an environment variable overrides it. The functions
// here implement that one rule, so that the eight services do not each carry
// their own slightly different copy of it.
//
// All of them share the same behaviour, which is worth stating once rather than
// eight times: an unset variable and an empty variable both mean "no override",
// and a variable whose value cannot be parsed as the requested type is treated
// the same way. A typo in a port number therefore starts the service on its
// default port instead of refusing to start. That is deliberate for a set of
// long-running camera services where staying up on a sensible default beats
// failing to boot, but it does mean a misspelled value fails quietly — so a
// service that ignores a setting you are sure you set is worth checking for a
// typo. Use LookupString when a caller needs to tell "unset" from "set to
// something unparseable" and act on the difference.
package envconf

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// String returns the value of the environment variable key, or fallback when
// the variable is unset or empty.
func String(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// LookupString returns the value of the environment variable key and whether it
// was set to a non-empty value. Use it in the rare case where an empty setting
// means something different from an absent one.
func LookupString(key string) (string, bool) {
	v := os.Getenv(key)
	return v, v != ""
}

// Int returns the environment variable key parsed as an int, or fallback when
// the variable is unset, empty, or not a valid integer.
func Int(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

// Int64 returns the environment variable key parsed as an int64, or fallback
// when the variable is unset, empty, or not a valid integer.
func Int64(key string, fallback int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return fallback
}

// Uint32 returns the environment variable key parsed as a uint32, or fallback
// when the variable is unset, empty, negative, too large, or not a number.
func Uint32(key string, fallback uint32) uint32 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseUint(v, 10, 32); err == nil {
			return uint32(n)
		}
	}
	return fallback
}

// Bool returns the environment variable key parsed as a boolean, or fallback
// when the variable is unset, empty, or none of the accepted spellings.
// True is "true", "1", "yes" or "on"; false is "false", "0", "no" or "off".
// Matching ignores case, so "TRUE" and "True" both work.
func Bool(key string, fallback bool) bool {
	switch strings.ToLower(os.Getenv(key)) {
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off":
		return false
	}
	return fallback
}

// Duration returns the environment variable key parsed as a time.Duration, or
// fallback when the variable is unset, empty, or not a valid duration.
// The value uses Go's duration syntax, for example "500ms", "10s" or "2m30s";
// a bare number such as "10" has no unit and is therefore not valid.
func Duration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}

// Seconds returns the environment variable key parsed as a whole number of
// seconds, or fallback when the variable is unset, empty, or not a number.
// Several services express intervals as a plain second count in variables named
// *_SEC; this keeps that convention in one place instead of spreading
// time.Duration(Int(...)) * time.Second across their config code.
func Seconds(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return time.Duration(n) * time.Second
		}
	}
	return fallback
}
