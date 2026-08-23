# syntax=docker/dockerfile:1

# Image de realtime-service.
#
# Go compile en un binaire unique : l'image finale n'a besoin ni d'un runtime,
# ni d'une bibliotheque standard, ni des sources. Quelques megaoctets la ou un
# service Spring en pese trois cents.

FROM golang:1.26-alpine AS build
WORKDIR /build

# Les dependances d'abord : cette couche ne change que lorsque go.mod bouge,
# alors que les sources changent a chaque commit.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .

# CGO_ENABLED=0 rend le binaire statique : il ne depend d'aucune bibliotheque
# systeme et tournerait meme sur une image vide.
# -trimpath retire les chemins de la machine de build, -s -w les tables de
# symboles : plus leger, et rien qui renseigne un attaquant sur l'arborescence.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/realtime ./cmd/realtime

FROM alpine:3 AS runtime

# Un utilisateur sans privileges. Par defaut un conteneur tourne en root, et
# une execution de code arbitraire donnerait alors root sur le conteneur.
RUN addgroup -S ojino && adduser -S -G ojino ojino

WORKDIR /app
COPY --from=build /out/realtime ./realtime
USER ojino

EXPOSE 8090

# Demarrage quasi instantane, contrairement a une JVM : dix secondes de grace
# suffisent largement.
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget -qO- http://localhost:8090/health >/dev/null || exit 1

ENTRYPOINT ["./realtime"]
