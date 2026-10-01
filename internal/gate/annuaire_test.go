package gate

import (
	"context"
	"crypto/ed25519"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mcprogue/internal/annuaire"
	"mcprogue/internal/backend"
	"mcprogue/internal/backendtest"
	"mcprogue/internal/chaine"
	"mcprogue/internal/directive"
	"mcprogue/internal/journal"
	"mcprogue/internal/registre"
	"mcprogue/internal/rejeu"
)

// annuaireTest génère l'annuaire de démonstration, avec des clés neuves.
func annuaireTest(t *testing.T) (*annuaire.Local, *annuaire.Cles) {
	t.Helper()
	src, err := annuaire.LireSourceDemo(filepath.Join("..", "..", annuaire.FichierSourceDemo))
	if err != nil {
		t.Fatal(err)
	}
	d, cles, err := annuaire.Generer(*src, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := annuaire.Encoder(d)
	if err != nil {
		t.Fatal(err)
	}
	l, err := annuaire.Charger(b, cles.Autorite.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	return l, cles
}

// Au démarrage, la source d'identité et la référence de l'annuaire sont
// consignées au journal.
func TestAnnuaire_ConsigneAuDemarrage(t *testing.T) {
	l, _ := annuaireTest(t)
	b := monter(t, func(b *banc) { b.config = func(c *Config) { c.Annuaire = l } })
	var vu *journal.Entree
	for _, e := range b.entrees(t) {
		if e.Type == journal.AnnuaireCharge {
			vu = &e
		}
	}
	ref := l.Reference()
	if vu == nil || vu.SourceIdentite != "annuaire-local-demo" || vu.Annuaire == nil ||
		*vu.Annuaire != (journal.RefAnnuaire{ID: ref.ID, Version: ref.Version, Empreinte: ref.Empreinte}) {
		t.Fatalf("entrée annuaire_charge %+v", vu)
	}
	// Sans annuaire, pas d'entrée.
	for _, e := range monter(t, nil).entrees(t) {
		if e.Type == journal.AnnuaireCharge {
			t.Fatal("annuaire_charge sans annuaire")
		}
	}
}

const jeuDemandeur = `{
  "jeu": {"id": "gate-demandeur", "version": 1, "date_effet": "2026-01-01T00:00:00Z", "auteur": "juriste@cabinet.fr"},
  "directives": [
    {"id": "U-001", "version": 1, "statut": "active", "date_effet": "2026-01-01T00:00:00Z",
     "texte_fr": "Un prestataire externe n'attribue pas de prêt.",
     "fondements": [{"texte": "Politique interne", "article": "4"}],
     "declencheur": {"evenement": "appel_outil", "cible": {"serveur": "banque-synthetique", "outil": "attribuer_pret"}},
     "condition": {"demandeur": {"groupe": "prestataires-externes"}},
     "effets": [{"effet": "refuser"}]}
  ]
}`

// monterDemandeur monte un gate avec l'annuaire de démo et, si identite
// n'est pas vide, le jeton d'identité qu'elle signe pour elle-même.
func monterDemandeur(t *testing.T, identite string) *banc {
	t.Helper()
	l, cles := annuaireTest(t)
	return monter(t, func(b *banc) {
		b.docJeu = jeuDemandeur
		jeton := ""
		if identite != "" {
			var err error
			if jeton, err = annuaire.SignerIdentite(cles.Identites[identite], identite, identite, b.horloge, time.Hour); err != nil {
				t.Fatal(err)
			}
		}
		b.config = func(c *Config) { c.Annuaire, c.JetonDemandeur = l, jeton }
	})
}

func ouverture(t *testing.T, b *banc) journal.Entree {
	t.Helper()
	for _, e := range b.entrees(t) {
		if e.Type == journal.SessionOuverte {
			return e
		}
	}
	t.Fatal("pas de session_ouverte")
	return journal.Entree{}
}

func TestDemandeur_ResoluConsigneFourni(t *testing.T) {
	pret := map[string]any{"montant": 1.0, "siren": "1"}
	// Prestataire externe : refusé, demandeur consigné à l'ouverture et
	// dans l'appel.
	b := monterDemandeur(t, "bruno.prestataire@exemple.test")
	r := appelerAgent(t, b.agent(t), "banque-synthetique__attribuer_pret", pret)
	if !r.IsError || !strings.Contains(texte(r), "U-001") {
		t.Fatalf("%s", texte(r))
	}
	o := ouverture(t, b)
	if o.Demandeur == nil || o.Demandeur.Identite != "bruno.prestataire@exemple.test" ||
		strings.Join(o.Demandeur.Groupes, ",") != "prestataires-externes" || o.SourceIdentite != "annuaire-local-demo" || o.Annuaire == nil {
		t.Fatalf("session_ouverte %+v", o)
	}
	a := b.appels(t)[0]
	if a.Demandeur == nil || a.Demandeur.Identite != o.Demandeur.Identite {
		t.Fatalf("appel %+v", a.Demandeur)
	}
	b.rejouer(t)
	// Le demandeur de l'appel altéré : écart.
	es := b.entrees(t)
	for i := range es {
		if es[i].Type == journal.AppelT {
			es[i].Demandeur = &journal.Demandeur{Identite: "claire.analyste@exemple.test", Groupes: []string{"analystes-credit"}}
		}
	}
	rs, _ := rejeu.RejouerJournal(es, b.coffre, rejeu.Catalogue{b.jeu}, rejeu.Options{})
	if len(rs) != 1 || len(rs[0].Ecarts) == 0 {
		t.Fatalf("altération du demandeur non détectée : %+v", rs)
	}

	// Analyste : autorisé.
	b = monterDemandeur(t, "claire.analyste@exemple.test")
	if r := appelerAgent(t, b.agent(t), "banque-synthetique__attribuer_pret", pret); r.IsError {
		t.Fatalf("%s", texte(r))
	}
	b.rejouer(t)

	// Sans jeton : demandeur absent, la directive est indéterminée, donc
	// refus.
	b = monterDemandeur(t, "")
	r = appelerAgent(t, b.agent(t), "banque-synthetique__attribuer_pret", pret)
	if !r.IsError || !strings.Contains(texte(r), "U-001") {
		t.Fatalf("%s", texte(r))
	}
	if o := ouverture(t, b); o.Demandeur != nil || !strings.Contains(o.Message, "aucun jeton") {
		t.Fatalf("session_ouverte %+v", o)
	}
	if app := b.appels(t)[0].Decision.Applications[0]; app.Resultat != "indetermine" {
		t.Fatalf("application %+v", app)
	}
	b.rejouer(t)
}

// Un jeton refusé au lancement empêche le démarrage ; un jeton sans
// annuaire aussi.
func TestDemandeur_JetonRefuseAuLancement(t *testing.T) {
	l, cles := annuaireTest(t)
	h := time.Date(2026, 10, 4, 9, 0, 0, 0, time.UTC)
	perime, _ := annuaire.SignerIdentite(cles.Identites["claire.analyste@exemple.test"], "claire.analyste@exemple.test",
		"claire.analyste@exemple.test", h.Add(-3*time.Hour), time.Hour)
	for nom, cfg := range map[string]func(*Config){
		"périmé":        func(c *Config) { c.Annuaire, c.JetonDemandeur = l, perime },
		"sans annuaire": func(c *Config) { c.JetonDemandeur = perime },
	} {
		_, err := Demarrer(context.Background(), essaiConfig(t, cfg))
		if err == nil || !strings.Contains(err.Error(), "jeton du demandeur") {
			t.Errorf("%s : %v", nom, err)
		}
	}
}

// essaiConfig est une configuration minimale, modifiée par cfg.
func essaiConfig(t *testing.T, cfg func(*Config)) Config {
	t.Helper()
	jeu, err := directive.Charger([]byte(jeuDemandeur))
	if err != nil {
		t.Fatal(err)
	}
	reg := &registre.Registre{Meta: registre.MetaRegistre{ID: "test", Version: 1, Auteur: "ops"},
		Serveurs: []registre.Serveur{backendtest.HTTP("banque-synthetique", backendtest.Banque, "https://banque.lab/mcp", h64, "banque.lab")}}
	if err := registre.Valider(reg); err != nil {
		t.Fatal(err)
	}
	coffre, err := journal.OuvrirCoffre(filepath.Join(t.TempDir(), "coffre.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { coffre.Fermer() })
	c := Config{Registre: reg, Jeu: *jeu, Journal: journal.Nouveau(chaine.Nouvelle(&tampon{}, cle)), Coffre: coffre,
		Horloge: func() time.Time { return time.Date(2026, 10, 4, 9, 0, 0, 0, time.UTC) },
		Connecter: func(context.Context, registre.Serveur, backend.Options) (backend.Backend, error) {
			return &faux{nom: "banque-synthetique", att: attestee("tls-pin")}, nil
		}}
	cfg(&c)
	return c
}
