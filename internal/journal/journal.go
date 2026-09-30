// Package journal est le contrat commun au rejeu, à l'agent d'analyse et
// aux écrans : une chaîne d'entrées typées, dont l'entrée appel contient
// tout ce qu'il faut pour rejouer Decide à l'identique.
//
// Ce qui est en clair : serveur, outil, effet, directives et fondements,
// attestation, instantané de session, empreintes, noms des champs retirés,
// identités professionnelles d'approbation, statut et provenance de la
// source. Ce qui n'est que sous empreinte : les arguments d'origine et
// transmis, la réponse de l'outil, le jeton. Les arguments vont dans le
// Coffre, fichier séparé et purgeable, que l'agent d'analyse n'ouvre jamais.
package journal

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"mcprogue/internal/approval"
	"mcprogue/internal/chaine"
	"mcprogue/internal/directive"
)

// Type d'une entrée. Toutes vont dans la même chaîne.
type Type string

const (
	JeuCharge       Type = "jeu_charge"
	RegistreCharge  Type = "registre_charge"
	AttestationT    Type = "attestation"
	SessionOuverte  Type = "session_ouverte"
	AppelT          Type = "appel"
	SessionFermee   Type = "session_fermee"
	AlerteT         Type = "alerte"
	IdentiteChangee Type = "identite_changee"
	// PurgeT est écrite par chaque purge du coffre : sans elle, une pierre
	// tombale n'est qu'une affirmation non chaînée.
	PurgeT Type = "purge"
)

// Statuts de résultat d'un appel. Les quatre premiers sont ceux d'une
// consultation ; non_transmis est le refus de l'engine, qui n'en est pas une.
const (
	StatutOK           = "ok"
	StatutErreur       = "erreur"
	StatutIndisponible = "indisponible"
	StatutRefuse       = "refuse"
	StatutNonTransmis  = "non_transmis"
)

type RefJeu struct {
	ID        string `json:"id"`
	Version   int    `json:"version"`
	Empreinte string `json:"empreinte"`
}

// Appel ne porte que des empreintes et des noms de champs.
type Appel struct {
	EmpreinteOrigine   string   `json:"empreinte_origine"`
	EmpreinteTransmise string   `json:"empreinte_transmise"`
	ChampsRetires      []string `json:"champs_retires,omitempty"`
}

type Attestation struct {
	Mode    string `json:"mode"`
	Attendu string `json:"attendu,omitempty"`
	Observe string `json:"observe,omitempty"`
	Schemas bool   `json:"schemas"`
	Valide  bool   `json:"valide"` // recalculée par directive.Attestation.Valide
	Detail  string `json:"detail,omitempty"`
}

type Consultation struct {
	Seq              int       `json:"seq"`
	Serveur          string    `json:"serveur"`
	Outil            string    `json:"outil"`
	TS               time.Time `json:"ts"`
	Resultat         string    `json:"resultat"`
	Empreinte        string    `json:"empreinte"`
	EmpreinteReponse string    `json:"empreinte_reponse,omitempty"`
}

type Application struct {
	ID            string              `json:"id"`
	Version       int                 `json:"version"`
	Verrou        string              `json:"verrou"`
	Fondement     directive.Fondement `json:"fondement"`
	Statut        string              `json:"statut"`
	Effet         string              `json:"effet"`
	Declenchee    bool                `json:"declenchee"`
	Appliquee     bool                `json:"appliquee"`
	Indetermine   bool                `json:"indetermine,omitempty"`
	RevisionEchue bool                `json:"revision_echue,omitempty"`
	ChampsRetires []string            `json:"champs_retires,omitempty"`
	Motif         string              `json:"motif"`
}

type Decision struct {
	Effet        string        `json:"effet"`
	Motif        string        `json:"motif"`
	Applications []Application `json:"applications"`
	Alertes      []string      `json:"alertes,omitempty"`
}

