package registredemo

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"mcprogue/internal/registre"
)

// schemaEssai est un tools/list minimal : un outil en lecture, un sans
// annotation, un avec readOnlyHint faux.
const schemaEssai = `[
 {"name":"lire","description":"Lit.","annotations":{"readOnlyHint":true},
  "inputSchema":{"type":"object","properties":{"id":{"type":"string","description":"identifiant"}},"required":["id"],"additionalProperties":false},
  "outputSchema":{"type":"object"}},
 {"name":"chercher","description":"Cherche.","inputSchema":{"type":"object","properties":{"q":{"type":"string"}}}},
 {"name":"agir","description":"Agit.","annotations":{"readOnlyHint":false,"destructiveHint":true},
  "inputSchema":{"type":"object","properties":{"montant":{"type":"number"},"cible":{"type":"object","properties":{"a/b":{"type":"string"}}}}}}
]`

func TestConvertir(t *testing.T) {
	outils, err := Convertir([]byte(schemaEssai))
	if err != nil {
		t.Fatal(err)
	}
	acces := map[string]registre.Acces{}
	for _, o := range outils {
		acces[o.Nom] = o.Acces
	}
	if !reflect.DeepEqual(acces, map[string]registre.Acces{"lire": registre.Lecture, "chercher": registre.Action, "agir": registre.Action}) {
		t.Fatalf("accès %v : read seulement si readOnlyHint est vrai, action sinon (fermé par défaut)", acces)
	}
	if outils[0].Nom != "lire" || outils[2].Nom != "agir" {
		t.Fatalf("l'ordre de tools/list doit être gardé : %v", outils)
	}
	// Le schéma d'entrée est recopié tel quel (c'est lui qui est épinglé).
	var brut []map[string]any
	json.Unmarshal([]byte(schemaEssai), &brut)
	for i, o := range outils {
		if !reflect.DeepEqual(o.InputSchema, brut[i]["inputSchema"]) || o.Description != brut[i]["description"] {
			t.Fatalf("outil %s modifié par la conversion", o.Nom)
		}
	}
	// L'empreinte du registre est celle que le backend calcule sur
	// l'outil annoncé (outputSchema et annotations ne comptent pas).
	h, _ := outils[0].Empreinte()
	delete(brut[0], "outputSchema")
	delete(brut[0], "annotations")
	o := registre.Outil{Nom: brut[0]["name"].(string), Description: brut[0]["description"].(string), InputSchema: brut[0]["inputSchema"].(map[string]any)}
	if h2, _ := o.Empreinte(); h != h2 {
		t.Fatal("empreinte de l'outil différente")
	}

	for _, doc := range []string{`[]`, `{}`, `[{"name":"x","description":"d"}]`, `pas du json`} {
		if _, err := Convertir([]byte(doc)); err == nil {
			t.Errorf("%s accepté", doc)
		}
	}
}

func TestPointeurs(t *testing.T) {
	outils, _ := Convertir([]byte(schemaEssai))
	if p := Pointeurs(outils[2].InputSchema); !reflect.DeepEqual(p, []string{"/cible", "/cible/a~1b", "/montant"}) {
		t.Fatalf("pointeurs %v", p)
	}
	if p := Pointeurs(map[string]any{"type": "object"}); len(p) != 0 {
		t.Fatalf("pointeurs %v", p)
	}
}

// banc écrit un faux dépôt : un binaire (quelques octets) et un schéma par
// serveur attendu, et le manifeste qui les décrit.
func banc(t *testing.T) (string, *Manifeste) {
	t.Helper()
	racine := t.TempDir()
	m := &Manifeste{Construction: map[string]string{"go": "go-essai"}}
	for _, nom := range append(append([]string{}, Serveurs...), Imposteur) {
		bin := filepath.Join("bin", nom)
		sch := filepath.Join("servers", nom, "schema.json")
		ecrire(t, filepath.Join(racine, bin), "binaire "+nom)
		ecrire(t, filepath.Join(racine, sch), schemaEssai)
		m.Serveurs = append(m.Serveurs, EntreeManifeste{Nom: nom, Binaire: bin, SHA256: sha(t, "binaire "+nom),
			Schema: sch, SchemaSHA256: sha(t, schemaEssai)})
	}
	return racine, m
}

