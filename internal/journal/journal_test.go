package journal

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mcprogue/internal/approval"
	"mcprogue/internal/chaine"
	"mcprogue/internal/directive"
)

var t0 = time.Date(2026, 10, 4, 9, 0, 0, 0, time.UTC)

func decisionExemple() directive.Decision {
	return directive.Decision{
		Effet: directive.AttendreValidation, Motif: "en attente",
		Arguments:        map[string]any{"siren": "1"},
		ChampsRetires:    []string{"/nom"},
		EmpreinteOrigine: "o", EmpreinteTransmise: "t",
		Applications: []directive.Application{{ID: "D-1", Version: 1, Verrou: directive.ApprobationHumaine,
			Fondement: directive.Fondement{Texte: "RGPD", Article: "22"}, Statut: directive.Active,
			Effet: directive.AttendreValidation, Declenchee: true, Appliquee: true, Motif: "validation humaine requise"}},
		Alertes: []string{"a"},
	}
}

func TestEcrivain_EcritChaineEtPublie(t *testing.T) {
	var buf bytes.Buffer
	cle := []byte(strings.Repeat("k", 32))
	e := Nouveau(chaine.Nouvelle(&buf, cle))
	e.Horloge(func() time.Time { return t0 })
	ch, stop := e.Abonner(4)
	defer stop()

	ecrite, err := e.Ecrire(Entree{Type: SessionOuverte, Session: "s1", Appelant: "spiffe://lab/a"})
	if err != nil {
		t.Fatal(err)
	}
	if ecrite.MAC == "" || ecrite.Prev != chaine.Genese() || !ecrite.TS.Equal(t0) {
		t.Fatalf("entrée écrite %+v", ecrite)
	}
	att := directive.Attestation{Mode: "binaire", Attendu: "a", Observe: "a", Schemas: true, Verifiee: false}
	appel, err := e.Ecrire(Entree{
		Type: AppelT, Session: "s1", Seq: 1, Appelant: "spiffe://lab/a",
		Jeu: &RefJeu{ID: "credit-pme", Version: 1, Empreinte: "sha256:x"},
		Serveur: "banque", Outil: "attribuer_pret", OutilExpose: "banque__attribuer_pret",
		Appel:       &Appel{EmpreinteOrigine: "o", EmpreinteTransmise: "t", ChampsRetires: []string{"/nom"}},
		Attestation: DepuisAttestation(att),
		EtatSession: DepuisConsultations([]directive.Consultation{{Seq: 0, Serveur: "taux", Outil: "get", Resultat: directive.ResultatOK, Empreinte: "e", Horodatage: t0}}),
		Decision:    DepuisDecision(decisionExemple()),
		Resultat:    &Resultat{Statut: StatutNonTransmis},
	})
	if err != nil {
		t.Fatal(err)
	}
	if appel.Prev != ecrite.MAC {
		t.Fatal("la seconde entrée doit s'accrocher à la première")
	}
	if !appel.Attestation.Valide {
		t.Error("l'attestation doit être recalculée valide malgré Verifiee=false")
	}

	// Les abonnés reçoivent le même format, dans l'ordre.
	r1, r2 := <-ch, <-ch
	if r1.Type != SessionOuverte || r2.Type != AppelT || r2.MAC != appel.MAC {
		t.Fatalf("abonné: %+v %+v", r1, r2)
	}

	// Relecture et vérification.
	if n, err := Verifier(bytes.NewReader(buf.Bytes()), cle); err != nil || n != 2 {
		t.Fatalf("vérification %d %v", n, err)
	}
	lues, err := Lire(bytes.NewReader(buf.Bytes()))
	if err != nil || len(lues) != 2 {
		t.Fatal(err)
	}
	d := lues[1].Decision.VersDirective()
	if d.Effet != directive.AttendreValidation || len(d.Applications) != 1 || d.Applications[0].ID != "D-1" {
		t.Errorf("décision relue %+v", d)
	}
	if cs := VersConsultations(lues[1].EtatSession); len(cs) != 1 || cs[0].Resultat != directive.ResultatOK {
		t.Errorf("consultations relues %+v", cs)
	}

	// Le journal ne porte aucun argument : seulement des empreintes.
	if bytes.Contains(buf.Bytes(), []byte(`"siren"`)) || bytes.Contains(buf.Bytes(), []byte(`"arguments`)) {
		t.Error("un argument est apparu dans le journal")
	}
}

