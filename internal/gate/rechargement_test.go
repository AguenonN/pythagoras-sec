package gate

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"mcprogue/internal/directive"
	"mcprogue/internal/journal"
	"mcprogue/internal/rejeu"
)

// jeuTestV2 est jeuTest en version 2, avec une directive de refus en plus :
// le taux officiel est désormais interdit.
var jeuTestV2 = strings.Replace(
	strings.Replace(jeuTest, `"id": "gate-test", "version": 1`, `"id": "gate-test", "version": 2`, 1),
	"\n  ]\n}", `,
    {"id": "D-009", "version": 1, "statut": "active", "date_effet": "2026-01-01T00:00:00Z",
     "texte_fr": "Le taux officiel ne doit plus être consulté.",
     "fondement": {"texte": "Circulaire interne", "article": "3"},
     "verrou": "interdiction", "cible": {"serveur": "taux-officiel", "outil": "get_taux"}, "effet": "refuser"}
  ]
}`, 1)

func versionsJeu(t *testing.T, doc string) directive.Jeu {
	t.Helper()
	j, err := directive.Charger([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	return *j
}

// jeuxCharges relève, dans l'ordre, les versions des entrées jeu_charge.
func jeuxCharges(es []journal.Entree) []int {
	var out []int
	for _, e := range es {
		if e.Type == journal.JeuCharge {
			out = append(out, e.Jeu.Version)
		}
	}
	return out
}

func (b *banc) rejouerAvec(t *testing.T, cat rejeu.Catalogue) []rejeu.Rejeu {
	t.Helper()
	rs, err := rejeu.RejouerJournal(b.entrees(t), b.coffre, cat, rejeu.Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rs {
		if !r.Rejouable || len(r.Ecarts) > 0 {
			t.Errorf("seq %d : rejouable %v, écarts %+v", r.Seq, r.Rejouable, r.Ecarts)
		}
	}
	return rs
}

// Pendant une session, une version qui ajoute une directive de refus :
// l'appel suivant est décidé sous la nouvelle version, le précédent reste
// rejouable sous l'ancienne. Puis retour arrière vers l'ancienne.
func TestRecharger_BasculeEtRetourArriere(t *testing.T) {
	b := monter(t, nil)
	v1, v2 := b.jeu, versionsJeu(t, jeuTestV2)
	cs := b.agent(t)

	if r := appelerAgent(t, cs, "taux-officiel__get_taux", map[string]any{"devise": "EUR"}); r.IsError {
		t.Fatalf("sous v1 : %q", texte(r))
	}

	res, err := b.g.Recharger([]byte(jeuTestV2))
	if err != nil || !res.Bascule || res.Jeu.Meta.Version != 2 || b.g.Jeu().Meta.Empreinte != v2.Meta.Empreinte {
		t.Fatalf("%+v %v", res.Jeu.Meta, err)
	}
	r := appelerAgent(t, cs, "taux-officiel__get_taux", map[string]any{"devise": "EUR"})
	if !r.IsError || !strings.Contains(texte(r), "D-009 v1") || !strings.Contains(texte(r), "Circulaire interne art. 3") {
		t.Fatalf("sous v2 : %q", texte(r))
	}

	// Retour arrière : la v1, à l'identique, redevient active.
	res, err = b.g.Recharger([]byte(jeuTest))
	if err != nil || !res.Bascule || b.g.Jeu().Meta.Empreinte != v1.Meta.Empreinte {
		t.Fatalf("%+v %v", res.Jeu.Meta, err)
	}
	if r := appelerAgent(t, cs, "taux-officiel__get_taux", map[string]any{"devise": "EUR"}); r.IsError {
		t.Fatalf("après retour arrière : %q", texte(r))
	}

	es := b.entrees(t)
	if got := jeuxCharges(es); fmt.Sprint(got) != "[1 2 1]" {
		t.Fatalf("jeu_charge %v", got)
	}
	var charge2 journal.Entree
	for _, e := range es {
		if e.Type == journal.JeuCharge && e.Jeu.Version == 2 {
			charge2 = e
		}
	}
	if charge2.Jeu.ID != "gate-test" || charge2.Jeu.Empreinte != v2.Meta.Empreinte || !strings.Contains(charge2.Message, "remplace gate-test v1") {
		t.Fatalf("jeu_charge v2 %+v", charge2)
	}

	as := b.appels(t)
	if len(as) != 3 || as[0].Jeu.Version != 1 || as[1].Jeu.Version != 2 || as[2].Jeu.Version != 1 {
		t.Fatalf("versions citées %d %d %d", as[0].Jeu.Version, as[1].Jeu.Version, as[2].Jeu.Version)
	}
	if as[0].Session != as[1].Session || as[1].Decision.Effet != "refuser" || as[2].Decision.Effet != "autoriser" {
		t.Fatalf("%+v", as)
	}

	// Chaque appel se rejoue sous la version qu'il cite, sans écart.
	rs := b.rejouerAvec(t, rejeu.Catalogue{v1, v2})
	if len(rs) != 3 || rs[0].Journalise != directive.Autoriser || rs[1].Journalise != directive.Refuser {
		t.Fatalf("%+v", rs)
	}
	// Et l'appel précédent la bascule est lié à la v1 : sans elle, il n'est
	// pas rejouable.
	if _, err := rejeu.RejouerJournal(b.entrees(t), b.coffre, rejeu.Catalogue{v2}, rejeu.Options{}); err == nil ||
		!strings.Contains(err.Error(), "gate-test v1 introuvable") {
		t.Fatalf("erreur %v", err)
	}
}

// Un document refusé laisse l'ancien jeu actif et écrit une alerte.
func TestRecharger_Refus(t *testing.T) {
	b := monter(t, nil)
	cs := b.agent(t)
	v1 := b.jeu.Meta.Empreinte

	autreID := strings.Replace(jeuTestV2, `"id": "gate-test"`, `"id": "autre-jeu"`, 1)
	v1Reecrite := strings.Replace(jeuTest, "Pas de prêt décidé", "Aucun prêt décidé", 1)
	cas := []struct {
		nom, doc string
		veut     error
		motif    string
	}{
		{"illisible", `{"jeu": `, nil, "document illisible"},
		{"invalide", strings.Replace(jeuTestV2, `"effet": "refuser"}
  ]`, `"effet": "autoriser"}
  ]`, 1), nil, "non admis"},
		{"autre identifiant", autreID, ErrJeuAutreID, "identifiant"},
		{"version réécrite", v1Reecrite, ErrVersionReecrite, "autre empreinte"},
	}
	for _, c := range cas {
		res, err := b.g.Recharger([]byte(c.doc))
		if err == nil || (c.veut != nil && !errors.Is(err, c.veut)) || res.Bascule {
			t.Fatalf("%s : %v", c.nom, err)
		}
		if b.g.Jeu().Meta.Empreinte != v1 {
			t.Fatalf("%s : le jeu actif a changé", c.nom)
		}
	}
	if _, err := b.g.RechargerFichier("/nexiste/pas.json"); err == nil {
		t.Fatal("fichier absent accepté")
	}

	// L'appel suivant est toujours décidé sous la v1.
	if r := appelerAgent(t, cs, "taux-officiel__get_taux", map[string]any{"devise": "EUR"}); r.IsError {
		t.Fatalf("%q", texte(r))
	}
	es := b.entrees(t)
	if got := jeuxCharges(es); fmt.Sprint(got) != "[1]" {
		t.Fatalf("jeu_charge %v", got)
	}
	alertes := 0
	for _, e := range es {
		if e.Type == journal.AlerteT && strings.Contains(e.Message, "rechargement du jeu refusé, gate-test v1 reste actif") {
			alertes++
		}
	}
	if alertes != len(cas)+1 {
		t.Fatalf("%d alertes", alertes)
	}
	b.rejouer(t)
}

// Un jeu identique au jeu actif ne bascule rien et n'écrit rien.
func TestRecharger_Identique(t *testing.T) {
	b := monter(t, nil)
	avant := len(b.entrees(t))
	res, err := b.g.Recharger([]byte(jeuTest))
	if err != nil || res.Bascule {
		t.Fatalf("%+v %v", res, err)
	}
	if apres := len(b.entrees(t)); apres != avant {
		t.Fatalf("%d entrées écrites", apres-avant)
	}
}

// Des rechargements concurrents d'appels de plusieurs sessions : chaque
// appel est décidé sous un seul jeu, celui qu'il cite, et le journal entier
// se rejoue sans écart.
func TestRecharger_BasculeAtomique(t *testing.T) {
	b := monter(t, nil)
	v1, v2 := b.jeu, versionsJeu(t, jeuTestV2)
	const agents, appels = 4, 10
	var wg sync.WaitGroup
	for i := 0; i < agents; i++ {
		cs := b.agent(t)
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < appels; j++ {
				appelerAgent(t, cs, "taux-officiel__get_taux", map[string]any{"devise": "EUR"})
			}
		}()
	}
	fini := make(chan struct{})
	go func() { wg.Wait(); close(fini) }()
	docs := []string{jeuTestV2, jeuTest}
	for k := 0; ; k++ {
		select {
		case <-fini:
		default:
			if _, err := b.g.Recharger([]byte(docs[k%2])); err != nil {
				t.Error(err)
			}
			time.Sleep(time.Millisecond)
			continue
		}
		break
	}

	rs := b.rejouerAvec(t, rejeu.Catalogue{v1, v2})
	if len(rs) != agents*appels {
		t.Fatalf("%d rejeux", len(rs))
	}
	vues := map[int]int{}
	for _, a := range b.appels(t) {
		vues[a.Jeu.Version]++
		veut := "autoriser"
		if a.Jeu.Version == 2 {
			veut = "refuser"
		}
		if a.Decision.Effet != veut {
			t.Fatalf("seq %d sous v%d : %s", a.Seq, a.Jeu.Version, a.Decision.Effet)
		}
	}
	if vues[1] == 0 || vues[2] == 0 {
		t.Fatalf("les bascules n'ont pas été entrelacées avec les appels : %v", vues)
	}
}

// Le retour à une version inférieure est permis, et son entrée jeu_charge
// le dit : retour_arriere, version de départ, version d'arrivée. Une
// montée de version ne porte aucune de ces marques.
func TestRecharger_RetourArriereMarque(t *testing.T) {
	b := monter(t, nil)
	cs := b.agent(t)
	for _, doc := range []string{jeuTestV2, jeuTest, jeuTestV2} {
		if res, err := b.g.Recharger([]byte(doc)); err != nil || !res.Bascule {
			t.Fatalf("%+v %v", res, err)
		}
		appelerAgent(t, cs, "taux-officiel__get_taux", map[string]any{"devise": "EUR"})
	}

	var charges []journal.Entree
	for _, e := range b.entrees(t) {
		if e.Type == journal.JeuCharge {
			charges = append(charges, e)
		}
	}
	if len(charges) != 4 {
		t.Fatalf("%d entrées jeu_charge", len(charges))
	}
	// Démarrage v1, montée v2, retour v1, montée v2.
	for i, e := range charges {
		retour := i == 2
		if e.RetourArriere != retour {
			t.Fatalf("jeu_charge %d (v%d) : retour_arriere %v", i, e.Jeu.Version, e.RetourArriere)
		}
		if !retour && (e.VersionDepart != 0 || e.VersionArrivee != 0) {
			t.Fatalf("jeu_charge %d : versions %d → %d sur une montée", i, e.VersionDepart, e.VersionArrivee)
		}
	}
	r := charges[2]
	if r.VersionDepart != 2 || r.VersionArrivee != 1 || r.Jeu.Version != 1 || !strings.Contains(r.Message, "retour arrière de v2 à v1") {
		t.Fatalf("retour arrière %+v", r)
	}
	// La ligne brute porte bien les trois champs.
	if !strings.Contains(string(b.journal.octets()), `"retour_arriere":true`) ||
		!strings.Contains(string(b.journal.octets()), `"version_depart":2`) ||
		!strings.Contains(string(b.journal.octets()), `"version_arrivee":1`) {
		t.Fatal("champs absents de la ligne écrite")
	}
	b.rejouerAvec(t, rejeu.Catalogue{b.jeu, versionsJeu(t, jeuTestV2)})
}
