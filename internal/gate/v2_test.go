package gate

import (
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcprogue/internal/directive"
	"mcprogue/internal/journal"
	"mcprogue/internal/rejeu"
)

// jeuV2 lit tout ce que le gate fournit désormais à Decide : l'accès et
// les étiquettes de la cible (registre), les étiquettes et la source des
// consultations, le sous-agent déclaré.
const jeuV2 = `{
  "jeu": {"id": "gate-v2", "version": 1, "date_effet": "2026-01-01T00:00:00Z", "auteur": "juriste@cabinet.fr"},
  "directives": [
    {"id": "V-001", "version": 1, "statut": "active", "date_effet": "2026-01-01T00:00:00Z",
     "texte_fr": "Aucune action sans consultation préalable d'une source officielle.",
     "fondements": [{"texte": "Politique interne", "article": "1"}],
     "declencheur": {"evenement": "appel_outil", "cible": {"serveur": "*", "outil": "*", "acces": "action"}},
     "condition": {"non": {"session.a_consulte": {"etiquette": "officiel", "critere": "donnees_recues"}}},
     "effets": [{"effet": "refuser"}]},
    {"id": "V-002", "version": 1, "statut": "active", "date_effet": "2026-01-01T00:00:00Z",
     "texte_fr": "Le sous-agent espion ne lit pas les sources officielles.",
     "fondements": [{"texte": "Politique interne", "article": "2"}],
     "declencheur": {"evenement": "appel_outil", "cible": {"etiquette": "officiel"}},
     "condition": {"sous_agent_declare": {"egal": "espion"}},
     "effets": [{"effet": "refuser"}]},
    {"id": "V-003", "version": 1, "statut": "active", "date_effet": "2026-01-01T00:00:00Z",
     "texte_fr": "Une décision prise sur une donnée officielle rejouée est marquée.",
     "fondements": [{"texte": "Politique interne", "article": "3"}],
     "declencheur": {"evenement": "appel_outil", "cible": {"serveur": "banque-synthetique", "outil": "attribuer_pret"}},
     "condition": {"et": [
       {"metadonnee_reponse": {"etiquette": "officiel", "champ": "source.origine", "op": "==", "valeur": "rejeu"}},
       {"etiquette_cible": {"contient": "decision_credit"}}
     ]},
     "effets": [{"effet": "marquer", "etiquette": "donnee-rejouee"}]}
  ]
}`

func monterV2(t *testing.T, modifier func(*banc)) *banc {
	t.Helper()
	return monter(t, func(b *banc) {
		b.docJeu = jeuV2
		b.reg.Serveurs[0].Etiquettes = []string{"officiel", "externe"}
		b.reg.Serveurs[1].Outils[0].Etiquettes = []string{"decision_credit"}
		b.taux.rep = &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "taux"}},
			StructuredContent: map[string]any{"source": map[string]any{"origine": "rejeu", "date": "2026-10-01T00:00:00Z"}}}
		if modifier != nil {
			modifier(b)
		}
	})
}

