package identity

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/mindfire-test/meiosis/pkg/crypto"
	specv1 "github.com/mindfire-test/meiosis/pkg/spec/v1"
)

func testParams(t *testing.T) IssueParams {
	t.Helper()
	return IssueParams{
		Principal: "agent:impl-3",
		Intent:    "int_" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		IssuedBy:  "human:lakin",
		Scope:     specv1.Scope{Allow: []string{"internal/auth/**"}, Mode: specv1.ScopeModeEnforce},
		IssuedAt:  time.Unix(1_700_000_000, 0).UTC(),
	}
}

func TestIssueProducesAValidSignedToken(t *testing.T) {
	issuer, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}
	token, err := Issue(testParams(t), issuer.PrivateKey)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if validationErr := token.Validate(); validationErr != nil {
		t.Fatalf("Validate() error = %v", validationErr)
	}
	if token.Signature == "" {
		t.Fatal("Issue() produced an unsigned token")
	}
	if !token.ExpiresAt.Equal(token.IssuedAt.Add(DefaultTTL)) {
		t.Fatalf("Issue() ExpiresAt = %v, want IssuedAt+DefaultTTL", token.ExpiresAt)
	}

	other, err := Issue(testParams(t), issuer.PrivateKey)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if token.ID == other.ID {
		t.Fatal("Issue() produced two tokens with the same ID")
	}
}

func TestIssueHonoursTTL(t *testing.T) {
	issuer, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}
	params := testParams(t)
	params.TTL = 5 * time.Minute
	token, err := Issue(params, issuer.PrivateKey)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if want := params.IssuedAt.Add(5 * time.Minute); !token.ExpiresAt.Equal(want) {
		t.Fatalf("ExpiresAt = %v, want %v", token.ExpiresAt, want)
	}
}

func TestVerifyAcceptsAFreshValidToken(t *testing.T) {
	issuer, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}
	token, err := Issue(testParams(t), issuer.PrivateKey)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	now := token.IssuedAt.Add(time.Minute)
	if err := Verify(token, issuer.PublicKey, nil, now); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
}

func TestVerifyRejectsExpiredToken(t *testing.T) {
	issuer, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}
	token, err := Issue(testParams(t), issuer.PrivateKey)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if err := Verify(token, issuer.PublicKey, nil, token.ExpiresAt); err != ErrTokenExpired {
		t.Fatalf("Verify() error = %v, want ErrTokenExpired", err)
	}
	if err := Verify(token, issuer.PublicKey, nil, token.ExpiresAt.Add(time.Hour)); err != ErrTokenExpired {
		t.Fatalf("Verify() error = %v, want ErrTokenExpired", err)
	}
}

func TestVerifyRejectsTamperedToken(t *testing.T) {
	issuer, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}
	token, err := Issue(testParams(t), issuer.PrivateKey)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	tampered := token
	tampered.Principal = "agent:someone-else"
	if verifyErr := Verify(tampered, issuer.PublicKey, nil, token.IssuedAt); verifyErr != ErrTokenAltered {
		t.Fatalf("Verify() error = %v, want ErrTokenAltered", verifyErr)
	}

	wrongKey, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}
	if err := Verify(token, wrongKey.PublicKey, nil, token.IssuedAt); err != ErrTokenAltered {
		t.Fatalf("Verify() error = %v, want ErrTokenAltered for wrong issuer key", err)
	}
}

func TestVerifyRejectsRevokedToken(t *testing.T) {
	issuer, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}
	token, err := Issue(testParams(t), issuer.PrivateKey)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	list := NewMemoryRevocationList()
	if err := Verify(token, issuer.PublicKey, list, token.IssuedAt); err != nil {
		t.Fatalf("Verify() error = %v before revocation", err)
	}
	list.Revoke(token.ID)
	if err := Verify(token, issuer.PublicKey, list, token.IssuedAt); err != ErrTokenRevoked {
		t.Fatalf("Verify() error = %v, want ErrTokenRevoked", err)
	}
}

func TestAuthorizeEnforcesScope(t *testing.T) {
	issuer, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}
	params := testParams(t)
	params.Scope = specv1.Scope{Allow: []string{"internal/auth/**"}, Deny: []string{"internal/auth/secrets/**"}, Mode: specv1.ScopeModeEnforce}
	token, err := Issue(params, issuer.PrivateKey)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	now := token.IssuedAt.Add(time.Minute)

	if err := Authorize(token, issuer.PublicKey, nil, "internal/auth/token.go", now); err != nil {
		t.Fatalf("Authorize() error = %v for allowed path", err)
	}
	if err := Authorize(token, issuer.PublicKey, nil, "internal/billing/invoice.go", now); !errors.Is(err, ErrScopeDenied) {
		t.Fatalf("Authorize() error = %v, want ErrScopeDenied for out-of-scope path", err)
	}
	if err := Authorize(token, issuer.PublicKey, nil, "internal/auth/secrets/key.pem", now); !errors.Is(err, ErrScopeDenied) {
		t.Fatalf("Authorize() error = %v, want ErrScopeDenied for denied path", err)
	}
}

func TestAuthorizeRejectsExpiredBeforeCheckingScope(t *testing.T) {
	issuer, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}
	token, err := Issue(testParams(t), issuer.PrivateKey)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if err := Authorize(token, issuer.PublicKey, nil, "internal/auth/token.go", token.ExpiresAt); err != ErrTokenExpired {
		t.Fatalf("Authorize() error = %v, want ErrTokenExpired", err)
	}
}

func TestEncodeDecodeRoundTripsAndIsDeterministic(t *testing.T) {
	issuer, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}
	token, err := Issue(testParams(t), issuer.PrivateKey)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	first, err := Encode(token)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	second, err := Encode(token)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("Encode() not deterministic: %q != %q", first, second)
	}

	decoded, err := Decode(first)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if !reflect.DeepEqual(decoded, token) {
		t.Fatalf("Decode() round trip mismatch: got %+v, want %+v", decoded, token)
	}
}
