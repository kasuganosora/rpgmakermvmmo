package middleware

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSecret = "test-jwt-secret-32bytes-padded!!"

func TestGenerateToken_Valid(t *testing.T) {
	tok, err := GenerateToken(42, testSecret, time.Hour)
	require.NoError(t, err)
	assert.NotEmpty(t, tok)
}

func TestParseToken_Valid(t *testing.T) {
	tok, err := GenerateToken(99, testSecret, time.Hour)
	require.NoError(t, err)

	claims, err := ParseToken(tok, testSecret)
	require.NoError(t, err)
	assert.Equal(t, int64(99), claims.AccountID)
}

func TestParseToken_WrongSecret(t *testing.T) {
	tok, err := GenerateToken(1, testSecret, time.Hour)
	require.NoError(t, err)

	_, err = ParseToken(tok, "wrong-secret")
	assert.Error(t, err)
}

func TestParseToken_Expired(t *testing.T) {
	tok, err := GenerateToken(1, testSecret, -time.Second)
	require.NoError(t, err)

	_, err = ParseToken(tok, testSecret)
	assert.Error(t, err)
}

func TestParseToken_Malformed(t *testing.T) {
	_, err := ParseToken("not.a.jwt", testSecret)
	assert.Error(t, err)
}

func TestParseToken_Empty(t *testing.T) {
	_, err := ParseToken("", testSecret)
	assert.Error(t, err)
}

func TestGenerateToken_DifferentAccounts(t *testing.T) {
	t1, _ := GenerateToken(1, testSecret, time.Hour)
	t2, _ := GenerateToken(2, testSecret, time.Hour)
	assert.NotEqual(t, t1, t2)

	c1, _ := ParseToken(t1, testSecret)
	c2, _ := ParseToken(t2, testSecret)
	assert.Equal(t, int64(1), c1.AccountID)
	assert.Equal(t, int64(2), c2.AccountID)
}
