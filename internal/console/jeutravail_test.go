package console

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// jeuSans écrit le jeu en vigueur privé de directives (comme
// « lancer-demo.sh --sans-directive » : empreinte retirée, le chargeur la
// recalcule).
func jeuSans(t *testing.T, brut []byte, ids ...string) []byte {
	t.Helper()
	var doc map[string]any
	must(t, json.Unmarshal(brut, &doc))
	retirer := map[string]bool{}
	for _, id := range ids {
		retirer[id] = true
	}
	var gardees []any
	for _, d := range doc["directives"].([]any) {
		if !retirer[d.(map[string]any)["id"].(string)] {
			gardees = append(gardees, d)
		}
	}
	doc["directives"] = gardees
	delete(doc["jeu"].(map[string]any), "empreinte")
	delete(doc["jeu"].(map[string]any), "signature")
	out, err := json.Marshal(doc)
	must(t, err)
	return out
}

// demandeSous exécute une demande avec un autre jeu à la place de demo.json
// (le simulateur le charge au démarrage de la demande), puis le remet.
func (b *banc) demandeSous(jeu []byte, demandeur, texte string) string {
	b.t.Helper()
	chemin := filepath.Join(b.racine, "demo", "directives", "demo.json")
	orig, err := os.ReadFile(chemin)
	must(b.t, err)
	must(b.t, os.WriteFile(chemin, jeu, 0o644))
	defer func() { must(b.t, os.WriteFile(chemin, orig, 0o644)) }()
	return b.demande(demandeur, texte)
}

// Une session lancée avec un jeu de travail (« sans la directive D-001 », ou
// un fichier de demo/scenarios/jeux/) se rejoue : la console retrouve
// l'ensemble par l'empreinte consignée au journal.
func TestSessionsSousJeuDeTravail(t *testing.T) {
	b := nouveauBanc(t)
	orig, err := os.ReadFile(filepath.Join(b.racine, "demo", "directives", "demo.json"))
	must(t, err)
	texte := "Peut-on accorder le prêt demandé sans gel vérifié ?"

	idRef := b.demande(claire, texte)
	idSans := b.demandeSous(jeuSans(t, orig, "D-001"), claire, texte)
	// Deux directives en moins : pas dérivable d'un seul retrait, mais présent
	// dans un fichier des jeux de travail des scénarios.
	fichier := jeuSans(t, orig, "D-002", "D-003")
	must(t, os.MkdirAll(filepath.Join(b.racine, "demo", "scenarios", "jeux"), 0o755))
	must(t, os.WriteFile(filepath.Join(b.racine, "demo", "scenarios", "jeux", "travail.json"), fichier, 0o644))
	idFichier := b.demandeSous(fichier, claire, texte)

	jeux := b.jeux()
	for _, id := range []string{idSans, idFichier} {
		st := b.etat(id)
		var ref bool
		for _, e := range st.Entrees {
			if e.Jeu != nil {
				_, ref = trouverJeu(jeux, e.Jeu)
				break
			}
		}
		if ref {
			t.Fatalf("%s : le jeu de travail ne doit pas être dans les règles connues (prémisse)", id)
		}
	}

	va := b.Dep.AvantApres(b.tous(), jeux, "D-001", nil)
	if va.Compteurs.Appels == 0 || va.Compteurs.NonRejouables != 0 {
		t.Fatalf("sessions sous jeu de travail non rejouables : %+v", va.Compteurs)
	}
	regles := map[string]string{}
	for _, s := range va.Sessions {
		regles[s.Journal] = s.Regles
	}
	if regles[idRef] != "" || regles[idSans] != "sans la directive D-001" || regles[idFichier] != "règles de travail « travail »" {
		t.Fatalf("règles des sessions : %+v", regles)
	}
	// « Sans D-001 » : la session sous le jeu courant (idRef) est refusée par
	// D-001 ; sous le jeu sans D-001 (idSans) elle ne l'est pas. L'écran 4 les
	// rejoue toutes deux, avec et sans la directive, sans « non rejouable ».
	for _, id := range []string{idSans, idFichier} {
		v, ok := b.Dep.Chaine(b.etat(id), jeux, "")
		if !ok || v.Regles == nil {
			t.Fatalf("%s : règles de la session non retrouvées", id)
		}
	}
	e := b.etat(idSans)
	ch, _ := b.Dep.Chaine(e, jeux, "")
	if dernier := ch.Etapes[len(ch.Etapes)-1]; dernier.Badge == BadgeRefuse && dernier.Outil == "attribuer_pret" && len(dernier.Apps) > 0 && dernier.Apps[0].ID == "D-001" {
		t.Fatalf("D-001 ne devait pas s'appliquer sous le jeu sans D-001 : %+v", dernier)
	}
	if n := b.Dep.File(b.tous(), jeux, ch.Ouverte).Erreur; n != "" {
		t.Fatalf("file : %s", n)
	}
}

// Une directive qui n'existe que dans un jeu de travail (D-009 avant son
// activation) se compare : « sans » = ce jeu privé d'elle, « avec » = ce jeu.
func TestComparaisonDUneDirectiveDeJeuDeTravail(t *testing.T) {
	b := nouveauBanc(t)
	orig, err := os.ReadFile(filepath.Join(b.racine, "demo", "directives", "demo.json"))
	must(t, err)
	var doc map[string]any
	must(t, json.Unmarshal(orig, &doc))
	var d4 map[string]any
	for _, d := range doc["directives"].([]any) {
		if d.(map[string]any)["id"] == "D-004" {
			brut, _ := json.Marshal(d)
			must(t, json.Unmarshal(brut, &d4))
		}
	}
	d4["id"] = "D-099"
	doc["directives"] = append(doc["directives"].([]any), d4)
	delete(doc["jeu"].(map[string]any), "empreinte")
	avec, err := json.Marshal(doc)
	must(t, err)
	must(t, os.MkdirAll(filepath.Join(b.racine, "demo", "scenarios", "jeux"), 0o755))
	must(t, os.WriteFile(filepath.Join(b.racine, "demo", "scenarios", "jeux", "avec-d099.json"), avec, 0o644))
	b.demandeSous(avec, claire, "Peut-on accorder le prêt demandé sans gel vérifié ?")

	va := b.Dep.AvantApres(b.tous(), b.jeux(), "D-099", nil)
	trouvee := false
	for _, d := range va.Directives {
		trouvee = trouvee || d.ID == "D-099"
	}
	if !trouvee || va.Choix == nil || va.Choix.ID != "D-099" || va.LibelleAvant != "Sans la directive D-099" || va.LibelleApres != "Avec la directive D-099" {
		t.Fatalf("directive du jeu de travail : %+v %q", va.Choix, va.LibelleApres)
	}
	if va.Compteurs.Appels == 0 || va.Compteurs.NonRejouables != 0 {
		t.Fatalf("compteurs : %+v", va.Compteurs)
	}
	// Elle n'entre pas dans les règles en vigueur.
	for _, j := range b.jeux() {
		for _, d := range j.Jeu.Directives {
			if d.ID == "D-099" {
				t.Fatal("un jeu de travail ne doit pas entrer dans les règles connues")
			}
		}
	}
}
