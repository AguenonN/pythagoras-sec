package console

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"mcprogue/internal/directive"
	"mcprogue/internal/relecture"
)

func jsonIndent(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ---------------------------------------------------------------------------
// Service du compilateur (contrats §4)
// ---------------------------------------------------------------------------

// Question du compilateur, avec son origine : un modèle ou du code.
type Question struct {
	ID      string   `json:"id"`
	Texte   string   `json:"texte"`
	Options []string `json:"options,omitempty"`
	Origine string   `json:"origine"` // mistral | code
}

// NonObservable : le texte décrit un critère qu'aucun appel d'outil ne montre.
type NonObservable struct {
	Extrait string `json:"extrait"`
	Raison  string `json:"raison"`
}

// ReponseQuestion est la réponse du juriste à une question.
type ReponseQuestion struct {
	Question string `json:"question"`
	Reponse  string `json:"reponse"`
}

// RequeteCompilateur est la requête du §4 (une ligne de JSON sur l'entrée).
type RequeteCompilateur struct {
	Conversation any               `json:"conversation"`
	Texte        string            `json:"texte"`
	Fondement    string            `json:"fondement"`
	Auteur       string            `json:"auteur"`
	Reponses     []ReponseQuestion `json:"reponses"`
	JeuExistant  string            `json:"jeu_existant"`
	DateEffet    string            `json:"date_effet"`
}

// ReponseCompilateur est la réponse du §4.
type ReponseCompilateur struct {
	Conversation  string         `json:"conversation"`
	Etat          string         `json:"etat"` // questions | non_observable | jeu | refuse | erreurs
	Questions     []Question     `json:"questions"`
	NonObservable *NonObservable `json:"non_observable"`
	Relecture     string         `json:"relecture"`
	JeuCandidat   *string        `json:"jeu_candidat"`
	Erreurs       []string       `json:"erreurs"`
}

// Compilation est ce que l'écran de rédaction reçoit : la réponse du
// service et, quand un jeu candidat existe, ses directives ajoutées avec
// leur origine et leur relecture (celle de internal/relecture, la même que
// « cmd/directive relire »).
type Compilation struct {
	ReponseCompilateur
	Ajoutees []DirAjoutee `json:"ajoutees,omitempty"`
}

// DirAjoutee est une directive que le jeu candidat ajoute aux règles.
type DirAjoutee struct {
	ID         string                `json:"id"`
	Version    int                   `json:"version"`
	Statut     string                `json:"statut"`
	Titre      string                `json:"titre"`
	Texte      string                `json:"texte"`
	Fondements []directive.Fondement `json:"fondements"`
	Relecture  string                `json:"relecture"`
}

// racineSure rend le chemin absolu d'un fichier annoncé par le service, s'il
// est bien sous demo/execution/compilation/ : un chemin rendu par un
// programme extérieur n'est jamais suivi tel quel.
func (d *Depot) cheminCandidat(chemin string) (string, error) {
	comp, err := filepath.Abs(d.Chemins().Compilation)
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(chemin)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(abs, comp+string(filepath.Separator)) {
		return "", fmt.Errorf("le résultat de la compilation est hors de %s", d.cheminsRel().Compilation)
	}
	return abs, nil
}

// Compiler appelle le service du compilateur : une requête, une réponse. La
// console ne compile rien et n'appelle aucun modèle ; le service garde l'état
// de la conversation dans demo/execution/compilation/<id>/.
func (d *Depot) Compiler(texte, fondement, conversation string, reponses []ReponseQuestion, proposition string) (*Compilation, error) {
	texte, fondement = strings.TrimSpace(texte), strings.TrimSpace(fondement)
	if texte == "" && conversation == "" {
		return nil, refus(400, "Le texte de la directive est vide.")
	}
	if len(texte) > 20000 || len(fondement) > 2000 {
		return nil, refus(400, "Le texte est trop long.")
	}
	if conversation != "" && !reIdSur.MatchString(conversation) {
		return nil, refus(400, "Conversation inconnue.")
	}
	if len(d.Commandes.Compilateur) == 0 {
		return nil, refus(501, "Le service du compilateur n'est pas configuré.")
	}
	var conv any
	if conversation != "" {
		conv = conversation
	}
	req := RequeteCompilateur{Conversation: conv, Texte: texte, Fondement: fondement, Auteur: d.Juriste,
		Reponses: reponses, JeuExistant: d.cheminsRel().Directives, DateEffet: time.Now().In(Paris()).Format(time.RFC3339)}
	if req.Reponses == nil {
		req.Reponses = []ReponseQuestion{}
	}
	corps, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	ctx, annuler := context.WithTimeout(context.Background(), 3*time.Minute)
	defer annuler()
	out, errOut, err := d.Commandes.executer(ctx, d.Commandes.Compilateur, corps)
	if err != nil && len(bytes.TrimSpace(out)) == 0 {
		return nil, refus(502, "Le service du compilateur n'a pas répondu : %s", secretsRetires(premiereLigne(errOut, err)))
	}
	var rep ReponseCompilateur
	if e := json.Unmarshal(bytes.TrimSpace(out), &rep); e != nil {
		return nil, refus(502, "La réponse du compilateur est illisible.")
	}
	if rep.Conversation != "" && !reIdSur.MatchString(rep.Conversation) {
		return nil, refus(502, "Le compilateur a rendu un identifiant de conversation invalide.")
	}
	switch rep.Etat {
	case "questions", "non_observable", "jeu", "refuse", "erreurs":
	default:
		return nil, refus(502, "État du compilateur inconnu : %q.", rep.Etat)
	}
	c := &Compilation{ReponseCompilateur: rep}
	if c.Questions == nil {
		c.Questions = []Question{}
	}
	if c.Erreurs == nil {
		c.Erreurs = []string{}
	}
	if rep.Etat == "jeu" && rep.JeuCandidat != nil {
		cand, _, err := d.candidat(rep.Conversation)
		if err != nil {
			return nil, refus(502, "Les règles produites par le compilateur sont refusées : %v", err)
		}
		cour, _ := d.courant()
		if cour != nil {
			c.Ajoutees = ajoutees(*cand, *cour)
		}
		// La relecture affichée est celle de la console (déterministe), pas
		// seulement celle que le service rapporte.
		if len(c.Ajoutees) > 0 {
			var parts []string
			for _, a := range c.Ajoutees {
				parts = append(parts, a.Relecture)
			}
			c.Relecture = strings.Join(parts, "\n\n")
		}
	}
	if proposition != "" && rep.Conversation != "" {
		d.noterConversation(proposition, rep.Conversation, texte)
	}
	return c, nil
}

// noterConversation rattache la conversation du compilateur à la décision du
// juriste sur la proposition (et le texte retenu si c'est une reprise).
func (d *Depot) noterConversation(proposition, conversation, texte string) {
	dp := d.LireDecision(proposition)
	if dp == nil || dp.Decision == "rejetee" {
		return
	}
	dp.ConversationCompilateur = conversation
	if strings.TrimSpace(texte) != "" {
		dp.TexteRetenu = texte
	}
	if b, err := jsonIndent(dp); err == nil {
		ecritureMu.Lock()
		_ = ecrireAtomique(filepath.Join(d.Chemins().Propositions, proposition+".decision.json"), b, 0o644)
		ecritureMu.Unlock()
	}
}

var reCandidat = regexp.MustCompile(`^demo\.v(\d+)\.json$`)

// candidat charge le jeu candidat le plus récent de la conversation (le
// service en écrit un par version : demo.vN.json). Il passe par le chargeur
// strict et la forme unique des directives.
func (d *Depot) candidat(conversation string) (*directive.Jeu, string, error) {
	if !reIdSur.MatchString(conversation) {
		return nil, "", errors.New("conversation inconnue")
	}
	dir := filepath.Join(d.Chemins().Compilation, conversation)
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, "", errors.New("aucun résultat de compilation pour cette conversation")
	}
	best, bestN := "", 0
	for _, e := range ents {
		if m := reCandidat.FindStringSubmatch(e.Name()); m != nil {
			if n, _ := strconv.Atoi(m[1]); n > bestN {
				best, bestN = e.Name(), n
			}
		}
	}
	if best == "" {
		return nil, "", errors.New("aucun résultat de compilation pour cette conversation")
	}
	chemin, err := d.cheminCandidat(filepath.Join(dir, best))
	if err != nil {
		return nil, "", err
	}
	brut, err := os.ReadFile(chemin)
	if err != nil {
		return nil, "", err
	}
	j, err := chargerOctets(brut)
	return j, chemin, err
}

