package console

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"mcprogue/internal/directive"
	"mcprogue/internal/journal"
	"mcprogue/internal/rejeu"
)

// Badges de la frise, dans la langue de l'écran.
const (
	BadgeAutorise   = "autorise"
	BadgeCaviarde   = "caviarde"
	BadgeAttente    = "en_attente"
	BadgeRefuse     = "refuse"
	BadgeIndispo    = "indisponible"
	BadgeNonAttesta = "source_non_attestee"
)

var libelleBadge = map[string]string{
	BadgeAutorise:   "Autorisé",
	BadgeCaviarde:   "Caviardé",
	BadgeAttente:    "En attente de validation",
	BadgeRefuse:     "Refusé",
	BadgeIndispo:    "Source indisponible",
	BadgeNonAttesta: "Source non attestée",
}

// Lien relie une décision à une consultation antérieure qui a fondé une
// provenance.
type Lien struct {
	Seq       int    `json:"seq"`
	Serveur   string `json:"serveur"`
	Outil     string `json:"outil"`
	Resultat  string `json:"resultat"`
	Directive string `json:"directive"`
}

// AppVue est une directive telle que la décision la cite.
type AppVue struct {
	ID        string              `json:"id"`
	Version   int                 `json:"version"`
	Fondement directive.Fondement `json:"fondement"`
	Texte     string              `json:"texte,omitempty"` // texte du juriste, si le jeu est connu
	Effet     string              `json:"effet"`
	Declench  bool                `json:"declenchee"`
	Appliquee bool                `json:"appliquee"`
	Resultat  string              `json:"resultat,omitempty"`
	Motif     string              `json:"motif"`
}

// Etape est un appel de la session, de bout en bout.
type Etape struct {
	Index       int                  `json:"index"`  // position de l'entrée dans le journal
	Numero      int                  `json:"numero"` // numéro de l'entrée (Index + 1)
	Prev        string               `json:"prev,omitempty"`
	Acces       string               `json:"acces,omitempty"`
	Empreinte   string               `json:"empreinte_transmise,omitempty"`
	Evaluees    int                  `json:"evaluees"`
	Appliquees  int                  `json:"appliquees"`
	Indeterm    int                  `json:"indeterminees"`
	Seq         int                  `json:"seq"`
	TS          time.Time            `json:"ts"`
	SousAgent   string               `json:"sous_agent,omitempty"`
	Serveur     string               `json:"serveur"`
	Outil       string               `json:"outil"`
	Attestation *journal.Attestation `json:"attestation,omitempty"`
	Effet       string               `json:"effet"`
	Origine     string               `json:"origine,omitempty"`
	Motif       string               `json:"motif"`
	Badge       string               `json:"badge"`
	Libelle     string               `json:"libelle"`
	Apps        []AppVue             `json:"applications"`
	Statut      string               `json:"statut"`
	Transmis    bool                 `json:"transmis"`
	Source      *journal.Source      `json:"source,omitempty"`
	DureeMs     int64                `json:"duree_ms,omitempty"`
	Champs      []string             `json:"champs_retires,omitempty"`
	Liens       []Lien               `json:"provenance,omitempty"`
	Validation  *journal.Validation  `json:"validation,omitempty"`
	Alertes     []string             `json:"alertes,omitempty"`
	Suspect     bool                 `json:"suspect,omitempty"` // après le premier écart de chaîne
}

// ServeurVue est un serveur rencontré, avec son attestation.
type ServeurVue struct {
	Nom         string               `json:"nom"`
	Attestation *journal.Attestation `json:"attestation,omitempty"`
}

// SessionResume est une ligne de la liste des sessions.
type SessionResume struct {
	ID       string    `json:"id"`
	Appelant string    `json:"appelant"`
	Ouverte  time.Time `json:"ouverte"`
	Fermee   bool      `json:"fermee"`
	Appels   int       `json:"appels"`
}

