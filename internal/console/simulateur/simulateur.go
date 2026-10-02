// Package simulateur est un orchestrateur et un gate simulés, pour mettre au
// point la console sans les sessions « démo réelle » et « gate ». Il respecte
// les contrats de la démonstration :
//
//   - §2 : un dossier par demande, demo/execution/sessions/<AAAAMMJJ-HHMMSS>-<court>/,
//     avec journal.jsonl (chaîné, clé MCP_GATE_CLE_JOURNAL), coffre.jsonl,
//     etat.json (réécrit atomiquement), reponse.json, notifications.jsonl ;
//   - §3 : quand un appel attend une validation, l'orchestrateur reste
//     vivant, relit approbations/<empreinte>/*.jwt toutes les secondes (10
//     minutes au plus), reprend l'appel avec les jetons, et lit rejet.json
//     comme une fin « bloque ».
//
// Les décisions sont celles de directive.Decide ; les jetons sont vérifiés
// par annuaire.VerifierApprobation avec les règles du gate (un jeton par
// rôle, valideurs distincts, jamais le demandeur, usage unique). Les
// serveurs sont fictifs. Ce n'est pas le gate : il ne sert qu'à produire des
// journaux v2 réels pour la console.
package simulateur

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"mcprogue/internal/annuaire"
	"mcprogue/internal/approval"
	"mcprogue/internal/controle"
	"mcprogue/internal/directive"
	"mcprogue/internal/journal"
)

// Config règle le simulateur.
type Config struct {
	// Racine du dépôt ; Demo : « demo » sous la racine.
	Racine string
	Demo   string
	Cle    []byte
	// Maintenant : horloge (défaut time.Now).
	Maintenant func() time.Time
	// Sondage : période de relecture des jetons (défaut 1 s, contrats §3).
	Sondage time.Duration
	// Attente : durée maximale d'attente d'une validation (défaut 10 min).
	Attente time.Duration
	// Pause entre deux appels, pour que la frise de la console se voie
	// avancer (défaut aucune).
	Pause time.Duration
}

type serveurFictif struct {
	acces     directive.Acces
	etiquette []string
}

var serveurs = map[string]serveurFictif{
	"taux-officiel":         {directive.AccesLecture, nil},
	"gels-avoirs":           {directive.AccesLecture, nil},
	"recherche-entreprises": {directive.AccesLecture, nil},
	"banque-synthetique":    {directive.AccesAction, nil},
	"crm-client":            {directive.AccesLecture, []string{"donnees_client"}},
	"rapports":              {directive.AccesAction, []string{"production_rapport"}},
}

var reNombre = regexp.MustCompile(`\d[\d \x{a0}.]*\d|\d`)

// montantDe lit un montant dans la demande (« 3 000 000 », « 400000 ») ;
// 150 000 à défaut.
func montantDe(texte string) float64 {
	best := 0.0
	for _, m := range reNombre.FindAllString(texte, -1) {
		s := strings.NewReplacer(" ", "", " ", "", ".", "").Replace(m)
		if v, err := strconv.ParseFloat(s, 64); err == nil && v >= 1000 && v > best {
			best = v
		}
	}
	if best == 0 {
		return 150000
	}
	return best
}

type appelPrevu struct {
	serveur, outil string
	args           map[string]any
}

// scenario choisit les appels d'après la demande : un rapport (données
// client puis rapport), un prêt (taux, gels, recherche, attribution), avec
// « sans gel » pour omettre la vérification des gels.
func scenario(texte string) []appelPrevu {
	t := strings.ToLower(texte)
	if strings.Contains(t, "rapport") {
		return []appelPrevu{
			{"crm-client", "lire_dossier_client", map[string]any{"dossier": "DOS-FICTIF-002"}},
			{"rapports", "produire_rapport", map[string]any{"dossier": "DOS-FICTIF-002", "type": "synthese"}},
		}
	}
	appels := []appelPrevu{
		{"taux-officiel", "get_taux_usure", map[string]any{"date": "2026-10-04"}},
	}
	if !strings.Contains(t, "sans gel") {
		appels = append(appels, appelPrevu{"gels-avoirs", "verifier_gel", map[string]any{"nom": "ENTREPRISE FICTIVE SA"}})
	}
	appels = append(appels,
		appelPrevu{"recherche-entreprises", "rechercher_entreprise", map[string]any{"siren": "552100554", "dirigeant": "A. Fictif", "note": "client sensible"}},
		appelPrevu{"banque-synthetique", "attribuer_pret", map[string]any{"dossier": "DOS-FICTIF-002", "montant": montantDe(texte)}})
	return appels
}

