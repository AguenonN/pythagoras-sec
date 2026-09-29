package main

import (
	"encoding/json"
	"strings"
	"testing"

	"mcprogue/servers/internal/essai"
	"mcprogue/servers/internal/rejeu"
)

// Les tests tournent en REJEU sur l'enregistrement versionné : aucun réseau.

func TestCasPositifEntite(t *testing.T) {
	cs := essai.Connecter(t, Serveur(rejeu.Nouveau("enregistrements", rejeu.Rejeu)))
	var out SortieVerification
	res := essai.Appeler(t, cs, "verifier_gel", map[string]any{"nom": "JSC ROSOBORONEXPORT"}, &out)
	if res.IsError || out.Statut != rejeu.StatutOK || out.NombreCorrespondances == 0 {
		t.Fatalf("attendu une correspondance: %+v", out)
	}
	c := out.Correspondances[0]
	if c.IDRegistre != 4493 || c.Type != "exacte" || c.Nature != "Personne morale" || len(c.Fondements) == 0 {
		t.Fatalf("correspondance inattendue: %+v", c)
	}
	if out.Source == nil || out.Source.Origine != rejeu.OrigineRejeu || out.Source.Filtre == "" || out.Avertissement == "" {
		t.Fatalf("le rejeu partiel doit être visible: %+v %q", out.Source, out.Avertissement)
	}
	if out.DatePublicationRegistre == "" {
		t.Fatal("date de publication du registre absente")
	}
}

func TestCasNegatif(t *testing.T) {
	cs := essai.Connecter(t, Serveur(rejeu.Nouveau("enregistrements", rejeu.Rejeu)))
	var out SortieVerification
	res := essai.Appeler(t, cs, "verifier_gel", map[string]any{"nom": "Tricots Saint-James"}, &out)
	if res.IsError || out.Statut != rejeu.StatutOK || out.NombreCorrespondances != 0 || out.Correspondances == nil {
		t.Fatalf("attendu zéro correspondance: %+v", out)
	}
}

func TestNomVide(t *testing.T) {
	cs := essai.Connecter(t, Serveur(rejeu.Nouveau("enregistrements", rejeu.Rejeu)))
	var out SortieVerification
	res := essai.Appeler(t, cs, "verifier_gel", map[string]any{"nom": " -- "}, &out)
	if !res.IsError || out.Statut != rejeu.StatutRefuse {
		t.Fatalf("attendu requete_refusee: %+v", out)
	}
}

func TestSansEnregistrementIndisponible(t *testing.T) {
	cs := essai.Connecter(t, Serveur(rejeu.Nouveau(t.TempDir(), rejeu.Rejeu)))
	var out SortieVerification
	res := essai.Appeler(t, cs, "verifier_gel", map[string]any{"nom": "x"}, &out)
	if !res.IsError || out.Statut != rejeu.StatutIndisponible {
		t.Fatalf("attendu indisponible: %+v", out)
	}
}

func TestComparer(t *testing.T) {
	cas := []struct {
		cherche, nom, attendu string
	}{
		{"Rosoboronexport", "JSC ROSOBORONEXPORT", "partielle"},
		{"jsc rosoboronexport", "JSC “ROSOBORONEXPORT”", "exacte"},
		{"Société Générale", "SOCIETE GENERALE", "exacte"},
		{"Rosoboron", "JSC ROSOBORONEXPORT", ""},
	}
	for _, c := range cas {
		if got := comparer(mots(c.cherche), mots(c.nom)); got != c.attendu {
			t.Errorf("comparer(%q, %q) = %q, attendu %q", c.cherche, c.nom, got, c.attendu)
		}
	}
}

func TestFiltreRetireLesPersonnesPhysiques(t *testing.T) {
	brut := `{"Publications":{"DatePublication":"2026-01-01","PublicationDetail":[
	 {"IdRegistre":1,"Nature":"Personne physique","Nom":"DUPONT","RegistreDetail":[{"TypeChamp":"PRENOM","Valeur":[{"Prenom":"Jean"}]}]},
	 {"IdRegistre":2,"Nature":"Personne morale","Nom":"ACME","RegistreDetail":[
	   {"TypeChamp":"ALIAS","Valeur":[{"Alias":"ACME LLC","Commentaire":"dirigée par M. X"}]},
	   {"TypeChamp":"MOTIFS","Valeur":[{"Motifs":"détenue par M. X"}]},
	   {"TypeChamp":"FONDEMENT_JURIDIQUE","Valeur":[{"FondementJuridique":1,"FondementJuridiqueLabel":"R (UE) 269/2014"}]}]}]}}`
	f, err := filtrer([]byte(brut))
	if err != nil {
		t.Fatal(err)
	}
	for _, interdit := range []string{"DUPONT", "Jean", "M. X", "MOTIFS", "Commentaire"} {
		if strings.Contains(string(f), interdit) {
			t.Errorf("le registre filtré contient %q: %s", interdit, f)
		}
	}
	var p publication
	if err := json.Unmarshal(f, &p); err != nil || len(p.Publications.PublicationDetail) != 1 || p.Publications.DatePublication != "2026-01-01" {
		t.Fatalf("filtré inattendu: %s", f)
	}
}

func TestEnregistrementSansPersonnePhysique(t *testing.T) {
	c := rejeu.Nouveau("enregistrements", rejeu.Rejeu)
	rep, err := c.Obtenir(t.Context(), rejeu.Requete{URL: URLRegistre})
	if err != nil {
		t.Fatal(err)
	}
	var p publication
	if err := json.Unmarshal(rep.Corps, &p); err != nil {
		t.Fatal(err)
	}
	for _, e := range p.Publications.PublicationDetail {
		if e.Nature != "Personne morale" && e.Nature != "Navire" {
			t.Fatalf("entrée %d de nature %q dans l'enregistrement", e.IDRegistre, e.Nature)
		}
		for _, d := range e.RegistreDetail {
			if !champsConserves[d.TypeChamp] {
				t.Fatalf("champ %s dans l'enregistrement", d.TypeChamp)
			}
		}
	}
}

func TestSchema(t *testing.T) {
	cs := essai.Connecter(t, Serveur(rejeu.Nouveau("enregistrements", rejeu.Rejeu)))
	essai.VerifierSchema(t, cs, "schema.json")
}
