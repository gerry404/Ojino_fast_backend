package hub

import (
	"sync"
)

// sendBuffer est la profondeur de la file de sortie d'un client.
//
// Assez pour absorber une rafale, assez peu pour qu'un client qui ne lit plus
// soit repere vite. Un tampon trop grand ne fait que retarder le probleme en
// consommant de la memoire.
const sendBuffer = 32

// Client est une connexion ouverte.
//
// La sortie passe par un canal plutot que par une ecriture directe : une
// WebSocket n'accepte qu'un seul ecrivain a la fois, et faire ecrire plusieurs
// goroutines dessus corrompt la trame.
type Client struct {
	ID     string
	UserID string

	send chan Event

	closeOnce sync.Once
	closed    chan struct{}
}

// NewClient cree un client pret a recevoir.
func NewClient(id, userID string) *Client {
	return &Client{
		ID:     id,
		UserID: userID,
		send:   make(chan Event, sendBuffer),
		closed: make(chan struct{}),
	}
}

// Send met un evenement en file.
//
// Ne bloque jamais : si la file est pleine, le client est trop lent et on le
// laisse tomber. Bloquer ici figerait le hub, et donc tous les autres clients,
// a cause d'un seul reseau lent.
func (c *Client) Send(event Event) bool {
	// Le canal ferme est teste a part, avant tout envoi. Le mettre dans le meme
	// select ne protegerait de rien : Go choisit au hasard parmi les cas prets,
	// et un envoi sur canal ferme panique au lieu d'etre simplement "non pret".
	select {
	case <-c.closed:
		return false
	default:
	}

	select {
	case c.send <- event:
		return true
	default:
		return false
	}
}

// Events expose la file de sortie a la boucle d'ecriture.
func (c *Client) Events() <-chan Event {
	return c.send
}

// Closed se ferme quand le client est retire du hub.
func (c *Client) Closed() <-chan struct{} {
	return c.closed
}

// Close libere le client. Appelable plusieurs fois sans danger : la deconnexion
// peut etre detectee des deux cotes en meme temps.
//
// Seul le canal de fermeture est ferme. Le canal d'envoi ne l'est jamais : en
// Go, seul l'emetteur peut fermer un canal sans risque, et ici plusieurs
// goroutines y ecrivent. Les evenements restes en file sont simplement
// abandonnes au ramasse-miettes avec le client.
func (c *Client) Close() {
	c.closeOnce.Do(func() {
		close(c.closed)
	})
}
