// Package config lit la configuration depuis l'environnement.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config regroupe tous les reglages du service.
//
// Lus une fois au demarrage et jamais relus : un service temps reel qui
// rechargerait sa configuration en cours de route devrait aussi decider quoi
// faire des connexions deja ouvertes, ce qui ne vaut pas la complexite.
type Config struct {
	Port string

	// JWTSecret doit etre identique a celui des services Spring. C'est ce
	// partage qui permet de valider un jeton sans appeler l'auth-service.
	JWTSecret []byte
	JWTIssuer string

	// InternalToken protege les routes appelees par les services Spring, qui
	// n'ont pas de jeton d'utilisateur sous la main.
	InternalToken string

	AllowedOrigins        []string
	MaxConnectionsPerUser int
}

// Load lit l'environnement et refuse de demarrer sur une configuration
// dangereuse.
//
// Echouer au demarrage vaut mieux que tourner avec un secret trop court : le
// probleme se voit tout de suite, au lieu de n'apparaitre que le jour ou
// quelqu'un forge un jeton.
func Load() (*Config, error) {
	secret := getenv("OJINO_JWT_SECRET", "")
	if len(secret) < 32 {
		return nil, fmt.Errorf(
			"OJINO_JWT_SECRET doit faire au moins 32 caracteres (actuellement %d)", len(secret))
	}

	internalToken := getenv("OJINO_INTERNAL_TOKEN", "")
	if internalToken == "" {
		return nil, fmt.Errorf("OJINO_INTERNAL_TOKEN est obligatoire")
	}

	maxConns, err := strconv.Atoi(getenv("MAX_CONNECTIONS_PER_USER", "5"))
	if err != nil || maxConns < 1 {
		return nil, fmt.Errorf("MAX_CONNECTIONS_PER_USER doit etre un entier positif")
	}

	return &Config{
		Port:                  getenv("PORT", "8090"),
		JWTSecret:             []byte(secret),
		JWTIssuer:             getenv("OJINO_JWT_ISSUER", "ojino-auth"),
		InternalToken:         internalToken,
		AllowedOrigins:        splitAndTrim(getenv("ALLOWED_ORIGINS", "")),
		MaxConnectionsPerUser: maxConns,
	}, nil
}

// AllowsOrigin dit si une page servie par cette origine peut ouvrir une
// WebSocket.
//
// Une liste vide autorise tout : c'est commode en developpement, et c'est
// precisement pourquoi elle doit etre renseignee en production.
func (c *Config) AllowsOrigin(origin string) bool {
	if len(c.AllowedOrigins) == 0 {
		return true
	}
	for _, allowed := range c.AllowedOrigins {
		if allowed == origin {
			return true
		}
	}
	return false
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func splitAndTrim(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
