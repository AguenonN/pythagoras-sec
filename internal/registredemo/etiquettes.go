package registredemo

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"mcprogue/internal/registre"
)

// FichierEtiquettes est le fichier versionné des étiquettes du registre de
// démonstration, relatif à la racine du dépôt.
const FichierEtiquettes = "demo/etiquettes.json"

// Etiquettes est demo/etiquettes.json : les étiquettes que le registre de
// démo pose sur ses serveurs et ses outils. Outils est indexé par
// "serveur/outil", ou "*/outil" pour l'outil de ce nom sur tout serveur.
type Etiquettes struct {
	Commentaire string              `json:"commentaire,omitempty"`
	Version     int                 `json:"version"`
	Serveurs    map[string][]string `json:"serveurs"`
	Outils      map[string][]string `json:"outils"`
}

// LireEtiquettes lit et décode le fichier des étiquettes, strictement.
func LireEtiquettes(chemin string) (*Etiquettes, error) {
	b, err := os.ReadFile(chemin)
	if err != nil {
		return nil, fmt.Errorf("registredemo: étiquettes: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var e Etiquettes
	if err := dec.Decode(&e); err != nil {
		return nil, fmt.Errorf("registredemo: étiquettes illisibles: %w", err)
	}
	if dec.More() {
		return nil, errors.New("registredemo: étiquettes : contenu inattendu après le document")
	}
	if e.Version < 1 {
		return nil, errors.New("registredemo: étiquettes : version doit être >= 1")
	}
	return &e, nil
}

// Appliquer pose les étiquettes sur les serveurs et les outils du
// registre. Un serveur ou un outil nommé que le registre ne connaît pas est
// une erreur (faute de frappe, qui laisserait une directive sans cible) ;
// une règle "*/outil" peut ne rien toucher : elle vaut pour les outils à
// venir. Les étiquettes sont validées par registre.Valider, que
// l'appelant refait ensuite (l'empreinte change).
func (e *Etiquettes) Appliquer(r *registre.Registre) error {
	var errs []error
	for _, nom := range triees(e.Serveurs) {
		trouve := false
		for i := range r.Serveurs {
			if r.Serveurs[i].Nom == nom {
				r.Serveurs[i].Etiquettes = union(r.Serveurs[i].Etiquettes, e.Serveurs[nom])
				trouve = true
			}
		}
		if !trouve {
			errs = append(errs, fmt.Errorf("registredemo: étiquettes : serveur %s absent du registre", nom))
		}
	}
	for _, cle := range triees(e.Outils) {
		serveur, outil, ok := strings.Cut(cle, "/")
		if !ok || serveur == "" || outil == "" || strings.Contains(outil, "/") {
			errs = append(errs, fmt.Errorf("registredemo: étiquettes : %q doit s'écrire serveur/outil ou */outil", cle))
			continue
		}
		trouve := false
		for i := range r.Serveurs {
			s := &r.Serveurs[i]
			if serveur != "*" && s.Nom != serveur {
				continue
			}
			for j := range s.Outils {
				if s.Outils[j].Nom == outil {
					s.Outils[j].Etiquettes = union(s.Outils[j].Etiquettes, e.Outils[cle])
					trouve = true
				}
			}
		}
		if !trouve && serveur != "*" {
			errs = append(errs, fmt.Errorf("registredemo: étiquettes : outil %s absent du registre", cle))
		}
	}
	return errors.Join(errs...)
}

func triees(m map[string][]string) []string {
	cles := make([]string, 0, len(m))
	for k := range m {
		cles = append(cles, k)
	}
	sort.Strings(cles)
	return cles
}

// union rend a ∪ b, trié, sans doublon ; nil si vide (le champ reste omis).
func union(a, b []string) []string {
	vus := map[string]bool{}
	var out []string
	for _, x := range append(append([]string{}, a...), b...) {
		if !vus[x] {
			vus[x] = true
			out = append(out, x)
		}
	}
	sort.Strings(out)
	return out
}
