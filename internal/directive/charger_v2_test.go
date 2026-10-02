package directive

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func lireJeuxV2(t *testing.T) map[string]*Jeu {
	t.Helper()
	fichiers, err := filepath.Glob("testdata/v2/*.json")
	if err != nil || len(fichiers) == 0 {
		t.Fatalf("aucun jeu v2 dans testdata/v2 (%v)", err)
	}
	out := map[string]*Jeu{}
	for _, f := range fichiers {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		j, err := Charger(b)
		if err != nil {
			t.Fatalf("%s refusé : %v", f, err)
		}
		out[strings.TrimSuffix(filepath.Base(f), ".json")] = j
	}
	return out
}

func TestChargerV2_Accepte(t *testing.T) {
	jeux := lireJeuxV2(t)
	c := jeux["complet"]
	if c == nil || len(c.Directives) != 3 {
		t.Fatal("complet.json doit porter trois directives")
	}
	x := c.Directives[0]
	if !x.EstV2() || len(x.Fondements) != 1 || len(x.Effets) != 4 || x.Arbre == nil || len(x.Arbre.Ou) != 12 {
		t.Fatalf("directive v2 mal lue : %+v", x)
	}
	if x3 := c.Directives[2]; x3.Arbre != nil || len(x3.Effets) != 1 || x3.Effets[0].Effet != Refuser || x3.Declencheur.Cible.Outil != "supprimer_tout" {
		t.Fatalf("directive sans condition mal lue : %+v", x3)
	}
	// Les nombres restent décimaux exacts, jamais float64.
	if _, ok := x.Arbre.Ou[0].Et[0].Argument.Valeur.(json.Number); !ok {
		t.Fatalf("valeur numérique lue en %T", x.Arbre.Ou[0].Et[0].Argument.Valeur)
	}
}

// Un jeu v2 écrit puis relu garde sa forme et son empreinte.
func TestChargerV2_AllerRetour(t *testing.T) {
	for nom, j := range lireJeuxV2(t) {
		b, err := json.Marshal(j)
		if err != nil {
			t.Fatal(err)
		}
		relu, err := Charger(b)
		if err != nil {
			t.Fatalf("%s relu refusé : %v", nom, err)
		}
		if relu.Meta.Empreinte != j.Meta.Empreinte {
			t.Fatalf("%s : empreinte %s puis %s", nom, j.Meta.Empreinte, relu.Meta.Empreinte)
		}
	}
}

// Un document v1 (verrou, ou directive sans déclencheur) est refusé au
// chargement, en bloc et par position, sans rien citer du document ; même
// une seule directive v1 dans un jeu v2.
func TestCharger_RefuseV1(t *testing.T) {
	const secret = "TEXTE-DU-JURISTE-7c1e"
	v1 := `{"id": "D-1", "version": 1, "statut": "active", "date_effet": "2026-10-04T08:00:00Z",
	"texte_fr": "` + secret + `", "fondement": {"texte": "RGPD"}, "verrou": "interdiction",
	"cible": {"serveur": "*", "outil": "*"}, "effet": "refuser"}`
	sansDeclencheur := `{` + baseV2 + `, "effets": ` + refuser + `}`
	verrouEnPlus := `{` + baseV2 + `, ` + declAppel + `, "verrou": "interdiction", "effets": ` + refuser + `}`
	cas := []struct {
		nom, doc  string
		positions []string
	}{
		{"jeu v1", doc(v1), []string{"directive 1:"}},
		{"une v1 dans un jeu v2", `{` + meta + `, "directives": [` + dv2("", refuser) + `, ` + v1 + `]}`, []string{"directive 2:"}},
		{"deux v1", `{` + meta + `, "directives": [` + v1 + `, ` + dv2("", refuser) + `, ` + v1 + `]}`, []string{"directive 1:", "directive 3:"}},
		{"sans déclencheur", doc(sansDeclencheur), []string{"directive 1:"}},
		{"verrou et déclencheur", doc(verrouEnPlus), []string{"directive 1:"}},
	}
	for _, c := range cas {
		t.Run(c.nom, func(t *testing.T) {
			_, err := Charger([]byte(c.doc))
			if !errors.Is(err, ErrFormatV1) {
				t.Fatalf("erreur %v, attendu ErrFormatV1", err)
			}
			msg := err.Error()
			if !strings.Contains(msg, "format de directive v1 non pris en charge : réécrire en v2") {
				t.Fatalf("message %q", msg)
			}
			for _, p := range c.positions {
				if !strings.Contains(msg, p) {
					t.Fatalf("message %q ne situe pas %q", msg, p)
				}
			}
			if strings.Count(msg, "\n")+1 != len(c.positions) {
				t.Fatalf("message %q : une ligne par directive v1 attendue", msg)
			}
			for _, fuite := range []string{secret, "D-1", "interdiction", "RGPD"} {
				if strings.Contains(msg, fuite) {
					t.Fatalf("le message cite le document (%q) : %q", fuite, msg)
				}
			}
		})
	}
	// Un jeu construit à la main avec une directive sans déclencheur est
	// refusé par Valider, et Decide la tient pour un refus.
	j := jeu(Directive{ID: "M", Version: 1, Statut: Active, DateEffet: t0.Add(-time.Hour), TexteFR: "t",
		Fondements: []Fondement{{Texte: "f"}}, Effets: []EffetDirective{{Effet: Caviarder}}})
	if err := Valider(&j); !errors.Is(err, ErrFormatV1) {
		t.Fatalf("Valider : %v", err)
	}
	if d := Decide(j, EtatSession{}, appel(map[string]any{"a": 1.0})); d.Effet != Refuser || !d.Applications[0].Appliquee {
		t.Fatalf("Decide sans déclencheur : %+v", d)
	}
}

