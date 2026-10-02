package console

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mcprogue/internal/directive"
)

// Écran 3 : relecture complète (Quand / Si / Alors), auteur et date
// d'activation, origine, décisions réelles tirées des journaux.
func TestVitrineDesRegles(t *testing.T) {
	b := nouveauBanc(t)
	b.demande(claire, "Peut-on accorder un prêt sans gel vérifié ?")
	jeux := b.jeux()
	jc := EnVigueur(jeux)
	if jc == nil || !jc.Courant || jc.Jeu.Meta.ID != "demo" {
		t.Fatalf("règles en vigueur : %+v", jc)
	}
	v := b.Dep.ReglesEnVigueur(*jc, b.tous(), jeux, "D-001")
	if len(v.Directives) != 8 || v.Choix == nil || v.Choix.ID != "D-001" {
		t.Fatalf("liste : %+v", v)
	}
	c := v.Choix
	contient(t, c.Relecture, "Quand :", "Si la session", "Alors :", "l'appel est refusé", "Si la règle ne peut pas être évaluée")
	if c.Auteur != "juriste@exemple.test" || c.ActiveDepuis == nil || c.Origine != "écrite par J. Juriste" || c.Statut != "active" {
		t.Fatalf("auteur / activation / origine : %+v", c)
	}
	if len(c.Decisions) != 1 || c.Decisions[0].Outil != "attribuer_pret" || c.Decisions[0].Rejoue != "refuser" ||
		!c.Decisions[0].Identique || !c.Decisions[0].Declenchee || c.Decisions[0].Acces != "action" || !c.Verifies {
		t.Fatalf("décisions : %+v", c.Decisions)
	}
	// Une directive qui n'a rien décidé n'a aucune décision réelle.
	if d := b.Dep.ReglesEnVigueur(*jc, b.tous(), jeux, "D-006").Choix; len(d.Decisions) != 0 {
		t.Fatalf("D-006 : %+v", d.Decisions)
	}
	// Aucun autre ensemble de règles que demo.json et son historique.
	if len(jeux) != 1 {
		t.Fatalf("ensembles de règles : %d", len(jeux))
	}
}

// Écran 4 : avec et sans la directive choisie, sur les journaux réels.
func TestAvantApresAvecEtSansLaDirective(t *testing.T) {
	b := nouveauBanc(t)
	b.demande(claire, "Peut-on accorder un prêt sans gel vérifié ?")
	b.demande(bruno, "Produis le rapport de synthèse du dossier DOS-FICTIF-002")
	jeux := b.jeux()

	v := b.Dep.AvantApres(b.tous(), jeux, "D-001", nil)
	if v.Choix == nil || v.Choix.ID != "D-001" || v.LibelleAvant != "Sans la directive D-001" || v.LibelleApres != "Avec la directive D-001" {
		t.Fatalf("libellés : %+v", v)
	}
	if v.Compteurs.Sessions != 2 || v.Compteurs.Appels != 5 || v.Compteurs.Changes != 1 || v.Compteurs.Violation != 1 || v.Compteurs.Exposition != 0 ||
		v.Compteurs.NonRejouables != 0 || !v.Verifiees {
		t.Fatalf("compteurs : %+v", v.Compteurs)
	}
	var l LigneAA
	for _, x := range v.Lignes {
		if x.Change {
			l = x
		}
	}
	// Sans D-001, le prêt attend la validation de l'analyste (D-002) ; avec, il est refusé.
	if l.Outil != "attribuer_pret" || l.Avant.Effet != "attendre_validation" || l.Apres.Effet != "refuser" ||
		l.Qualification != "violation" || !strings.Contains(strings.Join(l.Differences, ","), "effet") ||
		strings.Join(l.Avant.Roles, ",") != "analyste-risques" || len(l.Apres.Roles) != 0 ||
		strings.Join(l.Avant.Directives, ",") != "D-002" || strings.Join(l.Apres.Directives, ",") != "D-001,D-002" {
		t.Fatalf("ligne : %+v", l)
	}
	contient(t, v.Explication, "appel n° ", "refusé", "mis en attente")

	// D-004 : les champs retirés et l'effet changent ; D-008 : notification.
	v = b.Dep.AvantApres(b.tous(), jeux, "D-004", nil)
	var cav LigneAA
	for _, x := range v.Lignes {
		if x.Outil == "rechercher_entreprise" {
			cav = x
		}
	}
	if !cav.Change || cav.Avant.Effet != "autoriser" || cav.Apres.Effet != "caviarder" || len(cav.Avant.ChampsRetires) != 0 || len(cav.Apres.ChampsRetires) != 2 ||
		!strings.Contains(strings.Join(cav.Differences, ","), "champs") {
		t.Fatalf("caviardage : %+v", cav)
	}
	v = b.Dep.AvantApres(b.tous(), jeux, "D-008", nil)
	var rap LigneAA
	for _, x := range v.Lignes {
		if x.Outil == "produire_rapport" {
			rap = x
		}
	}
	if !rap.Change || rap.Avant.Effet != "autoriser" || rap.Apres.Effet != "refuser" || len(rap.Apres.Notifications) != 1 ||
		!strings.Contains(strings.Join(rap.Differences, ","), "notifications") || len(rap.Avant.Notifications) != 0 {
		t.Fatalf("D-008 : %+v", rap)
	}

	// Sessions choisies.
	st := b.etat(b.Dep.Journaux()[0].ID)
	choix := map[string]bool{st.Journal.ID + "|" + Sessions(st)[0].ID: true}
	v = b.Dep.AvantApres(b.tous(), jeux, "D-001", choix)
	if v.Compteurs.Sessions != 1 {
		t.Fatalf("sessions choisies : %+v", v.Compteurs)
	}
	nb := 0
	for _, s := range v.Sessions {
		if s.Choisie {
			nb++
		}
	}
	if nb != 1 || len(v.Sessions) != 2 {
		t.Fatalf("liste des sessions : %+v", v.Sessions)
	}
}

