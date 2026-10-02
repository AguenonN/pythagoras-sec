package console

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mcprogue/internal/journal"
)

// Un prêt sans validation requise : le simulateur enchaîne les appels, la
// chaîne est vérifiée, les badges viennent de l'engine.
func TestSessionRefusParGels(t *testing.T) {
	b := nouveauBanc(t)
	id := b.demande(claire, "Peut-on accorder le prêt demandé sans gel vérifié ?")
	st := b.etat(id)
	if !st.Verif.OK || st.Verif.Lignes != len(st.Entrees) {
		t.Fatalf("chaîne : %+v", st.Verif)
	}
	if st.Dossier == nil || st.Dossier.Statut != StatutTermine || st.Reponse == nil {
		t.Fatalf("dossier : %+v reponse %+v", st.Dossier, st.Reponse)
	}
	v, ok := b.Dep.Chaine(st, b.jeux(), Sessions(st)[0].ID)
	if !ok {
		t.Fatal("session")
	}
	last := v.Etapes[len(v.Etapes)-1]
	if last.Badge != BadgeRefuse || last.Outil != "attribuer_pret" {
		t.Fatalf("dernier appel : %+v", last)
	}
	if v.Demande == "" || v.Demandeur == nil || v.Demandeur.Identite != claire {
		t.Fatalf("demandeur / demande : %+v", v)
	}
	if v.DemandeurFiche == nil || !strings.Contains(v.DemandeurFiche.Nom, "Claire") || v.SourceIdentite == "" {
		t.Fatalf("fiche : %+v source %q", v.DemandeurFiche, v.SourceIdentite)
	}
	if last.DureeUs <= 0 {
		t.Fatalf("durée mesurée absente : %d", last.DureeUs)
	}
	if len(last.Apps) == 0 || last.Apps[0].ID != "D-001" || last.Apps[0].Titre == "" {
		t.Fatalf("directives appliquées : %+v", last.Apps)
	}
	p, _ := b.Dep.Poste(st, b.jeux(), v.Session)
	if p.Reponse.Texte == "" || !p.Reponse.Terminee || len(p.Reponse.DirectivesCitees) == 0 {
		t.Fatalf("réponse : %+v", p.Reponse)
	}
	if p.Cout.Refuses != 1 || p.Cout.Mesures != p.Cout.Appels || p.Cout.MoyenneUs <= 0 || p.Cout.ChampsRetires != 2 {
		t.Fatalf("coût : %+v", p.Cout)
	}
}

