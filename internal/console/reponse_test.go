package console

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Texte d'une réponse telle que l'orchestrateur l'écrit : Markdown du
// modèle, puis le relevé « Décisions de l'engine ». La balise img ne doit
// jamais devenir un élément.
const texteReponseMarkdown = "Le dossier **DOS-FICTIF-003** est recevable.\n\n" +
	"Points vérifiés :\n- gels des avoirs : aucun\n- seuil de l'usure : consulté via banque-synthetique__get_taux_interne\n\n" +
	"<img src=x onerror=alert(1)> reste du texte.\n\n" +
	"Décisions de l'engine :\n- analyse-dossier · banque-synthetique__evaluer_dossier : transmis, données reçues\n" +
	"- decision · banque-synthetique__attribuer_pret : refusé (D-001)"

// dossierReponse écrit une demande sans journal (etat.json, reponse.json).
func (b *banc) dossierReponse(id, texte string) {
	b.t.Helper()
	d := filepath.Join(b.Dep.Donnees, id)
	must(b.t, os.MkdirAll(d, 0o755))
	must(b.t, os.WriteFile(filepath.Join(d, "etat.json"), []byte(`{"statut":"termine","demande":"Le dossier DOS-FICTIF-003 est-il recevable ?","demandeur":"`+claire+`","debut":"2026-10-04T10:00:00Z","fin":"2026-10-04T10:00:03Z","attente":null}`), 0o644))
	r, _ := json.Marshal(map[string]any{"texte": texte, "statut": "termine", "directives_citees": []string{"D-001"}})
	must(b.t, os.WriteFile(filepath.Join(d, "reponse.json"), r, 0o644))
}

// Point 3 (console-3) : le relevé « Décisions de l'engine » est retiré du
// texte et rendu à part.
func TestReponseSansReleve(t *testing.T) {
	b := nouveauBanc(t)
	b.dossierReponse("20261004-100000-mdwn", texteReponseMarkdown)
	_, corps := get(t, b.Srv, "/api/journal/20261004-100000-mdwn/poste")
	var v struct {
		Reponse Reponse `json:"reponse"`
	}
	must(t, json.Unmarshal([]byte(corps), &v))
	if strings.Contains(v.Reponse.Texte, "Décisions de l'engine") || !strings.HasSuffix(v.Reponse.Texte, "reste du texte.") {
		t.Errorf("texte : %q", v.Reponse.Texte)
	}
	if !strings.HasPrefix(v.Reponse.DetailTechnique, "- analyse-dossier · banque-synthetique__evaluer_dossier") {
		t.Errorf("détail : %q", v.Reponse.DetailTechnique)
	}
	for _, c := range []struct{ entree, corps, releve string }{
		{"Réponse seule.", "Réponse seule.", ""},
		{"Décisions de l'engine :\n- a", "", "- a"},
		{"Cf. « Décisions de l'engine : » plus haut.", "Cf. « Décisions de l'engine : » plus haut.", ""},
	} {
		if c2, r2 := separerReleve(c.entree); c2 != c.corps || r2 != c.releve {
			t.Errorf("%q → %q, %q", c.entree, c2, r2)
		}
	}
}

// Point 3 (console-3) : libellés lisibles, d'après le catalogue du dépôt.
func TestLibellesOutils(t *testing.T) {
	racine, _ := filepath.Abs("../..")
	d := &Depot{Commandes: Commandes{Racine: racine}}
	l := d.LibellesOutils()
	p := l["banque-synthetique__attribuer_pret"]
	if p.Libelle != "Attribution de prêt — banque" || p.Technique != "banque-synthetique__attribuer_pret" || !strings.Contains(p.Description, "prêt") {
		t.Errorf("attribuer_pret : %+v", p)
	}
	if l["gels-avoirs__verifier_gel"].Libelle != "Vérification des gels d'avoirs — DG Trésor" {
		t.Errorf("verifier_gel : %+v", l["gels-avoirs__verifier_gel"])
	}
	for k, v := range l {
		if strings.Contains(v.Libelle, "__") || strings.Contains(v.Libelle, "_") {
			t.Errorf("%s : libellé technique %q", k, v.Libelle)
		}
	}
	// Outil absent de la table : le début de sa description.
	if got := libelleDe("nouveau-serveur", "lire_x", "Lit un registre public (source X) : champs."); got != "Lit un registre public — nouveau-serveur" {
		t.Errorf("repli : %q", got)
	}
	if got := libelleDe("s", "lire_x", ""); got != "Lire x — s" {
		t.Errorf("repli sans description : %q", got)
	}
	srv := &Serveur{Depot: d}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "http://127.0.0.1/api/outils", nil))
	if !strings.Contains(rec.Body.String(), `"Attribution de prêt — banque"`) {
		t.Errorf("/api/outils : %.300s", rec.Body.String())
	}
}

// Point 3 (console-3), dans le navigateur : Markdown rendu (gras, listes,
// paragraphes), balise du texte jamais interprétée, relevé replié derrière
// « voir le détail technique », noms d'outils en libellés.
func TestNavigateurReponse(t *testing.T) {
	py := navigateur(t)
	b := nouveauBanc(t)
	b.dossierReponse("20261004-100000-mdwn", texteReponseMarkdown)
	web, _ := filepath.Abs("web")
	b.Srv.Dev = web
	srv := httptest.NewServer(b.Srv.Handler())
	defer srv.Close()
	script, _ := filepath.Abs("testdata/navigateur/reponse.py")
	out, err := exec.Command(py, script, srv.URL, "20261004-100000-mdwn").CombinedOutput()
	t.Logf("%s", out)
	if err != nil {
		t.Fatalf("réponse mal rendue : %v", err)
	}
}
