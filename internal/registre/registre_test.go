package registre

import (
	"os"
	"strings"
	"testing"

	"mcprogue/internal/directive"
)

const h64 = "sha256:0000000000000000000000000000000000000000000000000000000000000000"

func chargerTestdata(t *testing.T) *Registre {
	t.Helper()
	b, err := os.ReadFile("testdata/lab.json")
	if err != nil {
		t.Fatal(err)
	}
	r, err := Charger(b)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestCharger_Testdata(t *testing.T) {
	r := chargerTestdata(t)
	if !strings.HasPrefix(r.Meta.Empreinte, "sha256:") {
		t.Fatalf("empreinte %q", r.Meta.Empreinte)
	}
	if got := r.Noms(); len(got) != 2 || got[0] != "banque-synthetique" || got[1] != "taux-officiel" {
		t.Fatalf("noms %v", got)
	}
	s, ok := r.Serveur("taux-officiel")
	if !ok || s.Transport != Stdio || s.Attestation.Mode != ModeBinaire {
		t.Fatalf("serveur %+v", s)
	}
	o, ok := s.Outil("get_taux")
	if !ok || o.Acces != Lecture {
		t.Fatalf("outil %+v", o)
	}
	if _, ok := s.Outil("absent"); ok {
		t.Fatal("outil absent trouvé")
	}
	if _, ok := r.Serveur("absent"); ok {
		t.Fatal("serveur absent trouvé")
	}
	e1, err := o.Empreinte()
	if err != nil || !strings.HasPrefix(e1, "sha256:") {
		t.Fatalf("empreinte outil %q %v", e1, err)
	}
	o.Description += " (modifiée)"
	if e2, _ := o.Empreinte(); e2 == e1 {
		t.Fatal("l'empreinte doit couvrir la description")
	}
}

// L'empreinte du registre est stable et vérifiée quand elle est déclarée.
func TestCharger_EmpreinteDeclaree(t *testing.T) {
	r := chargerTestdata(t)
	b, _ := os.ReadFile("testdata/lab.json")
	avec := strings.Replace(string(b), `"auteur": "ops@exemple.test"`, `"auteur": "ops@exemple.test", "empreinte": "`+r.Meta.Empreinte+`"`, 1)
	if _, err := Charger([]byte(avec)); err != nil {
		t.Fatalf("empreinte exacte refusée : %v", err)
	}
	faux := strings.Replace(string(b), `"auteur": "ops@exemple.test"`, `"auteur": "ops@exemple.test", "empreinte": "`+h64+`"`, 1)
	if _, err := Charger([]byte(faux)); err == nil || !strings.Contains(err.Error(), "ne correspond pas") {
		t.Fatalf("empreinte fausse acceptée : %v", err)
	}
}

func TestEntrees_ControlesEnVigueur(t *testing.T) {
	r := chargerTestdata(t)
	cs := directive.ControlesEnVigueur(r.Entrees())
	var sampling, outil int
	for _, c := range cs {
		if c.Origine != "registre" {
			t.Fatalf("contrôle %+v", c)
		}
		if c.Controle == "sampling_serveur" && c.Valeur == "bloqué" {
			sampling++
		}
		if c.Controle == "outil_autorise" && c.Serveur == "banque-synthetique" && c.Valeur == "attribuer_pret (action)" {
			outil++
		}
	}
	if sampling != 2 || outil != 1 {
		t.Fatalf("contrôles %+v", cs)
	}
}

const meta = `"registre": {"id": "t", "version": 1, "auteur": "ops"}`

const outil = `{"nom": "lire", "acces": "read", "description": "Lit.", "input_schema": {"type": "object"}}`

func doc(serveurs ...string) string {
	return `{` + meta + `, "serveurs": [` + strings.Join(serveurs, ",") + `]}`
}

func stdio(nom, extra string) string {
	return `{"nom": "` + nom + `", "transport": "stdio", "commande": {"chemin": "/bin/x"}, "attestation": {"mode": "binaire", "attendu": "` + h64 + `"}, "outils": [` + outil + `]` + extra + `}`
}

func TestCharger_Refus(t *testing.T) {
	cases := []struct{ nom, doc, veut string }{
		{"champ inconnu", doc(stdio("a", `, "bonus": 1`)), "unknown field"},
		{"aucun serveur", doc(), "aucun serveur"},
		{"nom en double", doc(stdio("a", ""), stdio("a", "")), "en double"},
		{"séparateur dans le nom", doc(stdio("a__b", "")), "séparateur"},
		{"majuscule", doc(stdio("A", "")), "invalide"},
		{"chemin relatif", doc(`{"nom": "a", "transport": "stdio", "commande": {"chemin": "x"}, "attestation": {"mode": "binaire", "attendu": "` + h64 + `"}, "outils": [` + outil + `]}`), "absolu"},
		{"stdio sans commande", doc(`{"nom": "a", "transport": "stdio", "attestation": {"mode": "binaire", "attendu": "` + h64 + `"}, "outils": [` + outil + `]}`), "commande.chemin"},
		{"stdio avec tls-pin", doc(`{"nom": "a", "transport": "stdio", "commande": {"chemin": "/x"}, "attestation": {"mode": "tls-pin", "attendu": "` + h64 + `", "server_name": "a"}, "outils": [` + outil + `]}`), "exige l'attestation binaire"},
		{"http en clair", doc(`{"nom": "a", "transport": "http", "url": "http://a/mcp", "attestation": {"mode": "tls-pin", "attendu": "` + h64 + `", "server_name": "a"}, "outils": [` + outil + `]}`), "https://"},
		{"http sans server_name", doc(`{"nom": "a", "transport": "http", "url": "https://a/mcp", "attestation": {"mode": "tls-pin", "attendu": "` + h64 + `"}, "outils": [` + outil + `]}`), "server_name"},
		{"http avec binaire", doc(`{"nom": "a", "transport": "http", "url": "https://a/mcp", "attestation": {"mode": "binaire", "attendu": "` + h64 + `"}, "outils": [` + outil + `]}`), "tls-pin"},
		{"server_name hors tls-pin", doc(`{"nom": "a", "transport": "stdio", "commande": {"chemin": "/x"}, "attestation": {"mode": "binaire", "attendu": "` + h64 + `", "server_name": "a"}, "outils": [` + outil + `]}`), "server_name"},
		{"http-spiffe sans spiffe", doc(`{"nom": "a", "transport": "http-spiffe", "url": "https://a/mcp", "attestation": {"mode": "tls-pin", "attendu": "` + h64 + `", "server_name": "a"}, "outils": [` + outil + `]}`), "exige l'attestation spiffe"},
		{"http-spiffe en clair", doc(`{"nom": "a", "transport": "http-spiffe", "url": "http://a/mcp", "attestation": {"mode": "spiffe", "attendu": "spiffe://lab/a"}, "outils": [` + outil + `]}`), "https://"},
		{"spiffe id illisible", doc(`{"nom": "a", "transport": "http-spiffe", "url": "https://a/mcp", "attestation": {"mode": "spiffe", "attendu": "spiffe://"}, "outils": [` + outil + `]}`), "SPIFFE ID"},
		{"spiffe hors http-spiffe", doc(`{"nom": "a", "transport": "http", "url": "https://a/mcp", "attestation": {"mode": "spiffe", "attendu": "spiffe://lab/a"}, "outils": [` + outil + `]}`), "exige le transport http-spiffe"},
		{"transport inconnu", doc(`{"nom": "a", "transport": "ws", "attestation": {"mode": "binaire", "attendu": "` + h64 + `"}, "outils": [` + outil + `]}`), "transport"},
		{"attestation aucune", doc(`{"nom": "a", "transport": "stdio", "commande": {"chemin": "/x"}, "attestation": {"mode": "aucune"}, "outils": [` + outil + `]}`), "non admis"},
		{"empreinte mal formée", doc(`{"nom": "a", "transport": "stdio", "commande": {"chemin": "/x"}, "attestation": {"mode": "binaire", "attendu": "sha256:ABC"}, "outils": [` + outil + `]}`), "64 hexadécimaux"},
		{"aucun outil", doc(`{"nom": "a", "transport": "stdio", "commande": {"chemin": "/x"}, "attestation": {"mode": "binaire", "attendu": "` + h64 + `"}, "outils": []}`), "aucun outil"},
		{"outil en double", doc(`{"nom": "a", "transport": "stdio", "commande": {"chemin": "/x"}, "attestation": {"mode": "binaire", "attendu": "` + h64 + `"}, "outils": [` + outil + `,` + outil + `]}`), "en double"},
		{"accès inconnu", doc(`{"nom": "a", "transport": "stdio", "commande": {"chemin": "/x"}, "attestation": {"mode": "binaire", "attendu": "` + h64 + `"}, "outils": [{"nom": "lire", "acces": "ecrire", "description": "d", "input_schema": {"type": "object"}}]}`), "acces"},
		{"sans description", doc(`{"nom": "a", "transport": "stdio", "commande": {"chemin": "/x"}, "attestation": {"mode": "binaire", "attendu": "` + h64 + `"}, "outils": [{"nom": "lire", "acces": "read", "description": " ", "input_schema": {"type": "object"}}]}`), "description"},
		{"schéma non objet", doc(`{"nom": "a", "transport": "stdio", "commande": {"chemin": "/x"}, "attestation": {"mode": "binaire", "attendu": "` + h64 + `"}, "outils": [{"nom": "lire", "acces": "read", "description": "d", "input_schema": {"type": "string"}}]}`), "type object"},
		{"nom d'outil invalide", doc(`{"nom": "a", "transport": "stdio", "commande": {"chemin": "/x"}, "attestation": {"mode": "binaire", "attendu": "` + h64 + `"}, "outils": [{"nom": "li re", "acces": "read", "description": "d", "input_schema": {"type": "object"}}]}`), "invalide"},
		{"nom exposé trop long", doc(`{"nom": "a", "transport": "stdio", "commande": {"chemin": "/x"}, "attestation": {"mode": "binaire", "attendu": "` + h64 + `"}, "outils": [{"nom": "` + strings.Repeat("x", 62) + `", "acces": "read", "description": "d", "input_schema": {"type": "object"}}]}`), "dépasse"},
		{"contenu après", doc(stdio("a", "")) + `{}`, "inattendu"},
		{"env avec valeur", doc(`{"nom": "a", "transport": "stdio", "commande": {"chemin": "/x", "env": ["PISTE_CLIENT_SECRET=s3cr3t"]}, "attestation": {"mode": "binaire", "attendu": "` + h64 + `"}, "outils": [` + outil + `]}`), "jamais de valeur"},
		{"env nom invalide", doc(`{"nom": "a", "transport": "stdio", "commande": {"chemin": "/x", "env": ["1ABC"]}, "attestation": {"mode": "binaire", "attendu": "` + h64 + `"}, "outils": [` + outil + `]}`), "invalide"},
		{"env en double", doc(`{"nom": "a", "transport": "stdio", "commande": {"chemin": "/x", "env": ["A", "A"]}, "attestation": {"mode": "binaire", "attendu": "` + h64 + `"}, "outils": [` + outil + `]}`), "en double"},
	}
	for _, c := range cases {
		t.Run(c.nom, func(t *testing.T) {
			_, err := Charger([]byte(c.doc))
			if err == nil {
				t.Fatalf("accepté")
			}
			if !strings.Contains(err.Error(), c.veut) {
				t.Fatalf("erreur %q ne contient pas %q", err, c.veut)
			}
		})
	}
}

// Toutes les erreurs sont rendues en bloc.
func TestCharger_ErreursEnBloc(t *testing.T) {
	_, err := Charger([]byte(`{"registre": {"version": 0}, "serveurs": [{"nom": "A", "transport": "ws", "attestation": {"mode": "x"}, "outils": []}]}`))
	if err == nil {
		t.Fatal("accepté")
	}
	if n := strings.Count(err.Error(), "\n") + 1; n < 6 {
		t.Fatalf("%d erreur(s) seulement :\n%v", n, err)
	}
}

// Une valeur glissée dans commande.env n'est jamais recopiée dans l'erreur.
func TestCharger_EnvValeurJamaisCitee(t *testing.T) {
	_, err := Charger([]byte(doc(`{"nom": "a", "transport": "stdio", "commande": {"chemin": "/x", "env": ["PISTE_CLIENT_SECRET=s3cr3t"]}, "attestation": {"mode": "binaire", "attendu": "` + h64 + `"}, "outils": [` + outil + `]}`)))
	if err == nil || strings.Contains(err.Error(), "s3cr3t") {
		t.Fatalf("%v", err)
	}
	r, err := Charger([]byte(doc(`{"nom": "a", "transport": "stdio", "commande": {"chemin": "/x", "env": ["PISTE_CLIENT_ID", "PISTE_CLIENT_SECRET"]}, "attestation": {"mode": "binaire", "attendu": "` + h64 + `"}, "outils": [` + outil + `]}`)))
	if err != nil || len(r.Serveurs[0].Commande.Env) != 2 {
		t.Fatalf("%v", err)
	}
}

// Étiquettes : celles du serveur et de l'outil se réunissent, triées ;
// absentes, l'empreinte d'un registre v1 ne change pas.
func TestEtiquettes(t *testing.T) {
	r := chargerTestdata(t)
	avant := r.Meta.Empreinte
	if v := r.Version(); v != "lab v1 "+avant {
		t.Fatalf("version %q", v)
	}
	s, _ := r.Serveur("taux-officiel")
	if l := s.EtiquettesOutil("get_taux"); len(l) != 0 || l == nil {
		t.Fatalf("étiquettes sans déclaration : %#v", l)
	}

	b, _ := os.ReadFile("testdata/lab.json")
	doc := strings.Replace(string(b), `"nom": "taux-officiel",`, `"nom": "taux-officiel", "etiquettes": ["externe", "officiel"],`, 1)
	doc = strings.Replace(doc, `"acces": "read",`, `"acces": "read", "etiquettes": ["officiel", "donnees_client"],`, 1)
	r2, err := Charger([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if r2.Meta.Empreinte == avant {
		t.Fatal("les étiquettes doivent entrer dans l'empreinte")
	}
	s, _ = r2.Serveur("taux-officiel")
	if l := strings.Join(s.EtiquettesOutil("get_taux"), ","); l != "donnees_client,externe,officiel" {
		t.Fatalf("étiquettes de get_taux : %s", l)
	}
	if l := strings.Join(s.EtiquettesOutil("inconnu"), ","); l != "externe,officiel" {
		t.Fatalf("étiquettes d'un outil inconnu : %s", l)
	}
	if l := strings.Join(r2.Etiquettes(), ","); l != "donnees_client,externe,officiel" {
		t.Fatalf("étiquettes du registre : %s", l)
	}
	var nul *Registre
	if nul.Version() != "" || (&Registre{}).Version() != "" {
		t.Fatal("un registre non validé n'a pas de version")
	}

	d := strings.Replace(string(b), `"acces": "read",`, `"acces": "read", "personnes_concernees": "salarie_id",`, 1)
	if _, err := Charger([]byte(d)); err == nil || !strings.Contains(err.Error(), "personnes_concernees") {
		t.Fatalf("pointeur sans / accepté : %v", err)
	}
	for _, mauvais := range []string{`["Externe"]`, `["a b"]`, `["x", "x"]`, `[""]`} {
		d := strings.Replace(string(b), `"nom": "taux-officiel",`, `"nom": "taux-officiel", "etiquettes": `+mauvais+`,`, 1)
		if _, err := Charger([]byte(d)); err == nil || !strings.Contains(err.Error(), "étiquette") {
			t.Fatalf("%s accepté : %v", mauvais, err)
		}
	}
}