type Approbation struct {
	Mecanisme      string `json:"mecanisme"`
	ApprouvePar    string `json:"approuve_par"`
	Ticket         string `json:"ticket,omitempty"`
	Mandant        string `json:"mandant,omitempty"`
	Delegue        string `json:"delegue,omitempty"`
	EmpreinteJeton string `json:"empreinte_jeton,omitempty"`
}

// Source dit d'où la donnée vient : en direct ou du cache de rejeu, avec
// la date et l'empreinte de l'enregistrement. Le cache est une source dont
// la fraîcheur se lit ici.
type Source struct {
	Origine   string `json:"origine"` // live | rejeu
	Date      string `json:"date,omitempty"`
	Empreinte string `json:"empreinte,omitempty"`
}

type Resultat struct {
	Statut           string  `json:"statut"`
	EmpreinteReponse string  `json:"empreinte_reponse,omitempty"`
	DureeMs          int64   `json:"duree_ms,omitempty"`
	Source           *Source `json:"source,omitempty"`
}

// Entree est une ligne du journal. Les champs absents d'un type restent
// omis ; prev et mac sont posés par la chaîne.
type Entree struct {
	TS       time.Time `json:"ts"`
	Type     Type      `json:"type"`
	Session  string    `json:"session,omitempty"`
	Seq      int       `json:"seq,omitempty"`
	Appelant string    `json:"appelant,omitempty"`

	// SousAgentDeclare est l'étiquette que l'agent déclare dans
	// _meta.sous_agent d'un tools/call. Elle n'est pas prouvée et aucune
	// décision ne la lit : l'identité prouvée est Appelant, fixée par le
	// transport à l'ouverture de la session.
	SousAgentDeclare string `json:"sous_agent_declare,omitempty"`

	Jeu *RefJeu `json:"jeu,omitempty"`

	// Retour arrière d'un jeu_charge : le jeu rechargé à chaud a une
	// version inférieure à celle du jeu qu'il remplace. VersionDepart est
	// la version remplacée, VersionArrivee celle chargée (égale à
	// Jeu.Version).
	RetourArriere  bool `json:"retour_arriere,omitempty"`
	VersionDepart  int  `json:"version_depart,omitempty"`
	VersionArrivee int  `json:"version_arrivee,omitempty"`

	Serveur     string `json:"serveur,omitempty"`
	OutilExpose string `json:"outil_expose,omitempty"`
	Outil       string `json:"outil,omitempty"`

	Appel       *Appel         `json:"appel,omitempty"`
	Attestation *Attestation   `json:"attestation,omitempty"`
	EtatSession []Consultation `json:"etat_session,omitempty"`
	Decision    *Decision      `json:"decision,omitempty"`
	Approbation *Approbation   `json:"approbation,omitempty"`
	Resultat    *Resultat      `json:"resultat,omitempty"`

	Message         string `json:"message,omitempty"`
	NbConsultations int    `json:"nb_consultations,omitempty"`
	DernierSeq      int    `json:"dernier_seq,omitempty"`

	// Purge : empreintes d'origine purgées, motif, auteur ; la date est TS.
	Empreintes []string   `json:"empreintes,omitempty"`
	Motif      string     `json:"motif,omitempty"`
	Auteur     string     `json:"auteur,omitempty"`
	PurgeAvant *time.Time `json:"purge_avant,omitempty"`

	Prev string `json:"prev,omitempty"`
	MAC  string `json:"mac,omitempty"`
}

// ---------------------------------------------------------------------------
// Conversions avec les types de directive, qui n'ont pas d'étiquettes JSON.
// ---------------------------------------------------------------------------

func DepuisDecision(d directive.Decision) *Decision {
	out := &Decision{Effet: string(d.Effet), Motif: d.Motif, Alertes: d.Alertes, Applications: []Application{}}
	for _, a := range d.Applications {
		out.Applications = append(out.Applications, Application{
			ID: a.ID, Version: a.Version, Verrou: string(a.Verrou), Fondement: a.Fondement,
			Statut: string(a.Statut), Effet: string(a.Effet), Declenchee: a.Declenchee, Appliquee: a.Appliquee,
			Indetermine: a.Indetermine, RevisionEchue: a.RevisionEchue, ChampsRetires: a.ChampsRetires, Motif: a.Motif,
		})
	}
	return out
}

