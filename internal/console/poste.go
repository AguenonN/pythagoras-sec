package console

import (
	"fmt"
	"time"

	"mcprogue/internal/journal"
)

// Cout est le coût des règles pour une session, mesuré au journal : aucune
// estimation. Un ancien journal sans decision.duree_us donne « non mesuré ».
type Cout struct {
	Appels        int `json:"appels"`
	Refuses       int `json:"refuses"`
	Caviardes     int `json:"caviardes"`
	Validations   int `json:"validations"` // appels mis en attente de validation
	ChampsRetires int `json:"champs_retires"`
	// Temps ajouté par l'engine : duree_us de chaque appel, de l'entrée de
	// l'appel dans le pipeline jusqu'à la décision rendue.
	Mesures    int   `json:"mesures"`
	NonMesures int   `json:"non_mesures"`
	MoyenneUs  int64 `json:"moyenne_us"`
	MaxUs      int64 `json:"max_us"`
	TotalUs    int64 `json:"total_us"`
	ServeursMs int64 `json:"serveurs_ms"` // durée des consultations, hors règles
	// Attente humaine : horodatage de l'approbation acceptée moins celui de
	// l'appel en attente.
	Attentes        int   `json:"attentes_mesurees"`
	AttenteSecondes int64 `json:"attente_secondes"`
}

// VuePoste est l'écran 2.
type VuePoste struct {
	*VueChaine
	Reponse Reponse `json:"reponse"`
	Cout    Cout    `json:"cout"`
}

// Reponse est la réponse au conseiller. Le texte final vient de
// reponse.json (écrit par l'orchestrateur) ; ce que l'engine a rendu à
// l'agent se déduit de la décision, il n'est pas journalisé en texte.
type Reponse struct {
	// Texte et Statut : reponse.json, absents tant que la demande n'est pas
	// terminée.
	Texte            string   `json:"texte,omitempty"`
	Statut           string   `json:"statut,omitempty"`
	DirectivesCitees []string `json:"directives_citees,omitempty"`
	Terminee         bool     `json:"terminee"`
	// Badge, Titre, Paragraphes : décision de l'engine au dernier appel.
	Badge       string   `json:"badge"`
	Titre       string   `json:"titre"`
	Paragraphes []string `json:"paragraphes"`
}

// Poste construit l'écran 2.
func (d *Depot) Poste(st *Etat, jeux []JeuCharge, session string) (*VuePoste, bool) {
	c, ok := d.Chaine(st, jeux, session)
	if !ok {
		return nil, false
	}
	p := &VuePoste{VueChaine: c}
	attendus := map[string]time.Time{} // empreinte transmise → horodatage de l'attente
	for _, e := range c.Etapes {
		p.Cout.Appels++
		switch e.Badge {
		case BadgeRefuse, BadgeNonAttesta:
			p.Cout.Refuses++
		case BadgeCaviarde:
			p.Cout.Caviardes++
		}
		if e.Effet == "attendre_validation" && !e.Transmis {
			p.Cout.Validations++
			attendus[e.Empreinte] = e.TS
		}
		p.Cout.ServeursMs += e.DureeMs
		p.Cout.ChampsRetires += len(e.Champs)
		if e.DureeUs > 0 {
			p.Cout.Mesures++
			p.Cout.TotalUs += e.DureeUs
			if e.DureeUs > p.Cout.MaxUs {
				p.Cout.MaxUs = e.DureeUs
			}
		} else {
			p.Cout.NonMesures++
		}
	}
	if p.Cout.Mesures > 0 {
		p.Cout.MoyenneUs = p.Cout.TotalUs / int64(p.Cout.Mesures)
	}
	for _, en := range st.Entrees {
		if en.Type != journal.ApprobationT || en.Session != session || en.Approbation == nil ||
			en.Approbation.Statut != journal.ApprobationAcceptee {
			continue
		}
		if t0, ok := attendus[en.Approbation.EmpreinteTransmise]; ok && en.TS.After(t0) {
			p.Cout.Attentes++
			p.Cout.AttenteSecondes += int64(en.TS.Sub(t0) / time.Second)
			delete(attendus, en.Approbation.EmpreinteTransmise)
		}
	}
	p.Reponse = reponseFinale(c, st)
	return p, true
}

// reponseFinale résume la réponse au conseiller : le texte de reponse.json
// et, séparément, ce que l'engine a rendu à l'agent au dernier appel.
func reponseFinale(c *VueChaine, st *Etat) Reponse {
	r := Reponse{Paragraphes: []string{}}
	if st.Reponse != nil {
		r.Texte, r.Statut, r.DirectivesCitees, r.Terminee = st.Reponse.Texte, st.Reponse.Statut, st.Reponse.DirectivesCitees, true
	}
	if len(c.Etapes) == 0 {
		r.Badge, r.Titre = BadgeIndispo, "Aucun appel d'outil pour cette demande."
		return r
	}
	e := c.Etapes[len(c.Etapes)-1]
	r.Badge = e.Badge
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
		if len(e.ValidePar) > 0 {
			r.Titre = fmt.Sprintf("Appel %s transmis après validation humaine.", e.Outil)
		}
	}
	r.Paragraphes = append(r.Paragraphes, "Motif de l'engine : "+e.Motif)
	return r
}
