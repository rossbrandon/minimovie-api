package auth

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateUsername(t *testing.T) {
	tests := []struct {
		name     string
		username string
		wantCode string
		wantErr  bool
	}{
		{name: "valid simple", username: "alice", wantErr: false},
		{name: "valid with digits", username: "alice42", wantErr: false},
		{name: "valid with dash", username: "alice-bob", wantErr: false},
		{name: "valid with underscore", username: "alice_bob", wantErr: false},
		{name: "valid min length", username: "abc", wantErr: false},
		{name: "valid 30 chars", username: "abcdefghij0123456789abcdefghij", wantErr: false},

		{name: "too short", username: "ab", wantErr: true, wantCode: "invalid_username"},
		{name: "too long", username: "abcdefghij0123456789abcdefghijk", wantErr: true, wantCode: "invalid_username"},
		{name: "empty", username: "", wantErr: true, wantCode: "invalid_username"},

		{name: "uppercase rejected", username: "Alice", wantErr: true, wantCode: "invalid_username"},
		{name: "starts with dash", username: "-alice", wantErr: true, wantCode: "invalid_username"},
		{name: "ends with dash", username: "alice-", wantErr: true, wantCode: "invalid_username"},
		{name: "starts with underscore", username: "_alice", wantErr: true, wantCode: "invalid_username"},
		{name: "ends with underscore", username: "alice_", wantErr: true, wantCode: "invalid_username"},
		{name: "adjacent dashes", username: "ali--ce", wantErr: true, wantCode: "invalid_username"},
		{name: "adjacent underscores", username: "ali__ce", wantErr: true, wantCode: "invalid_username"},
		{name: "mixed adjacency dash-underscore", username: "ali-_ce", wantErr: true, wantCode: "invalid_username"},
		{name: "mixed adjacency underscore-dash", username: "ali_-ce", wantErr: true, wantCode: "invalid_username"},
		{name: "all digits", username: "12345", wantErr: true, wantCode: "invalid_username"},
		{name: "all same char", username: "aaaa", wantErr: true, wantCode: "invalid_username"},
		{name: "space in name", username: "ali ce", wantErr: true, wantCode: "invalid_username"},

		{name: "reserved admin", username: "admin", wantErr: true, wantCode: "invalid_username"},
		{name: "reserved root", username: "root", wantErr: true, wantCode: "invalid_username"},
		{name: "reserved minimovie", username: "minimovie", wantErr: true, wantCode: "invalid_username"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUsername(tt.username)
			if !tt.wantErr {
				assert.NoError(t, err)
				return
			}

			assert.Error(t, err)
			var ve *ValidationError
			if assert.True(t, errors.As(err, &ve)) {
				assert.Equal(t, tt.wantCode, ve.Code)
			}
		})
	}
}
