package rejeu

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcprogue/internal/chaine"
	"mcprogue/internal/directive"
	"mcprogue/internal/journal"
	"mcprogue/internal/session"
)

var (
	cle = []byte(strings.Repeat("k", 32))
	t0  = time.Date(2026, 10, 4, 9, 0, 0, 0, time.UTC)
)

func chargerJeu(t *testing.T) directive.Jeu {
	t.Helper()
	b, err := os.ReadFile("../directive/testdata/credit-pme.json")
	if err != nil {
		t.Fatal(err)
	}
	j, err := directive.Charger(b)
	if err != nil {
		t.Fatal(err)
	}
	if j.Meta.Empreinte == "" {
		t.Fatal("le chargeur doit poser l'empreinte")
	}
	return *j
}

type scenario struct {
	jeu        directive.Jeu
	coffre     *journal.Coffre
	cheminC    string
	journalBuf *bytes.Buffer
	entrees    []journal.Entree
}

var attestee = directive.Attestation{Mode: "binaire", Attendu: "a", Observe: "a", Schemas: true, Verifiee: true}

// derouler joue une session comme l'engine le fera : tour, Decide sur
// l'état de session, dépôt au coffre, transmission si l'effet le permet
// (l'attente de validation est supposée approuvée), entrée appel.
func derouler(t *testing.T) *scenario {
	t.Helper()
	sc := &scenario{jeu: chargerJeu(t), journalBuf: &bytes.Buffer{}}
	sc.cheminC = filepath.Join(t.TempDir(), "coffre.jsonl")
	var err error
	if sc.coffre, err = journal.OuvrirCoffre(sc.cheminC); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sc.coffre.Fermer() })
	j := journal.Nouveau(chaine.Nouvelle(sc.journalBuf, cle))
	maintenant := t0
	horloge := func() time.Time { return maintenant }
	j.Horloge(horloge)
	sc.coffre.Horloge(horloge)
	g := session.Nouveau(session.Config{Horloge: horloge}, j)
	ss := new(mcp.ServerSession)

	appels := []struct {
		serveur, outil string
		args           map[string]any
	}{
		{"taux-officiel", "get_taux", map[string]any{"devise": "EUR"}},
		{"banque-synthetique", "attribuer_pret", map[string]any{"montant": 100000.0, "siren": "552100554"}},
		{"banque-synthetique", "attribuer_pret", map[string]any{"montant": 600000.0, "siren": "552100554"}},
		{"recherche-entreprises", "verifier", map[string]any{"siren": "552100554", "nom": "Dupont"}},
	}
	for _, a := range appels {
		maintenant = maintenant.Add(time.Minute)
		tour, err := g.Entrer(context.Background(), ss, "spiffe://lab/agent")
		if err != nil {
			t.Fatal(err)
		}
		etat := tour.Etat()
		dec := directive.Decide(sc.jeu, etat, directive.Appel{
			Serveur: a.serveur, Outil: a.outil, Arguments: a.args, Attestation: attestee, Horodatage: maintenant,
		})
		if err := sc.coffre.Deposer(journal.Depot{Empreinte: dec.EmpreinteOrigine, Outil: a.outil,
			ArgumentsOrigine: a.args, ArgumentsTransmis: dec.Arguments}); err != nil {
			t.Fatal(err)
		}
		res := &journal.Resultat{Statut: journal.StatutNonTransmis}
		if dec.Effet != directive.Refuser {
			c, _ := tour.Transmettre(context.Background(), a.serveur, a.outil, dec.EmpreinteTransmise,
				func(context.Context) session.Retour {
					return session.Retour{Resultat: directive.ResultatOK, EmpreinteReponse: "rep-" + a.outil}
				})
			res = &journal.Resultat{Statut: string(c.Resultat), EmpreinteReponse: c.EmpreinteReponse}
		}
		if _, err := j.Ecrire(journal.Entree{
			TS: maintenant, Type: journal.AppelT, Session: tour.SessionID(), Seq: tour.Seq(), Appelant: etat.Appelant,
			Jeu: journal.DepuisJeu(sc.jeu), Serveur: a.serveur, Outil: a.outil,
			Appel:       &journal.Appel{EmpreinteOrigine: dec.EmpreinteOrigine, EmpreinteTransmise: dec.EmpreinteTransmise, ChampsRetires: dec.ChampsRetires},
			Attestation: journal.DepuisAttestation(attestee),
			EtatSession: journal.DepuisConsultations(etat.Consultations),
			Decision:    journal.DepuisDecision(dec),
			Resultat:    res,
		}); err != nil {
			t.Fatal(err)
		}
		tour.Terminer()
	}
	g.Fermer(ss, session.MotifClient)
	sc.relire(t)
	return sc
}