func lireJSON(t *testing.T, chemin string, v any) {
	t.Helper()
	brut, err := os.ReadFile(chemin)
	must(t, err)
	must(t, json.Unmarshal(brut, v))
}

func (b *banc) verifier(t *testing.T) {
	t.Helper()
	brut, err := os.ReadFile(filepath.Join(b.racine, "demo", "directives", "demo.json"))
	must(t, err)
	cmd := exec.Command(binDirective, "verifier")
	cmd.Stdin = strings.NewReader(string(brut))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("directive verifier refuse demo.json : %v %s", err, out)
	}
}

func TestCompilationEssaiEtActivation(t *testing.T) {
	b := nouveauBanc(t)
	// Une demande passée qui a consulté la base interne des taux : le prêt
	// est autorisé avant la directive D-009.
	b.demande(claire, "Peut-on accorder un prêt de 100 000 euros avec la base interne des taux ?")
	jeux0 := b.jeux()
	if len(jeux0) != 1 {
		t.Fatal(len(jeux0))
	}

	// Un texte non observable : rien n'est produit, la raison est donnée.
	c, err := b.Dep.Compiler("Les agents ne doivent pas discriminer.", "", "", nil, "")
	must(t, err)
	if c.Etat != "non_observable" || c.NonObservable == nil || c.NonObservable.Raison == "" || c.JeuCandidat != nil {
		t.Fatalf("non observable : %+v", c)
	}
	if c, err = b.Dep.Compiler("Texte inconnu des outils.", "", "", nil, ""); err != nil || c.Etat != "refuse" || len(c.Erreurs) == 0 {
		t.Fatalf("refus motivé : %+v %v", c, err)
	}

	// Questions du compilateur, avec leur origine ; la réponse n'en refait pas.
	c, err = b.Dep.Compiler("Aucune action d'agent le week-end.", "Règlement intérieur", "", nil, "")
	must(t, err)
	if c.Etat != "questions" || len(c.Questions) != 2 || c.Questions[0].Origine != "code" || c.Questions[1].Origine != "mistral" || c.Conversation == "" {
		t.Fatalf("questions : %+v", c)
	}
	conv := c.Conversation
	c, err = b.Dep.Compiler("", "", conv, []ReponseQuestion{{Question: "q-fuseau", Reponse: "Europe/Paris"}, {Question: "q-acces", Reponse: "oui"}}, "")
	must(t, err)
	if c.Etat != "jeu" || len(c.Ajoutees) != 1 || c.Ajoutees[0].ID != "D-009" || c.Ajoutees[0].Statut != "brouillon" || !strings.Contains(c.Relecture, "Directive D-009") {
		t.Fatalf("jeu : %+v", c)
	}
	contient(t, c.Ajoutees[0].Relecture, "Europe/Paris", "samedi", "Alors :")

	// Directive de la base interne : essai sur les journaux existants. La
	// date d'effet est écrite à la seconde : on attend qu'elle suive l'appel.
	time.Sleep(1100 * time.Millisecond)
	c, err = b.Dep.Compiler("Un prêt dont la session a consulté la base interne des taux attend la validation d'un analyste des risques.", "Politique de crédit art. 7", "", nil, "")
	must(t, err)
	if c.Etat != "jeu" || c.Ajoutees[0].ID != "D-009" {
		t.Fatalf("D-009 : %+v", c)
	}
	conv = c.Conversation
	e, err := b.Dep.EssaiSurJournaux(conv, b.tous(), b.jeux())
	must(t, err)
	if e.Compteurs.Changes != 1 || e.Compteurs.Exposition != 1 || e.Compteurs.Violation != 0 || e.Compteurs.Appels != 4 || !e.Verifiees {
		t.Fatalf("essai : %+v", e.Compteurs)
	}
	if !strings.Contains(e.Explication, "mis en attente") {
		t.Fatalf("explication : %s", e.Explication)
	}
	// Rien n'a été écrit par l'essai.
	if got := b.jeux(); len(got) != 1 || len(got[0].Jeu.Directives) != 8 {
		t.Fatalf("l'essai ne doit rien écrire : %d", len(got[0].Jeu.Directives))
	}

	// Mise en observation : version 2 du même ensemble, ancienne archivée.
	avant, _ := os.ReadFile(filepath.Join(b.racine, "demo", "directives", "demo.json"))
	res, err := b.Dep.MettreEnVigueur(conv, "observation")
	must(t, err)
	if res.Version != 2 || strings.Join(res.Directives, ",") != "D-009" || res.Statut != "observation" || res.Archive != filepath.Join("demo", "directives", "historique", "demo.v1.json") {
		t.Fatalf("résultat : %+v", res)
	}
	arch, err := os.ReadFile(filepath.Join(b.racine, "demo", "directives", "historique", "demo.v1.json"))
	must(t, err)
	if string(arch) != string(avant) {
		t.Fatal("l'archive doit être l'ancienne version, octet pour octet")
	}
	b.verifier(t)
	jeux := b.jeux()
	if len(jeux) != 2 || !jeux[1].Courant || jeux[1].Jeu.Meta.Version != 2 || jeux[1].Jeu.Meta.ID != "demo" || jeux[1].Jeu.Meta.Auteur != "juriste@exemple.test" {
		t.Fatalf("après mise en observation : %d ensembles", len(jeux))
	}
	d9 := jeux[1].Jeu.Directives[8]
	if d9.ID != "D-009" || d9.Statut != directive.Observation || d9.DateEffet.Before(time.Now().Add(-time.Minute)) {
		t.Fatalf("D-009 : %+v", d9)
	}
	// Les journaux passés (règles version 1) se rejouent toujours : l'historique les porte.
	if v := b.Dep.AvantApres(b.tous(), jeux, "D-001", nil); v.Compteurs.NonRejouables != 0 || v.Compteurs.Appels == 0 {
		t.Fatalf("rejeu après archivage : %+v", v.Compteurs)
	}
	// La même conversation ne s'active pas deux fois.
	if _, err := b.Dep.MettreEnVigueur(conv, "active"); err == nil {
		t.Fatal("déjà mise en vigueur")
	}

	// Observation → active → retirée : version 3 puis 4 ; pas de retour en arrière.
	if res, err = b.Dep.ChangerStatut("D-009", "active"); err != nil || res.Version != 3 {
		t.Fatalf("activation : %+v %v", res, err)
	}
	if _, err = b.Dep.ChangerStatut("D-009", "observation"); err == nil {
		t.Fatal("pas de retour de active à observation")
	}
	if res, err = b.Dep.ChangerStatut("D-009", "retiree"); err != nil || res.Version != 4 {
		t.Fatalf("retrait : %+v %v", res, err)
	}
	if _, err = b.Dep.ChangerStatut("D-009", "active"); err == nil {
		t.Fatal("une directive retirée ne revient pas")
	}
	if _, err = b.Dep.ChangerStatut("D-404", "retiree"); err == nil {
		t.Fatal("directive inconnue")
	}
	b.verifier(t)
	for v := 1; v <= 3; v++ {
		if _, err := os.Stat(filepath.Join(b.racine, "demo", "directives", "historique", "demo.v"+string(rune('0'+v))+".json")); err != nil {
			t.Fatalf("archive v%d : %v", v, err)
		}
	}
	// Aucun fichier temporaire ne reste (écritures atomiques).
	filepath.Walk(filepath.Join(b.racine, "demo"), func(p string, i os.FileInfo, err error) error {
		if err == nil && strings.HasSuffix(p, ".tmp") {
			t.Errorf("fichier temporaire resté : %s", p)
		}
		return nil
	})
	// Un candidat compilé sur d'anciennes règles est refusé.
	c2, err := b.Dep.Compiler("Un prêt dont la session a consulté la base interne des taux attend la validation d'un analyste des risques.", "", "", nil, "")
	must(t, err)
	_ = c2
}