// VersDirective rend ce qu'une comparaison a besoin de relire : effet,
// motif, applications, alertes. Les arguments ne sont pas dans le journal.
func (d *Decision) VersDirective() directive.Decision {
	out := directive.Decision{Effet: directive.Effet(d.Effet), Motif: d.Motif, Alertes: d.Alertes}
	for _, a := range d.Applications {
		out.Applications = append(out.Applications, directive.Application{
			ID: a.ID, Version: a.Version, Verrou: directive.Verrou(a.Verrou), Fondement: a.Fondement,
			Statut: directive.Statut(a.Statut), Effet: directive.Effet(a.Effet), Declenchee: a.Declenchee,
			Appliquee: a.Appliquee, Indetermine: a.Indetermine, RevisionEchue: a.RevisionEchue,
			ChampsRetires: a.ChampsRetires, Motif: a.Motif,
		})
	}
	return out
}

func DepuisAttestation(a directive.Attestation) *Attestation {
	return &Attestation{Mode: a.Mode, Attendu: a.Attendu, Observe: a.Observe, Schemas: a.Schemas, Valide: a.Valide(), Detail: a.Detail}
}

func (a *Attestation) VersDirective() directive.Attestation {
	if a == nil {
		return directive.Attestation{}
	}
	return directive.Attestation{Mode: a.Mode, Attendu: a.Attendu, Observe: a.Observe, Schemas: a.Schemas, Verifiee: a.Valide, Detail: a.Detail}
}

func DepuisConsultations(cs []directive.Consultation) []Consultation {
	out := make([]Consultation, 0, len(cs))
	for _, c := range cs {
		out = append(out, Consultation{Seq: c.Seq, Serveur: c.Serveur, Outil: c.Outil, TS: c.Horodatage,
			Resultat: string(c.Resultat), Empreinte: c.Empreinte, EmpreinteReponse: c.EmpreinteReponse})
	}
	return out
}

func VersConsultations(cs []Consultation) []directive.Consultation {
	out := make([]directive.Consultation, 0, len(cs))
	for _, c := range cs {
		out = append(out, directive.Consultation{Seq: c.Seq, Serveur: c.Serveur, Outil: c.Outil, Horodatage: c.TS,
			Resultat: directive.Resultat(c.Resultat), Empreinte: c.Empreinte, EmpreinteReponse: c.EmpreinteReponse})
	}
	return out
}

func DepuisJeu(j directive.Jeu) *RefJeu {
	return &RefJeu{ID: j.Meta.ID, Version: j.Meta.Version, Empreinte: j.Meta.Empreinte}
}

// ---------------------------------------------------------------------------
// Écrivain : chaîne sur disque plus abonnés en mémoire.
// ---------------------------------------------------------------------------

// Ecrivain écrit les entrées dans la chaîne et les publie aux abonnés. Un
// *Ecrivain nul n'écrit rien et ne publie rien. La publication ne bloque
// jamais l'engine : un abonné trop lent perd des entrées, et le compte des
// pertes est lisible.
type Ecrivain struct {
	chaine  *chaine.Chaine
	fichier *os.File
	horloge func() time.Time

	mu      sync.Mutex
	abonnes map[int]chan Entree
	suivant int
	perdues atomic.Int64
}

// Ouvrir ouvre ou crée le fichier du journal et reprend la chaîne.
func Ouvrir(chemin string, cle []byte) (*Ecrivain, error) {
	c, f, err := chaine.Ouvrir(chemin, cle)
	if err != nil {
		return nil, fmt.Errorf("journal: %w", err)
	}
	e := Nouveau(c)
	e.fichier = f
	return e, nil
}