// session est l'état d'une demande en cours d'exécution.
type session struct {
	c         Config
	src       *annuaire.Local
	jeu       directive.Jeu
	dir       string
	id        string
	j         *journal.Ecrivain
	coffre    *journal.Coffre
	demandeur *directive.Demandeur
	seq       int
	etat      directive.EtatSession
	utilises  map[string]bool
	reg       string
	etatJSON  map[string]any
	mu        sync.Mutex
	arret     func()
}

func (c Config) maintenant() time.Time {
	if c.Maintenant != nil {
		return c.Maintenant().UTC()
	}
	return time.Now().UTC()
}

func ecrireAtomique(chemin string, b []byte) error {
	tmp := chemin + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, chemin)
}

func (s *session) majEtat(statut string, attente map[string]any, fin bool, motif string) {
	s.etatJSON["statut"] = statut
	s.etatJSON["attente"] = attente
	if fin {
		s.etatJSON["fin"] = s.c.maintenant().Format(time.RFC3339)
	}
	if motif != "" {
		s.etatJSON["motif"] = motif
	}
	b, _ := json.MarshalIndent(s.etatJSON, "", "  ")
	_ = ecrireAtomique(filepath.Join(s.dir, "etat.json"), append(b, '\n'))
}

// Demande exécute une demande de bout en bout et rend le dossier de la
// session. Elle rend quand la demande est terminée ou bloquée.
func (c Config) Demande(demandeur, texte string) (string, error) {
	if c.Demo == "" {
		c.Demo = "demo"
	}
	if c.Sondage == 0 {
		c.Sondage = time.Second
	}
	if c.Attente == 0 {
		c.Attente = 10 * time.Minute
	}
	demo := filepath.Join(c.Racine, c.Demo)
	brut, err := os.ReadFile(filepath.Join(demo, "directives", "demo.json"))
	if err != nil {
		return "", err
	}
	jeu, err := directive.Charger(brut)
	if err != nil {
		return "", fmt.Errorf("règles : %w", err)
	}
	ab, err := os.ReadFile(filepath.Join(demo, "annuaire.json"))
	if err != nil {
		return "", err
	}
	autorite, err := annuaire.LireClePublique(filepath.Join(demo, "execution", "cles", annuaire.FichierAutoritePublic))
	if err != nil {
		return "", err
	}
	src, err := annuaire.Charger(ab, autorite)
	if err != nil {
		return "", err
	}
	now := c.maintenant()
	fiche, ok := src.Resoudre(demandeur, now)
	if !ok {
		return "", fmt.Errorf("demandeur %q inconnu de l'annuaire", demandeur)
	}
	court := approval.Digest("demande", map[string]any{"t": texte, "n": float64(now.UnixNano())})
	court = court[len(court)-6:]
	id := now.In(time.UTC).Format("20060102-150405") + "-" + court
	dir := filepath.Join(demo, "execution", "sessions", id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	s := &session{c: c, src: src, jeu: *jeu, dir: dir, id: id, utilises: map[string]bool{}, reg: "registre-demo-1",
		demandeur: &directive.Demandeur{Identite: fiche.Identite, Groupes: fiche.Groupes},
		etatJSON:  map[string]any{"demande": texte, "demandeur": demandeur, "debut": now.Format(time.RFC3339), "fin": nil}}
	s.majEtat("en_cours", nil, false, "")
	if s.j, err = journal.Ouvrir(filepath.Join(dir, "journal.jsonl"), c.Cle); err != nil {
		return dir, err
	}
	if s.coffre, err = journal.OuvrirCoffre(filepath.Join(dir, "coffre.jsonl")); err != nil {
		return dir, err
	}
	s.coffre.Journaliser(s.j)
	if s.arret, err = s.abonne(); err != nil {
		return dir, err
	}
	defer func() { s.arret(); s.coffre.Fermer() }()
	if err := s.executer(texte); err != nil {
		s.majEtat("erreur", nil, true, err.Error())
		return dir, err
	}
	return dir, nil
}

// abonne écrit notifications.jsonl, comme l'abonné « -notifications » du gate.
func (s *session) abonne() (func(), error) {
	f, err := os.OpenFile(filepath.Join(s.dir, "notifications.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	ch, desabonner := s.j.Abonner(256)
	fini := make(chan struct{})
	go func() {
		defer close(fini)
		enc := json.NewEncoder(f)
		for en := range ch {
			if en.Type == journal.NotificationT {
				enc.Encode(en)
			}
		}
	}()
	return func() { desabonner(); <-fini; f.Close() }, nil
}

func (s *session) ecrire(e journal.Entree) error {
	_, err := s.j.Ecrire(e)
	return err
}

func (s *session) executer(texte string) error {
	now := s.c.maintenant()
	ref := s.src.Reference()
	if err := s.ecrire(journal.Entree{TS: now, Type: journal.JeuCharge, Jeu: journal.DepuisJeu(s.jeu)}); err != nil {
		return err
	}
	if err := s.ecrire(journal.Entree{TS: now, Type: journal.AnnuaireCharge, SourceIdentite: s.src.Nom(),
		Annuaire: &journal.RefAnnuaire{ID: ref.ID, Version: ref.Version, Empreinte: ref.Empreinte}}); err != nil {
		return err
	}
	noms := make([]string, 0, len(serveurs))
	for n := range serveurs {
		noms = append(noms, n)
	}
	sort.Strings(noms)
	for _, n := range noms {
		if err := s.ecrire(journal.Entree{TS: now, Type: journal.AttestationT, Serveur: n, Attestation: journal.DepuisAttestation(attestee)}); err != nil {
			return err
		}
	}
	appelant := "spiffe://demo/orchestrateur"
	if err := s.ecrire(journal.Entree{TS: now, Type: journal.SessionOuverte, Session: s.id, Appelant: appelant,
		Demandeur: journal.DepuisDemandeur(s.demandeur), SourceIdentite: s.src.Nom(),
		Annuaire: &journal.RefAnnuaire{ID: ref.ID, Version: ref.Version, Empreinte: ref.Empreinte},
		Message:  "demandeur " + s.demandeur.Identite + " (jeton signé par lui-même)"}); err != nil {
		return err
	}
	s.etat = directive.EtatSession{ID: s.id, Appelant: appelant}
	var cites []string
	var dernier string
	for i, a := range scenario(texte) {
		if i > 0 && s.c.Pause > 0 {
			time.Sleep(s.c.Pause)
		}
		res, err := s.appeler(a)
		if err != nil {
			return err
		}
		cites = append(cites, res.cites...)
		dernier = res.resume
		if res.bloque {
			return s.cloturer(res.statut, res.resume, cites)
		}
	}
	return s.cloturer("termine", dernier, cites)
}

func (s *session) cloturer(statut, texte string, cites []string) error {
	if err := s.ecrire(journal.Entree{Type: journal.SessionFermee, Session: s.id, Message: "session terminée par le client"}); err != nil {
		return err
	}
	vus := map[string]bool{}
	liste := []string{}
	for _, c := range cites {
		if !vus[c] {
			vus[c] = true
			liste = append(liste, c)
		}
	}
	rep, _ := json.MarshalIndent(map[string]any{"texte": texte, "statut": statut, "directives_citees": liste}, "", "  ")
	if err := ecrireAtomique(filepath.Join(s.dir, "reponse.json"), append(rep, '\n')); err != nil {
		return err
	}
	s.majEtat(statut, nil, true, "")
	return nil
}

var attestee = directive.Attestation{Mode: "binaire", Attendu: "sha256:aaaa", Observe: "sha256:aaaa", Schemas: true, Verifiee: true}

type resultat struct {
	cites  []string
	resume string
	bloque bool
	statut string
}

func ids(dec directive.Decision) []string {
	var out []string
	for _, a := range dec.Applications {
		if a.Appliquee {
			out = append(out, a.ID)
		}
	}
	return out
}

// appeler traite un appel d'outil comme le gate : décision, notifications,
// attente éventuelle d'une validation humaine, transmission.
func (s *session) appeler(a appelPrevu) (resultat, error) {
	entreePipeline := time.Now()
	s.seq++
	seq := s.seq
	ts := s.c.maintenant()
	srv := serveurs[a.serveur]
	origine := approval.Digest(a.outil, a.args)
	if err := s.coffre.Deposer(journal.Depot{TS: ts, Empreinte: origine, Outil: a.outil, ArgumentsOrigine: a.args}); err != nil {
		return resultat{}, err
	}
	appel := directive.Appel{Serveur: a.serveur, Outil: a.outil, Arguments: a.args, Attestation: attestee, Horodatage: ts,
		Acces: srv.acces, EtiquettesCible: srv.etiquette, VersionRegistre: s.reg, Demandeur: s.demandeur}
	dec, parRegistre := controle.Registre(directive.Decide(s.jeu, s.etat, appel), a.serveur, attestee)
	remises, parNotif, err := s.notifier(dec, ts, seq, a)
	if err != nil {
		return resultat{}, err
	}
	if !parRegistre {
		dec, parNotif = journal.ControleNotifications(dec, remises)
	}
	decJ := journal.DepuisDecision(dec)
	switch {
	case parRegistre:
		decJ.Origine = controle.OrigineRegistre
	case parNotif:
		decJ.Origine = journal.OrigineNotification
	}
	decJ.DureeUs = (time.Since(entreePipeline).Nanoseconds() + 999) / 1000
	entree := journal.Entree{TS: ts, Type: journal.AppelT, Session: s.id, Seq: seq, Appelant: s.etat.Appelant,
		Jeu: journal.DepuisJeu(s.jeu), Serveur: a.serveur, Outil: a.outil,
		Appel:       &journal.Appel{EmpreinteOrigine: dec.EmpreinteOrigine, EmpreinteTransmise: dec.EmpreinteTransmise, ChampsRetires: dec.ChampsRetires},
		Attestation: journal.DepuisAttestation(attestee), EtatSession: journal.DepuisConsultations(s.etat.Consultations),
		Decision: decJ, Resultat: &journal.Resultat{Statut: journal.StatutNonTransmis},
		VersionRegistre: s.reg, Acces: string(srv.acces), EtiquettesCible: srv.etiquette,
		Demandeur: journal.DepuisDemandeur(s.demandeur), Remises: remises}
	cites := ids(dec)
	switch {
	case dec.Effet == directive.Refuser:
		if err := s.ecrire(entree); err != nil {
			return resultat{}, err
		}
		return resultat{cites: cites, bloque: true, statut: "termine",
			resume: fmt.Sprintf("Je n'ai pas pu mener la demande à terme : l'appel %s a été refusé (%s).", a.outil, strings.Join(cites, ", "))}, nil
	case dec.Effet == directive.AttendreValidation:
		if err := s.ecrire(entree); err != nil { // première tentative : retenue
			return resultat{}, err
		}
		appro, err := s.attendre(dec, a)
		if err != nil {
			return resultat{cites: cites, bloque: true, statut: "bloque",
				resume: "La validation humaine n'a pas été obtenue : la demande est bloquée (" + err.Error() + ")."}, nil
		}
		// Reprise : nouvelle entrée appel, avec l'approbation acceptée.
		s.seq++
		seq = s.seq
		ts = s.c.maintenant()
		reprise := entree
		reprise.TS, reprise.Seq = ts, seq
		reprise.Approbation = appro.ap
		if err := s.ecrire(journal.Entree{TS: ts, Type: journal.ApprobationT, Session: s.id, Seq: seq, Appelant: s.etat.Appelant,
			Serveur: a.serveur, Outil: a.outil, Approbation: appro.ap, Demandeur: entree.Demandeur}); err != nil {
			return resultat{}, err
		}
		entree = reprise
		s.majEtat("en_cours", nil, false, "")
	}
	if err := s.ecrire(journal.Entree{TS: entree.TS, Type: journal.TransmissionT, Session: s.id, Seq: entree.Seq, Appelant: s.etat.Appelant,
		Jeu: entree.Jeu, Serveur: a.serveur, Outil: a.outil, Appel: &journal.Appel{EmpreinteTransmise: dec.EmpreinteTransmise},
		Decision: entree.Decision}); err != nil {
		return resultat{}, err
	}
	rep := "rep-" + a.outil
	entree.Resultat = &journal.Resultat{Statut: journal.StatutOK, EmpreinteReponse: rep, DureeMs: 12,
		Source: &journal.Source{Origine: "direct"}}
	s.etat.Consultations = append(s.etat.Consultations, directive.Consultation{Seq: entree.Seq, Serveur: a.serveur, Outil: a.outil,
		Horodatage: entree.TS, Empreinte: dec.EmpreinteTransmise, Resultat: directive.ResultatOK, EmpreinteReponse: rep,
		Etiquettes: srv.etiquette})
	if err := s.ecrire(entree); err != nil {
		return resultat{}, err
	}
	return resultat{cites: cites, resume: s.resume(a, dec)}, nil
}

func (s *session) resume(a appelPrevu, dec directive.Decision) string {
	switch a.outil {
	case "attribuer_pret":
		m, _ := a.args["montant"].(float64)
		return fmt.Sprintf("Le prêt de %s € a été attribué pour le dossier DOS-FICTIF-002.", milliers(m))
	case "produire_rapport":
		return "Le rapport de synthèse du dossier DOS-FICTIF-002 a été produit."
	}
	return "Appel " + a.outil + " effectué."
}

func milliers(v float64) string {
	s := strconv.FormatInt(int64(v), 10)
	var out []string
	for len(s) > 3 {
		out = append([]string{s[len(s)-3:]}, out...)
		s = s[:len(s)-3]
	}
	out = append([]string{s}, out...)
	return strings.Join(out, " ")
}

// notifier écrit une entrée notification par notification demandée, avant
// toute transmission (contrat de l'engine §7).
func (s *session) notifier(dec directive.Decision, ts time.Time, seq int, a appelPrevu) ([]journal.Remise, bool, error) {
	var out []journal.Remise
	for _, n := range journal.DepuisDecision(dec).Notifications {
		en, err := s.j.EcrireRemise(journal.Entree{TS: ts, Session: s.id, Seq: seq, Appelant: s.etat.Appelant,
			Serveur: a.serveur, Outil: a.outil, Notification: &journal.Remise{Notification: n}})
		if err != nil {
			return nil, false, err
		}
		out = append(out, *en.Notification)
	}
	return out, false, nil
}

type repriseT struct{ ap *journal.Approbation }

var errRejet = errors.New("validation rejetée")

// attendre passe l'état à en_attente puis relit les jetons toutes les
// secondes (contrats §3), 10 minutes au plus.
func (s *session) attendre(dec directive.Decision, a appelPrevu) (repriseT, error) {
	roles, quatre := []string(nil), false
	if v := dec.Validation; v != nil {
		roles, quatre = v.Roles, v.QuatreYeux
	}
	attente := func(motif string) map[string]any {
		m := map[string]any{"empreinte_transmise": dec.EmpreinteTransmise, "roles": roles, "quatre_yeux": quatre, "directives": ids(dec)}
		if motif != "" {
			m["motif_jeton"] = motif
		}
		return m
	}
	if roles == nil {
		attente0 := attente("")
		attente0["roles"] = []string{}
		s.majEtat("en_attente", attente0, false, "")
	} else {
		s.majEtat("en_attente", attente(""), false, "")
	}
	dossier := filepath.Join(s.c.Racine, s.c.Demo, "execution", "approbations", dec.EmpreinteTransmise)
	limite := time.Now().Add(s.c.Attente)
	vu := ""
	for time.Now().Before(limite) {
		time.Sleep(s.c.Sondage)
		if _, err := os.Stat(filepath.Join(dossier, "rejet.json")); err == nil {
			s.majEtat("bloque", nil, true, "validation rejetée par un superviseur")
			return repriseT{}, errRejet
		}
		fichiers, _ := filepath.Glob(filepath.Join(dossier, "*.jwt"))
		sort.Strings(fichiers)
		attendus := len(roles)
		if attendus == 0 {
			attendus = 1
		}
		if len(fichiers) < attendus {
			continue
		}
		var jetons []string
		cle := ""
		for _, f := range fichiers {
			b, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			jetons = append(jetons, strings.TrimSpace(string(b)))
			cle += f + "|" + approval.Digest("j", map[string]any{"j": string(b)})
		}
		if cle == vu {
			continue // déjà refusés tels quels : on attend un jeton nouveau
		}
		vu = cle
		ts := s.c.maintenant()
		ap := s.verifier(dec, jetons, ts)
		if ap.Statut != journal.ApprobationAcceptee {
			if err := s.ecrire(journal.Entree{TS: ts, Type: journal.ApprobationT, Session: s.id, Seq: s.seq + 1, Appelant: s.etat.Appelant,
				Serveur: a.serveur, Outil: a.outil, Approbation: ap, Demandeur: journal.DepuisDemandeur(s.demandeur)}); err != nil {
				return repriseT{}, err
			}
			s.majEtat("en_attente", attente(ap.Motif), false, "")
			continue
		}
		return repriseT{ap: ap}, nil
	}
	s.majEtat("bloque", nil, true, "validation non reçue")
	return repriseT{}, errors.New("validation non reçue")
}

// verifier applique les règles du gate (internal/gate/validation.go).
func (s *session) verifier(dec directive.Decision, jetons []string, ts time.Time) *journal.Approbation {
	var roles []string
	quatre := false
	if v := dec.Validation; v != nil {
		roles, quatre = v.Roles, v.QuatreYeux
	}
	a := &journal.Approbation{Mecanisme: "annuaire", Statut: journal.ApprobationRefusee, EmpreinteTransmise: dec.EmpreinteTransmise, Roles: roles, QuatreYeux: quatre}
	refus := func(f string, args ...any) *journal.Approbation { a.Motif = fmt.Sprintf(f, args...); return a }
	vus := map[string]bool{}
	parRole := map[string]int{}
	for i, j := range jetons {
		ap, err := annuaire.VerifierApprobation(s.src, j, dec.EmpreinteTransmise, ts)
		if err != nil {
			return refus("jeton %d refusé : %v", i+1, err)
		}
		if ap.Valideur == s.demandeur.Identite {
			return refus("jeton %d : le valideur %s est le demandeur", i+1, ap.Valideur)
		}
		if vus[ap.Valideur] {
			return refus("jeton %d : %s a déjà validé cet appel ; chaque rôle demande un valideur distinct", i+1, ap.Valideur)
		}
		vus[ap.Valideur] = true
		parRole[ap.Role]++
		a.Valideurs = append(a.Valideurs, journal.Valideur{Identite: ap.Valideur, Role: ap.Role, Date: ap.Date, EmpreinteJeton: ap.EmpreinteJeton})
	}
	if len(roles) == 0 {
		if len(jetons) != 1 {
			return refus("un seul jeton attendu, %d reçus", len(jetons))
		}
	} else {
		var manquants []string
		for _, r := range roles {
			if parRole[r] == 0 {
				manquants = append(manquants, r)
			}
		}
		if len(manquants) > 0 {
			return refus("rôle(s) sans validation : %s", strings.Join(manquants, ", "))
		}
		if len(jetons) != len(roles) {
			return refus("un jeton par rôle requis (%d), %d reçus", len(roles), len(jetons))
		}
	}
	for i, v := range a.Valideurs {
		if s.utilises[v.EmpreinteJeton] {
			return refus("jeton déjà utilisé (jeton %d) : une validation ne vaut que pour un appel", i+1)
		}
	}
	for _, v := range a.Valideurs {
		s.utilises[v.EmpreinteJeton] = true
	}
	sort.Slice(a.Valideurs, func(i, j int) bool { return a.Valideurs[i].Role < a.Valideurs[j].Role })
	var noms []string
	for _, v := range a.Valideurs {
		noms = append(noms, v.Identite)
	}
	a.ApprouvePar = strings.Join(noms, ", ")
	a.Statut, a.Motif = journal.ApprobationAcceptee, ""
	return a
}
