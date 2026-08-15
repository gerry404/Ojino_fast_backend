// Package hub tient le registre des connexions ouvertes.
package hub

import "time"

// Event est ce qui circule vers les clients.
//
// Une enveloppe unique plutot qu'un type par message : le client n'a qu'un seul
// format a decoder, et ajouter un evenement ne casse pas les clients deja
// deployes — ils ignoreront simplement un type qu'ils ne connaissent pas.
type Event struct {
	Type      string `json:"type"`
	Payload   any    `json:"payload,omitempty"`
	Timestamp int64  `json:"ts"`
}

// Les types d'evenements servis.
//
// Ce service ne les produit presque jamais lui-meme : ce sont les services
// Spring qui les poussent. Il ne fait que les acheminer.
const (
	// EventNotification relaie une notification en direct, quand le service de
	// notification en emet une pour un utilisateur connecte.
	EventNotification = "notification"

	// EventAssistantChunk transporte un morceau de reponse de l'assistant.
	EventAssistantChunk = "assistant.chunk"

	// EventAssistantDone signale la fin d'une reponse.
	EventAssistantDone = "assistant.done"

	// EventPresence signale qu'un membre d'une salle arrive ou repart.
	EventPresence = "presence"

	// EventRoomState transporte l'etat d'une salle d'etude.
	EventRoomState = "room.state"

	// EventPong repond a un ping applicatif du client.
	EventPong = "pong"

	// EventError signale une requete client mal formee, sans fermer la
	// connexion : une commande invalide ne doit pas couter la session.
	EventError = "error"
)

// NewEvent horodate a la creation.
func NewEvent(eventType string, payload any) Event {
	return Event{
		Type:      eventType,
		Payload:   payload,
		Timestamp: time.Now().UnixMilli(),
	}
}
