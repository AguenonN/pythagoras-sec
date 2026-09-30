package registredemo

import (
	"encoding/json"
	"sort"
	"strings"

	"mcprogue/internal/registre"
)

// Catalogue est le catalogue que lit le compilateur de directives
// (agents/compilateur/catalogue.py) : serveurs, outils, arguments en
// pointeurs JSON. Il ne porte ni chemin ni empreinte : il ne dépend que
// des schémas épinglés, pas de la machine qui a construit les binaires.
type Catalogue struct {
	Origine  string             `json:"origine"`
	Serveurs []ServeurCatalogue `json:"serveurs"`
}

// ServeurCatalogue est un serveur du catalogue.
type ServeurCatalogue struct {
	Nom    string           `json:"nom"`
	Outils []OutilCatalogue `json:"outils"`
}

// OutilCatalogue est un outil du catalogue.
type OutilCatalogue struct {
	Nom         string   `json:"nom"`
	Acces       string   `json:"acces"`
	Description string   `json:"description"`
	Arguments   []string `json:"arguments"`
}

// OrigineCatalogue dit d'où vient le catalogue ; le compilateur l'ignore.
const OrigineCatalogue = "généré par cmd/registre-demo depuis demo/registre.json ; ne pas modifier à la main"

// CatalogueDepuis dérive le catalogue du registre, dans l'ordre du
// registre.
func CatalogueDepuis(r *registre.Registre) Catalogue {
	c := Catalogue{Origine: OrigineCatalogue}
	for _, s := range r.Serveurs {
		sc := ServeurCatalogue{Nom: s.Nom}
		for _, o := range s.Outils {
			sc.Outils = append(sc.Outils, OutilCatalogue{
				Nom: o.Nom, Acces: string(o.Acces), Description: o.Description, Arguments: Pointeurs(o.InputSchema),
			})
		}
		c.Serveurs = append(c.Serveurs, sc)
	}
	return c
}

// Pointeurs rend les pointeurs JSON des propriétés d'un schéma objet,
// triés, en descendant dans les propriétés des sous-objets. Un tableau est
// désigné en entier, pas élément par élément.
func Pointeurs(schema map[string]any) []string {
	out := []string{}
	var parcourir func(prefixe string, s map[string]any)
	parcourir = func(prefixe string, s map[string]any) {
		props, _ := s["properties"].(map[string]any)
		for nom, sous := range props {
			p := prefixe + "/" + echapper(nom)
			out = append(out, p)
			if m, ok := sous.(map[string]any); ok {
				parcourir(p, m)
			}
		}
	}
	parcourir("", schema)
	sort.Strings(out)
	return out
}

// echapper applique RFC 6901 : ~ devient ~0, / devient ~1.
func echapper(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1")
}

// EncoderCatalogue rend le document JSON du catalogue, indenté, terminé
// par un saut de ligne, sans échappement HTML.
func EncoderCatalogue(c Catalogue) ([]byte, error) {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(c); err != nil {
		return nil, err
	}
	return []byte(b.String()), nil
}
