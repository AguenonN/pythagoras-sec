package extension

// Preuve déterministe, hors ligne et sans Mistral, que chaque règle de
// l'opération ATLAS (demo/scenarios/jeux/operation.json) refuse son piège et
// laisse passer son témoin, en appelant directement l'engine (internal/directive.
// Decide). Complète les 3 scènes enregistrées avec Mistral (scenario_op_*_test.go)
// pour les règles dont l'enregistrement réel n'a pas été fait (budget Mistral).
// N'ajoute aucun code à l'engine : ce fichier n'est qu'un test.

import (
	"os"
	"testing"
	"time"

	d "mcprogue/internal/directive"
)

func chargerOperation(t *testing.T) d.Jeu {
	t.Helper()
	b, err := os.ReadFile("../../demo/scenarios/jeux/operation.json")
	if err != nil {
		t.Fatal(err)
	}
	jeu, err := d.Charger(b)
	if err != nil {
		t.Fatalf("operation.json refusé par le chargeur : %v", err)
	}
	return *jeu
}

// avant et après la date d'annonce (15/10/2026).
var (
	avant = time.Date(2026, 10, 4, 10, 0, 0, 0, time.UTC)
	apres = time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
)

func initie() *d.Demandeur  { return &d.Demandeur{Identite: "victor.ventoux@exemple.test", Groupes: []string{"inities-atlas"}} }
func equipe() *d.Demandeur  { return &d.Demandeur{Identite: "chloe.arceau@exemple.test", Groupes: []string{"equipe-atlas-cabinet", "cabinet-arceau"}} }
func dehors() *d.Demandeur  { return &d.Demandeur{Identite: "damien.arceau@exemple.test", Groupes: []string{"cabinet-arceau"}} }
func ordinaire() *d.Demandeur { return &d.Demandeur{Identite: "yanis.ventoux@exemple.test", Groupes: []string{"salaries-ventoux"}} }

// session ayant lu un document de la data room (donnees reçues).
func sessionDataRoomLue() d.EtatSession {
	return d.EtatSession{ID: "s", Consultations: []d.Consultation{
		{Serveur: "data-room", Outil: "lire_document", Resultat: d.ResultatOK, EmpreinteReponse: "sha256:x"},
	}}
}

func aUneMarque(dec d.Decision, etiquette string) bool {
	for _, m := range dec.Marques {
		if m.Etiquette == etiquette {
			return true
		}
	}
	return false
}

func aUneNotif(dec d.Decision, role string) bool {
	for _, n := range dec.Notifications {
		if n.DestinataireRole == role {
			return true
		}
	}
	return false
}

func TestOperation_D040_acces(t *testing.T) {
	jeu := chargerOperation(t)
	lire := func(doc string, dem *d.Demandeur) d.Decision {
		return d.Decide(jeu, d.EtatSession{ID: "s"}, d.Appel{
			Serveur: "data-room", Outil: "lire_document", Horodatage: avant,
			Arguments: map[string]any{"document_id": doc}, Demandeur: dem})
	}
	// Piège : un non-initié lit un document ATLAS -> refus + notif conformité.
	p := lire("ATLAS-DOC-02", dehors())
	if p.Effet != d.Refuser {
		t.Fatalf("D-040 piège : effet %q, attendu refuser", p.Effet)
	}
	if !aUneNotif(p, "conformite") {
		t.Fatalf("D-040 piège : conformité non notifiée")
	}
	// Témoin : un initié lit le même document -> autorisé (et tracé, D-046).
	tm := lire("ATLAS-DOC-02", initie())
	if tm.Effet != d.Autoriser {
		t.Fatalf("D-040 témoin (initié) : effet %q, attendu autoriser", tm.Effet)
	}
	if !aUneMarque(tm, "acces-information-privilegiee") {
		t.Fatalf("D-046 : lecture ATLAS non marquée au journal")
	}
	// Témoin : l'équipe cabinet lit aussi.
	if lire("ATLAS-DOC-02", equipe()).Effet != d.Autoriser {
		t.Fatalf("D-040 témoin (équipe cabinet) : attendu autoriser")
	}
}

func TestOperation_D046_tracabilite(t *testing.T) {
	jeu := chargerOperation(t)
	// Document public : lecture autorisée et NON marquée (D-046 fausse).
	dec := d.Decide(jeu, d.EtatSession{ID: "s"}, d.Appel{
		Serveur: "data-room", Outil: "lire_document", Horodatage: avant,
		Arguments: map[string]any{"document_id": "GEN-DOC-01"}, Demandeur: initie()})
	if dec.Effet != d.Autoriser {
		t.Fatalf("D-046 témoin : effet %q, attendu autoriser", dec.Effet)
	}
	if aUneMarque(dec, "acces-information-privilegiee") {
		t.Fatalf("D-046 témoin : document public marqué à tort")
	}
}

