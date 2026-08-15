package hub

import "testing"

// Le hub est le coeur du service : tout passe par lui, et une erreur ici fuit
// de la memoire a chaque connexion.

func TestRegisterAllowsSeveralConnectionsPerUser(t *testing.T) {
	h := New(3)

	// Quelqu'un peut avoir l'application ouverte sur son telephone et sur le
	// web : les deux doivent etre acceptees.
	for i, id := range []string{"a", "b", "c"} {
		if err := h.Register(NewClient(id, "user-1")); err != nil {
			t.Fatalf("connexion %d refusee : %v", i+1, err)
		}
	}

	if got := h.ConnectionCount("user-1"); got != 3 {
		t.Errorf("ConnectionCount = %d, attendu 3", got)
	}
}

func TestRegisterEnforcesThePerUserCap(t *testing.T) {
	h := New(2)
	_ = h.Register(NewClient("a", "user-1"))
	_ = h.Register(NewClient("b", "user-1"))

	// Sans plafond, un client qui boucle sur la reconnexion epuiserait la
	// memoire du serveur.
	if err := h.Register(NewClient("c", "user-1")); err != ErrTooManyConnections {
		t.Errorf("err = %v, attendu ErrTooManyConnections", err)
	}
}

func TestCapIsPerUserNotGlobal(t *testing.T) {
	h := New(1)
	_ = h.Register(NewClient("a", "user-1"))

	if err := h.Register(NewClient("b", "user-2")); err != nil {
		t.Errorf("le plafond d'un utilisateur ne doit pas gener les autres : %v", err)
	}
}

func TestUnregisterFreesTheSlot(t *testing.T) {
	h := New(1)
	client := NewClient("a", "user-1")
	_ = h.Register(client)

	h.Unregister(client)

	if err := h.Register(NewClient("b", "user-1")); err != nil {
		t.Errorf("la place n'a pas ete liberee : %v", err)
	}
}

func TestUnregisterRemovesTheEmptyUserEntry(t *testing.T) {
	h := New(2)
	client := NewClient("a", "user-1")
	_ = h.Register(client)
	h.Unregister(client)

	// Sans ce nettoyage, chaque utilisateur jamais revenu laisserait une entree
	// vide derriere lui.
	if users, conns := h.Stats(); users != 0 || conns != 0 {
		t.Errorf("Stats = (%d, %d), attendu (0, 0)", users, conns)
	}
}

func TestSendToUserReachesEveryConnection(t *testing.T) {
	h := New(3)
	phone := NewClient("phone", "user-1")
	web := NewClient("web", "user-1")
	_ = h.Register(phone)
	_ = h.Register(web)

	delivered := h.SendToUser("user-1", NewEvent(EventPong, nil))

	if delivered != 2 {
		t.Errorf("delivered = %d, attendu 2", delivered)
	}
	for name, client := range map[string]*Client{"phone": phone, "web": web} {
		if len(client.Events()) != 1 {
			t.Errorf("%s n'a pas recu l'evenement", name)
		}
	}
}

func TestSendToOfflineUserIsNotAnError(t *testing.T) {
	h := New(3)

	// Zero destinataire signifie "pas connecte", pas "echec".
	if delivered := h.SendToUser("fantome", NewEvent(EventPong, nil)); delivered != 0 {
		t.Errorf("delivered = %d, attendu 0", delivered)
	}
}

func TestSendNeverBlocksOnASaturatedClient(t *testing.T) {
	h := New(1)
	client := NewClient("a", "user-1")
	_ = h.Register(client)

	// On remplit la file sans jamais lire.
	for range sendBuffer {
		client.Send(NewEvent(EventPong, nil))
	}

    // Le test se terminerait en interblocage si Send attendait : un seul reseau
    // lent figerait le hub, et donc tous les autres clients.
	if delivered := h.SendToUser("user-1", NewEvent(EventPong, nil)); delivered != 0 {
		t.Errorf("delivered = %d, un client sature doit etre ignore", delivered)
	}
}

func TestSendToAClosedClientFails(t *testing.T) {
	client := NewClient("a", "user-1")
	client.Close()

	if client.Send(NewEvent(EventPong, nil)) {
		t.Error("un client ferme ne doit plus rien accepter")
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	client := NewClient("a", "user-1")

	// La deconnexion peut etre detectee des deux cotes en meme temps : une
	// double fermeture ferait paniquer sur un canal deja ferme.
	client.Close()
	client.Close()
}

func TestIsOnlineReflectsRegistration(t *testing.T) {
	h := New(2)
	client := NewClient("a", "user-1")

	if h.IsOnline("user-1") {
		t.Error("personne ne doit etre en ligne avant enregistrement")
	}

	_ = h.Register(client)
	if !h.IsOnline("user-1") {
		t.Error("l'utilisateur devrait etre en ligne")
	}

	h.Unregister(client)
	if h.IsOnline("user-1") {
		t.Error("l'utilisateur ne devrait plus etre en ligne")
	}
}

func TestBroadcastCountsEveryDelivery(t *testing.T) {
	h := New(2)
	_ = h.Register(NewClient("a", "user-1"))
	_ = h.Register(NewClient("b", "user-1"))
	_ = h.Register(NewClient("c", "user-2"))

	delivered := h.Broadcast([]string{"user-1", "user-2", "absent"},
		NewEvent(EventPong, nil))

	if delivered != 3 {
		t.Errorf("delivered = %d, attendu 3", delivered)
	}
}

func TestCloseAllEmptiesTheHub(t *testing.T) {
	h := New(2)
	client := NewClient("a", "user-1")
	_ = h.Register(client)

	h.CloseAll()

	if users, conns := h.Stats(); users != 0 || conns != 0 {
		t.Errorf("Stats = (%d, %d), attendu (0, 0)", users, conns)
	}
	// Les clients doivent etre fermes, pour qu'ils se reconnectent au lieu de
	// rester sur une socket morte.
	select {
	case <-client.Closed():
	default:
		t.Error("le client n'a pas ete ferme")
	}
}

// Sous -race, ce test attrape tout acces concurrent mal protege au registre.
func TestConcurrentRegisterAndSend(t *testing.T) {
	h := New(50)
	done := make(chan struct{})

	go func() {
		for i := range 100 {
			_ = h.Register(NewClient(string(rune(i)), "user-1"))
		}
		close(done)
	}()

	for range 100 {
		h.SendToUser("user-1", NewEvent(EventPong, nil))
		h.IsOnline("user-1")
		h.Stats()
	}
	<-done
}