// Du dépôt de la demande à la reprise de l'appel : file, signature, contrôles
// de l'engine tirés du journal, attente humaine mesurée.
func TestValidationDeBoutEnBout(t *testing.T) {
	b := nouveauBanc(t)
	id, fin := b.demandeAsync(claire, "Peut-on accorder un prêt de 120 000 euros ?")
	st := b.attendreStatut(id, StatutEnAttente)
	f := b.Dep.File(b.tous(), b.jeux(), time.Now())
	if len(f.Attentes) != 1 || len(f.Reprises) != 0 {
		t.Fatalf("file : %+v", f)
	}
	a := f.Attentes[0]
	if a.Outil != "attribuer_pret" || a.Directive != "D-002" || len(a.Roles) != 1 || a.Roles[0] != "analyste-risques" ||
		a.Requis != 1 || a.Deposes != 0 || a.Arguments["dossier"] != "DOS-FICTIF-002" || a.Arguments["montant"] != 120000.0 {
		t.Fatalf("attente : %+v", a)
	}
	if len(a.Valideurs) != 1 || a.Valideurs[0].Identite != anna {
		t.Fatalf("valideurs possibles : %+v", a.Valideurs)
	}
	if a.Controles.Etabli || a.Controles.Signature.Etat != "non_etabli" {
		t.Fatalf("les contrôles ne se présument pas : %+v", a.Controles)
	}
	// Un valideur sans le rôle est refusé avant toute signature.
	if _, err := b.Dep.Valider(id, diane, "analyste-risques"); err == nil || !strings.Contains(err.Error(), "ne détient pas") {
		t.Fatalf("valideur sans le rôle : %v", err)
	}
	if _, err := b.Dep.Valider(id, anna, "directeur-risques"); err == nil {
		t.Fatal("rôle qui n'est pas requis")
	}
	if _, err := b.Dep.Valider(id, anna, "analyste-risques"); err != nil {
		t.Fatal(err)
	}
	if err := <-fin; err != nil {
		t.Fatal(err)
	}
	st = b.etat(id)
	if st.Dossier.Statut != StatutTermine || st.Reponse == nil || !strings.Contains(st.Reponse.Texte, "attribué") {
		t.Fatalf("fin : %+v %+v", st.Dossier, st.Reponse)
	}
	if !st.Verif.OK {
		t.Fatalf("chaîne : %+v", st.Verif)
	}
	f = b.Dep.File(b.tous(), b.jeux(), time.Now())
	if len(f.Attentes) != 0 || len(f.Reprises) != 1 {
		t.Fatalf("après reprise : %+v", f)
	}
	c := f.Reprises[0].Controles
	if !c.Etabli || c.Statut != journal.ApprobationAcceptee || c.Signature.Etat != "ok" || c.RoleADate.Etat != "ok" ||
		c.QuatreYeux.Etat != "non_requis" || c.UnJetonParRole.Etat != "ok" || len(c.Valideurs) != 1 || c.Valideurs[0].Identite != anna {
		t.Fatalf("contrôles : %+v", c)
	}
	// Écran 1 : l'appel repris est autorisé après validation ; l'appel retenu
	// reste « en attente » et dit qui l'a validé ensuite.
	v, _ := b.Dep.Chaine(st, b.jeux(), Sessions(st)[0].ID)
	var attente, repris *Etape
	for i := range v.Etapes {
		e := &v.Etapes[i]
		switch {
		case e.Badge == BadgeAttente:
			attente = e
		case len(e.ValidePar) > 0:
			repris = e
		}
	}
	if attente == nil || repris == nil || attente.ValideAu != repris.Seq {
		t.Fatalf("attente %+v repris %+v", attente, repris)
	}
	if repris.Badge != BadgeAutorise || repris.Libelle != "Autorisé après validation" || !repris.Transmis ||
		len(repris.Approbations) == 0 || repris.Approbations[0].Statut != journal.ApprobationAcceptee {
		t.Fatalf("appel repris : %+v", repris)
	}
	p, _ := b.Dep.Poste(st, b.jeux(), v.Session)
	if p.Cout.Attentes != 1 || p.Cout.AttenteSecondes < 0 || p.Cout.Validations != 1 {
		t.Fatalf("attente humaine mesurée : %+v", p.Cout)
	}
}

