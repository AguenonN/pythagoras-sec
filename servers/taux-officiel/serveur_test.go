package main

import (
	"bytes"
	"os"
	"testing"
	"time"

	"mcprogue/servers/internal/essai"
	"mcprogue/servers/internal/rejeu"
)

// Les tests tournent en REJEU sur les enregistrements versionnés (obtenus le
// 2026-09-29) : aucun réseau. L'horloge est fixée au 2026-09-30.

var le30Septembre = func() time.Time { return time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC) }

func TestSeuilEnVigueurEtSuivant(t *testing.T) {
	cs := essai.Connecter(t, Serveur(rejeu.Nouveau("enregistrements", rejeu.Rejeu), le30Septembre))
	var out SortieTaux
	res := essai.Appeler(t, cs, "get_taux_usure", map[string]any{"categorie": "decouvert-societes-non-financieres"}, &out)
	if res.IsError || out.Statut != rejeu.StatutOK || out.EnVigueur == nil {
		t.Fatalf("%+v", out)
	}
	if out.EnVigueur.Trimestre != "2026-Q3" || out.EnVigueur.SeuilPourcent != 19.0 || out.EnVigueur.DatePublication == "" {
		t.Errorf("en vigueur: %+v", out.EnVigueur)
	}
	if out.Suivant == nil || out.Suivant.Trimestre != "2026-Q4" || out.Suivant.SeuilPourcent != 19.08 {
		t.Errorf("suivant: %+v", out.Suivant)
	}
	if out.IdentifiantSerie != "MIR1.Q.FR.R.A2Z.A.U.A.2313FR.EUR.N" || out.DateReference != "2026-09-30" {
		t.Errorf("sortie: %+v", out)
	}
	if out.Source == nil || out.Source.Origine != rejeu.OrigineRejeu || out.Source.Date == "" {
		t.Errorf("la provenance rejeu doit être visible: %+v", out.Source)
	}
}

func TestToutesLesCategories(t *testing.T) {
	cs := essai.Connecter(t, Serveur(rejeu.Nouveau("enregistrements", rejeu.Rejeu), le30Septembre))
	for _, c := range Categories {
		var out SortieTaux
		res := essai.Appeler(t, cs, "get_taux_usure", map[string]any{"categorie": c.Code, "date_reference": "2026-10-15"}, &out)
		if res.IsError || out.EnVigueur == nil || out.EnVigueur.Trimestre != "2026-Q4" || out.Suivant != nil {
			t.Errorf("%s au 2026-10-15: %+v", c.Code, out)
		}
	}
}

func TestRefus(t *testing.T) {
	cs := essai.Connecter(t, Serveur(rejeu.Nouveau("enregistrements", rejeu.Rejeu), le30Septembre))
	for _, args := range []map[string]any{
		{"categorie": "immobilier", "date_reference": "2026-09-30"},
		{"categorie": "pret-relais", "date_reference": "30/09/2026"},
	} {
		var out SortieTaux
		res := essai.Appeler(t, cs, "get_taux_usure", args, &out)
		if !res.IsError {
			t.Errorf("%v accepté: %+v", args, out)
		}
	}
	// Trimestre futur non publié : introuvable, pas d'invention.
	var out SortieTaux
	essai.Appeler(t, cs, "get_taux_usure", map[string]any{"categorie": "pret-relais", "date_reference": "2027-06-01"}, &out)
	if out.Statut != rejeu.StatutIntrouvable || out.EnVigueur != nil {
		t.Errorf("futur: %+v", out)
	}
}

func TestSansEnregistrementIndisponible(t *testing.T) {
	cs := essai.Connecter(t, Serveur(rejeu.Nouveau(t.TempDir(), rejeu.Rejeu), le30Septembre))
	var out SortieTaux
	res := essai.Appeler(t, cs, "get_taux_usure", map[string]any{"categorie": "pret-relais"}, &out)
	if !res.IsError || out.Statut != rejeu.StatutIndisponible {
		t.Fatalf("%+v", out)
	}
}

func TestSelectionner(t *testing.T) {
	obs := []Seuil{
		{Trimestre: "2026-Q4", Debut: "2026-10-01", Fin: "2026-12-31"},
		{Trimestre: "2026-Q3", Debut: "2026-07-01", Fin: "2026-09-30"},
		{Trimestre: "2026-Q2", Debut: "2026-04-01", Fin: "2026-06-30"},
	}
	for date, attendu := range map[string]string{"2026-09-30": "2026-Q3", "2026-10-01": "2026-Q4", "2026-07-01": "2026-Q3", "2026-01-01": ""} {
		v, _ := Selectionner(obs, date)
		got := ""
		if v != nil {
			got = v.Trimestre
		}
		if got != attendu {
			t.Errorf("%s: %q, attendu %q", date, got, attendu)
		}
	}
}

// L'instantané versionné est bien celui que produisent les enregistrements.
func TestInstantaneVersionne(t *testing.T) {
	fige, err := os.ReadFile("instantane_trimestre_precedent.json")
	if err != nil {
		t.Fatal(err)
	}
	inst, err := genererInstantane(t.Context(), rejeu.Nouveau("enregistrements", rejeu.Rejeu), "2026-09-30")
	if err != nil {
		t.Fatal(err)
	}
	b, err := encoderInstantane(inst)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b, fige) {
		t.Fatal("instantane_trimestre_precedent.json diffère de ce que produisent les enregistrements ; régénérer avec « taux-officiel instantane -date 2026-09-30 »")
	}
	if inst.TrimestreEnVigueur != "2026-Q3" || inst.TrimestreInstantane != "2026-Q2" || len(inst.Seuils) != len(Categories) {
		t.Fatalf("instantané: %+v", inst)
	}
}

func TestSchema(t *testing.T) {
	cs := essai.Connecter(t, Serveur(rejeu.Nouveau("enregistrements", rejeu.Rejeu), le30Septembre))
	essai.VerifierSchema(t, cs, "schema.json")
}
