package gate

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcprogue/internal/annuaire"
	"mcprogue/internal/journal"
	"mcprogue/internal/rejeu"
)

const jeuValidation = `{
  "jeu": {"id": "gate-validation", "version": 1, "date_effet": "2026-01-01T00:00:00Z", "auteur": "juriste@cabinet.fr"},
  "directives": [
    {"id": "T-001", "version": 1, "statut": "active", "date_effet": "2026-01-01T00:00:00Z",
     "texte_fr": "Au-delà de 2 000 000 euros, validation du directeur des risques, quatre yeux.",
     "fondements": [{"texte": "Politique de crédit", "article": "7.1"}],
     "declencheur": {"evenement": "appel_outil", "cible": {"serveur": "banque-synthetique", "outil": "attribuer_pret"}},
     "condition": {"argument": {"champ": "/montant", "op": ">", "valeur": 2000000}},
     "effets": [{"effet": "attendre_validation", "par_role": "directeur-risques", "quatre_yeux": true}]},
    {"id": "T-002", "version": 1, "statut": "active", "date_effet": "2026-01-01T00:00:00Z",
     "texte_fr": "Au-delà de 5 000 000 euros, validation du juridique en plus.",
     "fondements": [{"texte": "Politique de crédit", "article": "7.2"}],
     "declencheur": {"evenement": "appel_outil", "cible": {"serveur": "banque-synthetique", "outil": "attribuer_pret"}},
     "condition": {"argument": {"champ": "/montant", "op": ">", "valeur": 5000000}},
     "effets": [{"effet": "attendre_validation", "par_role": "juridique"}]}
  ]
}`

type bancValidation struct {
	*banc
	cles *annuaire.Cles
	ann  *annuaire.Local
	cs   *mcp.ClientSession
}

// monterValidation : annuaire de démo, demandeur (vide : aucun jeton).
func monterValidation(t *testing.T, demandeur string) *bancValidation {
	t.Helper()
	l, cles := annuaireTest(t)
	b := monter(t, func(b *banc) {
		b.docJeu = jeuValidation
		jeton := ""
		if demandeur != "" {
			jeton, _ = annuaire.SignerIdentite(cles.Identites[demandeur], demandeur, demandeur, b.horloge, time.Hour)
		}
		b.config = func(c *Config) { c.Annuaire, c.JetonDemandeur = l, jeton }
	})
	return &bancValidation{banc: b, cles: cles, ann: l, cs: b.agent(t)}
}

