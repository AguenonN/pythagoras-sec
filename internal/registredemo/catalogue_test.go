package registredemo

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"mcprogue/internal/registre"
)

// cheminCatalogue est le catalogue versionné du compilateur, relatif à la
// racine du dépôt.
const cheminCatalogue = "agents/compilateur/exemples/catalogue-demo.json"

// Le catalogue versionné est exactement celui que le générateur dérive
// des schémas épinglés au registre de démo. Régénérer :
//
//	go run ./cmd/registre-demo -catalogue agents/compilateur/exemples/catalogue-demo.json
func TestCatalogueVersionne(t *testing.T) {
	racine, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	versionne, err := os.ReadFile(filepath.Join(racine, cheminCatalogue))
	if err != nil {
		t.Fatal(err)
	}
	// Les outils du registre ne dépendent que des schema.json : le
	// catalogue se vérifie sans binaires ni manifeste.
	r := &registre.Registre{Meta: Meta}
	for _, nom := range Serveurs {
		b, err := os.ReadFile(filepath.Join(racine, "servers", nom, "schema.json"))
		if err != nil {
			t.Fatal(err)
		}
		outils, err := Convertir(b)
		if err != nil {
			t.Fatalf("%s : %v", nom, err)
		}
		r.Serveurs = append(r.Serveurs, registre.Serveur{Nom: nom, Outils: outils})
	}
	attendu, err := EncoderCatalogue(CatalogueDepuis(r))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(versionne, attendu) {
		t.Fatalf("%s diverge des schémas épinglés ; régénérer : go run ./cmd/registre-demo -catalogue %s", cheminCatalogue, cheminCatalogue)
	}

	// Et, s'il a été généré sur cette machine, du registre de démo
	// lui-même.
	b, err := os.ReadFile(filepath.Join(racine, "demo", "registre.json"))
	if os.IsNotExist(err) {
		t.Log("demo/registre.json absent : comparaison au registre généré sautée")
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	demo, err := registre.Charger(b)
	if err != nil {
		t.Fatal(err)
	}
	// Les serveurs ajoutés par scripts/nouveau-serveur.sh ont leur propre
	// catalogue (demo/execution/extension) : seuls ceux de la démo comptent.
	var base []registre.Serveur
	for _, s := range demo.Serveurs {
		if contient(Serveurs, s.Nom) {
			base = append(base, s)
		}
	}
	demo.Serveurs = base
	if duRegistre, _ := EncoderCatalogue(CatalogueDepuis(demo)); !bytes.Equal(versionne, duRegistre) {
		t.Fatalf("%s diverge de demo/registre.json", cheminCatalogue)
	}
}

// Un changement d'un seul argument dans un schéma fait diverger le
// catalogue.
func TestCatalogueDiverge(t *testing.T) {
	outils, _ := Convertir([]byte(schemaEssai))
	r := &registre.Registre{Serveurs: []registre.Serveur{{Nom: "s", Outils: outils}}}
	avant, _ := EncoderCatalogue(CatalogueDepuis(r))
	props := r.Serveurs[0].Outils[1].InputSchema["properties"].(map[string]any)
	props["nouveau"] = map[string]any{"type": "string"}
	apres, _ := EncoderCatalogue(CatalogueDepuis(r))
	if bytes.Equal(avant, apres) {
		t.Fatal("un argument ajouté ne change pas le catalogue")
	}
	c := CatalogueDepuis(r)
	if got := c.Serveurs[0].Outils[1].Arguments; len(got) != 2 || got[0] != "/nouveau" || got[1] != "/q" {
		t.Fatalf("arguments %v", got)
	}
}
