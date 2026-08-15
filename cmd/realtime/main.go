// Commande realtime : le service temps reel d'Ojino.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/gerry404/ojino-realtime/internal/api"
	"github.com/gerry404/ojino-realtime/internal/auth"
	"github.com/gerry404/ojino-realtime/internal/config"
	"github.com/gerry404/ojino-realtime/internal/hub"
	"github.com/gerry404/ojino-realtime/internal/ws"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	cfg, err := config.Load()
	if err != nil {
		// Refuser de demarrer plutot que de tourner avec une configuration
		// dangereuse : le probleme se voit tout de suite.
		logger.Error("configuration invalide", "err", err)
		os.Exit(1)
	}

	connections := hub.New(cfg.MaxConnectionsPerUser)
	verifier := auth.NewVerifier(cfg.JWTSecret, cfg.JWTIssuer)

	router := buildRouter(cfg, connections, verifier, logger)

	server := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: router,

		// Pas de WriteTimeout : il couperait les WebSocket au bout du delai,
		// alors qu'elles doivent rester ouvertes des heures. Les echeances sont
		// posees par connexion, dans le paquet ws.
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		logger.Info("service temps reel demarre", "port", cfg.Port)
		if err := server.ListenAndServe(); err != nil &&
			!errors.Is(err, http.ErrServerClosed) {
			logger.Error("le serveur s'est arrete", "err", err)
			os.Exit(1)
		}
	}()

	waitForShutdown(server, connections, logger)
}

func buildRouter(cfg *config.Config, connections *hub.Hub,
	verifier *auth.Verifier, logger *slog.Logger) *gin.Engine {

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())

	wsHandler := ws.NewHandler(connections, verifier, cfg, logger)
	internalHandler := api.NewInternalHandler(connections, logger)

	// L'unique point d'entree des clients.
	router.GET("/ws", wsHandler.Serve)

	// Les routes des services Spring, derriere le secret partage.
	internal := router.Group("/api/v1/internal")
	internal.Use(api.RequireInternalToken(cfg))
	{
		internal.POST("/push", internalHandler.Push)
		internal.POST("/broadcast", internalHandler.Broadcast)
		internal.GET("/presence/:userId", internalHandler.Presence)
	}

	// Sonde de sante, publique comme celle des services Spring.
	router.GET("/health", func(c *gin.Context) {
		users, conns := connections.Stats()
		c.JSON(http.StatusOK, gin.H{
			"status":      "UP",
			"users":       users,
			"connections": conns,
		})
	})

	return router
}

// waitForShutdown attend un signal puis ferme proprement.
//
// Les connexions sont fermees explicitement : sans cela, les clients resteraient
// sur une socket morte au lieu de se reconnecter tout de suite.
func waitForShutdown(server *http.Server, connections *hub.Hub,
	logger *slog.Logger) {

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("arret demande, fermeture des connexions")
	connections.CloseAll()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Error("arret force", "err", err)
	}
	logger.Info("service arrete")
}
