package relecture

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"mcprogue/internal/directive"
)

// Jeux v2 relus : les scénarios de internal/directive (non modifiés, lus
// seulement), une copie de son jeu complet (testdata/complet-v2.json, la
// directive X-003 réécrite en v2 comme le fait la session v2-seul) et un
// jeu de variantes qui emploie chaque nœud, chaque feuille, chaque effet et
// chaque évènement.
var casV2 = []struct{ nom, jeu, attendu string }{
	{"credit", "../directive/testdata/v2/credit.json", "testdata/v2/credit.txt"},
	{"weekend", "../directive/testdata/v2/weekend.json", "testdata/v2/weekend.txt"},
	{"top-management", "../directive/testdata/v2/top-management.json", "testdata/v2/top-management.txt"},
	{"rh", "../directive/testdata/v2/rh.json", "testdata/v2/rh.txt"},
	{"demandeur", "../directive/testdata/v2/demandeur.json", "testdata/v2/demandeur.txt"},
	{"complet", "testdata/complet-v2.json", "testdata/v2/complet.txt"},
	{"variantes", "testdata/variantes-v2.json", "testdata/v2/variantes.txt"},
	// Formes ajoutées par la session gate-v2 : $racine sous un
	// quantificateur, directives d'évènement évaluées par l'engine.
	{"gate-v2", "testdata/gate-v2.json", "testdata/v2/gate-v2.txt"},
}