// Nouveau écrit dans une chaîne déjà construite (tests, flux en mémoire).
func Nouveau(c *chaine.Chaine) *Ecrivain {
	return &Ecrivain{chaine: c, horloge: time.Now, abonnes: map[int]chan Entree{}}
}

// Horloge remplace l'horloge, pour les tests.
func (e *Ecrivain) Horloge(h func() time.Time) { e.horloge = h }

// Ecrire pose l'horodatage s'il manque, scelle, écrit, publie, et rend
// l'entrée telle qu'écrite (prev et mac remplis).
func (e *Ecrivain) Ecrire(en Entree) (Entree, error) {
	if e == nil {
		return en, nil
	}
	if en.TS.IsZero() {
		en.TS = e.horloge().UTC()
	}
	if en.Type == "" {
		return en, fmt.Errorf("journal: entrée sans type")
	}
	en.Prev, en.MAC = "", ""
	ligne, err := e.chaine.Ajouter(en)
	if err != nil {
		return en, fmt.Errorf("journal: %w", err)
	}
	if ligne != nil {
		var ecrite Entree
		if err := json.Unmarshal(ligne, &ecrite); err != nil {
			return en, fmt.Errorf("journal: relire l'entrée écrite: %w", err)
		}
		en = ecrite
	}
	e.publier(en)
	return en, nil
}

func (e *Ecrivain) publier(en Entree) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, ch := range e.abonnes {
		select {
		case ch <- en:
		default:
			e.perdues.Add(1)
		}
	}
}

// Abonner rend un canal recevant chaque entrée écrite, avec un tampon de
// taille donnée, et la fonction de désabonnement. C'est le point d'accroche
// des écrans temps réel ; aucun écran n'existe encore.
func (e *Ecrivain) Abonner(taille int) (<-chan Entree, func()) {
	if e == nil {
		ch := make(chan Entree)
		return ch, func() {}
	}
	if taille < 1 {
		taille = 1
	}
	ch := make(chan Entree, taille)
	e.mu.Lock()
	id := e.suivant
	e.suivant++
	e.abonnes[id] = ch
	e.mu.Unlock()
	var une sync.Once
	return ch, func() {
		une.Do(func() {
			e.mu.Lock()
			delete(e.abonnes, id)
			e.mu.Unlock()
			close(ch)
		})
	}
}

// Perdues compte les entrées qu'un abonné saturé n'a pas reçues.
func (e *Ecrivain) Perdues() int64 {
	if e == nil {
		return 0
	}
	return e.perdues.Load()
}

// Fermer ferme le fichier s'il y en a un.
func (e *Ecrivain) Fermer() error {
	if e == nil || e.fichier == nil {
		return nil
	}
	return e.fichier.Close()
}

// Lire relit un journal en entrées typées, sans vérifier la chaîne :
// Verifier s'en charge.
func Lire(r io.Reader) ([]Entree, error) {
	var out []Entree
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), chaine.LigneMax)
	for n := 1; sc.Scan(); n++ {
		ligne := bytes.TrimSpace(sc.Bytes())
		if len(ligne) == 0 {
			continue
		}
		var en Entree
		if err := json.Unmarshal(ligne, &en); err != nil {
			return out, fmt.Errorf("journal: ligne %d: %w", n, err)
		}
		out = append(out, en)
	}
	return out, sc.Err()
}

// Verifier contrôle la chaîne du journal.
func Verifier(r io.Reader, cle []byte) (int, error) { return chaine.Verifier(r, cle) }

// ---------------------------------------------------------------------------
// Coffre : les arguments, à part.
// ---------------------------------------------------------------------------

