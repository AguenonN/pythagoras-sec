package console

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ecrireMistral écrit, comme l'orchestrateur, un fichier par réponse du
// modèle : la requête porte tout l'historique (les appels déjà faits), la
// réponse le nouvel appel. Les fichiers portent un nom sans ordre (hachage) :
// seule la date de la réponse les ordonne.
func ecrireMistral(t *testing.T, dossier string, appels []map[string]any) {
	t.Helper()
	rep := filepath.Join(dossier, "mistral")
	must(t, os.MkdirAll(rep, 0o755))
	var historique []map[string]any
	base := time.Date(2026, 10, 2, 15, 0, 0, 0, time.UTC).Unix()
	// Les noms de fichier sont à l'envers : l'ordre vient de « created ».
	for i, a := range appels {
		id := fmt.Sprintf("tc%02d", i)
		args, _ := json.Marshal(a["args"])
		appel := map[string]any{"id": id, "type": "function", "function": map[string]any{"name": a["nom"], "arguments": string(args)}}
		doc := map[string]any{
			"requete": map[string]any{"messages": append([]map[string]any{{"role": "system", "content": "s"}}, historique...)},
			"reponse": map[string]any{"created": base + int64(i), "choices": []map[string]any{{"message": map[string]any{
				"role": "assistant", "content": "", "tool_calls": []any{appel}}}}},
		}
		brut, _ := json.Marshal(doc)
		must(t, os.WriteFile(filepath.Join(rep, fmt.Sprintf("%02x%s.json", 255-i, id)), brut, 0o644))
		historique = append(historique, map[string]any{"role": "assistant", "tool_calls": []any{appel}},
			map[string]any{"role": "tool", "content": "{}"})
	}
}

// Une demande avec caviardage : ce que le modèle a demandé à côté de ce que
// l'engine a décidé ; les champs retirés ne sont jamais rendus ; la
// délégation est « hors du gate ».
func TestDemandeEtDecide(t *testing.T) {
	b := nouveauBanc(t)
	id := b.demande(claire, "Peut-on accorder le prêt demandé sans gel vérifié ?")
	st := b.etat(id)
	ch, _ := b.Dep.Chaine(st, b.jeux(), "")

	appels := []map[string]any{{"nom": "deleguer", "args": map[string]any{"sous_agent": "credit", "tache": "Instruire la demande"}}}
	var secret string
	for _, e := range ch.Etapes {
		args, ok := st.Coffre.Arguments(e.EmpreinteOrigine)
		if !ok {
			t.Fatalf("arguments absents du coffre pour #%d", e.Seq)
		}
		appels = append(appels, map[string]any{"nom": e.Serveur + "__" + e.Outil, "args": args})
		for _, c := range e.Champs {
			if v, ok := args[strings.TrimPrefix(c, "/")]; ok && secret == "" {
				secret = fmt.Sprint(v)
			}
		}
	}
	if secret == "" {
		t.Skip("aucun champ retiré dans cette session de démonstration")
	}
	ecrireMistral(t, st.Journal.Dossier(), appels)

	code, corps := get(t, b.Srv, "/api/journal/"+id+"/demande")
	if code != 200 {
		t.Fatalf("%d %s", code, corps)
	}
	if strings.Contains(corps, `"`+secret+`"`) || strings.Contains(corps, ":"+secret+",") {
		t.Fatalf("la valeur d'un champ retiré apparaît : %q", secret)
	}
	var v VueDemande
	must(t, json.Unmarshal([]byte(corps), &v))
	if v.Fichiers != len(appels) || len(v.Demandes) != len(appels) {
		t.Fatalf("une ligne par appel demandé, dédoublonnée : %d fichiers, %d lignes", v.Fichiers, len(v.Demandes))
	}
	d0 := v.Demandes[0]
	if !d0.Delegation || !d0.HorsGate || d0.Decision != nil || d0.Outil != "deleguer" {
		t.Fatalf("délégation : %+v", d0)
	}
	retire, derniere := false, v.Demandes[len(v.Demandes)-1]
	for i, l := range v.Demandes[1:] {
		if l.Decision == nil || l.NonRecue || l.SansDemande || l.Decision.Seq != ch.Etapes[i].Seq || l.Outil != l.Decision.Serveur+"__"+l.Decision.Outil {
			t.Fatalf("ligne %d : %+v", i, l)
		}
		for _, a := range l.Arguments {
			if a.Retire {
				retire = true
				if a.Valeur != "" {
					t.Fatalf("champ retiré affiché : %+v", a)
				}
			}
		}
	}
	if !retire {
		t.Fatal("un champ retiré doit être signalé comme retiré")
	}
	if derniere.Decision.Badge != BadgeRefuse || len(derniere.Decision.Apps) == 0 || derniere.Decision.Apps[0].ID != "D-001" || derniere.Decision.Apps[0].Fondement.Texte == "" {
		t.Fatalf("décision : directive et fondement attendus : %+v", derniere.Decision)
	}
	// Sans fichier mistral/ : les décisions restent, sans demande retrouvée.
	must(t, os.RemoveAll(filepath.Join(st.Journal.Dossier(), "mistral")))
	_, corps = get(t, b.Srv, "/api/journal/"+id+"/demande")
	must(t, json.Unmarshal([]byte(corps), &v))
	if v.Fichiers != 0 || len(v.Demandes) != len(ch.Etapes) || !v.Demandes[0].SansDemande {
		t.Fatalf("sans fichier du modèle : %d lignes, %d fichiers", len(v.Demandes), v.Fichiers)
	}
}

// Appel mis en attente puis validé : une seule ligne, la décision d'attente
// et, à côté, l'appel repris avec ses valideurs.
func TestDemandeAttentePuisValidation(t *testing.T) {
	b := nouveauBanc(t)
	id, fin := b.demandeAsync(claire, "Peut-on accorder un prêt de 120 000 euros ?")
	b.attendreStatut(id, StatutEnAttente)
	if _, err := b.Dep.Valider(id, anna, "analyste-risques"); err != nil {
		t.Fatal(err)
	}
	if err := <-fin; err != nil {
		t.Fatal(err)
	}
	st := b.etat(id)
	ch, _ := b.Dep.Chaine(st, b.jeux(), "")
	var appels []map[string]any
	for _, e := range ch.Etapes {
		if e.Badge == BadgeAutorise && len(e.ValidePar) > 0 {
			continue // l'appel repris n'est pas redemandé : même demande du modèle
		}
		args, _ := st.Coffre.Arguments(e.EmpreinteOrigine)
		appels = append(appels, map[string]any{"nom": e.Serveur + "__" + e.Outil, "args": args})
	}
	ecrireMistral(t, st.Journal.Dossier(), appels)
	_, corps := get(t, b.Srv, "/api/journal/"+id+"/demande")
	var v VueDemande
	must(t, json.Unmarshal([]byte(corps), &v))
	if len(v.Demandes) != len(appels) {
		t.Fatalf("%d lignes pour %d demandes : %s", len(v.Demandes), len(appels), corps)
	}
	trouve := false
	for _, l := range v.Demandes {
		if l.Decision != nil && l.Decision.Badge == BadgeAttente {
			trouve = true
			if l.Suite == nil || l.Suite.Badge != BadgeAutorise || len(l.Suite.ValidePar) != 1 || l.Suite.ValidePar[0].Nom == "" {
				t.Fatalf("attente sans suite validée : %+v", l.Suite)
			}
		}
	}
	if !trouve {
		t.Fatal("l'appel en attente est introuvable")
	}
}