func (d *Depot) courant() (*directive.Jeu, []byte) {
	brut, err := os.ReadFile(d.Chemins().Directives)
	if err != nil {
		return nil, nil
	}
	j, err := chargerOctets(brut)
	if err != nil {
		return nil, nil
	}
	return j, brut
}

// ajoutees liste les directives du candidat qui ne sont pas dans les règles
// en vigueur, avec leur relecture.
func ajoutees(cand, cour directive.Jeu) []DirAjoutee {
	connu := map[string]bool{}
	for _, dd := range cour.Directives {
		connu[dd.ID] = true
	}
	var out []DirAjoutee
	for _, dd := range cand.Directives {
		if connu[dd.ID] {
			continue
		}
		out = append(out, DirAjoutee{ID: dd.ID, Version: dd.Version, Statut: string(dd.Statut), Titre: Titre(dd.TexteFR), Texte: dd.TexteFR,
			Fondements: fondementsDe(dd), Relecture: strings.TrimSpace(relecture.Directive(dd))})
	}
	return out
}

// ---------------------------------------------------------------------------
// Essai sur les journaux existants
// ---------------------------------------------------------------------------

// Essai : combien d'appels passés auraient changé de décision avec la
// directive ajoutée (mise en vigueur), par le même rejeu que l'écran
// avant / après.
type Essai struct {
	Directives  []string    `json:"directives"`
	Compteurs   CompteursAA `json:"compteurs"`
	Lignes      []LigneAA   `json:"lignes"`
	Explication string      `json:"explication"`
	Verifiees   bool        `json:"verifiees"`
}