// jeton signe une approbation à l'heure courante du banc.
func (v *bancValidation) jeton(t *testing.T, valideur, role, empreinte string) string {
	t.Helper()
	v.mu.Lock()
	h := v.horloge
	v.mu.Unlock()
	j, err := annuaire.SignerApprobation(v.cles.Identites[valideur], valideur, role, empreinte, h, 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return j
}

func (v *bancValidation) pret(t *testing.T, montant float64, jetons ...string) (*mcp.CallToolResult, Attente) {
	t.Helper()
	p := &mcp.CallToolParams{Name: "banque-synthetique__attribuer_pret", Arguments: map[string]any{"montant": montant, "siren": "1"}}
	switch len(jetons) {
	case 0:
	case 1:
		p.Meta = mcp.Meta{CleJetonApprobation: jetons[0]}
	default:
		l := make([]any, len(jetons))
		for i, j := range jetons {
			l[i] = j
		}
		p.Meta = mcp.Meta{CleJetonApprobation: l}
	}
	r, err := v.cs.CallTool(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	var a Attente
	if r.StructuredContent != nil {
		b, _ := json.Marshal(r.StructuredContent)
		json.Unmarshal(b, &a)
	}
	return r, a
}

func (v *bancValidation) approbations(t *testing.T) []journal.Entree {
	var out []journal.Entree
	for _, e := range v.entrees(t) {
		if e.Type == journal.ApprobationT {
			out = append(out, e)
		}
	}
	return out
}

const (
	claire = "claire.analyste@exemple.test"
	diane  = "diane.risques@exemple.test"
	marc   = "marc.ancien@exemple.test"
	julie  = "julie.juridique@exemple.test"
)

func TestValidation_RepriseAvecLeBonValideur(t *testing.T) {
	v := monterValidation(t, claire)
	r, a := v.pret(t, 3e6)
	if !r.IsError || a.Statut != StatutEnAttente || a.Directive != "T-001" || strings.Join(a.Roles, ",") != "directeur-risques" || !a.QuatreYeux {
		t.Fatalf("attente %+v %s", a, texte(r))
	}
	r, _ = v.pret(t, 3e6, v.jeton(t, diane, "directeur-risques", a.EmpreinteTransmise))
	if r.IsError {
		t.Fatalf("reprise : %s", texte(r))
	}
	if n := len(v.banque.appels()); n != 1 {
		t.Fatalf("%d transmissions", n)
	}
	ap := v.approbations(t)
	if len(ap) != 1 || ap[0].Approbation.Statut != journal.ApprobationAcceptee || ap[0].Seq != 2 ||
		len(ap[0].Approbation.Valideurs) != 1 || ap[0].Approbation.Valideurs[0].Identite != diane || ap[0].Approbation.ApprouvePar != diane {
		t.Fatalf("approbations %+v", ap)
	}
	appels := v.appels(t)
	if appels[1].Approbation == nil || appels[1].Resultat.Statut != journal.StatutOK || appels[1].Decision.Effet != "attendre_validation" {
		t.Fatalf("appel repris %+v", appels[1])
	}
	if strings.Contains(string(v.journal.octets()), "eyJ") {
		t.Fatal("un jeton est écrit en clair au journal")
	}
	v.rejouer(t)
	// Avec l'annuaire, le rejeu revérifie le rôle à la date de validation.
	rs, err := rejeu.RejouerJournal(v.entrees(t), v.coffre, rejeu.Catalogue{v.jeu}, rejeu.Options{Annuaire: v.ann})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rs {
		if len(r.Ecarts) != 0 {
			t.Fatalf("écarts %+v", r.Ecarts)
		}
	}
}

func TestValidation_Refus(t *testing.T) {
	cas := []struct {
		nom, demandeur string
		montant        float64
		jetons         func(v *bancValidation, emp string) []string
		motif          string
	}{
		{"rôle échu", claire, 3e6, func(v *bancValidation, e string) []string {
			return []string{v.jeton(t, marc, "directeur-risques", e)}
		}, "ne détenait pas le rôle"},
		{"rôle absent", claire, 3e6, func(v *bancValidation, e string) []string {
			return []string{v.jeton(t, julie, "directeur-risques", e)}
		}, "ne détenait pas le rôle"},
		{"valideur demandeur", diane, 3e6, func(v *bancValidation, e string) []string {
			return []string{v.jeton(t, diane, "directeur-risques", e)}
		}, "est le demandeur"},
		{"autre appel", claire, 3e6, func(v *bancValidation, e string) []string {
			return []string{v.jeton(t, diane, "directeur-risques", strings.Repeat("0", 64))}
		}, "autre appel"},
		{"sans demandeur, quatre yeux", "", 3e6, func(v *bancValidation, e string) []string {
			return []string{v.jeton(t, diane, "directeur-risques", e)}
		}, "demandeur n'est pas résolu"},
		{"rôle manquant", claire, 6e6, func(v *bancValidation, e string) []string {
			return []string{v.jeton(t, diane, "directeur-risques", e)}
		}, "rôle(s) sans validation : juridique"},
		{"jeton en trop", claire, 3e6, func(v *bancValidation, e string) []string {
			return []string{v.jeton(t, diane, "directeur-risques", e), v.jeton(t, julie, "juridique", e)}
		}, "un jeton par rôle"},
		{"illisible", claire, 3e6, func(v *bancValidation, e string) []string { return []string{"pas-un-jeton"} }, "jeton 1 refusé"},
	}
	for _, c := range cas {
		t.Run(c.nom, func(t *testing.T) {
			v := monterValidation(t, c.demandeur)
			_, a := v.pret(t, c.montant)
			r, a2 := v.pret(t, c.montant, c.jetons(v, a.EmpreinteTransmise)...)
			if !r.IsError || a2.Statut != StatutEnAttente || !strings.Contains(a2.MotifJeton, c.motif) {
				t.Fatalf("%+v %s", a2, texte(r))
			}
			if n := len(v.banque.appels()); n != 0 {
				t.Fatalf("%d transmissions", n)
			}
			ap := v.approbations(t)
			if len(ap) != 1 || ap[0].Approbation.Statut != journal.ApprobationRefusee || !strings.Contains(ap[0].Approbation.Motif, c.motif) {
				t.Fatalf("approbations %+v", ap)
			}
			v.rejouer(t)
		})
	}
}

// Deux rôles, deux valideurs distincts : transmis.
func TestValidation_DeuxRoles(t *testing.T) {
	v := monterValidation(t, claire)
	_, a := v.pret(t, 6e6)
	if strings.Join(a.Roles, ",") != "directeur-risques,juridique" {
		t.Fatalf("rôles %v", a.Roles)
	}
	r, _ := v.pret(t, 6e6, v.jeton(t, julie, "juridique", a.EmpreinteTransmise), v.jeton(t, diane, "directeur-risques", a.EmpreinteTransmise))
	if r.IsError {
		t.Fatalf("%s", texte(r))
	}
	v.rejouer(t)
}

// Le rejeu détecte une approbation falsifiée : valideur remplacé par un
// autre (dans l'appel et dans l'entrée approbation), rôle échu selon
// l'annuaire, ou demandeur validant lui-même.
func TestValidation_AlterationsDetectees(t *testing.T) {
	v := monterValidation(t, claire)
	_, a := v.pret(t, 3e6)
	v.pret(t, 3e6, v.jeton(t, diane, "directeur-risques", a.EmpreinteTransmise))
	for nom, c := range map[string]struct {
		valideur string
		annuaire bool
	}{"rôle échu": {marc, true}, "demandeur": {claire, false}} {
		t.Run(nom, func(t *testing.T) {
			es := v.entrees(t)
			for i := range es {
				if es[i].Approbation != nil {
					es[i].Approbation.Valideurs[0].Identite = c.valideur
				}
			}
			o := rejeu.Options{}
			if c.annuaire {
				o.Annuaire = v.ann
			}
			rs, _ := rejeu.RejouerJournal(es, v.coffre, rejeu.Catalogue{v.jeu}, o)
			if len(rs[1].Ecarts) == 0 || rs[1].Ecarts[0].Controle != rejeu.ControleApprobation {
				t.Fatalf("écarts %+v", rs[1].Ecarts)
			}
		})
	}
	// Approbation retirée de l'appel : statut incohérent.
	es := v.entrees(t)
	for i := range es {
		if es[i].Type == journal.AppelT {
			es[i].Approbation = nil
		}
	}
	rs, _ := rejeu.RejouerJournal(es, v.coffre, rejeu.Catalogue{v.jeu}, rejeu.Options{})
	if len(rs[1].Ecarts) == 0 {
		t.Fatal("approbation retirée non détectée")
	}
}