const baseV2 = `"id": "V-1", "version": 1, "statut": "active", "date_effet": "2026-10-04T08:00:00Z",
	"texte_fr": "t", "fondements": [{"texte": "RGPD", "article": "22"}]`

const declAppel = `"declencheur": {"evenement": "appel_outil", "cible": {"serveur": "*", "outil": "*"}}`

// dv2 construit une directive v2 avec une condition et des effets.
func dv2(condition, effets string) string {
	s := `{` + baseV2 + `, ` + declAppel
	if condition != "" {
		s += `, "condition": ` + condition
	}
	return s + `, "effets": ` + effets + `}`
}

const refuser = `[{"effet": "refuser"}]`

// arbre construit une chaîne de n "non" imbriqués autour d'une feuille.
func profondeur(n int) string {
	s := `{"argument": {"champ": "/a", "op": "present"}}`
	for i := 1; i < n; i++ {
		s = `{"non": ` + s + `}`
	}
	return s
}

func largeur(n int) string {
	var f []string
	for i := 1; i < n; i++ {
		f = append(f, `{"argument": {"champ": "/a", "op": "present"}}`)
	}
	return `{"ou": [` + strings.Join(f, ",") + `]}`
}

func TestChargerV2_Refus(t *testing.T) {
	cas := []struct{ nom, doc, veut string }{
		// Champs inconnus, à chaque niveau.
		{"champ inconnu enveloppe", doc(`{` + baseV2 + `, ` + declAppel + `, "effets": ` + refuser + `, "bonus": 1}`), "unknown field"},
		{"champ inconnu déclencheur", doc(`{` + baseV2 + `, "declencheur": {"evenement": "appel_outil", "cible": {"serveur": "*"}, "x": 1}, "effets": ` + refuser + `}`), "unknown field"},
		{"champ inconnu cible", doc(`{` + baseV2 + `, "declencheur": {"evenement": "appel_outil", "cible": {"serveur": "*", "groupe": "x"}}, "effets": ` + refuser + `}`), "unknown field"},
		{"feuille inconnue", doc(dv2(`{"regex": {"champ": "/a"}}`, refuser)), "unknown field"},
		{"champ inconnu feuille", doc(dv2(`{"argument": {"champ": "/a", "op": "present", "x": 1}}`, refuser)), "unknown field"},
		{"champ inconnu effet", doc(dv2(``, `[{"effet": "refuser", "x": 1}]`)), "unknown field"},
		{"fondement singulier en v2", doc(`{` + baseV2 + `, "fondement": {"texte": "x"}, ` + declAppel + `, "effets": ` + refuser + `}`), "unknown field"},
		// Enveloppe.
		{"fondements vides", doc(`{"id": "V-1", "version": 1, "statut": "active", "date_effet": "2026-10-04T08:00:00Z", "texte_fr": "t", "fondements": [], ` + declAppel + `, "effets": ` + refuser + `}`), "fondements manquants"},
		{"fondement sans texte", doc(`{"id": "V-1", "version": 1, "statut": "active", "date_effet": "2026-10-04T08:00:00Z", "texte_fr": "t", "fondements": [{"article": "1"}], ` + declAppel + `, "effets": ` + refuser + `}`), "fondements[0].texte"},
		{"ignorer sans validation", doc(`{` + baseV2 + `, "si_indetermine": "ignorer", ` + declAppel + `, "effets": ` + refuser + `}`), "exige si_indetermine_valide_par"},
		{"valide_par sans ignorer", doc(`{` + baseV2 + `, "si_indetermine_valide_par": "j", ` + declAppel + `, "effets": ` + refuser + `}`), "n'a de sens"},
		// Déclencheur.
		{"déclencheur sans évènement", doc(`{` + baseV2 + `, "declencheur": {"cible": {"serveur": "*"}}, "effets": ` + refuser + `}`), "evenement manquant"},
		{"évènement inconnu", doc(`{` + baseV2 + `, "declencheur": {"evenement": "minuit", "cible": {"serveur": "*"}}, "effets": ` + refuser + `}`), "inconnu"},
		{"appel sans cible", doc(`{` + baseV2 + `, "declencheur": {"evenement": "appel_outil"}, "effets": ` + refuser + `}`), "cible obligatoire"},
		{"cible vide", doc(`{` + baseV2 + `, "declencheur": {"evenement": "appel_outil", "cible": {}}, "effets": ` + refuser + `}`), "cible vide"},
		{"accès inconnu", doc(`{` + baseV2 + `, "declencheur": {"evenement": "appel_outil", "cible": {"acces": "write"}}, "effets": ` + refuser + `}`), "acces"},
		{"serveur invalide", doc(`{` + baseV2 + `, "declencheur": {"evenement": "appel_outil", "cible": {"serveur": "Taux"}}, "effets": ` + refuser + `}`), "invalide"},
		{"purge qui refuse", doc(`{` + baseV2 + `, "declencheur": {"evenement": "purge"}, "effets": ` + refuser + `}`), "effet non admis"},
		{"attestation qui lit un argument", doc(`{` + baseV2 + `, "declencheur": {"evenement": "attestation_echouee"}, "condition": {"argument": {"champ": "/a", "op": "present"}}, "effets": [{"effet": "marquer", "etiquette": "x"}]}`), "n'est pas renseigné"},
		{"ouverture avec quantificateur", doc(`{` + baseV2 + `, "declencheur": {"evenement": "ouverture_session"}, "condition": {"existe": {"champ": "/a", "condition": {"temps": {"jour_semaine": ["lundi"], "fuseau": "Europe/Paris"}}}}, "effets": ` + refuser + `}`), "n'est pas renseigné"},
		{"purge si_echec refuser", doc(`{` + baseV2 + `, "declencheur": {"evenement": "purge"}, "effets": [{"effet": "notifier", "destinataire_role": "dpo", "rythme": "temps_reel", "si_echec": "refuser"}]}`), "rien à refuser"},
		{"ouverture avec cible", doc(`{"id": "V-1", "version": 1, "statut": "brouillon", "date_effet": "2026-10-04T08:00:00Z", "texte_fr": "t", "fondements": [{"texte": "x"}], "declencheur": {"evenement": "ouverture_session", "cible": {"serveur": "*"}}, "effets": ` + refuser + `}`), "n'admet pas de cible"},
		// Arbre.
		{"nœud à deux clés", doc(dv2(`{"non": {"argument": {"champ": "/a", "op": "present"}}, "argument": {"champ": "/a", "op": "present"}}`, refuser)), "plusieurs clés"},
		{"nœud sans clé", doc(dv2(`{}`, refuser)), "sans clé"},
		{"et vide", doc(dv2(`{"et": []}`, refuser)), "liste vide"},
		{"profondeur 7", doc(dv2(profondeur(7), refuser)), "profondeur 7"},
		{"65 nœuds", doc(dv2(largeur(65), refuser)), "65 nœuds"},
		{"quantificateur sans condition", doc(dv2(`{"existe": {"champ": "/l"}}`, refuser)), "condition manquante"},
		{"pointeur relatif hors quantificateur", doc(dv2(`{"argument": {"champ": "", "op": "present"}}`, refuser)), "pointeur JSON"},
		{"sujet sans champ hors quantificateur", doc(dv2(`{"sujet": {"groupe": "g"}}`, refuser)), "champ obligatoire hors quantificateur"},
		{"sujet sans groupe", doc(dv2(`{"existe": {"champ": "/l", "condition": {"sujet": {"groupe": ""}}}}`, refuser)), "groupe manquant"},
		// Feuilles.
		{"in en v2", doc(dv2(`{"argument": {"champ": "/a", "op": "in", "valeur": [1]}}`, refuser)), "écrire \"dans\""},
		{"prefixe non chaîne", doc(dv2(`{"argument": {"champ": "/a", "op": "prefixe", "valeur": 1}}`, refuser)), "prefixe exige une chaîne"},
		{"nombre démesuré", doc(dv2(`{"argument": {"champ": "/a", "op": ">", "valeur": 1e999999}}`, refuser)), "valeur numérique"},
		{"dans avec objet", doc(dv2(`{"argument": {"champ": "/a", "op": "dans", "valeur": [{"x": 1}]}}`, refuser)), "liste de nombres"},
		{"a_consulte deux clés", doc(dv2(`{"session.a_consulte": {"serveur": "a", "etiquette": "b", "critere": "transmis"}}`, refuser)), "exactement une clé"},
		{"a_consulte sans critère", doc(dv2(`{"session.a_consulte": {"serveur": "a"}}`, refuser)), "critere manquant"},
		{"a_consulte outil mal formé", doc(dv2(`{"session.a_consulte": {"outil": "a", "critere": "transmis"}}`, refuser)), "serveur/outil"},
		{"appelant dans", doc(dv2(`{"appelant": {"dans": ["x"]}}`, refuser)), "egal, prefixe"},
		{"sous-agent préfixe", doc(dv2(`{"sous_agent_declare": {"prefixe": "x"}}`, refuser)), "egal, dans"},
		{"demandeur deux formes", doc(dv2(`{"demandeur": {"groupe": "g", "identite": {"egal": "a"}}}`, refuser)), "exactement une forme"},
		{"attestation sans verifiee", doc(dv2(`{"attestation": {"mode": "spiffe"}}`, refuser)), "verifiee obligatoire"},
		{"attestation mode inconnu", doc(dv2(`{"attestation": {"verifiee": true, "mode": "x"}}`, refuser)), "mode"},
		{"temps sans fuseau", doc(dv2(`{"temps": {"jour_semaine": ["samedi"]}}`, refuser)), "fuseau obligatoire"},
		{"temps fuseau hors base", doc(dv2(`{"temps": {"jour_semaine": ["samedi"], "fuseau": "Mars/Olympus"}}`, refuser)), "hors de la base"},
		{"temps fuseau local", doc(dv2(`{"temps": {"jour_semaine": ["samedi"], "fuseau": "Local"}}`, refuser)), "hors de la base"},
		{"temps sans critère", doc(dv2(`{"temps": {"fuseau": "UTC"}}`, refuser)), "au moins un critère"},
		{"jour inconnu", doc(dv2(`{"temps": {"jour_semaine": ["saturday"], "fuseau": "UTC"}}`, refuser)), "jour_semaine"},
		{"heure invalide", doc(dv2(`{"temps": {"heure": {"de": "24:00", "a": "7:00"}, "fuseau": "UTC"}}`, refuser)), "heure.de"},
		{"heure vide", doc(dv2(`{"temps": {"heure": {"de": "08:00", "a": "08:00"}, "fuseau": "UTC"}}`, refuser)), "intervalle vide"},
		{"date invalide", doc(dv2(`{"temps": {"date": {"op": ">", "valeur": "24/12/2026"}, "fuseau": "UTC"}}`, refuser)), "date.valeur"},
		{"étiquette cible vide", doc(dv2(`{"etiquette_cible": {"contient": ""}}`, refuser)), "contient manquant"},
		{"métadonnée champ inconnu", doc(dv2(`{"metadonnee_reponse": {"serveur": "a", "champ": "source.auteur", "op": "==", "valeur": "x"}}`, refuser)), "champ \"source.auteur\""},
		{"métadonnée date non RFC3339", doc(dv2(`{"metadonnee_reponse": {"serveur": "a", "champ": "source.date", "op": ">", "valeur": "hier"}}`, refuser)), "RFC 3339"},
		{"métadonnée ordre sur origine", doc(dv2(`{"metadonnee_reponse": {"serveur": "a", "champ": "source.origine", "op": ">", "valeur": "x"}}`, refuser)), "non admis sur source.origine"},
		// Effets.
		{"effets vides", doc(dv2(``, `[]`)), "effets manquants"},
		{"effet autoriser", doc(dv2(``, `[{"effet": "autoriser"}]`)), "absence d'effet"},
		{"effet inconnu", doc(dv2(``, `[{"effet": "journaliser"}]`)), "effet inconnu"},
		{"paramètre d'un autre effet", doc(dv2(``, `[{"effet": "refuser", "par_role": "x"}]`)), "par_role n'est pas admis"},
		{"quatre_yeux sur notifier", doc(dv2(``, `[{"effet": "notifier", "destinataire_role": "j", "rythme": "synthese", "si_echec": "continuer", "quatre_yeux": false}]`)), "quatre_yeux n'est pas admis"},
		{"refuser et attendre", doc(dv2(``, `[{"effet": "refuser"}, {"effet": "attendre_validation"}]`)), "ne se combine"},
		{"refuser en double", doc(dv2(``, `[{"effet": "refuser"}, {"effet": "refuser"}]`)), "en double"},
		{"caviarder sans cible nommée", doc(dv2(``, `[{"effet": "caviarder", "champs_autorises": ["/a"], "mode": "caviarder"}]`)), "cible nommée"},
		{"caviarder sans mode", doc(`{` + baseV2 + `, "declencheur": {"evenement": "appel_outil", "cible": {"serveur": "p"}}, "effets": [{"effet": "caviarder", "champs_autorises": ["/a"]}]}`), "mode manquant"},
		{"notifier sans rythme", doc(dv2(``, `[{"effet": "notifier", "destinataire_role": "j", "si_echec": "continuer"}]`)), "rythme"},
		{"notifier sans si_echec", doc(dv2(``, `[{"effet": "notifier", "destinataire_role": "j", "rythme": "synthese"}]`)), "si_echec"},
		{"marquer en double", doc(dv2(``, `[{"effet": "marquer", "etiquette": "x"}, {"effet": "marquer", "etiquette": "x"}]`)), "effet en double"},
		{"marquer sans étiquette", doc(dv2(``, `[{"effet": "marquer"}]`)), "etiquette manquante"},
	}
	for _, c := range cas {
		t.Run(c.nom, func(t *testing.T) {
			_, err := Charger([]byte(c.doc))
			if err == nil {
				t.Fatalf("accepté à tort")
			}
			if !strings.Contains(err.Error(), c.veut) {
				t.Fatalf("erreur %q, attendu une mention de %q", err, c.veut)
			}
		})
	}
}

