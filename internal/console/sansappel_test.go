package console

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mcprogue/internal/annuaire"
)

func get(t *testing.T, srv *Serveur, chemin string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1"+chemin, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

// aucunNull cherche une valeur null dans une réponse JSON : l'interface
// parcourt les listes, un null les fait échouer.
func aucunNull(t *testing.T, chemin string, v any, trace string) {
	t.Helper()
	switch x := v.(type) {
	case nil:
		t.Errorf("%s : null en %s", chemin, trace)
	case map[string]any:
		for k, e := range x {
			aucunNull(t, chemin, e, trace+"."+k)
		}
	case []any:
		for i, e := range x {
			aucunNull(t, chemin, e, trace+"[]"+string(rune('0'+i%10)))
		}
	}
}

// Une demande à laquelle l'orchestrateur répond sans appeler d'outil n'a ni
// session d'outils ni, parfois, de journal : elle s'affiche tout de même,
// avec le texte de sa réponse, son statut et son demandeur.
func TestDemandeSansAppelDOutil(t *testing.T) {
	b := nouveauBanc(t)
	sessions := b.Dep.Donnees
	// Dossier 1 : etat.json et reponse.json seuls. Dossier 2 : journal vide.
	for id, avecJournal := range map[string]bool{"20261004-100000-aaaa": false, "20261004-100100-bbbb": true} {
		d := filepath.Join(sessions, id)
		must(t, os.MkdirAll(d, 0o755))
		must(t, os.WriteFile(filepath.Join(d, "etat.json"), []byte(`{"statut":"termine","demande":"Liste des employés de la boite","demandeur":"`+claire+`","debut":"2026-10-04T10:00:00Z","fin":"2026-10-04T10:00:03Z","attente":null}`), 0o644))
		must(t, os.WriteFile(filepath.Join(d, "reponse.json"), []byte(`{"texte":"Cette demande sort de mon périmètre : je ne peux pas lister les employés.","statut":"refuse","directives_citees":[]}`), 0o644))
		if avecJournal {
			must(t, os.WriteFile(filepath.Join(d, "journal.jsonl"), nil, 0o644))
		}
	}
	code, corps := get(t, b.Srv, "/api/etat")
	if code != 200 {
		t.Fatalf("etat : %d %s", code, corps)
	}
	var etat struct {
		Journaux []struct {
			ID       string          `json:"id"`
			Demande  string          `json:"demande"`
			Statut   string          `json:"statut"`
			Sessions json.RawMessage `json:"sessions"`
		} `json:"journaux"`
	}
	must(t, json.Unmarshal([]byte(corps), &etat))
	if len(etat.Journaux) != 2 {
		t.Fatalf("une ligne par demande attendue : %s", corps)
	}
	for _, j := range etat.Journaux {
		if string(j.Sessions) != "[]" || j.Demande == "" || j.Statut != "termine" {
			t.Errorf("demande %s : %+v", j.ID, j)
		}
		for _, ecran := range []string{"poste", "chaine"} {
			code, corps := get(t, b.Srv, "/api/journal/"+j.ID+"/"+ecran)
			if code != 200 {
				t.Fatalf("%s %s : %d %s", ecran, j.ID, code, corps)
			}
			contient(t, corps, claire, "statut_session")
			if ecran == "poste" {
				contient(t, corps, "ne peux pas lister les employés", `"terminee":true`, `"statut":"refuse"`)
			}
			var v any
			must(t, json.Unmarshal([]byte(corps), &v))
			aucunNull(t, ecran, v, "")
		}
	}
}

// Un dossier de sessions vide : aucune liste de l'API ne vaut null.
func TestListesVidesJamaisNull(t *testing.T) {
	b := nouveauBanc(t)
	must(t, os.RemoveAll(b.Dep.Donnees))
	must(t, os.MkdirAll(b.Dep.Donnees, 0o755))
	for _, chemin := range []string{"/api/etat", "/api/identites", "/api/lancements", "/api/regles", "/api/avant-apres",
		"/api/file", "/api/notifications", "/api/redaction"} {
		code, corps := get(t, b.Srv, chemin)
		if code != 200 {
			if strings.Contains(corps, "null") {
				t.Errorf("%s : %d %s", chemin, code, corps)
			}
			continue
		}
		var v any
		must(t, json.Unmarshal([]byte(corps), &v))
		aucunNull(t, chemin, v, "")
	}
}

// Avec des sessions réelles aussi : aucune liste ne vaut null, quel que soit
// l'écran, qu'il y ait ou non des appels.
func TestListesPleinesJamaisNull(t *testing.T) {
	b := nouveauBanc(t)
	id := b.demande(claire, "Peut-on accorder le prêt demandé sans gel vérifié ?")
	sid := Sessions(b.etat(id))[0].ID
	for _, chemin := range []string{"/api/etat", "/api/identites", "/api/regles", "/api/avant-apres", "/api/file",
		"/api/notifications", "/api/redaction", "/api/journal/" + id + "/session/" + sid + "/chaine",
		"/api/journal/" + id + "/session/" + sid + "/poste", "/api/journal/" + id + "/poste"} {
		code, corps := get(t, b.Srv, chemin)
		if code != 200 {
			t.Errorf("%s : %d %s", chemin, code, corps)
			continue
		}
		var v any
		must(t, json.Unmarshal([]byte(corps), &v))
		aucunNull(t, chemin, v, "")
	}
}

// Session lancée avec --horloge-depart : la console signe à l'heure simulée
// (départ + temps écoulé) et le jeton est accepté par un gate dont l'horloge
// part du même instant ; signé à l'heure réelle, il serait périmé.
func TestSignatureALHeureSimulee(t *testing.T) {
	b := nouveauBanc(t)
	id, fin := b.demandeAsync(claire, "Peut-on accorder un prêt de 120 000 euros ?")
	st := b.attendreStatut(id, StatutEnAttente)
	att := st.Dossier.Attente
	depart := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Second)
	h, _ := json.Marshal(map[string]string{"depart": depart.Format(time.RFC3339),
		"reel": time.Now().UTC().Add(-30 * time.Second).Format(time.RFC3339Nano)})
	must(t, os.WriteFile(filepath.Join(st.Journal.Dossier(), "horloge.json"), h, 0o644))

	if _, err := b.Dep.Valider(id, anna, "analyste-risques"); err != nil {
		t.Fatal(err)
	}
	jeton, err := os.ReadFile(filepath.Join(b.racine, "demo", "execution", "approbations", att.EmpreinteTransmise, anna+".jwt"))
	must(t, err)
	loc, _, err := b.Dep.Annuaire()
	must(t, err)
	// Horloge du gate : départ + 30 s (temps écoulé) + une marge de lancement.
	gate := depart.Add(35 * time.Second)
	ap, err := annuaire.VerifierApprobation(loc, strings.TrimSpace(string(jeton)), att.EmpreinteTransmise, gate)
	if err != nil {
		t.Fatalf("jeton refusé à l'horloge simulée : %v", err)
	}
	if d := ap.Date.Sub(depart.Add(30 * time.Second)); d < -2*time.Second || d > 5*time.Second {
		t.Fatalf("iat %v loin de départ + 30 s", ap.Date)
	}
	if _, err := annuaire.VerifierApprobation(loc, strings.TrimSpace(string(jeton)), att.EmpreinteTransmise, time.Now()); err == nil {
		t.Fatal("le jeton doit porter l'heure simulée, pas l'heure réelle")
	}
	_ = fin
}