func TestV2_EntreesDuRegistre(t *testing.T) {
	b := monterV2(t, nil)
	cs := b.agent(t)
	pret := map[string]any{"montant": 1000.0, "siren": "123456789"}

	// 1. Action sans consultation officielle : V-001 vraie, pas
	// indéterminée (l'accès et la version du registre sont fournis).
	r := appelerAgent(t, cs, "banque-synthetique__attribuer_pret", pret)
	if !r.IsError || !strings.Contains(texte(r), "V-001") {
		t.Fatalf("1 : %s", texte(r))
	}
	// 2. Le sous-agent déclaré atteint Decide.
	r = appelerSousAgent(t, cs, "taux-officiel__get_taux", map[string]any{"devise": "EUR"}, "espion")
	if !r.IsError || !strings.Contains(texte(r), "V-002") {
		t.Fatalf("2 : %s", texte(r))
	}
	// 3. Lecture officielle, qui déclare une source rejouée.
	if r = appelerAgent(t, cs, "taux-officiel__get_taux", map[string]any{"devise": "EUR"}); r.IsError {
		t.Fatalf("3 : %s", texte(r))
	}
	// 4. L'action suit une consultation étiquetée officiel : autorisée, et
	// marquée par V-003 (source rejouée, cible étiquetée decision_credit).
	if r = appelerAgent(t, cs, "banque-synthetique__attribuer_pret", pret); r.IsError {
		t.Fatalf("4 : %s", texte(r))
	}

	a := b.appels(t)
	if len(a) != 4 {
		t.Fatalf("%d appels", len(a))
	}
	version := b.reg.Version()
	for i, e := range a {
		if e.VersionRegistre != version || version == "" {
			t.Fatalf("appel %d : version %q, registre %q", i+1, e.VersionRegistre, version)
		}
	}
	if a[0].Acces != "action" || strings.Join(a[0].EtiquettesCible, ",") != "decision_credit" {
		t.Fatalf("appel 1 : accès %q, étiquettes %v", a[0].Acces, a[0].EtiquettesCible)
	}
	v001 := a[0].Decision.Applications[0]
	if v001.ID != "V-001" || v001.Resultat != "vrai" || v001.Indetermine || len(v001.Fondements) != 1 {
		t.Fatalf("appel 1 : application %+v", v001)
	}
	if a[1].SousAgentDeclare != "espion" || a[1].Decision.Effet != "refuser" {
		t.Fatalf("appel 2 : %+v", a[1])
	}
	if a[2].Acces != "read" || strings.Join(a[2].EtiquettesCible, ",") != "externe,officiel" || a[2].Decision.Effet != "autoriser" {
		t.Fatalf("appel 3 : accès %q, étiquettes %v, effet %s", a[2].Acces, a[2].EtiquettesCible, a[2].Decision.Effet)
	}
	// L'instantané de l'appel 4 porte la consultation complétée.
	if len(a[3].EtatSession) != 1 {
		t.Fatalf("appel 4 : état %+v", a[3].EtatSession)
	}
	c := a[3].EtatSession[0]
	if c.Seq != 3 || strings.Join(c.Etiquettes, ",") != "externe,officiel" || c.VersionRegistre != version ||
		c.Source == nil || c.Source.Origine != "rejeu" || c.Source.Date != "2026-10-01T00:00:00Z" {
		t.Fatalf("consultation %+v (source %+v)", c, c.Source)
	}
	d := a[3].Decision
	if d.Effet != "autoriser" || len(d.Marques) != 1 || d.Marques[0].Etiquette != "donnee-rejouee" || d.Marques[0].Directive != "V-003" {
		t.Fatalf("appel 4 : décision %+v", d)
	}
	b.rejouer(t)
}

// Une consultation dont le serveur ne déclare pas de source : la feuille
// metadonnee_reponse est indéterminée, donc la directive refuse par
// défaut. L'engine ne présente pas son marqueur non_declaree comme une
// origine.
func TestV2_SourceNonDeclareeIndeterminee(t *testing.T) {
	b := monterV2(t, func(b *banc) {
		b.taux.rep = &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "taux"}}}
	})
	cs := b.agent(t)
	appelerAgent(t, cs, "taux-officiel__get_taux", map[string]any{"devise": "EUR"})
	r := appelerAgent(t, cs, "banque-synthetique__attribuer_pret", map[string]any{"montant": 1.0, "siren": "1"})
	if !r.IsError || !strings.Contains(texte(r), "V-003") {
		t.Fatalf("%s", texte(r))
	}
	a := b.appels(t)
	if s := a[1].EtatSession[0].Source; s != nil {
		t.Fatalf("source consignée dans l'instantané : %+v", s)
	}
	var v003 *directive.Application
	for _, app := range a[1].Decision.VersDirective().Applications {
		if app.ID == "V-003" {
			v003 = &app
		}
	}
	if v003 == nil || v003.Resultat != directive.Indetermine || v003.Effet != directive.Refuser {
		t.Fatalf("V-003 : %+v", v003)
	}
	b.rejouer(t)
}