// Depot est ce que le coffre garde pour une empreinte d'origine. Les
// arguments transmis sont ceux après caviardage, quand il y en a eu un.
type Depot struct {
	TS                time.Time      `json:"ts"`
	Empreinte         string         `json:"empreinte"`
	Outil             string         `json:"outil"`
	ArgumentsOrigine  map[string]any `json:"arguments_origine"`
	ArgumentsTransmis map[string]any `json:"arguments_transmis,omitempty"`
	// PurgeLe marque une pierre tombale : les arguments ont été effacés à
	// cette date, l'empreinte reste pour que le rejeu nomme la purge.
	PurgeLe *time.Time `json:"purge_le,omitempty"`
}

// Coffre est un fichier JSONL à permissions restreintes, indexé en mémoire
// par empreinte. Il n'est pas chaîné : son intégrité se prouve par les
// empreintes que le journal, lui, chaîne.
type Coffre struct {
	mu      sync.Mutex
	chemin  string
	fichier *os.File
	index   map[string]Depot
	purgees map[string]time.Time
	horloge func() time.Time
	journal *Ecrivain
}

// OuvrirCoffre ouvre ou crée le coffre et charge son index.
func OuvrirCoffre(chemin string) (*Coffre, error) {
	f, err := os.OpenFile(chemin, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("coffre: %w", err)
	}
	c := &Coffre{chemin: chemin, fichier: f, index: map[string]Depot{}, purgees: map[string]time.Time{}, horloge: time.Now}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 16<<20)
	for n := 1; sc.Scan(); n++ {
		ligne := bytes.TrimSpace(sc.Bytes())
		if len(ligne) == 0 {
			continue
		}
		var d Depot
		if err := json.Unmarshal(ligne, &d); err != nil {
			f.Close()
			return nil, fmt.Errorf("coffre: ligne %d illisible: %w", n, err)
		}
		if d.PurgeLe != nil {
			delete(c.index, d.Empreinte)
			c.purgees[d.Empreinte] = *d.PurgeLe
			continue
		}
		delete(c.purgees, d.Empreinte)
		c.index[d.Empreinte] = d
	}
	if err := sc.Err(); err != nil {
		f.Close()
		return nil, fmt.Errorf("coffre: %w", err)
	}
	return c, nil
}

// Deposer enregistre les arguments d'un appel. L'empreinte doit être celle
// des arguments d'origine pour l'outil donné, sinon le dépôt est refusé :
// un coffre qui accepterait n'importe quoi sous n'importe quelle empreinte
// ne prouverait rien au rejeu. Un dépôt déjà présent est ignoré.
func (c *Coffre) Deposer(d Depot) error {
	if c == nil {
		return nil
	}
	if d.Empreinte == "" || d.Outil == "" {
		return fmt.Errorf("coffre: dépôt sans empreinte ou sans outil")
	}
	if got := approval.Digest(d.Outil, d.ArgumentsOrigine); got != d.Empreinte {
		return fmt.Errorf("coffre: les arguments ne hachent pas vers l'empreinte déclarée")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, deja := c.index[d.Empreinte]; deja {
		return nil
	}
	d.PurgeLe = nil
	if d.TS.IsZero() {
		d.TS = c.horloge().UTC()
	}
	ligne, err := json.Marshal(d)
	if err != nil {
		return fmt.Errorf("coffre: encoder: %w", err)
	}
	if _, err := c.fichier.Write(append(ligne, '\n')); err != nil {
		return fmt.Errorf("coffre: écrire: %w", err)
	}
	c.index[d.Empreinte] = d
	// Un nouvel appel aux mêmes arguments les redépose : l'empreinte redevient
	// rejouable, et la pierre tombale est écrasée à la prochaine purge.
	delete(c.purgees, d.Empreinte)
	return nil
}

// Arguments rend les arguments d'origine d'une empreinte. C'est l'interface
// que le rejeu attend d'un résolveur.
func (c *Coffre) Arguments(empreinte string) (map[string]any, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	d, ok := c.index[empreinte]
	if !ok {
		return nil, false
	}
	return d.ArgumentsOrigine, true
}

func (c *Coffre) Fermer() error {
	if c == nil {
		return nil
	}
	return c.fichier.Close()
}
