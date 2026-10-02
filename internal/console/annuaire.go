package console

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"mcprogue/internal/annuaire"
)

// Annuaire lit l'annuaire signé de la démonstration et en vérifie la
// signature avec la clé publique de l'autorité (demo/execution/cles/
// autorite.pub). Sans cette vérification, rien n'est proposé : la console
// ne tient pas pour vrai un annuaire dont elle ne sait pas qui l'a signé.
func (d *Depot) Annuaire() (*annuaire.Local, *annuaire.Document, error) {
	ch := d.Chemins()
	brut, err := os.ReadFile(ch.Annuaire)
	if err != nil {
		return nil, nil, fmt.Errorf("annuaire introuvable (%s)", ch.Annuaire)
	}
	cle, err := annuaire.LireClePublique(ch.Autorite)
	if err != nil {
		return nil, nil, fmt.Errorf("clé publique de l'autorité illisible : %v", err)
	}
	loc, err := annuaire.Charger(brut, cle)
	if err != nil {
		return nil, nil, fmt.Errorf("annuaire refusé : %v", err)
	}
	var doc annuaire.Document
	if err := json.Unmarshal(brut, &doc); err != nil {
		return nil, nil, err
	}
	return loc, &doc, nil
}

// NomIdentite est le nom lisible d'une identité (annuaire), à défaut
// l'identité elle-même ; l'identité de démonstration du juriste s'écrit
// « J. Juriste ».
func (d *Depot) NomIdentite(id string) string {
	if _, doc, err := d.Annuaire(); err == nil {
		for _, i := range doc.Identites {
			if i.Identite == id && i.Nom != "" {
				return i.Nom
			}
		}
	}
	if id == "juriste@exemple.test" || id == "" {
		return "J. Juriste"
	}
	return id
}

// Valideur est une identité de l'annuaire qui détient un rôle à la date.
type Valideur struct {
	Identite string `json:"identite"`
	Nom      string `json:"nom"`
	Role     string `json:"role"`
}

// Valideurs liste, pour chaque rôle demandé, les identités de l'annuaire
// qui le détiennent à la date donnée. Sans rôle demandé : tout valideur de
// l'annuaire qui détient un rôle.
func (d *Depot) Valideurs(roles []string, a time.Time) ([]Valideur, error) {
	loc, doc, err := d.Annuaire()
	if err != nil {
		return nil, err
	}
	noms := map[string]string{}
	for _, i := range doc.Identites {
		noms[i.Identite] = i.Nom
	}
	voulu := map[string]bool{}
	for _, r := range roles {
		voulu[r] = true
	}
	var out []Valideur
	for _, id := range loc.Identites() {
		f, ok := loc.Resoudre(id, a)
		if !ok {
			continue
		}
		for _, r := range f.Roles {
			if len(voulu) == 0 || voulu[r] {
				out = append(out, Valideur{Identite: id, Nom: noms[id], Role: r})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Role != out[j].Role {
			return out[i].Role < out[j].Role
		}
		return out[i].Identite < out[j].Identite
	})
	return out, nil
}

// FicheDemandeur rend le nom, les groupes et les rôles d'une identité
// d'après l'annuaire, à la date donnée ; faux si elle n'y est pas.
type FicheDemandeur struct {
	Identite string   `json:"identite"`
	Nom      string   `json:"nom,omitempty"`
	Groupes  []string `json:"groupes,omitempty"`
	Roles    []string `json:"roles,omitempty"`
}

func (d *Depot) Fiche(id string, a time.Time) (FicheDemandeur, bool) {
	loc, doc, err := d.Annuaire()
	if err != nil {
		return FicheDemandeur{}, false
	}
	f, ok := loc.Resoudre(id, a)
	if !ok {
		return FicheDemandeur{}, false
	}
	out := FicheDemandeur{Identite: id, Groupes: f.Groupes, Roles: f.Roles}
	for _, i := range doc.Identites {
		if i.Identite == id {
			out.Nom = i.Nom
		}
	}
	return out, true
}

var errIdentite = errors.New("identité inconnue de l'annuaire")

func motsAvant(s, sep string) string {
	if i := strings.Index(s, sep); i >= 0 {
		return s[:i]
	}
	return s
}
