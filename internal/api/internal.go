// Package api porte les routes HTTP appelees par les services Spring.
package api

import (
	"crypto/subtle"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gerry404/ojino-realtime/internal/config"
	"github.com/gerry404/ojino-realtime/internal/hub"
)

// InternalHandler recoit les evenements que les services Spring poussent vers
// les clients connectes.
type InternalHandler struct {
	hub    *hub.Hub
	logger *slog.Logger
}

// NewInternalHandler construit le point d'entree interne.
func NewInternalHandler(h *hub.Hub, logger *slog.Logger) *InternalHandler {
	return &InternalHandler{hub: h, logger: logger}
}

// PushRequest est ce qu'un service Spring envoie.
type PushRequest struct {
	UserID  string `json:"userId"  binding:"required"`
	Type    string `json:"type"    binding:"required"`
	Payload any    `json:"payload"`
}

// BroadcastRequest cible plusieurs destinataires d'un coup.
type BroadcastRequest struct {
	UserIDs []string `json:"userIds" binding:"required,min=1"`
	Type    string   `json:"type"    binding:"required"`
	Payload any      `json:"payload"`
}

// Push envoie un evenement a toutes les connexions d'un utilisateur.
//
// Une reponse 200 avec zero destinataire n'est pas une erreur : c'est quelqu'un
// qui n'est simplement pas connecte. L'appelant doit le savoir — pour retomber
// sur une notification push, par exemple — mais pas le traiter comme un echec.
func (h *InternalHandler) Push(c *gin.Context) {
	var request PushRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request"})
		return
	}

	delivered := h.hub.SendToUser(request.UserID,
		hub.NewEvent(request.Type, request.Payload))

	c.JSON(http.StatusOK, gin.H{
		"delivered": delivered,
		"online":    delivered > 0,
	})
}

// Broadcast envoie le meme evenement a plusieurs utilisateurs.
func (h *InternalHandler) Broadcast(c *gin.Context) {
	var request BroadcastRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request"})
		return
	}

	delivered := h.hub.Broadcast(request.UserIDs,
		hub.NewEvent(request.Type, request.Payload))

	c.JSON(http.StatusOK, gin.H{"delivered": delivered})
}

// Presence dit si un utilisateur est connecte.
//
// Sert au service de notification : quelqu'un qui a l'application ouverte n'a
// pas besoin d'une notification push, et lui en envoyer une le derangerait pour
// rien.
func (h *InternalHandler) Presence(c *gin.Context) {
	userID := c.Param("userId")

	c.JSON(http.StatusOK, gin.H{
		"userId":      userID,
		"online":      h.hub.IsOnline(userID),
		"connections": h.hub.ConnectionCount(userID),
	})
}

// RequireInternalToken protege les routes internes.
//
// Meme reserve que du cote Spring : un secret partage ne distingue pas les
// services entre eux. C'est une solution d'attente, a remplacer par une vraie
// identite de service quand ils se multiplieront.
func RequireInternalToken(cfg *config.Config) gin.HandlerFunc {
	expected := []byte(cfg.InternalToken)

	return func(c *gin.Context) {
		provided := []byte(c.GetHeader("X-Internal-Token"))

		// Comparaison a temps constant : avec une comparaison ordinaire, la
		// duree laisse fuiter le nombre de caracteres corrects, ce qui permet de
		// reconstituer le secret octet par octet.
		if subtle.ConstantTimeCompare(expected, provided) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized,
				gin.H{"code": "invalid_internal_token"})
			return
		}

		c.Next()
	}
}