// VueChaine est l'écran 1 : la chaîne d'une session.
type VueChaine struct {
	Journal    string             `json:"journal"`
	Session    string             `json:"session"`
	Appelant   string             `json:"appelant"`
	Demandeur  *journal.Demandeur `json:"demandeur,omitempty"`
	Ouverte    time.Time          `json:"ouverte"`
	Fermee     bool               `json:"fermee"`
	Fermeture  string             `json:"fermeture,omitempty"`
	Jeu        *journal.RefJeu    `json:"-"`
	JeuConnu   bool               `json:"-"`
	Regles     *Regles            `json:"regles,omitempty"` // règles en vigueur ; absent si non disponibles
	SousAgents []string           `json:"sous_agents"`
	Serveurs   []ServeurVue       `json:"serveurs"`
	Etapes     []Etape            `json:"etapes"`
	Verif      Verification       `json:"verification"`
	Ecart      int                `json:"premier_ecart"`
	Signature  string             `json:"signature"` // change quand le journal change
}

// Regles décrit les règles en vigueur pour l'écran : depuis quand, et par qui.
type Regles struct {
	Depuis time.Time `json:"depuis"`
	Auteur string    `json:"auteur"`
}

func reglesDe(j *directive.Jeu) *Regles {
	if j == nil {
		return nil
	}
	return &Regles{Depuis: j.Meta.DateEffet, Auteur: j.Meta.Auteur}
}

// Titre est l'intitulé court d'une directive : son texte, coupé à la
// première phrase ou au mot qui précède 60 caractères. Le schéma n'a pas de
// titre ; le texte du juriste n'est jamais réécrit, seulement abrégé ici.
func Titre(texte string) string {
	t := strings.TrimSpace(texte)
	if i := strings.IndexAny(t, ".;:"); i > 0 {
		t = t[:i]
	}
	r := []rune(t)
	if len(r) > 60 {
		r = r[:60]
		if i := strings.LastIndex(string(r), " "); i > 20 {
			return string(r)[:i] + "…"
		}
		return string(r) + "…"
	}
	return string(r)
}

// Sessions liste les sessions ouvertes d'un journal.
func Sessions(st *Etat) []SessionResume {
	var out []SessionResume
	idx := map[string]int{}
	for _, e := range st.Entrees {
		switch e.Type {
		case journal.SessionOuverte:
			idx[e.Session] = len(out)
			out = append(out, SessionResume{ID: e.Session, Appelant: e.Appelant, Ouverte: e.TS})
		case journal.AppelT:
			if i, ok := idx[e.Session]; ok {
				out[i].Appels++
			}
		case journal.SessionFermee:
			if i, ok := idx[e.Session]; ok {
				out[i].Fermee = true
			}
		}
	}
	return out
}

func trouverJeu(jeux []JeuCharge, ref *journal.RefJeu) (*directive.Jeu, bool) {
	if ref == nil {
		return nil, false
	}
	for i := range jeux {
		m := jeux[i].Jeu.Meta
		if m.ID == ref.ID && m.Version == ref.Version && m.Empreinte == ref.Empreinte {
			return &jeux[i].Jeu, true
		}
	}
	return nil, false
}

func directiveDu(j *directive.Jeu, id string, version int) *directive.Directive {
	if j == nil {
		return nil
	}
	for i := range j.Directives {
		if j.Directives[i].ID == id && j.Directives[i].Version == version {
			return &j.Directives[i]
		}
	}
	return nil
}

// badge déduit le badge d'un appel et son libellé.
func badge(e journal.Entree) string {
	d := e.Decision
	if d == nil {
		return BadgeIndispo
	}
	statut := ""
	if e.Resultat != nil {
		statut = e.Resultat.Statut
	}
	switch directive.Effet(d.Effet) {
	case directive.Refuser:
		if d.Origine == "registre" || (e.Attestation != nil && !e.Attestation.Valide) {
			return BadgeNonAttesta
		}
		for _, a := range d.Applications {
			if a.Verrou == string(directive.SourceAttestee) && a.Appliquee {
				return BadgeNonAttesta
			}
		}
		return BadgeRefuse
	case directive.AttendreValidation:
		return BadgeAttente
	}
	if statut == journal.StatutIndisponible || statut == journal.StatutErreur {
		return BadgeIndispo
	}
	if directive.Effet(d.Effet) == directive.Caviarder {
		return BadgeCaviarde
	}
	return BadgeAutorise
}