// activees rend les règles en vigueur augmentées des directives ajoutées,
// toutes tenues pour actives : c'est ce que serait l'activation.
func activees(cour, cand directive.Jeu) (directive.Jeu, []directive.Directive) {
	connu := map[string]bool{}
	for _, dd := range cour.Directives {
		connu[dd.ID] = true
	}
	hyp := cour
	hyp.Directives = append([]directive.Directive(nil), cour.Directives...)
	var ajout []directive.Directive
	for _, dd := range cand.Directives {
		if connu[dd.ID] {
			continue
		}
		dd.Statut = directive.Active
		hyp.Directives = append(hyp.Directives, dd)
		ajout = append(ajout, dd)
	}
	return hyp, ajout
}

// EssaiSurJournaux rejoue tous les journaux sous les règles en vigueur,
// avec et sans les directives du candidat.
func (d *Depot) EssaiSurJournaux(conversation string, etats []*Etat, tous []JeuCharge) (*Essai, error) {
	cand, _, err := d.candidat(conversation)
	if err != nil {
		return nil, refus(404, "%v", err)
	}
	base := Courant(tous)
	if base == nil {
		return nil, refus(409, "Les règles en vigueur sont illisibles.")
	}
	hyp, ajout := activees(base.Jeu, *cand)
	if len(ajout) == 0 {
		return nil, refus(409, "Le résultat de la compilation n'ajoute aucune directive.")
	}
	e := &Essai{Directives: []string{}}
	for _, a := range ajout {
		e.Directives = append(e.Directives, a.ID)
	}
	// La date d'effet retenue est la plus tardive des directives ajoutées
	// (au plus tôt maintenant : une directive n'est jamais rétroactive).
	dir := ajout[0]
	for _, a := range ajout {
		if a.DateEffet.After(dir.DateEffet) {
			dir.DateEffet = a.DateEffet
		}
	}
	cmp := Comparaison{Avec: hyp, Sans: base.Jeu, Dir: dir, Tous: tous}
	e.Lignes, e.Compteurs, e.Verifiees = cmp.Rejouer(etats, nil)
	e.Explication = explication(e.Lignes, strings.Join(e.Directives, ", "))
	return e, nil
}

// ---------------------------------------------------------------------------
// Activation, observation, retrait (contrats §4)
// ---------------------------------------------------------------------------