func TestEcrivain_AbonneLentPerdSansBloquer(t *testing.T) {
	e := Nouveau(chaine.Nouvelle(&bytes.Buffer{}, nil))
	ch, stop := e.Abonner(1)
	for i := 0; i < 5; i++ {
		if _, err := e.Ecrire(Entree{Type: AlerteT, Message: "m"}); err != nil {
			t.Fatal(err)
		}
	}
	if e.Perdues() != 4 {
		t.Errorf("perdues %d, attendu 4", e.Perdues())
	}
	<-ch
	stop()
	stop() // idempotent
	if _, ouvert := <-ch; ouvert {
		t.Error("le canal doit être fermé au désabonnement")
	}
	if _, err := e.Ecrire(Entree{Type: AlerteT}); err != nil {
		t.Fatal("écrire après désabonnement doit fonctionner")
	}
}

func TestEcrivain_Nul(t *testing.T) {
	var e *Ecrivain
	if en, err := e.Ecrire(Entree{Type: AlerteT}); err != nil || en.Type != AlerteT {
		t.Fatal("un écrivain nul accepte tout sans rien faire")
	}
	if e.Perdues() != 0 {
		t.Fatal()
	}
	if _, err := Nouveau(chaine.Nouvelle(&bytes.Buffer{}, nil)).Ecrire(Entree{}); err == nil {
		t.Fatal("une entrée sans type est refusée")
	}
}

func TestOuvrir_ReprendLaChaine(t *testing.T) {
	chemin := filepath.Join(t.TempDir(), "journal.jsonl")
	e, err := Ouvrir(chemin, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Ecrire(Entree{Type: JeuCharge, Jeu: &RefJeu{ID: "j", Version: 1}}); err != nil {
		t.Fatal(err)
	}
	e.Fermer()
	e, err = Ouvrir(chemin, nil)
	if err != nil {
		t.Fatal(err)
	}
	en, err := e.Ecrire(Entree{Type: AlerteT, Message: "x"})
	if err != nil || en.Prev == chaine.Genese() {
		t.Fatalf("la reprise doit continuer la chaîne: %v %+v", err, en)
	}
	e.Fermer()
}

func TestCoffre(t *testing.T) {
	chemin := filepath.Join(t.TempDir(), "coffre.jsonl")
	c, err := OuvrirCoffre(chemin)
	if err != nil {
		t.Fatal(err)
	}
	args := map[string]any{"siren": "552100554", "nom": "Dupont"}
	emp := approval.Digest("verifier", args)
	if err := c.Deposer(Depot{Empreinte: emp, Outil: "verifier", ArgumentsOrigine: args, ArgumentsTransmis: map[string]any{"siren": "552100554"}}); err != nil {
		t.Fatal(err)
	}
	if err := c.Deposer(Depot{Empreinte: "sha-fausse", Outil: "verifier", ArgumentsOrigine: args}); err == nil {
		t.Fatal("une empreinte qui ne correspond pas doit être refusée")
	}
	if err := c.Deposer(Depot{Empreinte: emp, Outil: "verifier", ArgumentsOrigine: args}); err != nil {
		t.Fatal("un dépôt répété est ignoré sans erreur")
	}
	c.Fermer()

	c, err = OuvrirCoffre(chemin)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Fermer()
	got, ok := c.Arguments(emp)
	if !ok || got["nom"] != "Dupont" {
		t.Fatalf("relecture %v %v", got, ok)
	}
	if _, ok := c.Arguments("inconnue"); ok {
		t.Fatal()
	}
	var nul *Coffre
	if err := nul.Deposer(Depot{}); err != nil {
		t.Fatal("un coffre nul n'enregistre rien et ne se plaint pas")
	}
	// Le coffre ne contient qu'un dépôt malgré trois appels.
	b, _ := json.Marshal(c.index)
	if strings.Count(string(b), `"empreinte"`) != 1 {
		t.Errorf("index %s", b)
	}
}