// criteres extrait d'une directive ce qui compte comme « consulté ».
type critereConsultation struct {
	serveur, outil, etiquette string
	critere                   directive.Critere
	tout                      bool
}

func criteresProvenance(d *directive.Directive, verrou string) []critereConsultation {
	var out []critereConsultation
	if d != nil {
		if p := d.Provenance; p != nil {
			c := p.Critere
			if c == "" {
				c = directive.DonneesRecues
			}
			for _, s := range p.Serveurs {
				out = append(out, critereConsultation{serveur: s, critere: c})
			}
			for _, o := range p.Outils {
				out = append(out, critereConsultation{outil: o, critere: c})
			}
		}
		if d.Arbre != nil {
			parcourir(d.Arbre, func(n *directive.Noeud) {
				if f := n.AConsulte; f != nil {
					out = append(out, critereConsultation{serveur: f.Serveur, outil: f.Outil, etiquette: f.Etiquette, critere: f.Critere})
				}
			})
		}
	}
	if len(out) == 0 && verrou == string(directive.ProvenanceSession) {
		out = append(out, critereConsultation{tout: true, critere: directive.DonneesRecues})
	}
	return out
}

func parcourir(n *directive.Noeud, f func(*directive.Noeud)) {
	if n == nil {
		return
	}
	f(n)
	for _, e := range n.Et {
		parcourir(e, f)
	}
	for _, e := range n.Ou {
		parcourir(e, f)
	}
	parcourir(n.Non, f)
	if n.Existe != nil {
		parcourir(n.Existe.Condition, f)
	}
	if n.Tous != nil {
		parcourir(n.Tous.Condition, f)
	}
}

func correspond(c critereConsultation, k journal.Consultation) bool {
	if c.critere == directive.DonneesRecues && k.Resultat != journal.StatutOK {
		return false
	}
	switch {
	case c.tout:
		return true
	case c.serveur != "":
		return k.Serveur == c.serveur
	case c.outil != "":
		return k.Serveur+"/"+k.Outil == c.outil
	case c.etiquette != "":
		for _, e := range k.Etiquettes {
			if e == c.etiquette {
				return true
			}
		}
	}
	return false
}