func TestOperation_D041_message_externe(t *testing.T) {
	jeu := chargerOperation(t)
	envoyer := func(dest string, etat d.EtatSession) d.Decision {
		return d.Decide(jeu, etat, d.Appel{
			Serveur: "messagerie", Outil: "envoyer_message", Horodatage: avant,
			Arguments: map[string]any{"destination": dest, "objet": "o", "corps": "c"}, Demandeur: equipe()})
	}
	// Piège : message externe dans une session qui a lu la data room -> refus.
	if dec := envoyer("externe", sessionDataRoomLue()); dec.Effet != d.Refuser || !aUneNotif(dec, "conformite") {
		t.Fatalf("D-041 piège : effet %q (notif conformité %v), attendu refuser+notif", dec.Effet, aUneNotif(dec, "conformite"))
	}
	// Témoin : externe sans lecture de la data room -> passe.
	if envoyer("externe", d.EtatSession{ID: "s"}).Effet != d.Autoriser {
		t.Fatalf("D-041 témoin (pas de lecture) : attendu autoriser")
	}
	// Témoin : interne même après lecture -> passe.
	if envoyer("interne", sessionDataRoomLue()).Effet != d.Autoriser {
		t.Fatalf("D-041 témoin (interne) : attendu autoriser")
	}
}

func TestOperation_D042_partage_externe(t *testing.T) {
	jeu := chargerOperation(t)
	partager := func(dest string, etat d.EtatSession) d.Decision {
		return d.Decide(jeu, etat, d.Appel{
			Serveur: "data-room", Outil: "partager_document", Horodatage: avant,
			Arguments: map[string]any{"document_id": "ATLAS-DOC-02", "destinataire": "x", "destination": dest}, Demandeur: equipe()})
	}
	if dec := partager("externe", sessionDataRoomLue()); dec.Effet != d.Refuser || !aUneNotif(dec, "conformite") {
		t.Fatalf("D-042 piège : effet %q, attendu refuser+notif", dec.Effet)
	}
	if partager("interne", sessionDataRoomLue()).Effet != d.Autoriser {
		t.Fatalf("D-042 témoin (interne) : attendu autoriser")
	}
}

func TestOperation_D043_ordre(t *testing.T) {
	jeu := chargerOperation(t)
	ordre := func(titre string, dem *d.Demandeur, quand time.Time, etat d.EtatSession) d.Decision {
		return d.Decide(jeu, etat, d.Appel{
			Serveur: "bourse", Outil: "passer_ordre", Horodatage: quand,
			Arguments: map[string]any{"titre": titre, "sens": "achat", "quantite": 300, "pour_le_compte_de": "x"}, Demandeur: dem})
	}
	vide := d.EtatSession{ID: "s"}
	// Piège : un initié achète BIOLYS avant l'annonce -> refus + notif.
	if dec := ordre("BIOLYS", initie(), avant, vide); dec.Effet != d.Refuser || !aUneNotif(dec, "conformite") {
		t.Fatalf("D-043 piège (initié) : effet %q, attendu refuser+notif", dec.Effet)
	}
	// Piège : session initiée (a lu la data room), demandeur ordinaire -> refus.
	if ordre("BIOLYS", ordinaire(), avant, sessionDataRoomLue()).Effet != d.Refuser {
		t.Fatalf("D-043 piège (session initiée) : attendu refuser")
	}
	// Témoin : titre hors opération -> passe.
	if ordre("ALPHA", initie(), avant, vide).Effet != d.Autoriser {
		t.Fatalf("D-043 témoin (ALPHA) : attendu autoriser")
	}
	// Témoin : non-initié, session non initiée -> passe.
	if ordre("BIOLYS", ordinaire(), avant, vide).Effet != d.Autoriser {
		t.Fatalf("D-043 témoin (non-initié) : attendu autoriser")
	}
	// Témoin : après l'annonce -> passe.
	if ordre("BIOLYS", initie(), apres, vide).Effet != d.Autoriser {
		t.Fatalf("D-043 témoin (après 15/10) : attendu autoriser")
	}
}

func TestOperation_D044_D045_communique(t *testing.T) {
	jeu := chargerOperation(t)
	publier := func(quand time.Time) d.Decision {
		return d.Decide(jeu, d.EtatSession{ID: "s"}, d.Appel{
			Serveur: "communication", Outil: "publier_communique", Horodatage: quand,
			Arguments: map[string]any{"communique_id": "COM-ATLAS-01", "canal": "diffuseur réglementé"},
			Demandeur: &d.Demandeur{Identite: "adrien.arceau@exemple.test", Groupes: []string{"equipe-atlas-cabinet"}}})
	}
	// Piège : avant l'annonce -> attente, deux rôles distincts, quatre yeux.
	p := publier(avant)
	if p.Effet != d.AttendreValidation || p.Validation == nil {
		t.Fatalf("D-044/045 piège : effet %q (validation %v), attendu attendre_validation", p.Effet, p.Validation)
	}
	roles := map[string]bool{}
	for _, r := range p.Validation.Roles {
		roles[r] = true
	}
	if !roles["direction-juridique"] || !roles["conformite"] {
		t.Fatalf("D-044/045 : rôles de validation %v, attendus direction-juridique ET conformite", p.Validation.Roles)
	}
	if !p.Validation.QuatreYeux {
		t.Fatalf("D-044/045 : quatre yeux attendu")
	}
	// Témoin : après l'annonce -> publication libre.
	if publier(apres).Effet != d.Autoriser {
		t.Fatalf("D-044/045 témoin (après 15/10) : attendu autoriser")
	}
}
