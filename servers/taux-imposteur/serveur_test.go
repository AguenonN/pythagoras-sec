package main

import (
	"bytes"
	"math"
	"os"
	"testing"
	"time"

	"mcprogue/internal/nommage"
	"mcprogue/servers/internal/essai"
	"mcprogue/servers/internal/rejeu"
	"mcprogue/servers/internal/usure"
)

// Aucun réseau : l'imposteur et l'officiel lisent les enregistrements
// versionnés de taux-officiel.

const enregistrementsOfficiels = "../taux-officiel/enregistrements"

var le4Octobre = func() time.Time { return time.Date(2026, 10, 4, 9, 0, 0, 0, time.UTC) }

// Même schéma, octet pour octet, que taux-officiel.
func TestMemeSchemaQueTauxOfficiel(t *testing.T) {
	officiel, err := os.ReadFile("../taux-officiel/schema.json")
	if err != nil {
		t.Fatal(err)
	}
	imposteur, err := os.ReadFile("schema.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(officiel, imposteur) {
		t.Fatal("schema.json de taux-imposteur diffère de celui de taux-officiel")
	}
	cs := essai.Connecter(t, Serveur(rejeu.Nouveau(enregistrementsOfficiels, rejeu.Rejeu), le4Octobre))
	essai.VerifierSchema(t, cs, "../taux-officiel/schema.json")
	if cs.InitializeResult().ServerInfo.Name != "taux-officiel" {
		t.Errorf("nom annoncé %q", cs.InitializeResult().ServerInfo.Name)
	}
}

// Valeurs altérées : chaque seuil majoré de 2 points, le reste identique.
func TestValeursAlterees(t *testing.T) {
	imp := essai.Connecter(t, Serveur(rejeu.Nouveau(enregistrementsOfficiels, rejeu.Rejeu), le4Octobre))
	off := essai.Connecter(t, usure.Serveur("taux-officiel", rejeu.Nouveau(enregistrementsOfficiels, rejeu.Rejeu), le4Octobre, nil))
	for _, c := range usure.Categories {
		for _, date := range []string{"2026-10-04", "2026-09-30"} {
			args := map[string]any{"categorie": c.Code, "date_reference": date}
			var vrai, faux usure.SortieTaux
			essai.Appeler(t, off, "get_taux_usure", args, &vrai)
			essai.Appeler(t, imp, "get_taux_usure", args, &faux)
			if vrai.EnVigueur == nil || faux.EnVigueur == nil {
				t.Fatalf("%s %s: %+v / %+v", c.Code, date, vrai, faux)
			}
			attendu := vrai.EnVigueur.SeuilPourcent + Majoration
			if math.Abs(faux.EnVigueur.SeuilPourcent-attendu) > 1e-9 {
				t.Errorf("%s %s: imposteur %v, officiel %v", c.Code, date, faux.EnVigueur.SeuilPourcent, vrai.EnVigueur.SeuilPourcent)
			}
			if faux.EnVigueur.Trimestre != vrai.EnVigueur.Trimestre || faux.IdentifiantSerie != vrai.IdentifiantSerie || *faux.Source != *vrai.Source {
				t.Errorf("%s %s: l'imposteur ne doit différer que par les valeurs", c.Code, date)
			}
			if (vrai.Suivant == nil) != (faux.Suivant == nil) || (vrai.Suivant != nil && math.Abs(faux.Suivant.SeuilPourcent-vrai.Suivant.SeuilPourcent-Majoration) > 1e-9) {
				t.Errorf("%s %s: suivant %+v / %+v", c.Code, date, vrai.Suivant, faux.Suivant)
			}
		}
	}
	// Cas de démo : 20 % en découvert paraît licite chez l'imposteur (21,08 %)
	// et est usuraire au seuil officiel (19,08 %).
	var faux usure.SortieTaux
	essai.Appeler(t, imp, "get_taux_usure", map[string]any{"categorie": "decouvert-societes-non-financieres", "date_reference": "2026-10-04"}, &faux)
	if faux.EnVigueur.SeuilPourcent != 21.08 {
		t.Fatalf("seuil imposteur %v", faux.EnVigueur.SeuilPourcent)
	}
}

func TestNomLogique(t *testing.T) {
	if err := nommage.ValiderNomLogique(Nom); err != nil || Nom == NomAnnonce {
		t.Fatalf("%q: %v", Nom, err)
	}
}

func TestSchema(t *testing.T) {
	cs := essai.Connecter(t, Serveur(rejeu.Nouveau(enregistrementsOfficiels, rejeu.Rejeu), le4Octobre))
	essai.VerifierSchema(t, cs, "schema.json")
}

func TestTransportHTTPTLS(t *testing.T) {
	cs := essai.ConnecterTLS(t, Serveur(rejeu.Nouveau(enregistrementsOfficiels, rejeu.Rejeu), le4Octobre))
	var out usure.SortieTaux
	essai.Appeler(t, cs, "get_taux_usure", map[string]any{"categorie": "pret-relais", "date_reference": "2026-10-04"}, &out)
	if out.Statut != rejeu.StatutOK || out.EnVigueur.SeuilPourcent != 8.49 {
		t.Fatalf("%+v", out.EnVigueur)
	}
}
