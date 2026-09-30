package main

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcprogue/servers/internal/essai"
	"mcprogue/servers/internal/rejeu"
)

// Les tests tournent en REJEU sur l'enregistrement SYNTHÉTIQUE versionné
// (enregistrements/) : aucun réseau, aucune donnée réelle. Le registre
// complet réel vit dans enregistrements_locaux/, ignoré par Git.

var majSynthetique = flag.Bool("maj-synthetique", false, "réécrit enregistrements/ depuis registreSynthetique")

// registreSynthetique : entrées fictives, au format de l'API. Aucune ne
// désigne une personne, une entité ou un navire réels.
const registreSynthetique = `{"Publications":{"DatePublication":"2026-09-01T00:00:00+02:00","PublicationDetail":[` +
	`{"IdRegistre":900001,"Nature":"Personne morale","Nom":"SOCIETE FICTIVE ALPHA SANCTIONNEE","RegistreDetail":[` +
	`{"TypeChamp":"ALIAS","Valeur":[{"Alias":"ALPHA FICTIVE HOLDING"}]},` +
	`{"TypeChamp":"FONDEMENT_JURIDIQUE","Valeur":[{"FondementJuridique":1,"FondementJuridiqueLabel":"Règlement fictif 0000/001 (données de test)"}]},` +
	`{"TypeChamp":"REFERENCE_UE","Valeur":[{"ReferenceUe":"EU.FICTIF.1"}]}]},` +
	`{"IdRegistre":900002,"Nature":"Personne physique","Nom":"PERSONNE FICTIVE ZETA","RegistreDetail":[` +
	`{"TypeChamp":"FONDEMENT_JURIDIQUE","Valeur":[{"FondementJuridique":2,"FondementJuridiqueLabel":"Règlement fictif 0000/002 (données de test)"}]}]},` +
	`{"IdRegistre":900003,"Nature":"Navire","Nom":"NAVIRE FICTIF OMEGA","RegistreDetail":[` +
	`{"TypeChamp":"FONDEMENT_JURIDIQUE","Valeur":[{"FondementJuridique":3,"FondementJuridiqueLabel":"Règlement fictif 0000/003 (données de test)"}]}]}]}}`

const dateSynthetique = "2026-09-30T00:00:00Z"

// L'enregistrement versionné est exactement le registre synthétique ; avec
// -maj-synthetique, il est réécrit.
func TestEnregistrementSynthetique(t *testing.T) {
	dir := t.TempDir()
	if *majSynthetique {
		dir = "enregistrements"
	}
	chemin, err := rejeu.EcrireSynthetique(dir, URLRegistre, dateSynthetique, []byte(registreSynthetique))
	if err != nil {
		t.Fatal(err)
	}
	attendu, _ := os.ReadFile(chemin)
	fige, err := os.ReadFile(filepath.Join("enregistrements", filepath.Base(chemin)))
	if err != nil || !bytes.Equal(attendu, fige) {
		t.Fatalf("enregistrements/ ne contient pas le registre synthétique (relancer avec -maj-synthetique): %v", err)
	}
	fichiers, _ := filepath.Glob("enregistrements/*.json")
	if len(fichiers) != 1 {
		t.Fatalf("enregistrements/ doit contenir le seul registre synthétique, trouvé %v", fichiers)
	}
}

func TestCasPositifSynthetique(t *testing.T) {
	cs := essai.Connecter(t, Serveur(rejeu.Nouveau("enregistrements", rejeu.Rejeu)))
	var out SortieVerification
	res := essai.Appeler(t, cs, "verifier_gel", map[string]any{"nom": "Société fictive Alpha sanctionnée"}, &out)
	if res.IsError || out.Statut != rejeu.StatutOK || out.NombreCorrespondances == nil || *out.NombreCorrespondances != 1 {
		t.Fatalf("%+v", out)
	}
	c := (*out.Correspondances)[0]
	if c.IDRegistre != 900001 || c.Type != "exacte" || c.Nature != "Personne morale" || len(c.Fondements) == 0 {
		t.Fatalf("correspondance: %+v", c)
	}
	if out.DatePublicationRegistre != "2026-09-01T00:00:00+02:00" {
		t.Errorf("date de publication: %q", out.DatePublicationRegistre)
	}
	if out.Source == nil || out.Source.Origine != rejeu.OrigineRejeu || !out.Source.Synthetique || out.Avertissement == "" {
		t.Errorf("le caractère synthétique doit être visible: %+v %q", out.Source, out.Avertissement)
	}
}