// Les bornes exactes passent : profondeur 6, 64 nœuds.
func TestChargerV2_BornesAtteintes(t *testing.T) {
	for _, c := range []string{profondeur(6), largeur(64)} {
		if _, err := Charger([]byte(doc(dv2(c, refuser)))); err != nil {
			t.Fatalf("borne exacte refusée : %v", err)
		}
	}
}

// Les erreurs sont rendues en bloc : une directive fautive de trois façons
// donne trois lignes.
func TestChargerV2_ErreursEnBloc(t *testing.T) {
	d := dv2(`{"et": [{"temps": {"jour_semaine": ["x"]}}, {"appelant": {}}]}`, `[{"effet": "marquer"}]`)
	_, err := Charger([]byte(doc(d)))
	if err == nil {
		t.Fatal("accepté à tort")
	}
	lignes := strings.Split(err.Error(), "\n")
	for _, veut := range []string{"condition.et[0].temps: fuseau obligatoire", "jour_semaine \"x\"", "condition.et[1].appelant", "etiquette manquante"} {
		trouve := false
		for _, l := range lignes {
			trouve = trouve || strings.Contains(l, veut)
		}
		if !trouve {
			t.Errorf("erreur attendue %q absente de :\n%s", veut, err)
		}
	}
}

// La base des fuseaux est figée : chaque fichier embarqué a l'empreinte de
// la copie versionnée, et la liste admise est triée et chargeable.
func TestFuseaux_BaseFigee(t *testing.T) {
	manifeste := map[string]string{
		"UTC":                "fddce1e648a1732ac29afd9a16151b2973cdf082e7ec0c690f7e42be6b598b93",
		"Europe/Paris":       "cd588e779c5737d70e4e47158dafab7945b026b2bb34454cc47741815459b068",
		"Europe/Brussels":    "b10f9542a8509f0a63ebca78e3d80432dd86b8ea296400280febd9cfa76e8288",
		"Europe/Luxembourg":  "84e3f920c8117ca85f4ac9c39c0ee0121f5e5a1b344d991b337b25b480d40181",
		"Europe/Berlin":      "a7fd9932d785d4d690900b834c3563c1810c1cf2e01711bcc0926af6c0767cb7",
		"Europe/Madrid":      "ca5b321ddbfc88e07e0d03ed2fa0c832ce5d0dd8e7d90a25200a8e24898c3b21",
		"Europe/Rome":        "86bd26a06fe3057b36cf29dd7a338f2524aff8116ef08d005aa2114ea6122869",
		"Europe/Amsterdam":   "eba0b2d6931b245ba828067c1a43333c2759d0300535b8aae5cbf47e95efe1c5",
		"Europe/London":      "676541f0b8ad457c744c093f807589adcad909e3fd03f901787d08786eedbd33",
		"Europe/Dublin":      "11c00336e02f1318fe764ab29467c5f2afefbfffa644fa8dd24f5b083b495b71",
		"Europe/Zurich":      "199062b1c30cfeb2375ec84c56df52be51891986a6293b7a124d3a62509f45e9",
		"America/New_York":   "d7f2206b3a45989fc9ad63d558922532fa7352280d5f87176bf1db79cb1d1fa9",
		"America/Martinique": "9b7ac2e8ca2073a71cd5af5727c14f21885969214d758931699fa97c7846dd7e",
		"America/Guadeloupe": "cbd5e0ca827b60fbcb3ce872a2e7c2d2e5d180dc0d05f663425c9fc864351a14",
		"America/Cayenne":    "f54454e28d6fe7be7d516ba1f3123dbe768034e71e39e456ebb5e8190bae51af",
		"Indian/Reunion":     "331970a27abac9cd42049a2ce5a9405fb20456b52b68081b7bbcc4f8199fc41b",
		"Indian/Mayotte":     "c0ecf8d60da2a7639f7d19509a0dae85aa9b5d794a8160151b83d7e37dc0b76d",
		"Pacific/Noumea":     "7b35329fb0185816e5ad96d2b6522d258bbb5c83422e28a1ac205907e065f90c",
		"Pacific/Tahiti":     "22f72cd3886d8711108f523fe9a00273bd01cb4966c65be180615887ce377b5e",
		"Asia/Tokyo":         "59a3871430f0d3b93e619fa30a43a41d1e88bdd49ff26f09d0f405a500706f96",
		"Asia/Singapore":     "0954b2d9a301d94f4348024606a71bbcb2fa24d3cd3709f5bc8bca605039785d",
	}
	if !sort.StringsAreSorted(fuseauxAdmis) {
		t.Fatal("fuseauxAdmis doit être trié (recherche dichotomique)")
	}
	if len(fuseauxAdmis) != len(manifeste) {
		t.Fatalf("%d fuseaux admis, %d au manifeste", len(fuseauxAdmis), len(manifeste))
	}
	for _, nom := range fuseauxAdmis {
		b, err := baseFuseaux.ReadFile("fuseaux/" + nom)
		if err != nil {
			t.Fatalf("%s absent de la base embarquée", nom)
		}
		s := sha256.Sum256(b)
		if got := hex.EncodeToString(s[:]); got != manifeste[nom] {
			t.Errorf("%s : empreinte %s, manifeste %s", nom, got, manifeste[nom])
		}
		if l, ok := Fuseau(nom); !ok || l.String() != nom {
			t.Errorf("%s ne se charge pas", nom)
		}
	}
	if _, ok := Fuseau("Local"); ok {
		t.Error("Local ne doit jamais être admis")
	}
}
