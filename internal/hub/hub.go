package hub

import (
	"errors"
	"sync"
)

// ErrTooManyConnections signale qu'un utilisateur a atteint son plafond.
var ErrTooManyConnections = errors.New("trop de connexions simultanees")

// Hub tient le registre des connexions, indexees par utilisateur.
//
// Un mutex plutot qu'une goroutine centrale avec des canaux : le registre est
// lu bien plus souvent qu'il n'est ecrit, et un RWMutex laisse les lectures se
// faire en parallele. Une boucle centrale serialiserait tout pour rien.
//
// Tout tient en memoire, et c'est assume : l'etat se reconstruit a la
// reconnexion. Persister des connexions n'aurait aucun sens — elles n'existent
// plus si le processus s'arrete.
type Hub struct {
	mu      sync.RWMutex
	clients map[string]map[string]*Client // userID -> clientID -> client

	maxPerUser int
}

// New cree un hub vide.
func New(maxPerUser int) *Hub {
	return &Hub{
		clients:    make(map[string]map[string]*Client),
		maxPerUser: maxPerUser,
	}
}

// Register ajoute une connexion.
//
// Le plafond par utilisateur n'est pas du confort : un client qui boucle sur la
// reconnexion sans fermer proprement epuiserait la memoire du serveur en
// quelques minutes.
func (h *Hub) Register(client *Client) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	connections, exists := h.clients[client.UserID]
	if !exists {
		connections = make(map[string]*Client)
		h.clients[client.UserID] = connections
	}

	if len(connections) >= h.maxPerUser {
		return ErrTooManyConnections
	}

	connections[client.ID] = client
	return nil
}

// Unregister retire une connexion et la ferme.
//
// La map de l'utilisateur est supprimee quand elle se vide : sans cela, chaque
// utilisateur jamais revenu laisserait une entree vide derriere lui.
func (h *Hub) Unregister(client *Client) {
	h.mu.Lock()

	if connections, exists := h.clients[client.UserID]; exists {
		delete(connections, client.ID)
		if len(connections) == 0 {
			delete(h.clients, client.UserID)
		}
	}
	h.mu.Unlock()

	// Hors du verrou : fermer un client ne touche pas au registre, et le garder
	// verrouille bloquerait tout le monde pendant la fermeture.
	client.Close()
}

// SendToUser pousse un evenement vers toutes les connexions d'un utilisateur.
//
// Toutes, et pas une seule : quelqu'un peut avoir l'application ouverte sur son
// telephone et sur le web, et les deux doivent voir la meme chose.
//
// Renvoie le nombre de connexions servies. Zero n'est pas une erreur : c'est
// simplement quelqu'un qui n'est pas connecte.
func (h *Hub) SendToUser(userID string, event Event) int {
	h.mu.RLock()
	targets := make([]*Client, 0, h.maxPerUser)
	for _, client := range h.clients[userID] {
		targets = append(targets, client)
	}
	h.mu.RUnlock()

	// L'envoi se fait hors du verrou : Send peut echouer sur un client sature,
	// et on ne veut pas tenir le registre pendant ce temps.
	delivered := 0
	for _, client := range targets {
		if client.Send(event) {
			delivered++
		}
	}
	return delivered
}

// Broadcast pousse un evenement vers une liste d'utilisateurs.
func (h *Hub) Broadcast(userIDs []string, event Event) int {
	delivered := 0
	for _, userID := range userIDs {
		delivered += h.SendToUser(userID, event)
	}
	return delivered
}

// IsOnline dit si un utilisateur a au moins une connexion ouverte.
func (h *Hub) IsOnline(userID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return len(h.clients[userID]) > 0
}

// ConnectionCount donne le nombre de connexions d'un utilisateur.
func (h *Hub) ConnectionCount(userID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return len(h.clients[userID])
}

// Stats resume l'etat du hub, pour la sonde de sante.
func (h *Hub) Stats() (users int, connections int) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, clients := range h.clients {
		users++
		connections += len(clients)
	}
	return users, connections
}

// CloseAll ferme toutes les connexions. Utilise a l'arret du service, pour que
// les clients se reconnectent au lieu de rester sur une socket morte.
func (h *Hub) CloseAll() {
	h.mu.Lock()
	all := make([]*Client, 0)
	for _, connections := range h.clients {
		for _, client := range connections {
			all = append(all, client)
		}
	}
	h.clients = make(map[string]map[string]*Client)
	h.mu.Unlock()

	for _, client := range all {
		client.Close()
	}
}