// Quatre yeux et deux rôles : le compteur, un jeton refusé par l'engine
// (valideur dont le rôle est échu) et ce que la console en dit.
func TestValidationDeuxRolesEtJetonRefuse(t *testing.T) {
	b := nouveauBanc(t)
	id, fin := b.demandeAsync(claire, "Peut-on accorder un prêt de 3 000 000 euros ?")
	b.attendreStatut(id, StatutEnAttente)
	f := b.Dep.File(b.tous(), b.jeux(), time.Now())
	a := f.Attentes[0]
	if a.Requis != 2 || !a.QuatreYeux || len(a.Roles) != 2 {
		t.Fatalf("attente : %+v", a)
	}
	// Notification à la direction juridique, remise avant la transmission.
	var notifs []NotifLigne = Notifications(b.tous(), b.jeux())
	if len(notifs) != 1 || notifs[0].Destinataire != "direction-juridique" || notifs[0].Statut != "remise" || notifs[0].Directive != "D-005" || notifs[0].Source != "abonné" {
		t.Fatalf("notifications : %+v", notifs)
	}
	if _, err := b.Dep.Valider(id, anna, "analyste-risques"); err != nil {
		t.Fatal(err)
	}
	f = b.Dep.File(b.tous(), b.jeux(), time.Now())
	if f.Attentes[0].Deposes != 1 {
		t.Fatalf("compteur 1 / 2 : %+v", f.Attentes[0])
	}
	// Marc n'a plus le rôle : la console ne le propose pas.
	for _, v := range f.Attentes[0].Valideurs {
		if v.Identite == marc {
			t.Fatal("rôle échu proposé")
		}
	}
	if _, err := b.Dep.Valider(id, diane, "directeur-risques"); err != nil {
		t.Fatal(err)
	}
	if err := <-fin; err != nil {
		t.Fatal(err)
	}
	st := b.etat(id)
	v, _ := b.Dep.Chaine(st, b.jeux(), Sessions(st)[0].ID)
	var repris *Etape
	for i := range v.Etapes {
		if len(v.Etapes[i].ValidePar) > 0 {
			repris = &v.Etapes[i]
		}
	}
	if repris == nil || len(repris.ValidePar) != 2 || len(repris.Notifications) != 1 || repris.Notifications[0].Statut != "remise" {
		t.Fatalf("appel repris : %+v", repris)
	}
	c := controlesDe(st, repris.Empreinte)
	if c.QuatreYeux.Etat != "ok" || c.UnJetonParRole.Etat != "ok" || !strings.Contains(c.UnJetonParRole.Detail, "2 jetons") {
		t.Fatalf("contrôles : %+v", c)
	}
}

