package console

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"mcprogue/internal/journal"
)

// Statuts de etat.json (contrats §2).
const (
	StatutEnCours   = "en_cours"
	StatutEnAttente = "en_attente"
	StatutTermine   = "termine"
	StatutBloque    = "bloque"
	StatutErreur    = "erreur"
)

// AttenteEtat est le bloc « attente » de etat.json : ce qu'un jeton
// d'approbation doit viser et qui peut le signer.
type AttenteEtat struct {
	EmpreinteTransmise string   `json:"empreinte_transmise"`
	Roles              []string `json:"roles"`
	QuatreYeux         bool     `json:"quatre_yeux"`
	Directives         []string `json:"directives"`
	// MotifJeton : motif du refus des jetons à la dernière reprise.
	MotifJeton string `json:"motif_jeton,omitempty"`
}

// EtatDossier est etat.json, relu à chaque affichage.
type EtatDossier struct {
	Statut    string       `json:"statut"`
	Demande   string       `json:"demande,omitempty"`
	Demandeur string       `json:"demandeur,omitempty"`
	Debut     *time.Time   `json:"debut,omitempty"`
	Fin       *time.Time   `json:"fin,omitempty"`
	Motif     string       `json:"motif,omitempty"`
	Attente   *AttenteEtat `json:"attente,omitempty"`
}

// ReponseDossier est reponse.json : le texte final remis au conseiller.
type ReponseDossier struct {
	Texte            string   `json:"texte"`
	Statut           string   `json:"statut"`
	DirectivesCitees []string `json:"directives_citees"`
}

// identiteDe lit un demandeur écrit en chaîne ou en objet {identite|id}.
func identiteDe(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var o map[string]any
	if json.Unmarshal(raw, &o) == nil {
		for _, k := range []string{"identite", "id", "nom"} {
			if v, ok := o[k].(string); ok {
				return v
			}
		}
	}
	return ""
}

// listeIDs lit une liste de chaînes, ou d'objets portant un « id ».
func listeIDs(raw json.RawMessage) []string {
	var ss []string
	if json.Unmarshal(raw, &ss) == nil {
		return ss
	}
	var objets []map[string]any
	var out []string
	if json.Unmarshal(raw, &objets) == nil {
		for _, o := range objets {
			for _, k := range []string{"id", "directive"} {
				if v, ok := o[k].(string); ok {
					out = append(out, v)
					break
				}
			}
		}
	}
	return out
}

func lireEtatDossier(brut []byte) *EtatDossier {
	var w struct {
		Statut    string          `json:"statut"`
		Demande   string          `json:"demande"`
		Demandeur json.RawMessage `json:"demandeur"`
		Debut     *time.Time      `json:"debut"`
		Fin       *time.Time      `json:"fin"`
		Motif     string          `json:"motif"`
		Attente   *struct {
			Empreinte  string          `json:"empreinte_transmise"`
			Roles      json.RawMessage `json:"roles"`
			QuatreYeux bool            `json:"quatre_yeux"`
			Directives json.RawMessage `json:"directives"`
			MotifJeton string          `json:"motif_jeton"`
		} `json:"attente"`
	}
	if json.Unmarshal(brut, &w) != nil {
		return nil
	}
	e := &EtatDossier{Statut: w.Statut, Demande: w.Demande, Demandeur: identiteDe(w.Demandeur), Debut: w.Debut, Fin: w.Fin, Motif: w.Motif}
	if a := w.Attente; a != nil {
		e.Attente = &AttenteEtat{EmpreinteTransmise: a.Empreinte, QuatreYeux: a.QuatreYeux, MotifJeton: a.MotifJeton,
			Roles: listeIDs(a.Roles), Directives: listeIDs(a.Directives)}
	}
	return e
}

// lireDossier lit les fichiers de la session, à côté du journal. Un fichier
// absent ou illisible (en cours d'écriture) est simplement absent : l'écriture
// de l'orchestrateur est atomique, mais la console ne s'y fie pas.
func lireDossier(dir string) (*EtatDossier, *ReponseDossier, []journal.Entree) {
	var etat *EtatDossier
	var rep *ReponseDossier
	var notifs []journal.Entree
	if b, err := os.ReadFile(filepath.Join(dir, "etat.json")); err == nil {
		etat = lireEtatDossier(b)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "reponse.json")); err == nil {
		var r ReponseDossier
		if json.Unmarshal(b, &r) == nil {
			rep = &r
		}
	}
	if b, err := os.ReadFile(filepath.Join(dir, "notifications.jsonl")); err == nil {
		notifs = []journal.Entree{}
		for _, l := range bytes.Split(b, []byte{'\n'}) {
			l = bytes.TrimSpace(l)
			if len(l) == 0 {
				continue
			}
			var e journal.Entree
			if json.Unmarshal(l, &e) == nil && e.Type == journal.NotificationT {
				notifs = append(notifs, e)
			}
		}
	}
	return etat, rep, notifs
}
