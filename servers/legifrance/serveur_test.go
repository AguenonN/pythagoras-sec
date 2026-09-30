package main

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"mcprogue/servers/internal/essai"
	"mcprogue/servers/internal/rejeu"
)

// Les tests tournent en REJEU (ou MIXTE sans identifiants, donc sans
// réseau) sur les enregistrements versionnés. Horloge fixée au 2026-09-30.

var le30Septembre = func() time.Time { return time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC) }

const (
	l3146v2   = "LEGIARTI000032303335"
	l3146v3   = "LEGIARTI000052190104"
	l31351cmf = "LEGIARTI000027643291"
)

func TestVersionEnVigueurEtSuivante(t *testing.T) {
	cs := essai.Connecter(t, Serveur(rejeu.Nouveau("enregistrements", rejeu.Rejeu), nil, le30Septembre))
	var out SortieArticle
	res := essai.Appeler(t, cs, "lire_article", map[string]any{"identifiant": l3146v2}, &out)
	if res.IsError || out.Statut != rejeu.StatutOK {
		t.Fatalf("%+v", out)
	}
	if out.Numero != "L314-6" || out.Texte != "Code de la consommation" || !strings.HasPrefix(out.Contenu, "Constitue un prêt usuraire") {
		t.Errorf("article: %q %q %.80q", out.Numero, out.Texte, out.Contenu)
	}
	if out.EnVigueur == nil || out.EnVigueur.Identifiant != l3146v2 || out.EnVigueur.Version != "2.0" || out.EnVigueur.DateFin != "2026-11-20" {
		t.Errorf("en vigueur: %+v", out.EnVigueur)
	}
	if out.Suivante == nil || out.Suivante.Identifiant != l3146v3 || out.Suivante.DateDebut != "2026-11-20" {
		t.Errorf("suivante: %+v", out.Suivante)
	}
	if out.Source == nil || out.Source.Origine != rejeu.OrigineRejeu {
		t.Errorf("source: %+v", out.Source)
	}
}

// Demander une version qui n'est pas en vigueur à la date sert celle qui
// l'est.
func TestVersionServieSelonLaDate(t *testing.T) {
	cs := essai.Connecter(t, Serveur(rejeu.Nouveau("enregistrements", rejeu.Rejeu), nil, le30Septembre))
	var out SortieArticle
	essai.Appeler(t, cs, "lire_article", map[string]any{"identifiant": l3146v2, "date_reference": "2026-12-01"}, &out)
	if out.Statut != rejeu.StatutOK || out.EnVigueur == nil || out.EnVigueur.Identifiant != l3146v3 || out.Suivante != nil {
		t.Fatalf("%+v", out)
	}
	if out.IdentifiantDemande != l3146v2 || out.Contenu == "" {
		t.Fatalf("%+v", out)
	}
}

func TestCodeMonetaireEtFinancier(t *testing.T) {
	cs := essai.Connecter(t, Serveur(rejeu.Nouveau("enregistrements", rejeu.Rejeu), nil, le30Septembre))
	var out SortieArticle
	essai.Appeler(t, cs, "lire_article", map[string]any{"identifiant": l31351cmf}, &out)
	if out.Statut != rejeu.StatutOK || out.Numero != "L313-5-1" || !strings.Contains(out.Contenu, "découverts en compte") || out.EnVigueur.Etat != "VIGUEUR" {
		t.Fatalf("%+v", out)
	}
}

func TestRefusEtIndisponible(t *testing.T) {
	cs := essai.Connecter(t, Serveur(rejeu.Nouveau("enregistrements", rejeu.Rejeu), nil, le30Septembre))
	for _, args := range []map[string]any{
		{"identifiant": "LEGIARTI00003230333"},
		{"identifiant": `LEGIARTI000032303335","x":"`},
		{"identifiant": l3146v2, "date_reference": "demain"},
	} {
		var out SortieArticle
		if res := essai.Appeler(t, cs, "lire_article", args, &out); !res.IsError || out.Statut != rejeu.StatutRefuse {
			t.Errorf("%v: %+v", args, out)
		}
	}
	var out SortieArticle
	if res := essai.Appeler(t, cs, "lire_article", map[string]any{"identifiant": "LEGIARTI000006419280"}, &out); !res.IsError || out.Statut != rejeu.StatutIndisponible {
		t.Errorf("non enregistré: %+v", out)
	}
}

// MIXTE sans identifiants PISTE : le jeton échoue avant tout réseau et
// l'enregistrement est servi, avec le motif.
func TestMixteSansJetonBasculeSurEnregistrement(t *testing.T) {
	t.Setenv("PISTE_CLIENT_ID", "")
	t.Setenv("PISTE_CLIENT_SECRET", "")
	j := &Jeton{HTTP: &http.Client{Transport: interdit{t}}, Maintenant: time.Now}
	c := rejeu.Nouveau("enregistrements", rejeu.Mixte)
	c.HTTP.Transport = interdit{t}
	cs := essai.Connecter(t, Serveur(c, j, le30Septembre))
	var out SortieArticle
	essai.Appeler(t, cs, "lire_article", map[string]any{"identifiant": l31351cmf}, &out)
	if out.Statut != rejeu.StatutOK || out.Source == nil || out.Source.Origine != rejeu.OrigineRejeu || !strings.Contains(out.Source.Motif, "PISTE_CLIENT_ID") {
		t.Fatalf("%+v %+v", out, out.Source)
	}
}

type interdit struct{ t *testing.T }

func (i interdit) RoundTrip(r *http.Request) (*http.Response, error) {
	i.t.Errorf("appel réseau interdit: %s", r.URL)
	return nil, http.ErrHandlerTimeout
}

func TestChoisir(t *testing.T) {
	vs := []Version{
		{Identifiant: "mort", Etat: "MODIFIE_MORT_NE", DateDebut: "2016-07-01", DateFin: "2016-03-27"},
		{Identifiant: "v2", Etat: "ABROGE_DIFF", DateDebut: "2016-07-01", DateFin: "2026-11-20"},
		{Identifiant: "v3", Etat: "VIGUEUR_DIFF", DateDebut: "2026-11-20", DateFin: "2999-01-01"},
	}
	for date, attendu := range map[string][2]string{
		"2026-09-30": {"v2", "v3"},
		"2026-11-19": {"v2", "v3"},
		"2026-11-20": {"v3", ""},
		"2010-01-01": {"", "v2"},
	} {
		v, s := choisir(vs, date)
		got := [2]string{}
		if v != nil {
			got[0] = v.Identifiant
		}
		if s != nil {
			got[1] = s.Identifiant
		}
		if got != attendu {
			t.Errorf("%s: %v, attendu %v", date, got, attendu)
		}
	}
}

func TestTexteBrut(t *testing.T) {
	got := texteBrut(`<p>Premier  alinéa&nbsp;:</p><p>1° l&#39;un ;<br/>2° l&apos;autre.</p>`)
	if got != "Premier alinéa :\n1° l'un ;\n2° l'autre." {
		t.Fatalf("%q", got)
	}
}

func TestSchema(t *testing.T) {
	cs := essai.Connecter(t, Serveur(rejeu.Nouveau("enregistrements", rejeu.Rejeu), nil, le30Septembre))
	essai.VerifierSchema(t, cs, "schema.json")
}
