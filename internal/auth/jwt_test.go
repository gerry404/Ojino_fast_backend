package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// C'est la seule barriere entre une connexion et les donnees d'un eleve : ces
// tests couvrent les manieres de la contourner.

var secret = []byte("un-secret-de-test-suffisamment-long-32+")

func TestVerifyAcceptsAValidToken(t *testing.T) {
	verifier := NewVerifier(secret, "ojino-auth")
	token := sign(t, secret, jwt.MapClaims{
		"iss":   "ojino-auth",
		"sub":   "user-42",
		"exp":   time.Now().Add(time.Hour).Unix(),
		"roles": []any{"USER", "ADMIN"},
	}, jwt.SigningMethodHS256)

	claims, err := verifier.Verify(token)
	if err != nil {
		t.Fatalf("jeton valide refuse : %v", err)
	}
	if claims.UserID != "user-42" {
		t.Errorf("UserID = %q, attendu user-42", claims.UserID)
	}
	if !claims.HasRole("ADMIN") || claims.HasRole("INCONNU") {
		t.Errorf("roles mal lus : %v", claims.Roles)
	}
}

func TestVerifyRejectsAnotherSecret(t *testing.T) {
	verifier := NewVerifier(secret, "ojino-auth")
	token := sign(t, []byte("un-tout-autre-secret-de-32-caracteres+"), jwt.MapClaims{
		"iss": "ojino-auth",
		"sub": "user-42",
		"exp": time.Now().Add(time.Hour).Unix(),
	}, jwt.SigningMethodHS256)

	if _, err := verifier.Verify(token); err == nil {
		t.Error("un jeton signe ailleurs doit etre refuse")
	}
}

func TestVerifyRejectsAnExpiredToken(t *testing.T) {
	verifier := NewVerifier(secret, "ojino-auth")
	token := sign(t, secret, jwt.MapClaims{
		"iss": "ojino-auth",
		"sub": "user-42",
		"exp": time.Now().Add(-time.Minute).Unix(),
	}, jwt.SigningMethodHS256)

	if _, err := verifier.Verify(token); err == nil {
		t.Error("un jeton expire doit etre refuse")
	}
}

func TestVerifyRejectsAnUnexpectedIssuer(t *testing.T) {
	verifier := NewVerifier(secret, "ojino-auth")
	token := sign(t, secret, jwt.MapClaims{
		"iss": "un-autre-emetteur",
		"sub": "user-42",
		"exp": time.Now().Add(time.Hour).Unix(),
	}, jwt.SigningMethodHS256)

	if _, err := verifier.Verify(token); err == nil {
		t.Error("un emetteur inattendu doit etre refuse")
	}
}

func TestVerifyRejectsTheNoneAlgorithm(t *testing.T) {
	verifier := NewVerifier(secret, "ojino-auth")

	// L'attaque classique : signer avec "alg": "none" revient a n'avoir aucune
	// signature. Sans la contrainte d'algorithme, ce jeton passerait.
	token, err := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
		"iss": "ojino-auth",
		"sub": "attaquant",
		"exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("preparation du jeton : %v", err)
	}

	if _, err := verifier.Verify(token); err == nil {
		t.Error("l'algorithme none doit etre refuse")
	}
}

func TestVerifyRejectsATokenWithoutSubject(t *testing.T) {
	verifier := NewVerifier(secret, "ojino-auth")
	token := sign(t, secret, jwt.MapClaims{
		"iss": "ojino-auth",
		"exp": time.Now().Add(time.Hour).Unix(),
	}, jwt.SigningMethodHS256)

	// Sans sujet, on ne saurait pas a qui livrer les evenements.
	if _, err := verifier.Verify(token); err == nil {
		t.Error("un jeton sans sujet doit etre refuse")
	}
}

func TestVerifyRejectsATokenWithoutExpiry(t *testing.T) {
	verifier := NewVerifier(secret, "ojino-auth")
	token := sign(t, secret, jwt.MapClaims{
		"iss": "ojino-auth",
		"sub": "user-42",
	}, jwt.SigningMethodHS256)

	// Un jeton sans expiration serait valable pour toujours.
	if _, err := verifier.Verify(token); err == nil {
		t.Error("un jeton sans expiration doit etre refuse")
	}
}

func TestVerifyAcceptsATokenWithoutRoles(t *testing.T) {
	verifier := NewVerifier(secret, "ojino-auth")
	token := sign(t, secret, jwt.MapClaims{
		"iss": "ojino-auth",
		"sub": "user-42",
		"exp": time.Now().Add(time.Hour).Unix(),
	}, jwt.SigningMethodHS256)

	// Un compte sans privilege particulier reste un compte valide.
	claims, err := verifier.Verify(token)
	if err != nil {
		t.Fatalf("jeton sans roles refuse : %v", err)
	}
	if len(claims.Roles) != 0 {
		t.Errorf("Roles = %v, attendu vide", claims.Roles)
	}
}

func TestVerifyRejectsGarbage(t *testing.T) {
	verifier := NewVerifier(secret, "ojino-auth")

	for _, input := range []string{"", "pas-un-jeton", "a.b.c"} {
		if _, err := verifier.Verify(input); err == nil {
			t.Errorf("entree %q acceptee a tort", input)
		}
	}
}

func sign(t *testing.T, key []byte, claims jwt.MapClaims,
	method jwt.SigningMethod) string {

	t.Helper()
	token, err := jwt.NewWithClaims(method, claims).SignedString(key)
	if err != nil {
		t.Fatalf("signature du jeton : %v", err)
	}
	return token
}
