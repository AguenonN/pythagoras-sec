package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"mcprogue/internal/annuaire"
	"mcprogue/internal/registredemo"
)

// Sources du vocabulaire du compilateur, relatives à la racine du dépôt.
const (
	fichierAnnuaire   = "demo/annuaire-source.json"
	fichierGroupesRH  = "servers/referentiel-rh/groupes_fictifs.json"
	cheminVocabulaire = "agents/compilateur/exemples/vocabulaire-demo.json"
)

// Vocabulaire est ce que le compilateur de directives peut nommer en plus
// du catalogue : étiquettes du registre, rôles et groupes de l'annuaire,
// groupes des personnes concernées (référentiel RH). Chaque liste est triée
// par nom.
type Vocabulaire struct {
	Origine    string `json:"origine"`
	Etiquettes []Nom  `json:"etiquettes"`
	Roles      []Nom  `json:"roles"`
	Groupes    []Nom  `json:"groupes"`
}

// Nom est une entrée du vocabulaire.
type Nom struct {
	Nom         string `json:"nom"`
	Description string `json:"description"`
}

const origineVocabulaire = "généré par go run ./cmd/registre-demo -vocabulaire " + cheminVocabulaire +
	", depuis " + registredemo.FichierEtiquettes + ", " + fichierAnnuaire + " et " + fichierGroupesRH +
	" ; ne pas modifier à la main"

const (
	descriptionRole         = "rôle de l'annuaire (valideur, destinataire d'une notification)"
	descriptionGroupe       = "groupe de l'annuaire : personnes qui lancent les agents (feuille demandeur)"
	descriptionGroupeSujetF = "%s Référentiel RH : personne concernée par l'action (feuille sujet)."
)

// groupesRH est servers/referentiel-rh/groupes_fictifs.json, réduit à ce
// que le vocabulaire en lit.
type groupesRH struct {
	Groupes map[string]string `json:"groupes"`
}

// GenererVocabulaire lit les trois sources sous racine.
func GenererVocabulaire(racine string) (*Vocabulaire, error) {
	return generer(racine, false)
}

// GenererVocabulaireEtendu est GenererVocabulaire avec les étiquettes des
// serveurs ajoutés par l'équipe (registredemo.FichierSupplementaires).
func GenererVocabulaireEtendu(racine string) (*Vocabulaire, error) {
	return generer(racine, true)
}

func generer(racine string, etendu bool) (*Vocabulaire, error) {
	et, err := registredemo.LireEtiquettes(filepath.Join(racine, registredemo.FichierEtiquettes))
	if err != nil {
		return nil, err
	}
	if etendu {
		supp, err := registredemo.LireSupplementaires(racine)
		if err != nil {
			return nil, err
		}
		if e := supp.Etiquettes; e != nil {
			fusion := *et
			fusion.Serveurs, fusion.Outils = map[string][]string{}, map[string][]string{}
			for _, m := range []struct{ de, vers map[string][]string }{{et.Serveurs, fusion.Serveurs}, {e.Serveurs, fusion.Serveurs}, {et.Outils, fusion.Outils}, {e.Outils, fusion.Outils}} {
				for k, v := range m.de {
					m.vers[k] = append(m.vers[k], v...)
				}
			}
			et = &fusion
		}
	}
	an, err := annuaire.LireSourceDemo(filepath.Join(racine, fichierAnnuaire))
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(filepath.Join(racine, fichierGroupesRH))
	if err != nil {
		return nil, fmt.Errorf("vocabulaire: %w", err)
	}
	var rh groupesRH
	if err := json.Unmarshal(b, &rh); err != nil {
		return nil, fmt.Errorf("vocabulaire: %s illisible: %w", fichierGroupesRH, err)
	}
	return vocabulaireDepuis(et, an, rh)
}

func vocabulaireDepuis(et *registredemo.Etiquettes, an *annuaire.SourceDemo, rh groupesRH) (*Vocabulaire, error) {
	v := &Vocabulaire{Origine: origineVocabulaire, Etiquettes: []Nom{}, Roles: []Nom{}, Groupes: []Nom{}}

	// Étiquettes : chacune dit où le registre la pose.
	portees := map[string][]string{}
	for _, s := range trier(et.Serveurs) {
		for _, e := range et.Serveurs[s] {
			portees[e] = append(portees[e], "serveur "+s)
		}
	}
	for _, o := range trier(et.Outils) {
		lieu := "outil " + o
		if nom, ok := strings.CutPrefix(o, "*/"); ok {
			lieu = "outil " + nom + " (tout serveur)"
		}
		for _, e := range et.Outils[o] {
			portees[e] = append(portees[e], lieu)
		}
	}
	for _, e := range trier(portees) {
		v.Etiquettes = append(v.Etiquettes, Nom{e, "étiquette du registre, portée par : " + strings.Join(portees[e], ", ")})
	}

	roles, groupes := map[string]bool{}, map[string]string{}
	for _, id := range an.Identites {
		for _, r := range id.Roles {
			roles[r.Role] = true
		}
		for _, g := range id.Groupes {
			groupes[g] = descriptionGroupe
		}
	}
	for _, r := range trier(roles) {
		v.Roles = append(v.Roles, Nom{r, descriptionRole})
	}
	for _, g := range trier(rh.Groupes) {
		d := fmt.Sprintf(descriptionGroupeSujetF, strings.TrimSpace(rh.Groupes[g]))
		if a, ok := groupes[g]; ok {
			d = a + " ; " + d
		}
		groupes[g] = d
	}
	for _, g := range trier(groupes) {
		v.Groupes = append(v.Groupes, Nom{g, groupes[g]})
	}
	for _, l := range [][]Nom{v.Etiquettes, v.Roles, v.Groupes} {
		for _, n := range l {
			if n.Nom == "" {
				return nil, errors.New("vocabulaire: nom vide dans une source")
			}
		}
	}
	return v, nil
}

// EncoderVocabulaire écrit le vocabulaire comme le catalogue : indenté,
// sans échappement HTML, terminé par un saut de ligne.
func EncoderVocabulaire(v *Vocabulaire) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func trier[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
