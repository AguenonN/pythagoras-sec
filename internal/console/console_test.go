package console

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"mcprogue/internal/chaine"
	"mcprogue/internal/directive"
	"mcprogue/internal/journal"
)

var (
	cle = []byte(strings.Repeat("c", 32))
	t0  = time.Date(2026, 10, 4, 9, 0, 0, 0, time.UTC)
)

var attestee = directive.Attestation{Mode: "binaire", Attendu: "a", Observe: "a", Schemas: true, Verifiee: true}

// banc écrit un journal et un coffre réels, avec Decide, comme l'engine :
// une consultation de taux, un prêt retenu par D-001, un prêt refusé par
// D-002, une recherche caviardée par D-004.
func banc(t *testing.T) (*Depot, JeuCharge) {
	t.Helper()
	dep := &Depot{Cle: cle}
	jeux, refus := dep.ChargerJeux()
	if len(refus) != 0 || len(jeux) < 2 {
		t.Fatalf("jeux intégrés : %d, refus %v", len(jeux), refus)
	}
	jeu := jeux[0]
	if jeu.Jeu.Meta.Version != 1 {
		t.Fatalf("premier jeu : %s", jeu.Libelle())
	}
	dir := filepath.Join(t.TempDir(), "demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	j := journal.Nouveau(chaine.Nouvelle(&buf, cle))
	coffre, err := journal.OuvrirCoffre(filepath.Join(dir, "coffre.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer coffre.Fermer()
	coffre.Journaliser(j)
	ecrire := func(e journal.Entree) {
		if _, err := j.Ecrire(e); err != nil {
			t.Fatal(err)
		}
	}
	ecrire(journal.Entree{TS: t0, Type: journal.SessionOuverte, Session: "S1", Appelant: "spiffe://lab/agent",
		Demandeur: &journal.Demandeur{Identite: "conseiller@cabinet.fr", Groupes: []string{"conseillers"}}})
	var etat directive.EtatSession
	etat.Appelant = "spiffe://lab/agent"
	appels := []struct {
		serveur, outil string
		args           map[string]any
	}{
		{"taux-officiel", "get_taux_usure", map[string]any{"date": "2026-10-04"}},
		{"banque-synthetique", "attribuer_pret", map[string]any{"montant": 100000.0, "dossier": "DOS-1"}},
		{"banque-synthetique", "attribuer_pret", map[string]any{"montant": 600000.0, "dossier": "DOS-2"}},
		{"recherche-entreprises", "rechercher_entreprise", map[string]any{"siren": "552100554", "dirigeant": "X"}},
	}
	for i, a := range appels {
		ts := t0.Add(time.Duration(i+1) * time.Minute)
		dec := directive.Decide(jeu.Jeu, etat, directive.Appel{Serveur: a.serveur, Outil: a.outil, Arguments: a.args, Attestation: attestee, Horodatage: ts})
		if err := coffre.Deposer(journal.Depot{Empreinte: dec.EmpreinteOrigine, Outil: a.outil, ArgumentsOrigine: a.args, ArgumentsTransmis: dec.Arguments}); err != nil {
			t.Fatal(err)
		}
		res := &journal.Resultat{Statut: journal.StatutNonTransmis}
		if dec.Effet != directive.Refuser && dec.Effet != directive.AttendreValidation {
			ecrire(journal.Entree{TS: ts, Type: journal.TransmissionT, Session: "S1", Seq: i + 1, Appelant: etat.Appelant,
				Jeu: journal.DepuisJeu(jeu.Jeu), Serveur: a.serveur, Outil: a.outil,
				Appel: &journal.Appel{EmpreinteTransmise: dec.EmpreinteTransmise}, Decision: journal.DepuisDecision(dec)})
			res = &journal.Resultat{Statut: journal.StatutOK, EmpreinteReponse: "rep", DureeMs: 5}
			etat.Consultations = append(etat.Consultations, directive.Consultation{Seq: i + 1, Serveur: a.serveur, Outil: a.outil,
				Horodatage: ts, Empreinte: dec.EmpreinteTransmise, Resultat: directive.ResultatOK, EmpreinteReponse: "rep"})
		}
		ecrire(journal.Entree{TS: ts, Type: journal.AppelT, Session: "S1", Seq: i + 1, Appelant: etat.Appelant,
			Jeu: journal.DepuisJeu(jeu.Jeu), Serveur: a.serveur, Outil: a.outil,
			Appel:       &journal.Appel{EmpreinteOrigine: dec.EmpreinteOrigine, EmpreinteTransmise: dec.EmpreinteTransmise, ChampsRetires: dec.ChampsRetires},
			Attestation: journal.DepuisAttestation(attestee), EtatSession: journal.DepuisConsultations(etat.Consultations),
			Decision: journal.DepuisDecision(dec), Resultat: res})
	}
	ecrire(journal.Entree{TS: t0.Add(time.Hour), Type: journal.SessionFermee, Session: "S1", Message: "session terminée par le client"})
	if err := os.WriteFile(filepath.Join(dir, "journal.jsonl"), buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	dep.Donnees = filepath.Dir(dir)
	return dep, jeu
}

func lire(t *testing.T, dep *Depot) *Etat {
	t.Helper()
	js := dep.Journaux()
	if len(js) != 1 {
		t.Fatalf("journaux : %v", js)
	}
	st, err := dep.Lire(js[0])
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestChaineEtProvenance(t *testing.T) {
	dep, _ := banc(t)
	st := lire(t, dep)
	if !st.Verif.OK || st.Verif.Lignes != len(st.Entrees) {
		t.Fatalf("chaîne : %+v", st.Verif)
	}
	jeux, _ := dep.ChargerJeux()
	v, ok := Chaine(st, jeux, "S1")
	if !ok || len(v.Etapes) != 4 {
		t.Fatalf("étapes : %+v", v)
	}
	var got []string
	for _, e := range v.Etapes {
		got = append(got, e.Badge)
	}
	want := []string{BadgeAutorise, BadgeAttente, BadgeRefuse, BadgeCaviarde}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("badges %v, attendus %v", got, want)
	}
	// L'attente du prêt est fondée sur la consultation n° 1 (taux-officiel).
	l := v.Etapes[1].Liens
	if len(l) != 1 || l[0].Seq != 1 || l[0].Serveur != "taux-officiel" || l[0].Directive != "D-001" {
		t.Fatalf("liens : %+v", l)
	}
	if v.Etapes[1].Apps[0].Texte == "" {
		t.Fatal("le texte du juriste doit accompagner la directive quand le jeu est connu")
	}
	if v.Demandeur == nil || v.Demandeur.Identite != "conseiller@cabinet.fr" {
		t.Fatalf("demandeur : %+v", v.Demandeur)
	}
	if _, ok := Chaine(st, jeux, "inconnue"); ok {
		t.Fatal("session inconnue")
	}
}

func TestChaineRompueEnRouge(t *testing.T) {
	dep, _ := banc(t)
	chemin := dep.Journaux()[0].Chemin
	brut, _ := os.ReadFile(chemin)
	// Falsifie le motif du refus.
	alt := strings.Replace(string(brut), "refusé par D-002", "autorisé par D-002", 1)
	if alt == string(brut) {
		t.Fatal("rien à falsifier")
	}
	os.WriteFile(chemin, []byte(alt), 0o600)
	st := lire(t, dep)
	if st.Verif.OK || st.Verif.Erreur == "" {
		t.Fatalf("la falsification doit être détectée : %+v", st.Verif)
	}
	jeux, _ := dep.ChargerJeux()
	v, _ := Chaine(st, jeux, "S1")
	if v.Ecart < 0 || !v.Etapes[2].Suspect || !v.Etapes[3].Suspect || v.Etapes[0].Suspect {
		t.Fatalf("suspects à partir de l'écart %d : %v %v %v", v.Ecart, v.Etapes[0].Suspect, v.Etapes[2].Suspect, v.Etapes[3].Suspect)
	}
}

func TestSansCleNonVerifiable(t *testing.T) {
	dep, _ := banc(t)
	dep.Cle = nil
	st := lire(t, dep)
	if st.Verif.OK || !st.Verif.CleAbsente {
		t.Fatalf("sans clé, rien n'est vérifié : %+v", st.Verif)
	}
}

func TestLigneEnCoursDEcritureIgnoree(t *testing.T) {
	dep, _ := banc(t)
	chemin := dep.Journaux()[0].Chemin
	f, _ := os.OpenFile(chemin, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString(`{"ts":"2026-10-04T10:00:00Z","type":"appel","sess`)
	f.Close()
	if st := lire(t, dep); !st.Verif.OK {
		t.Fatalf("une ligne partielle n'est pas un écart : %+v", st.Verif)
	}
}

func TestPosteEtCout(t *testing.T) {
	dep, _ := banc(t)
	st := lire(t, dep)
	jeux, _ := dep.ChargerJeux()
	p, ok := Poste(st, jeux, "S1")
	if !ok {
		t.Fatal("poste")
	}
	c := p.Cout
	if c.Appels != 4 || c.Validations != 1 || c.Refuses != 1 || c.Caviardes != 1 || c.NonRejoues != 0 || c.EvaluationMoyenne <= 0 {
		t.Fatalf("coût : %+v", c)
	}
	if p.Reponse.Badge != BadgeCaviarde || !strings.Contains(p.Reponse.Titre, "rechercher_entreprise") || c.ChampsRetires != 1 {
		t.Fatalf("réponse finale : %+v, champs %d", p.Reponse, c.ChampsRetires)
	}
}

func TestRegles(t *testing.T) {
	dep, jeu := banc(t)
	jeux, _ := dep.ChargerJeux()
	etats := []*Etat{lire(t, dep)}
	jc := EnVigueur(etats, jeux)
	if jc == nil || jc.Jeu.Meta.Version != jeu.Jeu.Meta.Version {
		t.Fatalf("règles en vigueur : %+v", jc)
	}
	v := ReglesEnVigueur(*jc, etats, jeux, "D-001")
	if len(v.Directives) != 4 || v.Choix == nil || v.Choix.ID != "D-001" {
		t.Fatalf("liste : %+v", v)
	}
	if strings.Contains(v.Choix.Relecture, "Verrou") || v.Choix.Relecture == "" {
		t.Fatalf("relecture : %q", v.Choix.Relecture)
	}
	if len(v.Choix.Decisions) == 0 {
		t.Fatal("D-001 doit avoir une décision rejouée")
	}
	var d DecisionRejouee
	for _, x := range v.Choix.Decisions {
		if x.Seq == 2 {
			d = x
		}
	}
	if !d.Identique || !d.Declenchee || d.Journalise != "attendre_validation" || d.Seq != 2 || !v.Choix.Verifies {
		t.Fatalf("D-001 : %+v", d)
	}
	if t4 := Titre(jc.Jeu.Directives[0].TexteFR); t4 == "" || len([]rune(t4)) > 61 {
		t.Fatalf("titre : %q", t4)
	}
}

func TestAvantApres(t *testing.T) {
	dep, _ := banc(t)
	jeux, _ := dep.ChargerJeux()
	v, err := AvantApres(lire(t, dep), jeux, "S1", "D-001#2")
	if err != nil {
		t.Fatal(err)
	}
	if v.Choix == nil || v.Choix.Avant != "Avec D-001, version 1" || v.Choix.Apres != "Avec D-001, version 2" {
		t.Fatalf("choix : %+v (options %+v)", v.Choix, v.Options)
	}
	if v.Changes != 1 || v.Sessions != 1 || v.NonRejouables != 0 {
		t.Fatalf("changements : %+v", v)
	}
	var l LigneAA
	for _, x := range v.Lignes {
		if x.Change {
			l = x
		}
	}
	if l.Seq != 2 || l.Avant != "attendre_validation" || l.Apres != "refuser" || l.MotifApres == "" {
		t.Fatalf("ligne : %+v", l)
	}
	if !strings.Contains(v.Explication, "appel n° 2") {
		t.Fatalf("explication : %s", v.Explication)
	}
	// Une directive absente des règles en vigueur : « sans » / « avec ».
	if _, err := AvantApres(lire(t, dep), jeux, "S1", ""); err != nil {
		t.Fatal(err)
	}
}

func TestFile(t *testing.T) {
	dep, _ := banc(t)
	f := File("demo", lire(t, dep))
	if len(f) != 1 || f[0].Seq != 2 || f[0].Directive != "D-001" || f[0].Arguments["dossier"] != "DOS-1" {
		t.Fatalf("file : %+v", f)
	}
}

func TestRelire(t *testing.T) {
	jeux, _ := (&Depot{}).ChargerJeux()
	r := Relire(string(jeux[0].Brut))
	if !r.Valide || !strings.Contains(r.Relecture, "Directive D-001") {
		t.Fatalf("%+v", r)
	}
	if r := Relire("Interdire les prêts le week-end"); r.Valide || len(r.Questions) == 0 {
		t.Fatalf("texte libre : %+v", r)
	}
	if r := Relire(`{"jeu":{},"directives":[]}`); r.Valide || len(r.Erreurs) == 0 {
		t.Fatalf("jeu invalide : %+v", r)
	}
}

func TestHTTP(t *testing.T) {
	dep, _ := banc(t)
	srv := httptest.NewServer((&Serveur{Depot: dep}).Handler())
	defer srv.Close()
	get := func(chemin, host string) (*http.Response, []byte) {
		req, _ := http.NewRequest("GET", srv.URL+chemin, nil)
		if host != "" {
			req.Host = host
		}
		r, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer r.Body.Close()
		var b bytes.Buffer
		b.ReadFrom(r.Body)
		return r, b.Bytes()
	}
	if r, _ := get("/", ""); r.StatusCode != 200 || !strings.Contains(r.Header.Get("Content-Security-Policy"), "default-src 'self'") {
		t.Fatalf("page : %d %v", r.StatusCode, r.Header)
	}
	if r, _ := get("/api/etat", "evil.example"); r.StatusCode != http.StatusForbidden {
		t.Fatalf("hôte étranger : %d", r.StatusCode)
	}
	_, corps := get("/api/journal/demo/session/S1/chaine", "")
	var v VueChaine
	if err := json.Unmarshal(corps, &v); err != nil || len(v.Etapes) != 4 || !v.Verif.OK {
		t.Fatalf("chaîne : %v %s", err, corps)
	}
	if r, _ := get("/api/journal/..%2Fx/session/S1/chaine", ""); r.StatusCode != http.StatusNotFound {
		t.Fatalf("identifiant hors liste : %d", r.StatusCode)
	}
	for _, c := range []string{"/api/regles", "/api/regles?directive=D-002", "/api/avant-apres?journal=demo&session=S1", "/api/file", "/api/journal/demo/session/S1/poste"} {
		if r, b := get(c, ""); r.StatusCode != 200 {
			t.Fatalf("%s : %d %s", c, r.StatusCode, b)
		}
	}
}

// Rien ne doit être chargé depuis le réseau : aucune URL absolue dans les
// fichiers embarqués, hors texte affiché.
func TestAucuneRessourceExterne(t *testing.T) {
	re := regexp.MustCompile(`(?i)(src|href|action)\s*=\s*["']?(https?:)?//|url\(\s*["']?(https?:)?//|@import|fetch\(\s*["'\x60]https?:|new WebSocket|importScripts`)
	n := 0
	err := fs.WalkDir(webFS, "web", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		n++
		b, _ := webFS.ReadFile(p)
		if m := re.Find(b); m != nil {
			t.Errorf("%s charge une ressource externe : %q", p, m)
		}
		return nil
	})
	if err != nil || n < 3 {
		t.Fatalf("fichiers embarqués : %d, %v", n, err)
	}
}

// Le vocabulaire de l'écran : ni « jeu », ni « v1 » / « v2 ». Les lignes de
// commentaire sont ignorées ; le code lui-même n'emploie pas ces mots dans
// ses textes. La CSP interdit l'attribut style : le code doit passer par le
// CSSOM, et jamais par innerHTML.
func TestVocabulaireEtCSP(t *testing.T) {
	mot := regexp.MustCompile(`(?i)\bjeux?\b|\bv[12]\b`)
	err := fs.WalkDir(webFS, "web", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !(strings.HasSuffix(p, ".js") || strings.HasSuffix(p, ".html")) {
			return err
		}
		b, _ := webFS.ReadFile(p)
		for i, l := range strings.Split(string(b), "\n") {
			l = strings.TrimSpace(l)
			if strings.HasPrefix(l, "//") || strings.HasPrefix(l, "/*") || strings.HasPrefix(l, "*") {
				continue
			}
			if m := mot.FindString(l); m != "" {
				t.Errorf("%s:%d emploie %q : %s", p, i+1, m, l)
			}
			if strings.Contains(l, "innerHTML") || strings.Contains(l, "setAttribute('style'") {
				t.Errorf("%s:%d : innerHTML ou attribut style interdits (CSP) : %s", p, i+1, l)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
