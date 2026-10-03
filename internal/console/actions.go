package console

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// executer lance une commande (sans interpréteur : chaque argument reste un
// argument), dans la racine du dépôt, avec l'environnement de la console
// complété. Sortie standard et d'erreur sont rendues séparément ; une
// sortie est tronquée à 4 Mo.
func (c Commandes) executer(ctx context.Context, argv []string, entree []byte) (out, errOut []byte, err error) {
	if len(argv) == 0 {
		return nil, nil, errors.New("commande non configurée")
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = c.Racine
	cmd.Env = append(os.Environ(), c.Env...)
	if entree != nil {
		cmd.Stdin = bytes.NewReader(entree)
	}
	var so, se limite
	cmd.Stdout, cmd.Stderr = &so, &se
	err = cmd.Run()
	return so.Bytes(), se.Bytes(), err
}

// limite est un tampon borné.
type limite struct{ b bytes.Buffer }

func (l *limite) Write(p []byte) (int, error) {
	if l.b.Len() < 4<<20 {
		l.b.Write(p)
	}
	return len(p), nil
}
func (l *limite) Bytes() []byte { return l.b.Bytes() }

var reJeton = regexp.MustCompile(`^[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]*$`)

// ErreurAction est refusée à l'utilisateur, avec un message en français.
type ErreurAction struct {
	Code    int
	Message string
}

func (e *ErreurAction) Error() string { return e.Message }

func refus(code int, f string, a ...any) *ErreurAction {
	return &ErreurAction{Code: code, Message: fmt.Sprintf(f, a...)}
}

// Signature : « Signer et valider ». La console appelle mcp-approve valider,
// qui signe avec la clé du valideur ; elle dépose le jeton dans
// approbations/<empreinte>/<valideur>.jwt (contrats §3). L'empreinte et les
// rôles viennent de etat.json, pas du navigateur ; le valideur et son rôle
// doivent être dans l'annuaire.
func (d *Depot) Valider(journalID, valideur, role string) (*AttenteVue, error) {
	st, att, err := d.attenteDeSession(journalID)
	if err != nil {
		return nil, err
	}
	if d.LireRejet(att.EmpreinteTransmise) != nil {
		return nil, refus(409, "Cette demande a déjà été rejetée : aucune signature n'est plus acceptée.")
	}
	// Session lancée avec --horloge-depart : le gate vit à l'heure simulée, le
	// jeton doit être signé à cette heure et les rôles contrôlés à cette heure.
	maintenant := time.Now()
	argHorloge := []string{}
	if h, simulee := horlogeSimulee(st.Journal.Dossier(), maintenant); simulee {
		maintenant = h
		argHorloge = []string{"-horloge", h.UTC().Format(time.RFC3339)}
	}
	possibles, err := d.Valideurs(att.Roles, maintenant)
	if err != nil {
		return nil, refus(409, "Annuaire indisponible : %v", err)
	}
	ok := false
	for _, v := range possibles {
		if v.Identite == valideur && v.Role == role {
			ok = true
		}
	}
	if !ok {
		return nil, refus(403, "%s ne détient pas le rôle « %s » requis, d'après l'annuaire.", valideur, role)
	}
	ch := d.Chemins()
	argv := append(append([]string{}, d.Commandes.Approuver...), "valider",
		"-cles", ch.Cles, "-valideur", valideur, "-role", role, "-empreinte", att.EmpreinteTransmise)
	argv = append(argv, argHorloge...)
	ctx, annuler := context.WithTimeout(context.Background(), 60*time.Second)
	defer annuler()
	out, errOut, err := d.Commandes.executer(ctx, argv, nil)
	if err != nil {
		return nil, refus(502, "La signature a échoué : %s", premiereLigne(errOut, err))
	}
	jeton := strings.TrimSpace(string(out))
	if !reJeton.MatchString(jeton) {
		return nil, refus(502, "La commande de signature n'a pas rendu de jeton.")
	}
	ecritureMu.Lock()
	err = ecrireAtomique(filepath.Join(dossierApprobations(ch, att.EmpreinteTransmise), valideur+".jwt"), []byte(jeton+"\n"), 0o600)
	ecritureMu.Unlock()
	if err != nil {
		return nil, refus(500, "Le jeton n'a pas pu être déposé : %v", err)
	}
	a, _ := d.attenteDe(st, nil, time.Now())
	return &a, nil
}

// Rejeter écrit un refus consigné : approbations/<empreinte>/rejet.json, que
// l'orchestrateur lit comme une fin « bloque ». Le motif est obligatoire.
func (d *Depot) Rejeter(journalID, valideur, role, motif string) (*Rejet, error) {
	motif = strings.TrimSpace(motif)
	if motif == "" {
		return nil, refus(400, "Un rejet exige un motif.")
	}
	if utf8.RuneCountInString(motif) > 2000 {
		return nil, refus(400, "Le motif est trop long.")
	}
	_, att, err := d.attenteDeSession(journalID)
	if err != nil {
		return nil, err
	}
	possibles, err := d.Valideurs(nil, time.Now())
	if err != nil {
		return nil, refus(409, "Annuaire indisponible : %v", err)
	}
	ok, roleOk := false, role == ""
	for _, v := range possibles {
		if v.Identite == valideur {
			ok = true
			roleOk = roleOk || v.Role == role
		}
	}
	if !ok {
		return nil, refus(403, "%s n'est pas un valideur de l'annuaire.", valideur)
	}
	if !roleOk {
		return nil, refus(403, "%s ne détient pas le rôle « %s », d'après l'annuaire.", valideur, role)
	}
	r := Rejet{Valideur: valideur, Nom: d.NomIdentite(valideur), Role: role, Motif: motif, Date: time.Now().UTC()}
	b, err := jsonIndent(r)
	if err != nil {
		return nil, err
	}
	ecritureMu.Lock()
	err = ecrireAtomique(filepath.Join(dossierApprobations(d.Chemins(), att.EmpreinteTransmise), "rejet.json"), b, 0o644)
	ecritureMu.Unlock()
	if err != nil {
		return nil, refus(500, "Le rejet n'a pas pu être consigné : %v", err)
	}
	return &r, nil
}

// attenteDeSession rend l'état d'une session dont etat.json est en_attente.
func (d *Depot) attenteDeSession(journalID string) (*Etat, *AttenteEtat, error) {
	j, ok := d.Journal(journalID)
	if !ok {
		return nil, nil, refus(404, "Session inconnue.")
	}
	st, err := d.Lire(j)
	if err != nil {
		return nil, nil, refus(500, "Session illisible.")
	}
	if st.Dossier == nil || st.Dossier.Statut != StatutEnAttente || st.Dossier.Attente == nil {
		return nil, nil, refus(409, "Cette session n'attend plus de validation.")
	}
	if !reEmpreinte.MatchString(st.Dossier.Attente.EmpreinteTransmise) {
		return nil, nil, refus(409, "L'empreinte de l'appel en attente est illisible.")
	}
	return st, st.Dossier.Attente, nil
}

func premiereLigne(b []byte, err error) string {
	for _, l := range strings.Split(string(b), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			if len(l) > 300 {
				l = l[:300] + "…"
			}
			return l
		}
	}
	return err.Error()
}