// Couverture complète : les personnes physiques du registre sont cherchées.
func TestPersonnePhysiqueCouverte(t *testing.T) {
	cs := essai.Connecter(t, Serveur(rejeu.Nouveau("enregistrements", rejeu.Rejeu)))
	var out SortieVerification
	essai.Appeler(t, cs, "verifier_gel", map[string]any{"nom": "Personne fictive Zeta"}, &out)
	if out.Statut != rejeu.StatutOK || *out.NombreCorrespondances != 1 || (*out.Correspondances)[0].Nature != "Personne physique" {
		t.Fatalf("%+v", out)
	}
}

func TestAucuneCorrespondance(t *testing.T) {
	cs := essai.Connecter(t, Serveur(rejeu.Nouveau("enregistrements", rejeu.Rejeu)))
	var out SortieVerification
	res := essai.Appeler(t, cs, "verifier_gel", map[string]any{"nom": "Tricots Saint-James"}, &out)
	if res.IsError || out.Statut != rejeu.StatutOK || out.NombreCorrespondances == nil || *out.NombreCorrespondances != 0 ||
		out.Correspondances == nil || len(*out.Correspondances) != 0 || out.DatePublicationRegistre == "" || out.Source.Origine != rejeu.OrigineRejeu {
		t.Fatalf("attendu zéro correspondance explicite: %+v", out)
	}
}

// Enregistrement local absent en REJEU : indisponible, jamais « aucune
// correspondance ». Le résultat porte quand même date_publication_registre
// (vide) et source.origine (aucune).
func TestLocalAbsentIndisponible(t *testing.T) {
	cs := essai.Connecter(t, Serveur(rejeu.Nouveau(t.TempDir(), rejeu.Rejeu)))
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "verifier_gel", Arguments: map[string]any{"nom": "Personne fictive Zeta"}})
	if err != nil {
		t.Fatal(err)
	}
	brut := string(essai.JSON(t, res.StructuredContent))
	if !res.IsError {
		t.Fatalf("attendu isError: %s", brut)
	}
	for _, interdit := range []string{"nombre_correspondances", "correspondances"} {
		if strings.Contains(brut, `"`+interdit+`"`) {
			t.Errorf("un registre indisponible ne doit pas porter %q: %s", interdit, brut)
		}
	}
	for _, requis := range []string{`"statut":"indisponible"`, `"date_publication_registre":""`, `"origine":"aucune"`} {
		if !strings.Contains(brut, requis) {
			t.Errorf("résultat sans %s: %s", requis, brut)
		}
	}
}

func TestNomVide(t *testing.T) {
	cs := essai.Connecter(t, Serveur(rejeu.Nouveau("enregistrements", rejeu.Rejeu)))
	var out SortieVerification
	res := essai.Appeler(t, cs, "verifier_gel", map[string]any{"nom": " -- "}, &out)
	if !res.IsError || out.Statut != rejeu.StatutRefuse || out.Source == nil || out.Source.Origine != rejeu.OrigineAucune {
		t.Fatalf("attendu requete_refusee: %+v", out)
	}
}

func TestComparer(t *testing.T) {
	cas := []struct {
		cherche, nom, attendu string
	}{
		{"Alpha", "SOCIETE FICTIVE ALPHA", "partielle"},
		{"societe fictive alpha", "SOCIÉTÉ “FICTIVE” ALPHA", "exacte"},
		{"Société Générale", "SOCIETE GENERALE", "exacte"},
		{"Alph", "SOCIETE FICTIVE ALPHA", ""},
	}
	for _, c := range cas {
		if got := comparer(mots(c.cherche), mots(c.nom)); got != c.attendu {
			t.Errorf("comparer(%q, %q) = %q, attendu %q", c.cherche, c.nom, got, c.attendu)
		}
	}
}

// Le registre réel ne doit jamais être versionné : enregistrements_locaux/
// est ignoré par Git.
func TestDossierLocalIgnore(t *testing.T) {
	b, err := os.ReadFile(".gitignore")
	if err != nil || !strings.Contains(string(b), "enregistrements_locaux/") {
		t.Fatalf(".gitignore de gels-avoirs n'ignore pas enregistrements_locaux/: %v", err)
	}
	if filepath.Base(DossierLocal) != "enregistrements_locaux" {
		t.Fatalf("DossierLocal = %q", DossierLocal)
	}
}

func TestSchema(t *testing.T) {
	cs := essai.Connecter(t, Serveur(rejeu.Nouveau("enregistrements", rejeu.Rejeu)))
	essai.VerifierSchema(t, cs, "schema.json")
}