func (sc *scenario) relire(t *testing.T) {
	t.Helper()
	if _, err := journal.Verifier(bytes.NewReader(sc.journalBuf.Bytes()), cle); err != nil {
		t.Fatal(err)
	}
	var err error
	if sc.entrees, err = journal.Lire(bytes.NewReader(sc.journalBuf.Bytes())); err != nil {
		t.Fatal(err)
	}
}

func (sc *scenario) appel(t *testing.T, seq int) journal.Entree {
	t.Helper()
	for _, en := range sc.entrees {
		if en.Type == journal.AppelT && en.Seq == seq {
			return en
		}
	}
	t.Fatalf("pas d'appel %d", seq)
	return journal.Entree{}
}

func TestRejouer_CadreHistoriqueSansEcart(t *testing.T) {
	sc := derouler(t)
	rs, err := RejouerJournal(sc.entrees, sc.coffre, Catalogue{sc.jeu}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	attendus := []directive.Effet{directive.Autoriser, directive.AttendreValidation, directive.Refuser, directive.Caviarder}
	if len(rs) != len(attendus) {
		t.Fatalf("%d rejeux", len(rs))
	}
	for i, r := range rs {
		if !r.Rejouable || len(r.Ecarts) != 0 || r.Nature != Violation {
			t.Errorf("seq %d: %+v", r.Seq, r)
		}
		if r.Journalise != attendus[i] || r.Effet() != string(attendus[i]) || r.Rejouee != r.Origine {
			t.Errorf("seq %d: journalisé %s rejoué %s", r.Seq, r.Journalise, r.Effet())
		}
	}
}

func TestReconstituerEtat(t *testing.T) {
	sc := derouler(t)
	sid := sc.appel(t, 1).Session
	// Avant l'appel 4 : les appels 1 et 2 ont été transmis, le 3 refusé
	// par l'engine n'est pas une consultation.
	etat, err := ReconstituerEtat(sc.entrees, sid, 4)
	if err != nil {
		t.Fatal(err)
	}
	if etat.Appelant != "spiffe://lab/agent" || len(etat.Consultations) != 2 {
		t.Fatalf("état %+v", etat)
	}
	if c := etat.Consultations[0]; c.Seq != 1 || c.Serveur != "taux-officiel" || c.Resultat != directive.ResultatOK || c.EmpreinteReponse != "rep-get_taux" {
		t.Fatalf("consultation %+v", c)
	}
	if c := etat.Consultations[1]; c.Seq != 2 || c.Empreinte != sc.appel(t, 2).Appel.EmpreinteTransmise {
		t.Fatalf("consultation %+v", c)
	}
	if etat, _ := ReconstituerEtat(sc.entrees, sid, 1); len(etat.Consultations) != 0 {
		t.Fatal("avant le premier appel, rien n'a été consulté")
	}
	if _, err := ReconstituerEtat(sc.entrees, "inconnue", 1); err == nil {
		t.Fatal("une session jamais ouverte est une erreur")
	}
}

func TestRejouer_PurgeNommeeEtEntreeVerifiable(t *testing.T) {
	sc := derouler(t)
	en := sc.appel(t, 4)
	if _, err := sc.coffre.PurgerEmpreintes(en.Appel.EmpreinteOrigine); err != nil {
		t.Fatal(err)
	}
	// La chaîne du journal reste vérifiable après la purge.
	if n, err := journal.Verifier(bytes.NewReader(sc.journalBuf.Bytes()), cle); err != nil || n != len(sc.entrees) {
		t.Fatalf("vérification %d %v", n, err)
	}
	r, err := Rejouer(en, sc.coffre, Catalogue{sc.jeu}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Rejouable || r.Effet() != NonRejouable || r.Journalise != directive.Caviarder {
		t.Fatalf("rejeu %+v", r)
	}
	if len(r.Ecarts) != 1 || r.Ecarts[0].Cause != CausePurge || r.Ecarts[0].Controle != ControleArguments || r.Ecarts[0].Nature != Violation {
		t.Fatalf("écarts %+v", r.Ecarts)
	}
	// Sous une date forcée, le même écart est une exposition.
	r, _ = Rejouer(en, sc.coffre, Catalogue{sc.jeu}, Options{AuDate: t0.Add(24 * time.Hour)})
	if len(r.Ecarts) != 1 || r.Ecarts[0].Cause != CausePurge || r.Ecarts[0].Nature != Exposition {
		t.Fatalf("écarts %+v", r.Ecarts)
	}
}

func TestRejouer_AlterationDuCoffre(t *testing.T) {
	sc := derouler(t)
	sc.coffre.Fermer()
	brut, _ := os.ReadFile(sc.cheminC)
	if !bytes.Contains(brut, []byte("Dupont")) {
		t.Fatal("le coffre doit contenir l'argument")
	}
	os.WriteFile(sc.cheminC, bytes.ReplaceAll(brut, []byte("Dupont"), []byte("Durand")), 0o600)
	c, err := journal.OuvrirCoffre(sc.cheminC)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Fermer()
	r, err := Rejouer(sc.appel(t, 4), c, Catalogue{sc.jeu}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Rejouable || len(r.Ecarts) != 1 || r.Ecarts[0].Cause != CauseAlteration || r.Ecarts[0].Controle != ControleArguments {
		t.Fatalf("rejeu %+v", r)
	}
	if strings.Contains(r.Ecarts[0].Detail, "Durand") || strings.Contains(r.Ecarts[0].Detail, "Dupont") {
		t.Fatal("un écart ne cite jamais une valeur d'argument")
	}
}

type coffreVide struct{}

func (coffreVide) Arguments(string) (map[string]any, bool) { return nil, false }
func (coffreVide) Purgee(string) (time.Time, bool)         { return time.Time{}, false }

func TestRejouer_AbsenceSansPurgeEstUneAlteration(t *testing.T) {
	sc := derouler(t)
	r, _ := Rejouer(sc.appel(t, 1), coffreVide{}, Catalogue{sc.jeu}, Options{})
	if r.Rejouable || len(r.Ecarts) != 1 || r.Ecarts[0].Cause != CauseAlteration {
		t.Fatalf("rejeu %+v", r)
	}
}

func TestRejouer_AlterationDuJournal(t *testing.T) {
	sc := derouler(t)
	jeux := Catalogue{sc.jeu}

	en := sc.appel(t, 4)
	en.Appel.EmpreinteTransmise = en.Appel.EmpreinteOrigine // caviardage effacé
	r, _ := Rejouer(en, sc.coffre, jeux, Options{})
	if len(r.Ecarts) != 1 || r.Ecarts[0].Cause != CauseAlteration || r.Ecarts[0].Controle != ControleTransmis {
		t.Fatalf("transmis: %+v", r.Ecarts)
	}

	en = sc.appel(t, 3)
	en.Decision.Effet = string(directive.Autoriser) // refus maquillé
	r, _ = Rejouer(en, sc.coffre, jeux, Options{})
	if len(r.Ecarts) != 1 || r.Ecarts[0].Controle != ControleDecision || r.Ecarts[0].Nature != Violation {
		t.Fatalf("décision: %+v", r.Ecarts)
	}

	en = sc.appel(t, 1)
	en.Jeu.Empreinte = "sha256:autre"
	r, _ = Rejouer(en, sc.coffre, jeux, Options{})
	if r.Rejouable || len(r.Ecarts) != 1 || r.Ecarts[0].Controle != ControleJeu {
		t.Fatalf("jeu: %+v", r.Ecarts)
	}

	en = sc.appel(t, 1)
	en.Jeu.Version = 9
	if _, err := Rejouer(en, sc.coffre, jeux, Options{}); err == nil {
		t.Fatal("un jeu introuvable est une erreur")
	}
	if _, err := Rejouer(journal.Entree{Type: journal.AlerteT}, sc.coffre, jeux, Options{}); err != ErrPasUnAppel {
		t.Fatal(err)
	}
}

func TestRejouerJournal_InstantaneDeSessionAltere(t *testing.T) {
	sc := derouler(t)
	// On retire la consultation du taux de l'instantané de l'appel 2 : la
	// provenance ne déclenche plus, et la décision ne se retrouve plus.
	for i := range sc.entrees {
		if sc.entrees[i].Type == journal.AppelT && sc.entrees[i].Seq == 2 {
			sc.entrees[i].EtatSession = nil
		}
	}
	rs, err := RejouerJournal(sc.entrees, sc.coffre, Catalogue{sc.jeu}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	controles := map[string]bool{}
	for _, e := range rs[1].Ecarts {
		if e.Cause != CauseAlteration {
			t.Fatalf("écart %+v", e)
		}
		controles[e.Controle] = true
	}
	if !controles[ControleEtat] || !controles[ControleDecision] {
		t.Fatalf("écarts %+v", rs[1].Ecarts)
	}
}

func TestComparer_AuDateAvantLeJeu(t *testing.T) {
	sc := derouler(t)
	// Le 1er octobre, aucune directive n'est en vigueur : tout est autorisé.
	cmp, err := Comparer(sc.entrees, sc.coffre, Catalogue{sc.jeu}, Options{}, Options{AuDate: t0.AddDate(0, 0, -3)})
	if err != nil {
		t.Fatal(err)
	}
	a := string(directive.Autoriser)
	attendu := []Transition{
		{a, a, 1},
		{string(directive.Caviarder), a, 1},
		{string(directive.AttendreValidation), a, 1},
		{string(directive.Refuser), a, 1},
	}
	if len(cmp.Transitions) != len(attendu) {
		t.Fatalf("transitions %+v", cmp.Transitions)
	}
	for i := range attendu {
		if cmp.Transitions[i] != attendu[i] {
			t.Fatalf("transitions %+v", cmp.Transitions)
		}
	}
	for _, l := range cmp.Lignes {
		if l.Avant == l.Apres {
			if len(l.Ecarts) != 0 {
				t.Fatalf("ligne %+v", l)
			}
			continue
		}
		if len(l.Ecarts) != 1 || l.Ecarts[0].Cause != CauseDivergence || l.Ecarts[0].Nature != Exposition {
			t.Fatalf("ligne %+v", l)
		}
	}
}

func TestComparer_NouveauJeuEtPurge(t *testing.T) {
	sc := derouler(t)
	v2 := sc.jeu
	v2.Meta.Version = 2
	v2.Directives = append([]directive.Directive(nil), sc.jeu.Directives...)
	for i, d := range v2.Directives {
		if d.ID == "D-002" {
			d.Version = 2
			d.Condition = []directive.Condition{{Champ: "/montant", Op: directive.OpSup, Valeur: 50000.0}}
			v2.Directives[i] = d
		}
	}
	var err error
	if v2.Meta.Empreinte, err = directive.Empreinte(v2.Directives); err != nil {
		t.Fatal(err)
	}
	if _, err := sc.coffre.PurgerEmpreintes(sc.appel(t, 1).Appel.EmpreinteOrigine); err != nil {
		t.Fatal(err)
	}

	cmp, err := Comparer(sc.entrees, sc.coffre, Catalogue{sc.jeu, v2}, Options{}, Options{Jeu: &v2})
	if err != nil {
		t.Fatal(err)
	}
	par := map[int]Ligne{}
	for _, l := range cmp.Lignes {
		par[l.Seq] = l
	}
	// Appel 1 purgé : avant établi par le journal, après non rejouable.
	if l := par[1]; l.Avant != string(directive.Autoriser) || l.Apres != NonRejouable || len(l.Ecarts) != 1 || l.Ecarts[0].Cause != CausePurge {
		t.Fatalf("ligne 1 %+v", l)
	}
	// Appel 2 : 100 000 € passe sous le nouveau seuil.
	if l := par[2]; l.Avant != string(directive.AttendreValidation) || l.Apres != string(directive.Refuser) {
		t.Fatalf("ligne 2 %+v", l)
	}
	if l := par[3]; l.Avant != l.Apres || len(l.Ecarts) != 0 {
		t.Fatalf("ligne 3 %+v", l)
	}
	nb := 0
	for _, tr := range cmp.Transitions {
		nb += tr.Nombre
	}
	if nb != 4 || cmp.Transitions[0] != (Transition{string(directive.Autoriser), NonRejouable, 1}) {
		t.Fatalf("transitions %+v", cmp.Transitions)
	}
}