// transitions admises : jamais de retour en arrière silencieux.
var transitions = map[directive.Statut][]directive.Statut{
	directive.Brouillon:   {directive.Observation, directive.Active, directive.Retiree},
	directive.Observation: {directive.Active, directive.Retiree},
	directive.Active:      {directive.Retiree},
}

func transitionAdmise(de, vers directive.Statut) bool {
	for _, s := range transitions[de] {
		if s == vers {
			return true
		}
	}
	return false
}

// Resultat d'une écriture des règles.
type ResultatRegles struct {
	Version    int      `json:"version"`
	Directives []string `json:"directives"`
	Statut     string   `json:"statut"`
	Archive    string   `json:"archive"`
	Empreinte  string   `json:"empreinte"`
}

// ecrireRegles écrit la version n+1 des règles : vérification par le
// chargeur strict (« directive verifier » sans --exiger-brouillon),
// archivage de l'ancienne version dans historique/demo.v<n>.json, puis
// écriture atomique de demo.json. Rien n'est écrit si une étape échoue
// avant le renommage final.
func (d *Depot) ecrireRegles(nouveau directive.Jeu, ancien []byte, ancienVersion int) (*ResultatRegles, error) {
	nouveau.Meta.Empreinte = ""
	nouveau.Meta.Signature = ""
	brut, err := jsonIndent(nouveau)
	if err != nil {
		return nil, err
	}
	verif, err := chargerOctets(brut)
	if err != nil {
		return nil, refus(422, "Les règles ainsi modifiées sont refusées par le vérificateur : %v", err)
	}
	if verif.Meta.Version != ancienVersion+1 || verif.Meta.ID != "demo" {
		return nil, refus(422, "Les règles modifiées doivent garder l'identifiant demo et passer à la version %d.", ancienVersion+1)
	}
	ch := d.Chemins()
	arch := filepath.Join(ch.Historique, fmt.Sprintf("demo.v%d.json", ancienVersion))
	if ex, err := os.ReadFile(arch); err == nil && !bytes.Equal(ex, ancien) {
		return nil, refus(409, "L'archive de la version %d existe déjà et diffère : rien n'est écrit.", ancienVersion)
	} else if err != nil {
		if err := ecrireAtomique(arch, ancien, 0o644); err != nil {
			return nil, refus(500, "L'archivage de la version précédente a échoué : %v", err)
		}
	}
	if err := ecrireAtomique(ch.Directives, brut, 0o644); err != nil {
		return nil, refus(500, "L'écriture des règles a échoué : %v", err)
	}
	return &ResultatRegles{Version: verif.Meta.Version, Archive: filepath.Join(d.cheminsRel().Historique, filepath.Base(arch)), Empreinte: verif.Meta.Empreinte}, nil
}

// Mettre en vigueur : la directive ajoutée par le jeu candidat de la
// conversation passe en observation ou active.
func (d *Depot) MettreEnVigueur(conversation, statut string) (*ResultatRegles, error) {
	st := directive.Statut(statut)
	if st != directive.Observation && st != directive.Active {
		return nil, refus(400, "Statut %q : observation ou active seulement.", statut)
	}
	ecritureMu.Lock()
	defer ecritureMu.Unlock()
	cour, ancien := d.courant()
	if cour == nil {
		return nil, refus(409, "Les règles en vigueur sont illisibles : rien n'est écrit.")
	}
	cand, _, err := d.candidat(conversation)
	if err != nil {
		return nil, refus(404, "%v", err)
	}
	// Le candidat doit avoir été compilé sur les règles telles qu'elles sont :
	// toutes les directives en vigueur y figurent, inchangées.
	parID := map[string]directive.Directive{}
	for _, dd := range cand.Directives {
		parID[dd.ID] = dd
	}
	for _, dd := range cour.Directives {
		c, ok := parID[dd.ID]
		if !ok || !memeDirective(c, dd) {
			return nil, refus(409, "Les règles en vigueur ont changé depuis la compilation (%s) : recompilez le texte.", dd.ID)
		}
	}
	hyp, ajout := activees(*cour, *cand)
	if len(ajout) == 0 {
		return nil, refus(409, "Le résultat de la compilation n'ajoute aucune directive (déjà mise en vigueur ?).")
	}
	maintenant := time.Now().UTC().Truncate(time.Second)
	nouveau := *cour
	nouveau.Directives = append([]directive.Directive(nil), cour.Directives...)
	var ids []string
	for _, a := range ajout {
		a.Statut = st
		if a.DateEffet.Before(maintenant) {
			a.DateEffet = maintenant // jamais rétroactive
		}
		nouveau.Directives = append(nouveau.Directives, a)
		ids = append(ids, a.ID)
	}
	_ = hyp
	nouveau.Meta.Version = cour.Meta.Version + 1
	nouveau.Meta.DateEffet = maintenant
	nouveau.Meta.Auteur = d.Juriste
	res, err := d.ecrireRegles(nouveau, ancien, cour.Meta.Version)
	if err != nil {
		return nil, err
	}
	res.Directives, res.Statut = ids, statut
	d.noterOrigines(ids, conversation)
	return res, nil
}

