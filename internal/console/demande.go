package console

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"
)

// Écran « Demandé / décidé » : pour chaque appel, ce que le modèle a demandé
// (outil et arguments, lus dans <dossier>/mistral/*.json) à côté de ce que
// l'engine a décidé (journal). Les appels « deleguer » de l'orchestrateur ne
// passent pas par le gate : ils apparaissent comme des délégations.

// ArgVue est un argument demandé par le modèle. Un champ que l'engine a
// retiré n'est jamais rendu : Retire vaut vrai et Valeur est vide.
type ArgVue struct {
	Chemin string `json:"chemin"`
	Valeur string `json:"valeur,omitempty"`
	Retire bool   `json:"retire"`
}

// DemandeVue est une ligne : la demande du modèle et, s'il y en a une, la
// décision de l'engine, puis l'issue de la validation humaine.
type DemandeVue struct {
	Ordre      int       `json:"ordre"`
	TS         time.Time `json:"ts"`
	Outil      string    `json:"outil"`
	Delegation bool      `json:"delegation"`
	HorsGate   bool      `json:"hors_gate"`
	Arguments  []ArgVue  `json:"arguments"`
	Decision   *Etape    `json:"decision,omitempty"`
	// Intermediaires : reprises de l'appel remises en attente (jeton refusé
	// par l'engine) avant la validation qui l'a fait aboutir.
	Intermediaires []Etape `json:"intermediaires,omitempty"`
	Suite          *Etape  `json:"suite,omitempty"` // appel repris après la validation
	NonRecue       bool    `json:"non_recue"`       // demandé par le modèle, jamais vu par le gate
	SansDemande    bool    `json:"sans_demande"`    // décidé par le gate, sans demande retrouvée
}

// VueDemande est la réponse de l'API.
type VueDemande struct {
	Journal  string       `json:"journal"`
	Session  string       `json:"session"`
	Demandes []DemandeVue `json:"demandes"`
	// Fichiers : nombre de fichiers mistral/ lus ; zéro, l'écran dit que
	// les demandes du modèle ne sont pas disponibles pour cette session.
	Fichiers int `json:"fichiers"`
}

type appelModele struct {
	id      string
	nom     string
	args    map[string]any
	brut    string
	creeLe  time.Time
	ordre   int
	deleg   bool
	illisib bool
}

// lireDemandesModele lit <dossier>/mistral/*.json. Chaque fichier porte la
// requête (tout l'historique de la conversation) et la réponse : les appels
// d'outil se retrouvent donc dans plusieurs fichiers ; ils sont dédoublonnés
// par identifiant d'appel et ordonnés par la date de la réponse qui les a
// produits.
func lireDemandesModele(dossier string) ([]appelModele, int) {
	fichiers, _ := filepath.Glob(filepath.Join(dossier, "mistral", "*.json"))
	type echange struct {
		cree time.Time
		doc  struct {
			Requete struct {
				Messages []msgModele `json:"messages"`
			} `json:"requete"`
			Reponse struct {
				Created int64 `json:"created"`
				Choices []struct {
					Message msgModele `json:"message"`
				} `json:"choices"`
			} `json:"reponse"`
		}
	}
	var echanges []echange
	for _, f := range fichiers {
		brut, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var e echange
		if json.Unmarshal(brut, &e.doc) != nil {
			continue
		}
		e.cree = time.Unix(e.doc.Reponse.Created, 0).UTC()
		echanges = append(echanges, e)
	}
	sort.SliceStable(echanges, func(i, j int) bool { return echanges[i].cree.Before(echanges[j].cree) })
	var out []appelModele
	vus := map[string]bool{}
	ajouter := func(m msgModele, cree time.Time) {
		for _, tc := range m.ToolCalls {
			cle := tc.ID
			if cle == "" {
				cle = tc.Function.Name + "|" + argsBrut(tc.Function.Arguments)
			}
			if vus[cle] {
				continue
			}
			vus[cle] = true
			a := appelModele{id: tc.ID, nom: tc.Function.Name, creeLe: cree, ordre: len(out), deleg: tc.Function.Name == "deleguer"}
			a.brut = argsBrut(tc.Function.Arguments)
			if json.Unmarshal([]byte(a.brut), &a.args) != nil {
				a.illisib = true
			}
			out = append(out, a)
		}
	}
	for _, e := range echanges {
		for _, m := range e.doc.Requete.Messages {
			if m.Role == "assistant" {
				ajouter(m, e.cree) // déjà vu dans la réponse d'un fichier précédent, sinon date approchée
			}
		}
		for _, c := range e.doc.Reponse.Choices {
			ajouter(c.Message, e.cree)
		}
	}
	return out, len(echanges)
}

type msgModele struct {
	Role      string `json:"role"`
	ToolCalls []struct {
		ID       string `json:"id"`
		Function struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		} `json:"function"`
	} `json:"tool_calls"`
}

// argsBrut rend les arguments d'un appel : le modèle les envoie en chaîne JSON
// (ou, parfois, en objet).
func argsBrut(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return string(raw)
}

