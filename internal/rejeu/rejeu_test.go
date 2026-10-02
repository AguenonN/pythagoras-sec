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
	journal    *journal.Ecrivain
	horloge    *time.Time
	entrees    []journal.Entree
}

var dem = journal.DemandePurge{Motif: "durée de conservation échue", Auteur: "dpo@cabinet.fr"}

// purger purge par le coffre (entrée purge chaînée) et relit le journal.
func (sc *scenario) purger(t *testing.T, empreintes ...string) {
	t.Helper()
	*sc.horloge = sc.horloge.Add(time.Hour)
	if p, err := sc.coffre.PurgerEmpreintes(dem, empreintes...); err != nil || len(p) != len(empreintes) {
		t.Fatalf("purge %v %v", p, err)
	}
	sc.relire(t)
}

func (sc *scenario) purges() Purges { return PurgesChainees(sc.entrees) }

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
	sc.coffre.Journaliser(j)
	sc.journal, sc.horloge = j, &maintenant
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
			if _, err := j.Ecrire(journal.Entree{
				TS: maintenant, Type: journal.TransmissionT, Session: tour.SessionID(), Seq: tour.Seq(), Appelant: etat.Appelant,
				Jeu: journal.DepuisJeu(sc.jeu), Serveur: a.serveur, Outil: a.outil,
				Appel:    &journal.Appel{EmpreinteTransmise: dec.EmpreinteTransmise},
				Decision: journal.DepuisDecision(dec),
			}); err != nil {
				t.Fatal(err)
			}
			c, _ := tour.Transmettre(context.Background(), a.serveur, a.outil, dec.EmpreinteTransmise,
				func(context.Context) session.Retour {
					return session.Retour{Resultat: directive.ResultatOK, EmpreinteReponse: "rep-" + a.outil}
				})
			res = &journal.Resultat{Statut: string(c.Resultat), EmpreinteReponse: c.EmpreinteReponse}
		}
		var approbation *journal.Approbation
		if dec.Effet == directive.AttendreValidation {
			approbation = &journal.Approbation{Mecanisme: "jeton", ApprouvePar: "juriste@cabinet.fr"}
		}
		if _, err := j.Ecrire(journal.Entree{
			TS: maintenant, Type: journal.AppelT, Session: tour.SessionID(), Seq: tour.Seq(), Appelant: etat.Appelant,
			Jeu: journal.DepuisJeu(sc.jeu), Serveur: a.serveur, Outil: a.outil,
			Appel:       &journal.Appel{EmpreinteOrigine: dec.EmpreinteOrigine, EmpreinteTransmise: dec.EmpreinteTransmise, ChampsRetires: dec.ChampsRetires},
			Attestation: journal.DepuisAttestation(attestee),
			EtatSession: journal.DepuisConsultations(etat.Consultations),
			Decision:    journal.DepuisDecision(dec),
			Approbation: approbation,
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
	sc.purger(t, en.Appel.EmpreinteOrigine)
	// La chaîne du journal reste vérifiable après la purge.
	if n, err := journal.Verifier(bytes.NewReader(sc.journalBuf.Bytes()), cle); err != nil || n != len(sc.entrees) {
		t.Fatalf("vérification %d %v", n, err)
	}
	r, err := Rejouer(en, sc.coffre, Catalogue{sc.jeu}, sc.purges(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Rejouable || r.Effet() != NonRejouable || r.Journalise != directive.Caviarder {
		t.Fatalf("rejeu %+v", r)
	}
	// Une purge conforme n'a pas de nature, dans aucun cadre : jamais une
	// violation, pas davantage une exposition.
	for _, o := range []Options{{}, {AuDate: t0.Add(24 * time.Hour)}} {
		r, _ := Rejouer(en, sc.coffre, Catalogue{sc.jeu}, sc.purges(), o)
		if r.Nature != "" || len(r.Ecarts) != 1 || r.Ecarts[0].Cause != CausePurge ||
			r.Ecarts[0].Controle != ControleArguments || r.Ecarts[0].Nature != "" {
			t.Fatalf("cadre %+v: %+v", o, r)
		}
	}
}

// Une purge conforme rejouée dans le cadre historique, sur tout le
// journal, n'apparaît jamais comme violation.
func TestRejouerJournal_PurgeConformeJamaisViolation(t *testing.T) {
	sc := derouler(t)
	var toutes []string
	for _, en := range sc.entrees {
		if en.Type == journal.AppelT {
			toutes = append(toutes, en.Appel.EmpreinteOrigine)
		}
	}
	sc.purger(t, toutes...)
	rs, err := RejouerJournal(sc.entrees, sc.coffre, Catalogue{sc.jeu}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 4 {
		t.Fatalf("%d rejeux", len(rs))
	}
	for _, r := range rs {
		if r.Rejouable || r.Nature == Violation {
			t.Fatalf("seq %d: %+v", r.Seq, r)
		}
		for _, e := range r.Ecarts {
			if e.Nature == Violation || e.Cause != CausePurge {
				t.Fatalf("seq %d: écart %+v", r.Seq, e)
			}
		}
	}
	cmp, err := Comparer(sc.entrees, sc.coffre, Catalogue{sc.jeu}, Options{}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range cmp.Lignes {
		for _, e := range l.Ecarts {
			if e.Nature == Violation {
				t.Fatalf("ligne %+v", l)
			}
		}
	}
}

func TestRejouer_PierreTombaleSansEntreePurgeEstUneAlteration(t *testing.T) {
	sc := derouler(t)
	en := sc.appel(t, 2)
	// Purge faite hors de la chaîne : le coffre écrit dans un autre journal.
	sc.coffre.Journaliser(journal.Nouveau(chaine.Nouvelle(&bytes.Buffer{}, nil)))
	sc.purger(t, en.Appel.EmpreinteOrigine)
	r, _ := Rejouer(en, sc.coffre, Catalogue{sc.jeu}, sc.purges(), Options{})
	if r.Rejouable || r.Nature != "" || len(r.Ecarts) != 1 || r.Ecarts[0].Cause != CauseAlteration ||
		r.Ecarts[0].Controle != ControleArguments || r.Ecarts[0].Nature != "" {
		t.Fatalf("rejeu %+v", r)
	}
	// Une entrée purge chaînée à une autre date ne couvre pas la pierre tombale.
	autre := Purges{en.Appel.EmpreinteOrigine: {t0}}
	if r, _ := Rejouer(en, sc.coffre, Catalogue{sc.jeu}, autre, Options{}); r.Ecarts[0].Cause != CauseAlteration {
		t.Fatalf("date différente: %+v", r.Ecarts)
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
	r, err := Rejouer(sc.appel(t, 4), c, Catalogue{sc.jeu}, nil, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Rejouable || len(r.Ecarts) != 1 || r.Ecarts[0].Cause != CauseAlteration ||
		r.Ecarts[0].Controle != ControleArguments || r.Ecarts[0].Nature != "" {
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
	r, _ := Rejouer(sc.appel(t, 1), coffreVide{}, Catalogue{sc.jeu}, nil, Options{})
	if r.Rejouable || len(r.Ecarts) != 1 || r.Ecarts[0].Cause != CauseAlteration {
		t.Fatalf("rejeu %+v", r)
	}
}

func TestRejouer_AlterationDuJournal(t *testing.T) {
	sc := derouler(t)
	jeux := Catalogue{sc.jeu}

	en := sc.appel(t, 4)
	en.Appel.EmpreinteTransmise = en.Appel.EmpreinteOrigine // caviardage effacé
	r, _ := Rejouer(en, sc.coffre, jeux, nil, Options{})
	if len(r.Ecarts) != 1 || r.Ecarts[0].Cause != CauseAlteration || r.Ecarts[0].Controle != ControleTransmis {
		t.Fatalf("transmis: %+v", r.Ecarts)
	}

	en = sc.appel(t, 3)
	en.Decision.Effet = string(directive.Autoriser) // refus maquillé
	r, _ = Rejouer(en, sc.coffre, jeux, nil, Options{})
	// Pris deux fois : une autorisation non transmise est incohérente, et
	// la décision rejouée est un refus.
	if len(r.Ecarts) != 2 || r.Ecarts[0].Controle != ControleStatut || r.Ecarts[1].Controle != ControleDecision ||
		r.Ecarts[1].Nature != Violation {
		t.Fatalf("décision: %+v", r.Ecarts)
	}

	en = sc.appel(t, 1)
	en.Jeu.Empreinte = "sha256:autre"
	r, _ = Rejouer(en, sc.coffre, jeux, nil, Options{})
	if r.Rejouable || len(r.Ecarts) != 1 || r.Ecarts[0].Controle != ControleJeu {
		t.Fatalf("jeu: %+v", r.Ecarts)
	}

	en = sc.appel(t, 1)
	en.Jeu.Version = 9
	if _, err := Rejouer(en, sc.coffre, jeux, nil, Options{}); err == nil {
		t.Fatal("un jeu introuvable est une erreur")
	}
	if _, err := Rejouer(journal.Entree{Type: journal.AlerteT}, sc.coffre, jeux, nil, Options{}); err != ErrPasUnAppel {
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
		if len(l.Ecarts) != 1 || l.Ecarts[0].Cause != CauseEffetDeRegle || l.Ecarts[0].Nature != Exposition {
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
			d.Arbre = &directive.Noeud{Argument: &directive.Condition{Champ: "/montant", Op: directive.OpSup, Valeur: 50000.0}}
			v2.Directives[i] = d
		}
	}
	var err error
	if v2.Meta.Empreinte, err = directive.Empreinte(v2.Directives); err != nil {
		t.Fatal(err)
	}
	sc.purger(t, sc.appel(t, 1).Appel.EmpreinteOrigine)

	cmp, err := Comparer(sc.entrees, sc.coffre, Catalogue{sc.jeu, v2}, Options{}, Options{Jeu: &v2})
	if err != nil {
		t.Fatal(err)
	}
	par := map[int]Ligne{}
	for _, l := range cmp.Lignes {
		par[l.Seq] = l
	}
	// Appel 1 purgé : avant établi par le journal, après non rejouable.
	if l := par[1]; l.Avant != string(directive.Autoriser) || l.Apres != NonRejouable || len(l.Ecarts) != 1 ||
		l.Ecarts[0].Cause != CausePurge || l.Ecarts[0].Nature != "" {
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

// Chaque appel transmis est précédé de son entrée transmission, et un appel
// non transmis n'en a pas : tout autre agencement est une altération.
func TestRejouerJournal_Transmission(t *testing.T) {
	sc := derouler(t)
	jeux := Catalogue{sc.jeu}
	// relu rend une copie fraîche du journal, qu'un cas peut altérer.
	relu := func() []journal.Entree {
		sc.relire(t)
		return sc.entrees
	}
	ecarts := func(es []journal.Entree, seq int) []Ecart {
		t.Helper()
		rs, err := RejouerJournal(es, sc.coffre, jeux, Options{})
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rs {
			if r.Seq == seq {
				return r.Ecarts
			}
		}
		t.Fatalf("pas de rejeu %d", seq)
		return nil
	}
	rang := func(es []journal.Entree, typ journal.Type, seq int) int {
		for i, e := range es {
			if e.Type == typ && e.Seq == seq {
				return i
			}
		}
		t.Fatalf("pas d'entrée %s %d", typ, seq)
		return -1
	}
	transmission := func(e []Ecart) bool {
		return len(e) == 1 && e[0].Cause == CauseAlteration && e[0].Controle == ControleTransmission
	}

	// Le journal honnête : aucune transmission orpheline, aucun écart.
	es := relu()
	if n := len(TransmissionsSansAppel(es)); n != 0 {
		t.Fatalf("%d orphelines", n)
	}
	for _, seq := range []int{1, 2, 3, 4} {
		if e := ecarts(es, seq); len(e) != 0 {
			t.Fatalf("seq %d : %+v", seq, e)
		}
	}

	// Transmission retirée d'un appel transmis.
	es = relu()
	i := rang(es, journal.TransmissionT, 1)
	es = append(es[:i:i], es[i+1:]...)
	if e := ecarts(es, 1); !transmission(e) || !strings.Contains(e[0].Detail, "0 entrée(s)") {
		t.Fatalf("retirée : %+v", e)
	}

	// Transmission ajoutée à un appel refusé par l'engine (seq 3).
	es = relu()
	faux := es[rang(es, journal.TransmissionT, 2)]
	faux.Seq = 3
	j := rang(es, journal.AppelT, 3)
	es = append(es[:j:j], append([]journal.Entree{faux}, es[j:]...)...)
	if e := ecarts(es, 3); !transmission(e) || !strings.Contains(e[0].Detail, "non transmis") {
		t.Fatalf("ajoutée : %+v", e)
	}

	// Empreinte transmise différente entre les deux entrées.
	es = relu()
	k := rang(es, journal.TransmissionT, 4)
	es[k].Appel = &journal.Appel{EmpreinteTransmise: "autre"}
	if e := ecarts(es, 4); !transmission(e) || !strings.Contains(e[0].Detail, "empreinte transmise") {
		t.Fatalf("empreinte : %+v", e)
	}

	// Transmission placée après l'appel.
	es = relu()
	k, l := rang(es, journal.TransmissionT, 2), rang(es, journal.AppelT, 2)
	es[k], es[l] = es[l], es[k]
	if e := ecarts(es, 2); !transmission(e) || !strings.Contains(e[0].Detail, "suit l'entrée appel") {
		t.Fatalf("ordre : %+v", e)
	}

	// Entrée appel disparue après la transmission : l'appel est parti sans
	// résultat journalisé, et cela se voit.
	es = relu()
	l = rang(es, journal.AppelT, 2)
	es = append(es[:l:l], es[l+1:]...)
	orph := TransmissionsSansAppel(es)
	if len(orph) != 1 || orph[0].Seq != 2 || orph[0].Serveur != "banque-synthetique" {
		t.Fatalf("orphelines %+v", orph)
	}
}

// Le statut du résultat doit être cohérent avec l'effet, même pour un appel
// qui n'est plus rejouable.
func TestRejouer_StatutEtEffet(t *testing.T) {
	sc := derouler(t)
	jeux := Catalogue{sc.jeu}
	cas := []struct {
		nom     string
		seq     int
		alterer func(*journal.Entree)
		veut    string
	}{
		{"refus transmis", 3, func(e *journal.Entree) { e.Resultat = &journal.Resultat{Statut: journal.StatutOK} }, "un refus n'est jamais transmis"},
		{"autorisation non transmise", 1, func(e *journal.Entree) { e.Resultat = &journal.Resultat{Statut: journal.StatutNonTransmis} }, "toujours transmise"},
		{"attente transmise sans approbation", 2, func(e *journal.Entree) { e.Approbation = nil }, "sans approbation"},
		{"sans résultat", 1, func(e *journal.Entree) { e.Resultat = nil }, "sans résultat"},
		{"effet inconnu", 1, func(e *journal.Entree) { e.Decision = &journal.Decision{Effet: "peut-etre"} }, "inconnu"},
	}
	for _, c := range cas {
		sc.relire(t)
		en := sc.appel(t, c.seq)
		c.alterer(&en)
		r, err := Rejouer(en, sc.coffre, jeux, nil, Options{})
		if err != nil {
			t.Fatalf("%s : %v", c.nom, err)
		}
		trouve := false
		for _, e := range r.Ecarts {
			if e.Controle == ControleStatut && e.Cause == CauseAlteration && strings.Contains(e.Detail, c.veut) {
				trouve = true
			}
		}
		if !trouve {
			t.Fatalf("%s : %+v", c.nom, r.Ecarts)
		}
	}
	// L'attente approuvée et transmise est cohérente.
	sc.relire(t)
	if r, _ := Rejouer(sc.appel(t, 2), sc.coffre, jeux, nil, Options{}); len(r.Ecarts) != 0 {
		t.Fatalf("%+v", r.Ecarts)
	}
}