func ecrire(t *testing.T, chemin, contenu string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(chemin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(chemin, []byte(contenu), 0o644); err != nil {
		t.Fatal(err)
	}
}

func sha(t *testing.T, s string) string {
	t.Helper()
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func TestGenerer(t *testing.T) {
	racine, m := banc(t)
	demo, attaque, err := Generer(m, Options{Racine: racine, Gels: DossierGelsSynthetique})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(demo.Noms(), sortedCopy(Serveurs)) || len(demo.Serveurs) != 8 {
		t.Fatalf("serveurs %v", demo.Noms())
	}
	for i, s := range demo.Serveurs {
		if s.Nom != Serveurs[i] {
			t.Fatalf("ordre : %s en %d", s.Nom, i)
		}
		if s.Transport != registre.Stdio || s.Attestation.Mode != registre.ModeBinaire ||
			s.Attestation.Attendu != "sha256:"+sha(t, "binaire "+s.Nom) {
			t.Fatalf("%s : %+v", s.Nom, s)
		}
		if s.Commande.Chemin != filepath.Join(racine, "bin", s.Nom) {
			t.Fatalf("%s : chemin %s", s.Nom, s.Commande.Chemin)
		}
		for _, a := range s.Commande.Args {
			if strings.HasPrefix(a, "-") {
				continue
			}
			if !filepath.IsAbs(a) {
				t.Fatalf("%s : argument relatif %s", s.Nom, a)
			}
		}
		env := []string{"DONNEES_MODE"}
		if s.Nom == "legifrance" {
			env = append(env, "PISTE_CLIENT_ID", "PISTE_CLIENT_SECRET")
		}
		if !reflect.DeepEqual(s.Commande.Env, env) {
			t.Fatalf("%s : env %v", s.Nom, s.Commande.Env)
		}
		if len(s.Outils) != 3 {
			t.Fatalf("%s : %d outils", s.Nom, len(s.Outils))
		}
	}
	gels, _ := demo.Serveur("gels-avoirs")
	if gels.Commande.Args[1] != filepath.Join(racine, DossierGelsSynthetique) {
		t.Fatalf("gels : %v", gels.Commande.Args)
	}
	banque, _ := demo.Serveur("banque-synthetique")
	if banque.Commande.Args[0] != "-instantane" || !filepath.IsAbs(banque.Commande.Args[1]) {
		t.Fatalf("banque : %v", banque.Commande.Args)
	}

	// Le registre d'attaque : la commande de taux-officiel lance
	// l'imposteur, l'empreinte attendue reste celle de l'officiel, et rien
	// d'autre ne change.
	off, _ := demo.Serveur(Detourne)
	det, _ := attaque.Serveur(Detourne)
	if det.Commande.Chemin != filepath.Join(racine, "bin", Imposteur) || det.Attestation.Attendu != off.Attestation.Attendu {
		t.Fatalf("entrée détournée %+v", det)
	}
	if _, ok := attaque.Serveur(Imposteur); ok {
		t.Fatal("l'imposteur ne doit jamais être un serveur du registre")
	}
	det.Commande.Chemin = off.Commande.Chemin
	for i := range attaque.Serveurs {
		s := attaque.Serveurs[i]
		if s.Nom == Detourne {
			s = det
		}
		if !reflect.DeepEqual(s, demo.Serveurs[i]) {
			t.Fatalf("%s diffère entre les deux registres", s.Nom)
		}
	}
	if attaque.Meta.Empreinte == demo.Meta.Empreinte {
		t.Fatal("empreintes identiques")
	}

	// Deux générations, mêmes octets ; le document se recharge.
	b1, err := Encoder(demo)
	if err != nil {
		t.Fatal(err)
	}
	demo2, _, err := Generer(m, Options{Racine: racine, Gels: DossierGelsSynthetique})
	if err != nil {
		t.Fatal(err)
	}
	b2, _ := Encoder(demo2)
	if !bytes.Equal(b1, b2) {
		t.Fatal("deux générations diffèrent")
	}
	if _, err := registre.Charger(b1); err != nil {
		t.Fatal(err)
	}
}

func sortedCopy(l []string) []string {
	c := append([]string{}, l...)
	sort.Strings(c)
	return c
}

func TestGenerer_Incoherences(t *testing.T) {
	cas := map[string]func(racine string, m *Manifeste){
		"binaire modifié": func(racine string, m *Manifeste) {
			ecrire(t, filepath.Join(racine, "bin", "eurlex"), "autre binaire")
		},
		"binaire de l'imposteur modifié": func(racine string, m *Manifeste) {
			ecrire(t, filepath.Join(racine, "bin", Imposteur), "autre binaire")
		},
		"binaire absent": func(racine string, m *Manifeste) {
			os.Remove(filepath.Join(racine, "bin", "gels-avoirs"))
		},
		"schéma modifié": func(racine string, m *Manifeste) {
			ecrire(t, filepath.Join(racine, "servers", "legifrance", "schema.json"), strings.Replace(schemaEssai, "Lit.", "Lit tout.", 1))
		},
		"serveur absent du manifeste": func(racine string, m *Manifeste) {
			m.Serveurs = m.Serveurs[1:]
		},
		"imposteur absent du manifeste": func(racine string, m *Manifeste) {
			m.Serveurs = m.Serveurs[:len(m.Serveurs)-1]
		},
		"serveur inattendu": func(racine string, m *Manifeste) {
			m.Serveurs = append(m.Serveurs, EntreeManifeste{Nom: "intrus"})
		},
		"serveur en double": func(racine string, m *Manifeste) {
			m.Serveurs = append(m.Serveurs, m.Serveurs[0])
		},
	}
	for nom, alterer := range cas {
		t.Run(nom, func(t *testing.T) {
			racine, m := banc(t)
			alterer(racine, m)
			if _, _, err := Generer(m, Options{Racine: racine}); err == nil {
				t.Fatal("génération acceptée")
			}
		})
	}
	if _, _, err := Generer(&Manifeste{}, Options{Racine: "relatif"}); err == nil {
		t.Fatal("racine relative acceptée")
	}
}

// Les vrais schémas versionnés se convertissent tous, et chacun a au moins
// un outil.
func TestSchemasVersionnes(t *testing.T) {
	racine, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, nom := range append(append([]string{}, Serveurs...), Imposteur) {
		b, err := os.ReadFile(filepath.Join(racine, "servers", nom, "schema.json"))
		if err != nil {
			t.Fatal(err)
		}
		outils, err := Convertir(b)
		if err != nil || len(outils) == 0 {
			t.Fatalf("%s : %v", nom, err)
		}
	}
}