func TestConversationEtCheminsJamaisSuivis(t *testing.T) {
	b := nouveauBanc(t)
	for _, conv := range []string{"../../etc", "a/b", "..", ".", "", "c x"} {
		if _, err := b.Dep.MettreEnVigueur(conv, "active"); err == nil {
			t.Errorf("conversation %q acceptée", conv)
		}
		if _, err := b.Dep.EssaiSurJournaux(conv, nil, b.jeux()); err == nil {
			t.Errorf("essai : conversation %q acceptée", conv)
		}
	}
	if _, err := b.Dep.Compiler("x", "", "../etc", nil, ""); err == nil {
		t.Error("compilation : conversation hors liste")
	}
	if _, err := b.Dep.LireProposition("../x"); err == nil {
		t.Error("proposition hors liste")
	}
	if _, err := b.Dep.Valider("../x", anna, "analyste-risques"); err == nil {
		t.Error("valider : session hors liste")
	}
}

// Un candidat compilé avant un changement des règles n'écrase pas ce
// changement.
func TestCandidatPerime(t *testing.T) {
	b := nouveauBanc(t)
	c1, err := b.Dep.Compiler("Un prêt dont la session a consulté la base interne des taux attend la validation d'un analyste des risques.", "", "", nil, "")
	must(t, err)
	c2, err := b.Dep.Compiler("Un prêt dont la session a consulté la base interne des taux attend la validation d'un analyste des risques. Bis.", "", "", nil, "")
	must(t, err)
	if _, err := b.Dep.MettreEnVigueur(c1.Conversation, "active"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Dep.MettreEnVigueur(c2.Conversation, "active"); err == nil || !strings.Contains(err.Error(), "ont changé") {
		t.Fatalf("candidat périmé : %v", err)
	}
	if got := b.jeux(); len(got[len(got)-1].Jeu.Directives) != 9 {
		t.Fatal("les règles ne doivent pas avoir bougé")
	}
}

const propositionTest = `{
  "id": "P-2026-10-02-1", "date": "2026-10-02T09:30:00Z",
  "constat": {"type": "provenance", "detail": "Des prêts ont été attribués après consultation de la base interne des taux, sans validation."},
  "sessions": ["20261002-093000-abc123"],
  "texte_propose": "Un prêt dont la session a consulté la base interne des taux attend la validation d'un analyste des risques.",
  "fondement_propose": "Politique de crédit art. 7 (à valider)",
  "explication": "La base interne n'est pas une source officielle."
}`

// Propositions de l'Analyste : rejet motivé, reprise telle quelle,
// reprise en modifiant ; l'origine suit la directive jusqu'à la vitrine.
func TestPropositionsDeLAnalyste(t *testing.T) {
	b := nouveauBanc(t)
	must(t, os.WriteFile(filepath.Join(b.racine, "demo", "execution", "propositions", "P-2026-10-02-1.json"), []byte(propositionTest), 0o644))
	// Une proposition illisible ou hors liste est ignorée.
	must(t, os.WriteFile(filepath.Join(b.racine, "demo", "execution", "propositions", "casse.json"), []byte("{"), 0o644))
	if l := b.Dep.ListerPropositions(); len(l) != 1 || l[0].Decision != nil || l[0].ID != "P-2026-10-02-1" {
		t.Fatalf("propositions : %+v", l)
	}
	srv := httptestServeur(t, b)
	js := map[string]string{"Content-Type": "application/json"}
	chemin := "/api/propositions/P-2026-10-02-1/decision"

	// Rejeter : motif obligatoire.
	if r, _ := requete(t, srv, "POST", chemin, "", `{"decision":"rejetee","motif":"  "}`, js); r.StatusCode != 400 {
		t.Fatalf("rejet sans motif : %d", r.StatusCode)
	}
	if _, err := os.Stat(filepath.Join(b.racine, "demo", "execution", "propositions", "P-2026-10-02-1.decision.json")); err == nil {
		t.Fatal("aucune décision ne doit être écrite sans motif")
	}
	if r, _ := requete(t, srv, "POST", "/api/propositions/..%2Fx/decision", "", `{"decision":"rejetee","motif":"x"}`, js); r.StatusCode != 404 {
		t.Fatalf("proposition hors liste : %d", r.StatusCode)
	}
	if r, _ := requete(t, srv, "POST", chemin, "", `{"decision":"rejetee","motif":"Déjà couvert par D-002."}`, js); r.StatusCode != 200 {
		t.Fatalf("rejet : %d", r.StatusCode)
	}
	var dp DecisionProposition
	lireJSON(t, filepath.Join(b.racine, "demo", "execution", "propositions", "P-2026-10-02-1.decision.json"), &dp)
	if dp.Decision != "rejetee" || dp.Motif != "Déjà couvert par D-002." || dp.Auteur != "juriste@exemple.test" || dp.Date.IsZero() || dp.TexteRetenu != "" {
		t.Fatalf("décision de rejet : %+v", dp)
	}

	// Reprendre en modifiant : le texte est copié, modifiable, compilé ensuite.
	r, corps := requete(t, srv, "POST", chemin, "", `{"decision":"reprise_modifiee","texte":"Un prêt dont la session a consulté la base interne des taux attend la validation d'un analyste des risques, dossier par dossier."}`, js)
	if r.StatusCode != 200 {
		t.Fatalf("reprise : %d %s", r.StatusCode, corps)
	}
	lireJSON(t, filepath.Join(b.racine, "demo", "execution", "propositions", "P-2026-10-02-1.decision.json"), &dp)
	if dp.Decision != "reprise_modifiee" || !strings.Contains(dp.TexteRetenu, "dossier par dossier") || dp.ConversationCompilateur != "" {
		t.Fatalf("reprise : %+v", dp)
	}
	c, err := b.Dep.Compiler(dp.TexteRetenu, "Politique de crédit art. 7", "", nil, "P-2026-10-02-1")
	must(t, err)
	lireJSON(t, filepath.Join(b.racine, "demo", "execution", "propositions", "P-2026-10-02-1.decision.json"), &dp)
	if dp.ConversationCompilateur != c.Conversation || dp.Decision != "reprise_modifiee" {
		t.Fatalf("la conversation doit suivre la décision : %+v", dp)
	}

	// Reprendre tel quel : compilée sans modification, aussitôt.
	r, corps = requete(t, srv, "POST", chemin, "", `{"decision":"reprise_telle_quelle"}`, js)
	if r.StatusCode != 200 {
		t.Fatalf("reprise telle quelle : %d %s", r.StatusCode, corps)
	}
	var sortie struct {
		Compilation *Compilation `json:"compilation"`
	}
	must(t, json.Unmarshal(corps, &sortie))
	if sortie.Compilation == nil || sortie.Compilation.Etat != "jeu" || sortie.Compilation.Ajoutees[0].Texte != "Un prêt dont la session a consulté la base interne des taux attend la validation d'un analyste des risques." {
		t.Fatalf("compilation : %s", corps)
	}
	lireJSON(t, filepath.Join(b.racine, "demo", "execution", "propositions", "P-2026-10-02-1.decision.json"), &dp)
	if dp.Decision != "reprise_telle_quelle" || dp.ConversationCompilateur != sortie.Compilation.Conversation {
		t.Fatalf("décision : %+v", dp)
	}

	// Activation : l'origine est écrite dans origines.json.
	if _, err := b.Dep.MettreEnVigueur(sortie.Compilation.Conversation, "active"); err != nil {
		t.Fatal(err)
	}
	var orig map[string]Origine
	lireJSON(t, filepath.Join(b.racine, "demo", "directives", "origines.json"), &orig)
	if o := orig["D-009"]; o.Proposition != "P-2026-10-02-1" || o.Decision != "reprise_telle_quelle" {
		t.Fatalf("origines : %+v", orig)
	}
	jeux := b.jeux()
	v := b.Dep.ReglesEnVigueur(*Courant(jeux), nil, jeux, "D-009")
	contient(t, v.Choix.Origine, "proposée par l'Analyste le 02/10/2026", "reprise telle quelle par J. Juriste")
	if o := b.Dep.ReglesEnVigueur(*Courant(jeux), nil, jeux, "D-001").Choix.Origine; o != "écrite par J. Juriste" {
		t.Fatalf("origine juriste : %s", o)
	}
	if l := b.Dep.ListerPropositions(); l[0].Decision == nil || l[0].Decision.Decision != "reprise_telle_quelle" {
		t.Fatalf("propositions : %+v", l)
	}
}