// ---------------------------------------------------------------------------
// Lancement d'une demande (écran 2)
// ---------------------------------------------------------------------------

// Lancement est une demande lancée depuis le poste conseiller.
type Lancement struct {
	ID        string     `json:"id"`
	Demande   string     `json:"demande"`
	Demandeur string     `json:"demandeur"`
	Debut     time.Time  `json:"debut"`
	Fin       *time.Time `json:"fin,omitempty"`
	Termine   bool       `json:"termine"`
	Code      int        `json:"code"`
	Erreur    string     `json:"erreur,omitempty"`
}

// Lanceur suit les demandes lancées par la console, en mémoire.
type Lanceur struct {
	mu   sync.Mutex
	n    int
	List []*Lancement
}

const maxDemande = 2000

// Lancer exécute scripts/lancer-demo.sh demande --demandeur <identité>
// <texte> (contrats §2). Le demandeur est une identité de l'annuaire ; le
// texte reste un argument, jamais interprété par un interpréteur de
// commandes. Le processus tourne en arrière-plan : la frise se lit dans les
// fichiers de la session.
func (d *Depot) Lancer(l *Lanceur, demande, demandeur string) (*Lancement, error) {
	demande = strings.TrimSpace(demande)
	switch {
	case demande == "":
		return nil, refus(400, "La demande est vide.")
	case utf8.RuneCountInString(demande) > maxDemande || !utf8.ValidString(demande) || strings.ContainsRune(demande, 0):
		return nil, refus(400, "La demande est trop longue ou illisible.")
	case strings.HasPrefix(demande, "-"):
		return nil, refus(400, "La demande ne peut pas commencer par un tiret.")
	}
	if _, ok := d.Fiche(demandeur, time.Now()); !ok {
		return nil, refus(403, "Le demandeur « %s » n'est pas dans l'annuaire.", demandeur)
	}
	if len(d.Commandes.Lanceur) == 0 {
		return nil, refus(501, "Le lanceur de la démonstration n'est pas configuré.")
	}
	l.mu.Lock()
	enCours := 0
	for _, x := range l.List {
		if !x.Termine {
			enCours++
		}
	}
	if enCours >= 4 {
		l.mu.Unlock()
		return nil, refus(429, "Quatre demandes sont déjà en cours.")
	}
	l.n++
	lc := &Lancement{ID: fmt.Sprintf("L-%d", l.n), Demande: demande, Demandeur: demandeur, Debut: time.Now().UTC()}
	l.List = append(l.List, lc)
	l.mu.Unlock()

	argv := append(append([]string{}, d.Commandes.Lanceur...), "demande", "--demandeur", demandeur, demande)
	go func() {
		ctx, annuler := context.WithTimeout(context.Background(), 30*time.Minute)
		defer annuler()
		_, errOut, err := d.Commandes.executer(ctx, argv, nil)
		fin := time.Now().UTC()
		l.mu.Lock()
		defer l.mu.Unlock()
		lc.Fin, lc.Termine = &fin, true
		if err != nil {
			lc.Code = -1
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				lc.Code = ee.ExitCode()
			}
			lc.Erreur = secretsRetires(premiereLigne(errOut, err))
		}
	}()
	return lc, nil
}

