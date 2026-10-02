package console

import (
	"sort"
	"strings"
	"time"

	"mcprogue/internal/directive"
	"mcprogue/internal/journal"
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
	Titre     string              `json:"titre,omitempty"`
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

	// Ce que la décision porte au journal et que l'écran montre.
	OrigineLibelle string          `json:"origine_libelle,omitempty"`
	Etiquettes     []string        `json:"etiquettes,omitempty"`
	Notifications  []NotifVue      `json:"notifications,omitempty"`
	Approbations   []ApprobVue     `json:"approbations,omitempty"`
	DureeUs        int64           `json:"duree_us"`                // 0 : non mesurée (ancien journal)
	ValidePar      []Valideur      `json:"valide_par,omitempty"`    // approbation acceptée, appel transmis
	ValideAu       int             `json:"valide_au_seq,omitempty"` // appel en attente : n° de l'appel repris
	Sujets         []journal.Sujet `json:"sujets,omitempty"`
	NonResolus     int             `json:"sujets_non_resolus,omitempty"`
}

// NotifVue est une notification demandée par une décision, avec le sort
// que le journal consigne pour sa remise.
type NotifVue struct {
	Directive string `json:"directive"`
	Version   int    `json:"version"`
	Role      string `json:"destinataire_role"`
	Rythme    string `json:"rythme"`
	SiEchec   string `json:"si_echec"`
	Statut    string `json:"statut,omitempty"` // remise | echec | planifiee ; vide : non consignée
	Canal     string `json:"canal,omitempty"`
	Remis     int    `json:"remis,omitempty"`
}

// ApprobVue est une entrée approbation du journal : acceptée ou refusée.
type ApprobVue struct {
	TS         time.Time          `json:"ts"`
	Statut     string             `json:"statut"`
	Motif      string             `json:"motif,omitempty"`
	Roles      []string           `json:"roles,omitempty"`
	QuatreYeux bool               `json:"quatre_yeux,omitempty"`
	Valideurs  []journal.Valideur `json:"valideurs,omitempty"`
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
	Journal   string             `json:"journal"`
	Session   string             `json:"session"`
	Appelant  string             `json:"appelant"`
	Demandeur *journal.Demandeur `json:"demandeur,omitempty"`
	// DemandeurFiche : nom, groupes et rôles d'après l'annuaire signé ;
	// SourceIdentite : d'où l'identité a été prouvée (journal).
	DemandeurFiche *FicheDemandeur      `json:"demandeur_fiche,omitempty"`
	SourceIdentite string               `json:"source_identite,omitempty"`
	AnnuaireRef    *journal.RefAnnuaire `json:"annuaire,omitempty"`
	Message        string               `json:"message,omitempty"`
	Demande        string               `json:"demande,omitempty"` // etat.json
	StatutSession  string               `json:"statut_session,omitempty"`
	Notifications  []NotifVue           `json:"notifications,omitempty"` // toutes celles de la session
	Ouverte        time.Time            `json:"ouverte"`
	Fermee         bool                 `json:"fermee"`
	Fermeture      string               `json:"fermeture,omitempty"`
	Jeu            *journal.RefJeu      `json:"-"`
	JeuConnu       bool                 `json:"-"`
	Regles         *Regles              `json:"regles,omitempty"` // règles en vigueur ; absent si non disponibles
	SousAgents     []string             `json:"sous_agents"`
	Serveurs       []ServeurVue         `json:"serveurs"`
	Etapes         []Etape              `json:"etapes"`
	Verif          Verification         `json:"verification"`
	Ecart          int                  `json:"premier_ecart"`
	Signature      string               `json:"signature"` // change quand le journal change
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
}

func criteresProvenance(d *directive.Directive) []critereConsultation {
	var out []critereConsultation
	if d != nil && d.Arbre != nil {
		parcourir(d.Arbre, func(n *directive.Noeud) {
			if f := n.AConsulte; f != nil {
				out = append(out, critereConsultation{serveur: f.Serveur, outil: f.Outil, etiquette: f.Etiquette, critere: f.Critere})
			}
		})
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
		cs := criteresProvenance(directiveDu(jeu, a.ID, a.Version))
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