// Le gate refuse un jeu v2 dont une directive vise ou lit un nom que le
// registre ne connaît pas : étiquette jamais posée, serveur, outil.
func TestV2_CiblesHorsRegistre(t *testing.T) {
	jeu, err := directive.Charger([]byte(strings.NewReplacer(
		`"etiquette": "officiel", "critere"`, `"etiquette": "inconnue", "critere"`,
		`"cible": {"etiquette": "officiel"}`, `"cible": {"serveur": "absent", "outil": "x"}`,
		`{"etiquette_cible": {"contient": "decision_credit"}}`, `{"session.a_consulte": {"outil": "taux-officiel/absent", "critere": "transmis"}}`,
	).Replace(jeuV2)))
	if err != nil {
		t.Fatal(err)
	}
	b := monterV2(t, nil)
	err = verifierCibles(*jeu, b.reg)
	if !errors.Is(err, ErrCibleHorsRegistre) {
		t.Fatalf("erreur %v", err)
	}
	for _, attendu := range []string{"V-001 attend dans sa condition l'étiquette inconnue", "V-002 vise le serveur absent",
		"V-003 attend dans sa condition l'outil taux-officiel/absent"} {
		if !strings.Contains(err.Error(), attendu) {
			t.Errorf("%q absent de :\n%v", attendu, err)
		}
	}
	// Le jeu v2 d'origine passe.
	if err := verifierCibles(b.jeu, b.reg); err != nil {
		t.Fatal(err)
	}
}

// Un journal v2 se rejoue sans écart ; chaque champ v2 que Decide a lu, ou
// qu'il a produit, est contrôlé : l'altérer fait apparaître un écart.
func TestV2_AlterationsDetectees(t *testing.T) {
	b := monterV2(t, nil)
	cs := b.agent(t)
	pret := map[string]any{"montant": 1000.0, "siren": "123456789"}
	appelerAgent(t, cs, "banque-synthetique__attribuer_pret", pret)
	appelerAgent(t, cs, "taux-officiel__get_taux", map[string]any{"devise": "EUR"})
	appelerAgent(t, cs, "banque-synthetique__attribuer_pret", pret)
	b.rejouer(t)

	cas := map[string]func(appels []*journal.Entree){
		"accès":                     func(a []*journal.Entree) { a[0].Acces = "read" },
		"version du registre":       func(a []*journal.Entree) { a[0].VersionRegistre = "" },
		"étiquettes de la cible":    func(a []*journal.Entree) { a[2].EtiquettesCible = nil },
		"étiquettes consultées":     func(a []*journal.Entree) { a[2].EtatSession[0].Etiquettes = []string{"externe"} },
		"source consultée":          func(a []*journal.Entree) { a[2].EtatSession[0].Source = nil },
		"marque effacée":            func(a []*journal.Entree) { a[2].Decision.Marques = nil },
		"sous-agent":                func(a []*journal.Entree) { a[1].SousAgentDeclare = "espion" },
		"source de la consultation": func(a []*journal.Entree) { a[1].Resultat.Source.Origine = "direct" },
	}
	for nom, alterer := range cas {
		t.Run(nom, func(t *testing.T) {
			es := b.entrees(t)
			var appels []*journal.Entree
			for i := range es {
				if es[i].Type == journal.AppelT {
					appels = append(appels, &es[i])
				}
			}
			alterer(appels)
			rs, err := rejeu.RejouerJournal(es, b.coffre, rejeu.Catalogue{b.jeu}, rejeu.Options{})
			if err != nil {
				t.Fatal(err)
			}
			n := 0
			for _, r := range rs {
				n += len(r.Ecarts)
			}
			if n == 0 {
				t.Fatal("altération non détectée")
			}
		})
	}
}