// aplatir rend les feuilles d'un document JSON avec leur pointeur (« /a/0/b »),
// en ordre stable. Un champ dont le pointeur, ou un préfixe, est retiré n'est
// pas rendu : seule sa présence l'est.
func aplatir(v any, chemin string, retires map[string]bool, out *[]ArgVue) {
	if retires[chemin] {
		*out = append(*out, ArgVue{Chemin: chemin, Retire: true})
		return
	}
	switch x := v.(type) {
	case map[string]any:
		cles := make([]string, 0, len(x))
		for k := range x {
			cles = append(cles, k)
		}
		sort.Strings(cles)
		for _, k := range cles {
			aplatir(x[k], chemin+"/"+strings.NewReplacer("~", "~0", "/", "~1").Replace(k), retires, out)
		}
	case []any:
		for i, e := range x {
			aplatir(e, fmt.Sprintf("%s/%d", chemin, i), retires, out)
		}
	default:
		b, _ := json.Marshal(x)
		c := chemin
		if c == "" {
			c = "/"
		}
		*out = append(*out, ArgVue{Chemin: c, Valeur: string(b)})
	}
}

// argumentsVue prépare les arguments à l'affichage ; chaque champ retiré par
// l'engine (champs_retires de l'appel) est signalé sans sa valeur, y compris
// quand le champ est absent des arguments rendus.
func argsVue(args map[string]any, retires []string) []ArgVue {
	set := map[string]bool{}
	for _, r := range retires {
		set[r] = true
	}
	out := []ArgVue{}
	aplatir(args, "", set, &out)
	vus := map[string]bool{}
	for _, a := range out {
		if a.Retire {
			vus[a.Chemin] = true
		}
	}
	for _, r := range retires {
		if !vus[r] {
			out = append(out, ArgVue{Chemin: r, Retire: true})
		}
	}
	return out
}

func memeArguments(a, b map[string]any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	var xa, ya any
	_ = json.Unmarshal(x, &xa)
	_ = json.Unmarshal(y, &ya)
	return reflect.DeepEqual(xa, ya)
}

// Demande construit l'écran « Demandé / décidé » d'une session : chaque appel
// demandé par le modèle, rapproché de la décision de l'engine.
func (d *Depot) Demande(st *Etat, jeux []JeuCharge, session string) (*VueDemande, bool) {
	ch, ok := d.Chaine(st, jeux, session)
	if !ok {
		return nil, false
	}
	v := &VueDemande{Journal: ch.Journal, Session: ch.Session, Demandes: []DemandeVue{}}
	modele, nb := lireDemandesModele(st.Journal.Dossier())
	v.Fichiers = nb

	// Les appels repris après validation sont rattachés à l'appel retenu.
	suivants := map[int]bool{}
	parSeq := map[int]*Etape{}
	for i := range ch.Etapes {
		parSeq[ch.Etapes[i].Seq] = &ch.Etapes[i]
	}
	for i := range ch.Etapes {
		if e := ch.Etapes[i]; e.Badge == BadgeAttente && e.ValideAu > 0 {
			suivants[e.ValideAu] = true
		}
	}
	var gate []*Etape
	premierEnAttente := map[string]*Etape{} // empreinte transmise → appel retenu la première fois
	inter := map[*Etape][]Etape{}
	for i := range ch.Etapes {
		e := &ch.Etapes[i]
		if suivants[e.Seq] && e.Badge != BadgeAttente {
			continue
		}
		if e.Badge == BadgeAttente && e.Empreinte != "" {
			if p := premierEnAttente[e.Empreinte]; p != nil {
				inter[p] = append(inter[p], *e) // même appel remis en attente
				continue
			}
			premierEnAttente[e.Empreinte] = e
		}
		gate = append(gate, e)
	}
	pris := map[*Etape]bool{}
	nom := func(e *Etape) string { return e.Serveur + "__" + e.Outil }
	for _, m := range modele {
		l := DemandeVue{Ordre: len(v.Demandes) + 1, TS: m.creeLe, Outil: m.nom, Arguments: []ArgVue{}}
		if m.deleg {
			l.Delegation, l.HorsGate = true, true
			l.Arguments = argsVue(m.args, nil)
			v.Demandes = append(v.Demandes, l)
			continue
		}
		// Le premier appel du gate de même outil et mêmes arguments ; à défaut
		// d'arguments au coffre, le premier de même outil.
		var trouve *Etape
		for _, e := range gate {
			if pris[e] || nom(e) != m.nom {
				continue
			}
			if args, ok := st.Coffre.Arguments(e.EmpreinteOrigine); ok && m.args != nil && !memeArguments(args, m.args) {
				continue
			}
			trouve = e
			break
		}
		if trouve == nil {
			l.NonRecue = true
			l.Arguments = argsVue(m.args, nil)
			v.Demandes = append(v.Demandes, l)
			continue
		}
		pris[trouve] = true
		cp := *trouve
		l.Decision = &cp
		l.Intermediaires = inter[trouve]
		l.Arguments = argsVue(m.args, trouve.Champs)
		if trouve.ValideAu > 0 {
			if s := parSeq[trouve.ValideAu]; s != nil {
				c := *s
				l.Suite = &c
			}
		}
		v.Demandes = append(v.Demandes, l)
	}
	for _, e := range gate {
		if pris[e] {
			continue
		}
		cp := *e
		l := DemandeVue{Ordre: len(v.Demandes) + 1, TS: e.TS, Outil: nom(e), SansDemande: true, Decision: &cp, Arguments: []ArgVue{}, Intermediaires: inter[e]}
		if args, ok := st.Coffre.Arguments(e.EmpreinteOrigine); ok {
			l.Arguments = argsVue(args, e.Champs)
		}
		if e.ValideAu > 0 {
			if s := parSeq[e.ValideAu]; s != nil {
				c := *s
				l.Suite = &c
			}
		}
		v.Demandes = append(v.Demandes, l)
	}
	return v, true
}