// Un jeton que l'engine refuse (l'ancien directeur n'a plus son rôle) : la
// session reste en attente avec le motif, et les contrôles disent lequel a
// échoué, d'après le journal.
func TestJetonRefuseParLEngine(t *testing.T) {
	b := nouveauBanc(t)
	// Marc est signataire de l'annuaire mais son rôle est échu : la console
	// ne le propose pas, la signature est donc forcée au niveau de la
	// commande, comme le ferait un valideur mal habilité.
	id, fin := b.demandeAsync(claire, "Peut-on accorder un prêt de 120 000 euros ?")
	st := b.attendreStatut(id, StatutEnAttente)
	emp := st.Dossier.Attente.EmpreinteTransmise
	out, err := b.signerCmd("-cles", filepath.Join(b.racine, "demo", "execution", "cles"), "-valideur", marc, "-role", "directeur-risques", "-empreinte", emp)
	must(t, err)
	must(t, os.MkdirAll(filepath.Join(b.racine, "demo", "execution", "approbations", emp), 0o755))
	must(t, os.WriteFile(filepath.Join(b.racine, "demo", "execution", "approbations", emp, marc+".jwt"), []byte(out), 0o600))
	limite := time.Now().Add(10 * time.Second)
	var a AttenteVue
	for time.Now().Before(limite) {
		f := b.Dep.File(b.tous(), b.jeux(), time.Now())
		if len(f.Attentes) == 1 && f.Attentes[0].MotifJeton != "" {
			a = f.Attentes[0]
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if a.MotifJeton == "" {
		t.Fatal("le motif du refus n'est pas remonté")
	}
	if a.Controles.Statut != journal.ApprobationRefusee || a.Controles.RoleADate.Etat != "echec" || a.Controles.Signature.Etat != "non_etabli" {
		t.Fatalf("contrôles : %+v", a.Controles)
	}
	// La bonne signature reprend l'appel.
	if _, err := b.Dep.Valider(id, anna, "analyste-risques"); err != nil {
		t.Fatal(err)
	}
	// Marc a déposé un jeton refusé : un seul fichier de plus suffit à
	// l'orchestrateur simulé (comme le gate : un jeton par rôle exactement).
	_ = os.Remove(filepath.Join(b.racine, "demo", "execution", "approbations", emp, marc+".jwt"))
	if err := <-fin; err != nil {
		t.Fatal(err)
	}
}

func TestRejetConsigne(t *testing.T) {
	b := nouveauBanc(t)
	id, fin := b.demandeAsync(claire, "Peut-on accorder un prêt de 120 000 euros ?")
	st := b.attendreStatut(id, StatutEnAttente)
	emp := st.Dossier.Attente.EmpreinteTransmise
	if _, err := b.Dep.Rejeter(id, anna, "  "); err == nil {
		t.Fatal("un rejet exige un motif")
	}
	if _, err := b.Dep.Rejeter(id, claire, "non"); err == nil {
		t.Fatal("un demandeur n'est pas un valideur")
	}
	if _, err := b.Dep.Rejeter(id, anna, "Montant incohérent avec le dossier"); err != nil {
		t.Fatal(err)
	}
	if err := <-fin; err != nil {
		t.Fatal(err)
	}
	st = b.etat(id)
	if st.Dossier.Statut != StatutBloque || st.Reponse == nil || st.Reponse.Statut != "bloque" {
		t.Fatalf("fin : %+v", st.Dossier)
	}
	rj := b.Dep.LireRejet(emp)
	if rj == nil || rj.Valideur != anna || !strings.Contains(rj.Motif, "incohérent") {
		t.Fatalf("rejet : %+v", rj)
	}
	if _, err := b.Dep.Valider(id, anna, "analyste-risques"); err == nil {
		t.Fatal("plus de validation après le rejet")
	}
}

func TestRapportPrestataireNotifieEtRefuse(t *testing.T) {
	b := nouveauBanc(t)
	id := b.demande(bruno, "Produis le rapport de synthèse du dossier DOS-FICTIF-002")
	st := b.etat(id)
	v, _ := b.Dep.Chaine(st, b.jeux(), Sessions(st)[0].ID)
	last := v.Etapes[len(v.Etapes)-1]
	if last.Badge != BadgeRefuse || len(last.Etiquettes) == 0 || last.Etiquettes[0] != "production_rapport" ||
		len(last.Notifications) != 1 || last.Notifications[0].Role != "direction-juridique" {
		t.Fatalf("dernier appel : %+v", last)
	}
	// La consultation de données client fonde la décision (cadre de provenance).
	if len(last.Liens) != 1 || last.Liens[0].Outil != "lire_dossier_client" || last.Liens[0].Directive != "D-008" {
		t.Fatalf("provenance : %+v", last.Liens)
	}
}

func TestChaineRompueEnRouge(t *testing.T) {
	b := nouveauBanc(t)
	id := b.demande(claire, "Peut-on accorder le prêt demandé sans gel vérifié ?")
	chemin := filepath.Join(b.Dep.Donnees, id, "journal.jsonl")
	brut, _ := os.ReadFile(chemin)
	alt := strings.Replace(string(brut), "refusé par D-001", "autorisé par D-001", 1)
	if alt == string(brut) {
		t.Fatal("rien à falsifier")
	}
	must(t, os.WriteFile(chemin, []byte(alt), 0o600))
	st := b.etat(id)
	if st.Verif.OK || st.Verif.Erreur == "" {
		t.Fatalf("la falsification doit être détectée : %+v", st.Verif)
	}
	v, _ := b.Dep.Chaine(st, b.jeux(), Sessions(st)[0].ID)
	n := len(v.Etapes)
	if v.Ecart < 0 || !v.Etapes[n-1].Suspect || v.Etapes[0].Suspect {
		t.Fatalf("suspects à partir de l'écart %d : %v %v", v.Ecart, v.Etapes[0].Suspect, v.Etapes[n-1].Suspect)
	}
}

func TestSansCleNonVerifiable(t *testing.T) {
	b := nouveauBanc(t)
	b.demande(claire, "Peut-on accorder un prêt sans gel ?")
	b.Dep.Cle = nil
	st := b.etat(b.Dep.Journaux()[0].ID)
	if st.Verif.OK || !st.Verif.CleAbsente {
		t.Fatalf("sans clé, rien n'est vérifié : %+v", st.Verif)
	}
}

func TestLigneEnCoursDEcritureIgnoree(t *testing.T) {
	b := nouveauBanc(t)
	id := b.demande(claire, "Peut-on accorder un prêt sans gel ?")
	f, _ := os.OpenFile(filepath.Join(b.Dep.Donnees, id, "journal.jsonl"), os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString(`{"ts":"2026-10-04T10:00:00Z","type":"appel","sess`)
	f.Close()
	if st := b.etat(id); !st.Verif.OK {
		t.Fatalf("une ligne partielle n'est pas un écart : %+v", st.Verif)
	}
}

// « Lancer les agents » : la console exécute le lanceur (ici le simulateur,
// qui respecte les contrats §2 et §3) ; la session apparaît, la demande aussi.
func TestLancerUneDemande(t *testing.T) {
	b := nouveauBanc(t)
	b.Dep.Commandes.Lanceur = []string{binSimulateur}
	b.Dep.Commandes.Env = []string{"MCP_GATE_CLE_JOURNAL=" + hexa(cleTest), "SIMULATEUR_PAUSE=10ms"}
	var l Lanceur
	for _, mauvais := range []struct{ demande, demandeur string }{{"", claire}, {"-rf /", claire}, {"Un prêt ?", "inconnu@exemple.test"}, {"Un prêt ?", "x; rm -rf /"}} {
		if _, err := b.Dep.Lancer(&l, mauvais.demande, mauvais.demandeur); err == nil {
			t.Errorf("lancement accepté : %+v", mauvais)
		}
	}
	lc, err := b.Dep.Lancer(&l, "Peut-on accorder un prêt sans gel vérifié ?", claire)
	must(t, err)
	limite := time.Now().Add(20 * time.Second)
	for time.Now().Before(limite) {
		if x := l.Liste(); len(x) == 1 && x[0].Termine {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	x := l.Liste()
	if len(x) != 1 || !x[0].Termine || x[0].Code != 0 || x[0].Erreur != "" || x[0].ID != lc.ID {
		t.Fatalf("lancement : %+v", x)
	}
	js := b.Dep.Journaux()
	if len(js) != 1 {
		t.Fatalf("sessions : %v", js)
	}
	st := b.etat(js[0].ID)
	if st.Dossier == nil || st.Dossier.Demande != "Peut-on accorder un prêt sans gel vérifié ?" || st.Dossier.Demandeur != claire || st.Dossier.Statut != StatutTermine || !st.Verif.OK {
		t.Fatalf("session : %+v %+v", st.Dossier, st.Verif)
	}
	// Un lanceur qui échoue : l'erreur est dite, sans secret.
	b.Dep.Commandes.Lanceur = []string{"false"}
	if _, err := b.Dep.Lancer(&l, "Un prêt ?", claire); err != nil {
		t.Fatal(err)
	}
	for time.Now().Before(limite) {
		if x := l.Liste(); x[0].Termine {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if x := l.Liste(); !x[0].Termine || x[0].Code == 0 {
		t.Fatalf("échec attendu : %+v", x[0])
	}
}

func TestTitreSansEspaceFinale(t *testing.T) {
	for texte, attendu := range map[string]string{
		"Décision top management : au-delà de 2 000 000 euros, …": "Décision top management",
		"Gels vérifiés avant tout prêt : aucun prêt…":             "Gels vérifiés avant tout prêt",
		"Aucune action le week-end.":                              "Aucune action le week-end",
	} {
		if got := Titre(texte); got != attendu {
			t.Errorf("Titre(%q) = %q, attendu %q", texte, got, attendu)
		}
	}
}
