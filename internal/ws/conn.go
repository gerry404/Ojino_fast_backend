// Package ws porte la couche WebSocket : passage a l'echelle du protocole,
// boucles de lecture et d'ecriture.
package ws

import (
	"encoding/json"
	"log/slog"
	"time"

	"github.com/gorilla/websocket"

	"github.com/gerry404/ojino-realtime/internal/hub"
)

const (
	// writeWait borne l'ecriture d'une trame. Sans delai, un client dont le
	// reseau a disparu sans fermer la socket bloquerait la goroutine
	// indefiniment.
	writeWait = 10 * time.Second

	// pongWait est le temps accorde au client pour repondre a un ping.
	pongWait = 60 * time.Second

	// pingPeriod doit rester inferieur a pongWait, sinon le serveur declare le
	// client mort avant meme de lui avoir laisse le temps de repondre.
	pingPeriod = (pongWait * 9) / 10

	// maxMessageSize borne ce qu'un client peut envoyer. Ce service ne recoit
	// que de courtes commandes ; accepter davantage n'ouvrirait qu'une voie
	// pour saturer la memoire.
	maxMessageSize = 4096
)

// Conn relie une socket a un client du hub.
//
// Deux goroutines par connexion, et c'est le motif standard : une qui lit, une
// qui ecrit. Les melanger corromprait les trames, une WebSocket n'acceptant
// qu'un seul ecrivain a la fois.
type Conn struct {
	socket *websocket.Conn
	client *hub.Client
	hub    *hub.Hub
	logger *slog.Logger

	onCommand func(command Command)
}

// Command est ce qu'un client peut demander.
//
// Volontairement pauvre : ce service ne decide de rien. Le client s'abonne, se
// signale present, et c'est tout. Toute commande qui ressemblerait a une regle
// metier n'a rien a faire ici.
type Command struct {
	Action  string          `json:"action"`
	RoomID  string          `json:"roomId,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// Les actions acceptees.
const (
	ActionPing      = "ping"
	ActionJoinRoom  = "room.join"
	ActionLeaveRoom = "room.leave"
)

// NewConn cree le pont entre une socket et un client.
func NewConn(socket *websocket.Conn, client *hub.Client, h *hub.Hub,
	logger *slog.Logger, onCommand func(Command)) *Conn {

	return &Conn{
		socket:    socket,
		client:    client,
		hub:       h,
		logger:    logger,
		onCommand: onCommand,
	}
}

// ReadPump lit les commandes du client jusqu'a la fermeture.
//
// C'est elle qui detecte la deconnexion, y compris silencieuse, grace au
// mecanisme de ping/pong : un client qui ne repond plus est retire.
func (c *Conn) ReadPump() {
	defer func() {
		c.hub.Unregister(c.client)
		_ = c.socket.Close()
	}()

	c.socket.SetReadLimit(maxMessageSize)
	_ = c.socket.SetReadDeadline(time.Now().Add(pongWait))

	// Chaque pong repousse l'echeance : tant que le client repond, la connexion
	// reste ouverte, meme s'il n'envoie aucune commande.
	c.socket.SetPongHandler(func(string) error {
		return c.socket.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		_, message, err := c.socket.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err,
				websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				c.logger.Debug("fermeture inattendue",
					"user", c.client.UserID, "err", err)
			}
			return
		}

		var command Command
		if err := json.Unmarshal(message, &command); err != nil {
			// Une commande mal formee ne coute pas la session : on le signale et
			// on continue. Fermer serait puni pour une faute de frappe.
			c.client.Send(hub.NewEvent(hub.EventError,
				map[string]string{"message": "commande illisible"}))
			continue
		}

		c.handle(command)
	}
}

// WritePump ecrit les evenements et entretient le ping.
//
// Seule goroutine autorisee a ecrire sur la socket.
func (c *Conn) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.socket.Close()
	}()

	for {
		select {
		case <-c.client.Closed():
			// Le hub a ferme le client : on prend conge proprement, pour qu'il
			// sache qu'il doit se reconnecter plutot que d'attendre sur une
			// socket morte.
			_ = c.socket.SetWriteDeadline(time.Now().Add(writeWait))
			_ = c.socket.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			return

		case event := <-c.client.Events():
			_ = c.socket.SetWriteDeadline(time.Now().Add(writeWait))

			if err := c.socket.WriteJSON(event); err != nil {
				c.logger.Debug("ecriture impossible",
					"user", c.client.UserID, "err", err)
				return
			}

		case <-ticker.C:
			_ = c.socket.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.socket.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *Conn) handle(command Command) {
	switch command.Action {
	case ActionPing:
		c.client.Send(hub.NewEvent(hub.EventPong, nil))

	case ActionJoinRoom, ActionLeaveRoom:
		if c.onCommand != nil {
			c.onCommand(command)
		}

	default:
		c.client.Send(hub.NewEvent(hub.EventError,
			map[string]string{"message": "action inconnue : " + command.Action}))
	}
}