// liens relie les applications déclenchées aux consultations qui les ont
// fondées : l'instantané de session que l'entrée porte est celui que Decide
// a lu.
func liens(e journal.Entree, jeu *directive.Jeu) []Lien {
	var out []Lien
	vus := map[[2]any]bool{}
	for _, a := range e.Decision.Applications {
		if !a.Declenchee {
			continue
		}
		cs := criteresProvenance(directiveDu(jeu, a.ID, a.Version), a.Verrou)
		for _, k := range e.EtatSession {
			for _, c := range cs {
				if correspond(c, k) && !vus[[2]any{a.ID, k.Seq}] {
					vus[[2]any{a.ID, k.Seq}] = true
					out = append(out, Lien{Seq: k.Seq, Serveur: k.Serveur, Outil: k.Outil, Resultat: k.Resultat, Directive: a.ID})
				}
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Seq < out[j].Seq })
	return out
}

// Chaine construit l'écran 1 pour une session.
func Chaine(st *Etat, jeux []JeuCharge, session string) (*VueChaine, bool) {
	v := &VueChaine{Journal: st.Journal.ID, Session: session, Verif: st.Verif, Ecart: st.Verif.PremierEcart(),
		SousAgents: []string{}, Serveurs: []ServeurVue{}, Etapes: []Etape{}}
	trouvee := false
	transmis := map[int]bool{}
	attestations := map[string]*journal.Attestation{}
	ordre := []string{}
	note := func(s string, a *journal.Attestation) {
		if _, ok := attestations[s]; !ok {
			ordre = append(ordre, s)
		}
		if a != nil || attestations[s] == nil {
			attestations[s] = a
		}
	}
	for _, e := range st.Entrees {
		if e.Type == journal.AttestationT && e.Serveur != "" {
			note(e.Serveur, e.Attestation)
		}
		if e.Session == session && e.Type == journal.TransmissionT {
			transmis[e.Seq] = true
		}
	}
	vusSA := map[string]bool{}
	for i, e := range st.Entrees {
		if e.Session != session {
			continue
		}
		switch e.Type {
		case journal.SessionOuverte:
			trouvee = true
			v.Appelant, v.Ouverte = e.Appelant, e.TS
			v.Demandeur = e.Demandeur
		case journal.SessionFermee:
			v.Fermee, v.Fermeture = true, e.Message
		case journal.AppelT:
			trouvee = true
			if e.Decision == nil {
				continue
			}
			if v.Jeu == nil && e.Jeu != nil {
				v.Jeu = e.Jeu
				j, ok := trouverJeu(jeux, e.Jeu)
				v.JeuConnu = ok
				v.Regles = reglesDe(j)
			}
			if v.Demandeur == nil && e.Demandeur != nil {
				v.Demandeur = e.Demandeur
			}
			jeu, _ := trouverJeu(jeux, e.Jeu)
			b := badge(e)
			et := Etape{Index: i, Numero: i + 1, Prev: e.Prev, Acces: e.Acces, Seq: e.Seq, TS: e.TS, SousAgent: e.SousAgentDeclare, Serveur: e.Serveur, Outil: e.Outil,
				Attestation: e.Attestation, Effet: e.Decision.Effet, Origine: e.Decision.Origine, Motif: e.Decision.Motif,
				Badge: b, Libelle: libelleBadge[b], Transmis: transmis[e.Seq], Alertes: e.Decision.Alertes,
				Validation: e.Decision.Validation, Apps: []AppVue{},
				Suspect: v.Ecart >= 0 && i >= v.Ecart}
			if e.Resultat != nil {
				et.Statut, et.Source, et.DureeMs = e.Resultat.Statut, e.Resultat.Source, e.Resultat.DureeMs
			}
			if e.Appel != nil {
				et.Champs = e.Appel.ChampsRetires
				et.Empreinte = e.Appel.EmpreinteTransmise
			}
			for _, a := range e.Decision.Applications {
				et.Evaluees++
				if a.Appliquee {
					et.Appliquees++
				}
				if a.Indetermine {
					et.Indeterm++
				}
				av := AppVue{ID: a.ID, Version: a.Version, Fondement: a.Fondement, Effet: a.Effet, Declench: a.Declenchee,
					Appliquee: a.Appliquee, Resultat: a.Resultat, Motif: a.Motif}
				if len(a.Fondements) > 0 {
					av.Fondement = a.Fondements[0]
				}
				if d := directiveDu(jeu, a.ID, a.Version); d != nil {
					av.Texte = d.TexteFR
				}
				et.Apps = append(et.Apps, av)
			}
			et.Liens = liens(e, jeu)
			v.Etapes = append(v.Etapes, et)
			if e.SousAgentDeclare != "" && !vusSA[e.SousAgentDeclare] {
				vusSA[e.SousAgentDeclare] = true
				v.SousAgents = append(v.SousAgents, e.SousAgentDeclare)
			}
			if e.Attestation != nil {
				note(e.Serveur, e.Attestation)
			} else {
				note(e.Serveur, nil)
			}
		}
	}
	if !trouvee {
		return nil, false
	}
	for _, s := range ordre {
		v.Serveurs = append(v.Serveurs, ServeurVue{Nom: s, Attestation: attestations[s]})
	}
	if n := len(st.Entrees); n > 0 {
		v.Signature = fmt.Sprintf("%d:%s", n, st.Entrees[n-1].MAC)
	}
	return v, true
}

// ---------------------------------------------------------------------------
// Écran 2 : poste conseiller
// ---------------------------------------------------------------------------

// Cout est le coût de la règle pour une session.
type Cout struct {
	Appels            int     `json:"appels"`
	Refuses           int     `json:"refuses"`
	Caviardes         int     `json:"caviardes"`
	Validations       int     `json:"validations"`
	ChampsRetires     int     `json:"champs_retires"`
	NonRejoues        int     `json:"non_rejoues"`
	EvaluationMoyenne float64 `json:"evaluation_moyenne_ms"` // temps de Decide, mesuré au rejeu
	EvaluationMax     float64 `json:"evaluation_max_ms"`
	ServeursMs        int64   `json:"serveurs_ms"` // durée des consultations, hors règle
}

// VuePoste est l'écran 2.
type VuePoste struct {
	*VueChaine
	Reponse Reponse `json:"reponse"`
	Cout    Cout    `json:"cout"`
}

// Poste construit l'écran 2. Le temps ajouté par la règle n'est pas
// journalisé : il est mesuré en rejouant chaque décision (Decide est pure),
// ce qui en donne l'ordre de grandeur, pas la mesure de l'appel d'origine.
func Poste(st *Etat, jeux []JeuCharge, session string) (*VuePoste, bool) {
	c, ok := Chaine(st, jeux, session)
	if !ok {
		return nil, false
	}
	p := &VuePoste{VueChaine: c}
	var somme, max time.Duration
	var n int
	for _, e := range c.Etapes {
		p.Cout.Appels++
		switch e.Badge {
		case BadgeRefuse, BadgeNonAttesta:
			p.Cout.Refuses++
		case BadgeCaviarde:
			p.Cout.Caviardes++
		case BadgeAttente:
			p.Cout.Validations++
		}
		p.Cout.ServeursMs += e.DureeMs
		p.Cout.ChampsRetires += len(e.Champs)
	}
	cat := rejeu.Catalogue{}
	for _, j := range jeux {
		cat = append(cat, j.Jeu)
	}
	purges := rejeu.PurgesChainees(st.Entrees)
	for _, en := range st.Entrees {
		if en.Type != journal.AppelT || en.Session != session || en.Jeu == nil {
			continue
		}
		t0 := time.Now()
		r, err := rejeu.Rejouer(en, st.Coffre, cat, purges, rejeu.Options{})
		d := time.Since(t0)
		if err != nil || !r.Rejouable {
			p.Cout.NonRejoues++
			continue
		}
		somme += d
		if d > max {
			max = d
		}
		n++
	}
	if n > 0 {
		p.Cout.EvaluationMoyenne = float64(somme) / float64(n) / float64(time.Millisecond)
		p.Cout.EvaluationMax = float64(max) / float64(time.Millisecond)
	}
	p.Reponse = reponseFinale(c)
	return p, true
}

// Reponse est la réponse au conseiller. Le texte écrit par l'agent n'est pas
// au journal (seule l'empreinte de la réponse d'un serveur l'est) : la
// console donne ce que l'engine a décidé, jamais un texte inventé.
type Reponse struct {
	Badge       string   `json:"badge"`
	Titre       string   `json:"titre"`
	Paragraphes []string `json:"paragraphes"`
}

// reponseFinale résume ce que l'agent a reçu au dernier appel de la session.
func reponseFinale(c *VueChaine) Reponse {
	if len(c.Etapes) == 0 {
		return Reponse{Badge: BadgeIndispo, Titre: "Aucun appel dans cette session.", Paragraphes: []string{}}
	}
	e := c.Etapes[len(c.Etapes)-1]
	r := Reponse{Badge: e.Badge, Paragraphes: []string{}}
	switch e.Badge {
	case BadgeRefuse:
		r.Titre = fmt.Sprintf("Demande refusée : l'appel %s n'a pas été transmis.", e.Outil)
	case BadgeNonAttesta:
		r.Titre = fmt.Sprintf("Source non attestée : l'appel %s a été refusé.", e.Outil)
	case BadgeAttente:
		r.Titre = fmt.Sprintf("Décision suspendue : une validation humaine est requise avant la transmission de l'appel %s.", e.Outil)
	case BadgeIndispo:
		r.Titre = fmt.Sprintf("Source indisponible : aucune donnée n'a été obtenue pour l'appel %s.", e.Outil)
	case BadgeCaviarde:
		r.Titre = fmt.Sprintf("Appel %s transmis après retrait de %d champ(s).", e.Outil, len(e.Champs))
	default:
		r.Titre = fmt.Sprintf("Appel %s autorisé : la réponse du serveur a été remise à l'agent.", e.Outil)
	}
	r.Paragraphes = append(r.Paragraphes, "Motif de l'engine : "+e.Motif)
	return r
}
