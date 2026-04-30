package auth

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

var reservedUsernames = map[string]bool{
	"admin": true, "administrator": true, "root": true, "support": true,
	"help": true, "staff": true, "system": true, "mod": true, "moderator": true,
	"api": true, "auth": true, "oauth": true, "login": true, "signin": true,
	"signup": true, "register": true, "account": true, "profile": true,
	"watchlist": true, "watchlists": true, "stats": true, "wrapped": true,
	"settings": true, "search": true, "terms": true, "privacy": true,
	"legal": true, "about": true, "home": true, "me": true, "mm": true,
	"minimovie": true, "owner": true, "security": true, "billing": true,
	"invoice": true, "abuse": true, "contact": true, "www": true, "mail": true,
	"ftp": true, "smtp": true, "dev": true, "staging": true, "prod": true,
	"public": true, "private": true, "movies": true, "series": true,
	"people": true, "person": true,
}

var usernameRegex = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9_-]{1,28}[a-z0-9])$`)

var adjacencyRegex = regexp.MustCompile(`--|__|_-|-_`)

func ValidateUsername(username string) error {
	normalized := norm.NFKC.String(username)
	if normalized != username {
		return ErrUsernameInvalid
	}

	lower := strings.ToLower(username)
	if lower != username {
		return ErrUsernameInvalid
	}

	if len(username) < 3 || len(username) > 30 {
		return ErrUsernameLength
	}

	if !usernameRegex.MatchString(username) {
		return ErrUsernameInvalid
	}

	if adjacencyRegex.MatchString(username) {
		return ErrUsernameInvalid
	}

	if isAllDigits(username) || isAllSameChar(username) {
		return ErrUsernameInvalid
	}

	if reservedUsernames[lower] {
		return ErrUsernameReserved
	}

	return nil
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func isAllSameChar(s string) bool {
	if len(s) == 0 {
		return false
	}
	first := rune(s[0])
	for _, r := range s[1:] {
		if r != first {
			return false
		}
	}
	return true
}

var (
	ErrUsernameInvalid  = &ValidationError{Code: "invalid_username", Message: "Username must be 3-30 lowercase alphanumeric characters, dashes, or underscores"}
	ErrUsernameLength   = &ValidationError{Code: "invalid_username", Message: "Username must be between 3 and 30 characters"}
	ErrUsernameReserved = &ValidationError{Code: "invalid_username", Message: "This username is reserved"}
	ErrUsernameTaken    = &ValidationError{Code: "username_taken", Message: "This username is already taken"}
)

type ValidationError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *ValidationError) Error() string {
	return e.Message
}