// Liste rend les lancements, du plus récent au plus ancien.
func (l *Lanceur) Liste() []Lancement {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := []Lancement{}
	for i := len(l.List) - 1; i >= 0; i-- {
		out = append(out, *l.List[i])
	}
	return out
}

// secretsRetires ôte d'un message toute valeur de secret de l'environnement
// (rien de secret ne s'affiche, même par accident dans la sortie d'un script).
func secretsRetires(s string) string {
	for _, k := range []string{"MISTRAL_API_KEY", "PISTE_CLIENT_ID", "PISTE_CLIENT_SECRET", "MCP_GATE_CLE_JOURNAL"} {
		if v := os.Getenv(k); len(v) >= 6 {
			s = strings.ReplaceAll(s, v, "[retiré]")
		}
	}
	return s
}

// horlogeSimulee rend l'heure du gate d'une session lancée avec
// --horloge-depart : le départ consigné dans horloge.json, plus le temps réel
// écoulé depuis le lancement (le calcul de « lancer-demo.sh valider »).
// Sans horloge.json, ou illisible : l'heure réelle, et false.
func horlogeSimulee(dossier string, maintenant time.Time) (time.Time, bool) {
	b, err := os.ReadFile(filepath.Join(dossier, "horloge.json"))
	if err != nil {
		return maintenant, false
	}
	var h struct {
		Depart string `json:"depart"`
		Reel   string `json:"reel"`
	}
	if json.Unmarshal(b, &h) != nil {
		return maintenant, false
	}
	depart, err1 := time.Parse(time.RFC3339Nano, h.Depart)
	reel, err2 := time.Parse(time.RFC3339Nano, h.Reel)
	if err1 != nil || err2 != nil {
		return maintenant, false
	}
	return depart.Add(maintenant.Sub(reel)).Truncate(time.Second), true
}
