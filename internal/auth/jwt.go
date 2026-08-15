// Package auth valide les jetons emis par l'auth-service.
package auth

import (
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

// ErrInvalidToken couvre tous les motifs de rejet.
//
// Volontairement unique : distinguer "signature invalide" de "jeton expire"
// dans la reponse renseignerait un attaquant sur ce qu'il doit corriger.
var ErrInvalidToken = errors.New("jeton invalide")

// Claims porte ce que le service a besoin de savoir de l'appelant.
//
// Rien de plus : ce service ne fait aucune regle metier, il n'a donc pas
// besoin de l'email ni du telephone, meme s'ils sont dans le jeton.
type Claims struct {
	UserID string
	Roles  []string
}

// HasRole dit si l'appelant porte ce role.
func (c Claims) HasRole(role string) bool {
	for _, r := range c.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// Verifier valide les jetons avec le secret partage.
type Verifier struct {
	secret []byte
	issuer string
	parser *jwt.Parser
}

// NewVerifier construit un verificateur.
//
// L'algorithme est fige a HS256 : sans cette contrainte, un jeton portant
// "alg": "none" serait accepte, ce qui revient a n'avoir aucune signature.
func NewVerifier(secret []byte, issuer string) *Verifier {
	return &Verifier{
		secret: secret,
		issuer: issuer,
		parser: jwt.NewParser(
			jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
			jwt.WithIssuer(issuer),
			jwt.WithExpirationRequired(),
		),
	}
}

// Verify controle la signature, l'emetteur et l'expiration.
func (v *Verifier) Verify(tokenString string) (Claims, error) {
	token, err := v.parser.Parse(tokenString, func(t *jwt.Token) (any, error) {
		return v.secret, nil
	})
	if err != nil || !token.Valid {
		return Claims{}, ErrInvalidToken
	}

	mapClaims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return Claims{}, ErrInvalidToken
	}

	subject, err := mapClaims.GetSubject()
	if err != nil || subject == "" {
		return Claims{}, fmt.Errorf("%w : sujet absent", ErrInvalidToken)
	}

	return Claims{UserID: subject, Roles: extractRoles(mapClaims)}, nil
}

// extractRoles lit le claim "roles" pose par l'auth-service.
//
// Un jeton sans roles reste valide : c'est un compte sans privilege
// particulier, pas un jeton casse.
func extractRoles(claims jwt.MapClaims) []string {
	raw, present := claims["roles"]
	if !present {
		return nil
	}

	// Le decodage JSON produit []any, jamais []string.
	values, ok := raw.([]any)
	if !ok {
		return nil
	}

	roles := make([]string, 0, len(values))
	for _, value := range values {
		if role, ok := value.(string); ok {
			roles = append(roles, role)
		}
	}
	return roles
}