// Demandes suggérées : titre, demande et demandeur lus dans les scénarios ;
// un scénario sans titre est ignoré ; la lecture ne lance rien.
func TestDemandesSuggerees(t *testing.T) {
	b := nouveauBanc(t)
	dest := filepath.Join(b.racine, "demo", "scenarios")
	must(t, os.MkdirAll(dest, 0o755))
	ents, err := os.ReadDir("testdata/scenarios")
	must(t, err)
	for _, e := range ents {
		brut, err := os.ReadFile(filepath.Join("testdata/scenarios", e.Name()))
		must(t, err)
		must(t, os.WriteFile(filepath.Join(dest, e.Name()), brut, 0o644))
	}
	must(t, os.WriteFile(filepath.Join(dest, "casse.json"), []byte("{pas du json"), 0o644))
	avant := b.Dep.Journaux()

	code, corps := get(t, b.Srv, "/api/suggestions")
	if code != 200 {
		t.Fatalf("%d %s", code, corps)
	}
	var rep struct{ Suggestions []Suggestion }
	must(t, json.Unmarshal([]byte(corps), &rep))
	if len(rep.Suggestions) != 2 {
		t.Fatalf("deux suggestions attendues (sans-titre et fichier cassé ignorés) : %+v", rep.Suggestions)
	}
	if s := rep.Suggestions[0]; s.Fichier != "pret-simple" || s.Titre != "Prêt sans gel vérifié" || s.Demandeur != claire ||
		!strings.Contains(s.Demande, "sans gel vérifié") {
		t.Fatalf("première suggestion : %+v", s)
	}
	if s := rep.Suggestions[1]; s.Fichier != "rapport" || s.Demandeur != bruno {
		t.Fatalf("seconde suggestion : %+v", s)
	}
	if apres := b.Dep.Journaux(); len(apres) != len(avant) {
		t.Fatal("lire les suggestions ne lance rien")
	}
	// Sans dossier de scénarios : liste vide, jamais null.
	must(t, os.RemoveAll(dest))
	if _, corps := get(t, b.Srv, "/api/suggestions"); !strings.Contains(corps, `"suggestions":[]`) {
		t.Fatalf("liste vide attendue : %s", corps)
	}
}