// noterOrigines écrit demo/directives/origines.json pour les directives qui
// viennent d'une proposition de l'Analyste (contrats §6) : la décision du
// juriste dont la conversation est celle du candidat.
func (d *Depot) noterOrigines(ids []string, conversation string) {
	ents, _ := os.ReadDir(d.Chemins().Propositions)
	prop, dec := "", ""
	for _, e := range ents {
		n := e.Name()
		if !strings.HasSuffix(n, ".decision.json") {
			continue
		}
		id := strings.TrimSuffix(n, ".decision.json")
		if dp := d.LireDecision(id); dp != nil && dp.ConversationCompilateur == conversation && dp.Decision != "rejetee" {
			prop, dec = id, dp.Decision
		}
	}
	if prop == "" {
		return
	}
	orig := d.LireOrigines()
	for _, id := range ids {
		orig[id] = Origine{Proposition: prop, Decision: dec}
	}
	if b, err := jsonIndent(orig); err == nil {
		_ = ecrireAtomique(d.Chemins().Origines, b, 0o644)
	}
}

// ChangerStatut fait passer une directive des règles en vigueur à un autre
// statut (observation → active, ou retrait), en version n+1 des règles. Les
// transitions admises : brouillon → observation, active ou retirée ;
// observation → active ou retirée ; active → retirée.
func (d *Depot) ChangerStatut(id, statut string) (*ResultatRegles, error) {
	vers := directive.Statut(statut)
	if vers != directive.Observation && vers != directive.Active && vers != directive.Retiree {
		return nil, refus(400, "Statut %q inconnu.", statut)
	}
	ecritureMu.Lock()
	defer ecritureMu.Unlock()
	cour, ancien := d.courant()
	if cour == nil {
		return nil, refus(409, "Les règles en vigueur sont illisibles : rien n'est écrit.")
	}
	maintenant := time.Now().UTC().Truncate(time.Second)
	nouveau := *cour
	nouveau.Directives = append([]directive.Directive(nil), cour.Directives...)
	trouve := false
	for i := range nouveau.Directives {
		dd := &nouveau.Directives[i]
		if dd.ID != id {
			continue
		}
		trouve = true
		if !transitionAdmise(dd.Statut, vers) {
			return nil, refus(409, "La directive %s est « %s » : elle ne peut pas passer à « %s ».", id, dd.Statut, vers)
		}
		dd.Statut = vers
		if vers != directive.Retiree && dd.DateEffet.Before(maintenant) {
			dd.DateEffet = maintenant
		}
	}
	if !trouve {
		return nil, refus(404, "Directive %s inconnue.", id)
	}
	nouveau.Meta.Version = cour.Meta.Version + 1
	nouveau.Meta.DateEffet = maintenant
	nouveau.Meta.Auteur = d.Juriste
	res, err := d.ecrireRegles(nouveau, ancien, cour.Meta.Version)
	if err != nil {
		return nil, err
	}
	res.Directives, res.Statut = []string{id}, statut
	return res, nil
}

// Brouillons : candidats que le service a compilés et que le juriste n'a pas
// encore mis en vigueur, pour la reprise d'une conversation.
func (d *Depot) Conversations() []string {
	ents, _ := os.ReadDir(d.Chemins().Compilation)
	var out []string
	for _, e := range ents {
		if e.IsDir() && reIdSur.MatchString(e.Name()) {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}