// TestJeuV2_Attendu compare la relecture de chaque jeu v2 au fichier
// attendu, octet pour octet, sur plusieurs chargements et sous trois
// fuseaux locaux changés en cours d'exécution.
func TestJeuV2_Attendu(t *testing.T) {
	for _, c := range casV2 {
		t.Run(c.nom, func(t *testing.T) {
			got := Jeu(charger(t, c.jeu))
			if *maj {
				if err := os.MkdirAll("testdata/v2", 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(c.attendu, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(c.attendu)
			if err != nil {
				t.Fatal(err)
			}
			if got != string(want) {
				t.Fatalf("relecture différente de %s :\n--- obtenu\n%s\n--- attendu\n%s", c.attendu, got, want)
			}
			local := time.Local
			defer func() { time.Local = local }()
			for _, tz := range []*time.Location{time.UTC, time.FixedZone("UTC+13", 13*3600), time.FixedZone("UTC-10", -10*3600)} {
				time.Local = tz
				for i := 0; i < 3; i++ {
					if again := Jeu(charger(t, c.jeu)); again != got {
						t.Fatalf("relecture non déterministe (fuseau %s, passe %d)", tz, i)
					}
				}
			}
		})
	}
}

// TestJeuV2_FuseauxMachine relance les comparaisons aux fichiers attendus
// dans un sous-processus, sous trois fuseaux de machine (variable TZ) : le
// fuseau du système ne doit rien changer à la relecture.
func TestJeuV2_FuseauxMachine(t *testing.T) {
	if os.Getenv("RELECTURE_SOUS_PROCESSUS") != "" {
		t.Skip("déjà dans un sous-processus")
	}
	for _, tz := range []string{"UTC", "Asia/Tokyo", "America/Los_Angeles"} {
		cmd := exec.Command(os.Args[0], "-test.run=^TestJeuV2_Attendu$", "-test.count=1")
		cmd.Env = append(os.Environ(), "TZ="+tz, "RELECTURE_SOUS_PROCESSUS=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("TZ=%s : %v\n%s", tz, err, out)
		}
	}
}

// TestJeuV2_Couverture vérifie que le jeu de variantes et les scénarios,
// ensemble, emploient chaque nœud, chaque feuille, chaque effet et chaque
// évènement : un gabarit non couvert serait un gabarit jamais relu.
func TestJeuV2_Couverture(t *testing.T) {
	vus := map[string]bool{}
	var parcourir func(n *directive.Noeud)
	parcourir = func(n *directive.Noeud) {
		if n == nil {
			return
		}
		for _, cle := range []struct {
			pose bool
			nom  string
		}{
			{n.Et != nil, "et"}, {n.Ou != nil, "ou"}, {n.Non != nil, "non"}, {n.Existe != nil, "existe"},
			{n.Tous != nil, "tous"}, {n.Argument != nil, "argument"}, {n.AConsulte != nil, "session.a_consulte"},
			{n.Appelant != nil, "appelant"}, {n.SousAgentDeclare != nil, "sous_agent_declare"},
			{n.Demandeur != nil, "demandeur"}, {n.Attestation != nil, "attestation"}, {n.Temps != nil, "temps"},
			{n.EtiquetteCible != nil, "etiquette_cible"}, {n.MetadonneeReponse != nil, "metadonnee_reponse"},
			{n.Sujet != nil, "sujet"},
		} {
			if cle.pose {
				vus[cle.nom] = true
			}
		}
		if n.Argument != nil {
			vus["op "+string(n.Argument.Op)] = true
		}
		for _, e := range append(append([]*directive.Noeud{}, n.Et...), n.Ou...) {
			parcourir(e)
		}
		parcourir(n.Non)
		if n.Existe != nil {
			parcourir(n.Existe.Condition)
		}
		if n.Tous != nil {
			parcourir(n.Tous.Condition)
		}
	}
	for _, c := range casV2 {
		for _, d := range charger(t, c.jeu).Directives {
			if !d.EstV2() {
				continue
			}
			vus["evenement "+string(d.Declencheur.Evenement)] = true
			for _, e := range d.Effets {
				vus["effet "+string(e.Effet)] = true
			}
			parcourir(d.Arbre)
		}
	}
	attendus := []string{
		"et", "ou", "non", "existe", "tous", "argument", "session.a_consulte", "appelant",
		"sous_agent_declare", "demandeur", "attestation", "temps", "etiquette_cible",
		"metadonnee_reponse", "sujet",
		"op >", "op >=", "op <", "op <=", "op ==", "op !=", "op present", "op absent", "op dans", "op prefixe",
		"effet refuser", "effet attendre_validation", "effet caviarder", "effet notifier", "effet marquer",
		"evenement appel_outil", "evenement ouverture_session", "evenement attestation_echouee", "evenement purge",
	}
	for _, a := range attendus {
		if !vus[a] {
			t.Errorf("%s n'est employé par aucun jeu relu", a)
		}
	}
}

// TestJeuV2_RienDInconnu : sur des jeux acceptés par le chargeur, aucun
// gabarit de repli (« inconnu », « absent », « nœud vide ») n'apparaît.
func TestJeuV2_RienDInconnu(t *testing.T) {
	for _, c := range casV2 {
		got := Jeu(charger(t, c.jeu))
		for _, repli := range []string{`inconnu "`, "(nœud", "(forme absente)", "(valeur absente)", "(cible absente)", "déclencheur absent", "aucun fondement"} {
			if strings.Contains(got, repli) {
				t.Errorf("%s : la relecture contient le repli %q", c.nom, repli)
			}
		}
	}
}

// TestJeu_SansV1 : le gabarit v1 est retiré ; la phrase sur le cumul des
// notifications et des marques est toujours écrite.
func TestJeu_SansV1(t *testing.T) {
	for _, c := range casV2 {
		if got := Jeu(charger(t, c.jeu)); !strings.Contains(got, "se cumulent") || strings.Contains(got, "Verrou ") {
			t.Errorf("%s : relecture inattendue", c.nom)
		}
	}
}

// TestDirectiveV2_Phrases fixe quelques phrases que le juriste relit, sur
// des directives minimales.
func TestDirectiveV2_Phrases(t *testing.T) {
	cases := []struct{ nom, directive, veut string }{
		{"week-end",
			`"declencheur": {"evenement": "appel_outil", "cible": {"serveur": "*", "outil": "*", "acces": "action"}}, "condition": {"temps": {"jour_semaine": ["samedi", "dimanche"], "fuseau": "Europe/Paris"}}, "effets": [{"effet": "refuser"}]`,
			"Quand : tout appel d'outil, sur tout serveur, pour un outil classé « action » au registre.\n" +
				"Si l'appel a lieu un samedi ou un dimanche, en heure locale du fuseau Europe/Paris\n" +
				"Alors :\n  - l'appel est refusé\n" +
				"Si la règle ne peut pas être évaluée pour un appel (information absente, illisible ou inconnue du registre), l'appel est refusé.\n"},
		{"heure à cheval sur minuit",
			`"declencheur": {"evenement": "appel_outil", "cible": {"serveur": "banque-synthetique"}}, "condition": {"temps": {"heure": {"de": "22:00", "a": "06:00"}, "fuseau": "Europe/Paris"}}, "effets": [{"effet": "attendre_validation", "par_role": "astreinte"}]`,
			"Quand : tout appel d'outil du serveur banque-synthetique.\n" +
				"Si l'appel a lieu entre 22:00 (inclus) et 06:00 (exclu), à cheval sur minuit, en heure locale du fuseau Europe/Paris\n" +
				"Alors :\n  - l'appel attend une validation humaine par le rôle « astreinte »\n" +
				"Si la règle ne peut pas être évaluée pour un appel (information absente, illisible ou inconnue du registre), l'appel est refusé.\n"},
		{"sans condition",
			`"declencheur": {"evenement": "appel_outil", "cible": {"serveur": "recherche-entreprises"}}, "effets": [{"effet": "caviarder", "champs_autorises": ["/siren", "/nom"], "mode": "caviarder"}]`,
			"Quand : tout appel d'outil du serveur recherche-entreprises.\n" +
				"Condition : aucune, la directive s'applique à chaque fois.\n" +
				"Alors :\n  - seuls les champs /siren, /nom sont transmis (un champ autorisé couvre aussi tout son contenu) ; tout autre champ est retiré avant transmission\n"},
	}
	for _, c := range cases {
		t.Run(c.nom, func(t *testing.T) {
			doc := `{"jeu": {"id": "t", "version": 1, "date_effet": "2026-10-04T08:00:00Z", "auteur": "j"}, "directives": [{"id": "D-1", "version": 1, "statut": "brouillon", "date_effet": "2026-10-04T08:00:00Z", "texte_fr": "t", "fondements": [{"texte": "RGPD"}], ` + c.directive + `}]}`
			jeu, err := directive.Charger([]byte(doc))
			if err != nil {
				t.Fatal(err)
			}
			got := Directive(jeu.Directives[0])
			_, regle, _ := strings.Cut(got, "Pour tous les appelants.\n")
			if regle != c.veut {
				t.Errorf("obtenu :\n%s\nattendu :\n%s", regle, c.veut)
			}
		})
	}
}
