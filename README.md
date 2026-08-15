# ojino-realtime

Le service temps réel d'Ojino. **Go + Gin + WebSocket.**

Il fait ce qu'un service Spring fait mal : tenir des dizaines de milliers de connexions
ouvertes. Une goroutine coûte quelques kilo-octets, un thread JVM coûte un mégaoctet.

## Ce qu'il possède

| Il possède | Il ne possède pas |
|---|---|
| Les connexions WebSocket ouvertes | L'identité — c'est `auth-service` |
| La présence (qui est en ligne) | Les données métier — elles restent chez leurs services |
| Les salles d'étude partagées | La logique d'envoi — c'est `notification-service` |
| Le relais des événements vers les clients | La génération de réponses — c'est `assistant-service` |

> **Il ne stocke presque rien.** L'état vit en mémoire, et se reconstruit à la reconnexion.
> Un service temps réel qui persiste tout perd l'avantage qui justifie son existence.

## Architecture

```
Client (mobile / web)
   │  WebSocket + JWT
   ▼
┌──────────────────────────────┐
│  ojino-realtime  (Go/Gin)    │
│                              │
│  hub ── connexions par user  │
│   │                          │
│   ├── présence               │
│   └── salles d'étude         │
└──────────────────────────────┘
   ▲  HTTP interne
   │
Services Spring (assistant, notification…)
```

Deux entrées :
- **WebSocket** pour les clients, authentifiée par le même JWT que le reste
- **HTTP interne** pour les services Spring, qui poussent un événement vers un utilisateur

## Démarrer

```bash
cp .env.example .env
go run ./cmd/realtime
```

Le service écoute sur **8090** (les services Spring occupent 8081 à 8089).

## Pourquoi Go ici et pas ailleurs

Le reste du backend est en Spring, et c'est le bon choix pour du métier : validation,
transactions, écosystème. Mais une connexion WebSocket ouverte pendant des heures est un
problème différent — c'est de la concurrence, pas du métier.

Ce service n'a donc **aucune règle métier**. S'il commence à décider de quelque chose,
c'est que quelque chose est mal placé.

## Configuration

| Variable | Rôle |
|---|---|
| `PORT` | Port d'écoute (défaut 8090) |
| `OJINO_JWT_SECRET` | **Identique** aux services Spring |
| `OJINO_JWT_ISSUER` | Émetteur attendu (`ojino-auth`) |
| `OJINO_INTERNAL_TOKEN` | Secret des routes internes |
| `ALLOWED_ORIGINS` | Origines autorisées à ouvrir une WebSocket |
| `MAX_CONNECTIONS_PER_USER` | Plafond par utilisateur |

## État

En construction. Voir les jalons dans les pull requests.
