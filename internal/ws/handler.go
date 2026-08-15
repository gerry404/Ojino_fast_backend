package ws

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/gerry404/ojino-realtime/internal/auth"
	"github.com/gerry404/ojino-realtime/internal/config"
	"github.com/gerry404/ojino-realtime/internal/hub"
)

// Handler accepte les connexions WebSocket.
type Handler struct {
	hub      *hub.Hub
	verifier *auth.Verifier
	config   *config.Config
	logger   *slog.Logger
	upgrader websocket.Upgrader
}

// NewHandler construit le point d'entree WebSocket.
func NewHandler(h *hub.Hub, verifier *auth.Verifier, cfg *config.Config,
	logger *slog.Logger) *Handler {

	return &Handler{
		hub:      h,
		verifier: verifier,
		config:   cfg,
		logger:   logger,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,

			// Le controle d'origine est explicite. Le defaut de la bibliotheque
			// refuse les origines croisees, ce qui casserait l'application web ;
			// tout accepter ouvrirait la porte a n'importe quel site.
			CheckOrigin: func(r *http.Request) bool {
				origin := r.Header.Get("Origin")
				if origin == "" {
					// Une application mobile n'envoie pas d'origine : ce n'est
					// pas un navigateur, la protection ne s'applique pas.
					return true
				}
				return cfg.AllowsOrigin(origin)
			},
		},
	}
}

// Serve authentifie puis passe la requete en WebSocket.
//
// L'authentification precede le passage a l'echelle du protocole : refuser
// apres avoir ouvert la socket obligerait a la refermer aussitot, et le client
// ne saurait pas pourquoi.
func (h *Handler) Serve(c *gin.Context) {
	claims, err := h.verifier.Verify(extractToken(c))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "invalid_token"})
		return
	}

	client := hub.NewClient(uuid.NewString(), claims.UserID)

	// Le plafond est verifie avant le passage en WebSocket, pour la meme
	// raison : mieux vaut un 429 lisible qu'une socket qui se ferme sans motif.
	if err := h.hub.Register(client); err != nil {
		h.logger.Warn("plafond de connexions atteint", "user", claims.UserID)
		c.JSON(http.StatusTooManyRequests, gin.H{"code": "too_many_connections"})
		return
	}

	socket, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		// Upgrade a deja repondu au client : il ne reste qu'a liberer le client
		// qu'on venait d'enregistrer.
		h.hub.Unregister(client)
		h.logger.Debug("passage en WebSocket impossible", "err", err)
		return
	}

	h.logger.Info("connexion ouverte",
		"user", claims.UserID, "client", client.ID)

	conn := NewConn(socket, client, h.hub, h.logger, nil)

	// L'ecriture part dans sa propre goroutine, la lecture garde celle-ci : la
	// requete HTTP doit rester vivante tant que la socket l'est.
	go conn.WritePump()
	conn.ReadPump()

	h.logger.Info("connexion fermee",
		"user", claims.UserID, "client", client.ID)
}

// extractToken lit le jeton, d'abord dans l'en-tete, sinon dans l'URL.
//
// Le parametre d'URL est necessaire : l'API WebSocket des navigateurs ne permet
// pas de poser d'en-tete. Il est moins bon — une URL se retrouve dans les
// journaux d'acces — d'ou l'ordre de preference.
func extractToken(c *gin.Context) string {
	header := c.GetHeader("Authorization")
	if after, found := strings.CutPrefix(header, "Bearer "); found {
		return after
	}
	return c.Query("token")
}
