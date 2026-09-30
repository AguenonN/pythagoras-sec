package main

import (
	"testing"

	"mcprogue/internal/nommage"

	"mcprogue/servers/internal/essai"
	"mcprogue/servers/internal/rejeu"
)

// Les tests tournent en REJEU sur les enregistrements versionnés : aucun
// réseau.

func TestEntreprisesDeDemo(t *testing.T) {
	c := rejeu.Nouveau("enregistrements", rejeu.Rejeu)
	cs := essai.Connecter(t, Serveur(c))
	cas := map[string]string{
		"405750753": "TRICOTS SAINT-JAMES",
		"382891752": "BIOCOOP",
		"508858230": "OPINEL DISTRIBUTION",
	}
	for siren, nom := range cas {
		var out SortieRecherche
		res := essai.Appeler(t, cs, "rechercher_entreprise", map[string]any{"siren": siren}, &out)
		if res.IsError || out.Statut != rejeu.StatutOK || out.Entreprise == nil {
			t.Fatalf("%s: %+v", siren, out)
		}
		e := out.Entreprise
		if e.Denomination != nom || e.Etat != "active" || e.PersonnePhysique || e.Siege == nil || e.FormeJuridiqueCode == "" {
			t.Errorf("%s: fiche inattendue %+v", siren, e)
		}
		if out.Source == nil || out.Source.Origine != rejeu.OrigineRejeu || out.Source.Date == "" {
			t.Errorf("%s: la provenance rejeu doit être visible: %+v", siren, out.Source)
		}
	}
}

func TestSansEnregistrementIndisponible(t *testing.T) {
	cs := essai.Connecter(t, Serveur(rejeu.Nouveau("enregistrements", rejeu.Rejeu)))
	var out SortieRecherche
	res := essai.Appeler(t, cs, "rechercher_entreprise", map[string]any{"siren": "732829320"}, &out)
	if !res.IsError || out.Statut != rejeu.StatutIndisponible || out.Message == "" {
		t.Fatalf("attendu indisponible explicite: %+v", out)
	}
}

func TestSirenInvalide(t *testing.T) {
	cs := essai.Connecter(t, Serveur(rejeu.Nouveau("enregistrements", rejeu.Rejeu)))
	for _, s := range []string{"12345678", "123456789", "40575075A"} {
		var out SortieRecherche
		res := essai.Appeler(t, cs, "rechercher_entreprise", map[string]any{"siren": s}, &out)
		if !res.IsError || out.Statut != rejeu.StatutRefuse {
			t.Errorf("%s: attendu requete_refusee, obtenu %+v", s, out)
		}
	}
}

func TestPasDePersonnePhysiqueEnregistree(t *testing.T) {
	if err := pasDePersonnePhysique([]byte(`{"results":[{"siren":"1","nature_juridique":"1000","statut_diffusion":"O"}]}`)); err == nil {
		t.Fatal("entrepreneur individuel enregistrable")
	}
	if err := pasDePersonnePhysique([]byte(`{"results":[{"siren":"1","nature_juridique":"5710","statut_diffusion":"P"}]}`)); err == nil {
		t.Fatal("diffusion partielle enregistrable")
	}
	e, err := extraire([]byte(`{"results":[{"siren":"123456782","nom_complet":"JEAN X","nature_juridique":"1000","statut_diffusion":"O","etat_administratif":"A","siege":{"siret":"1","adresse":"ICI"}}]}`), "123456782")
	if err != nil || e.Denomination != "" || e.Siege != nil || !e.PersonnePhysique {
		t.Fatalf("personne physique restituée: %+v %v", e, err)
	}
}

func TestNomLogique(t *testing.T) {
	if Nom != "recherche-entreprises" {
		t.Fatalf("nom logique %q", Nom)
	}
	if err := nommage.ValiderNomLogique(Nom); err != nil {
		t.Fatal(err)
	}
	if _, err := nommage.Exposer(Nom, "rechercher_entreprise"); err != nil {
		t.Fatal(err)
	}
}

func TestSchema(t *testing.T) {
	cs := essai.Connecter(t, Serveur(rejeu.Nouveau("enregistrements", rejeu.Rejeu)))
	essai.VerifierSchema(t, cs, "schema.json")
}
