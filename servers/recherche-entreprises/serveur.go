package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcprogue/servers/internal/rejeu"
)

// Nom est le nom logique du serveur (internal/nommage).
const Nom = "recherche-entreprises"

// APIBase est l'API Recherche d'entreprises (sans authentification).
const APIBase = "https://recherche-entreprises.api.gouv.fr/search"

// EntreeRecherche est l'argument de rechercher_entreprise.
type EntreeRecherche struct {
	Siren string `json:"siren" jsonschema:"numéro SIREN à 9 chiffres"`
}

// Siege est l'établissement siège.
type Siege struct {
	Siret      string `json:"siret"`
	Adresse    string `json:"adresse"`
	CodePostal string `json:"code_postal"`
	Commune    string `json:"commune"`
}

// Entreprise est la fiche rendue.
type Entreprise struct {
	Siren string `json:"siren"`
	// Denomination est vide pour un entrepreneur individuel : c'est le nom
	// d'une personne physique, que ce serveur ne restitue pas.
	Denomination          string `json:"denomination"`
	Sigle                 string `json:"sigle,omitempty"`
	FormeJuridiqueCode    string `json:"forme_juridique_code" jsonschema:"catégorie juridique INSEE (niveau III)"`
	FormeJuridique        string `json:"forme_juridique,omitempty" jsonschema:"libellé de la catégorie juridique, quand il est connu du serveur"`
	PersonnePhysique      bool   `json:"personne_physique" jsonschema:"vrai pour un entrepreneur individuel"`
	Etat                  string `json:"etat" jsonschema:"active ou cessee"`
	DateCreation          string `json:"date_creation,omitempty"`
	DateFermeture         string `json:"date_fermeture,omitempty"`
	Categorie             string `json:"categorie,omitempty" jsonschema:"PME, ETI ou GE"`
	ActivitePrincipale    string `json:"activite_principale" jsonschema:"code APE (NAF rév. 2)"`
	SectionActivite       string `json:"section_activite,omitempty"`
	Siege                 *Siege `json:"siege,omitempty"`
	DateMiseAJourRegistre string `json:"date_mise_a_jour,omitempty"`
}

// SortieRecherche est le résultat de rechercher_entreprise.
type SortieRecherche struct {
	rejeu.Entete
	Entreprise *Entreprise `json:"entreprise,omitempty"`
}

// Serveur construit le serveur MCP recherche-entreprises.
func Serveur(c *rejeu.Client) *mcp.Server {
	c.Restreindre("recherche-entreprises.api.gouv.fr")
	s := mcp.NewServer(&mcp.Implementation{Name: Nom, Version: "0.1.0"}, nil)
	mcp.AddTool(s, &mcp.Tool{
		Name: "rechercher_entreprise",
		Description: "Fiche d'une entreprise française par son SIREN, depuis l'API Recherche d'entreprises " +
			"(données SIRENE/RNE) : dénomination, forme juridique, état administratif, siège, activité. " +
			"Le champ source indique si la donnée est obtenue à l'instant (direct) ou lue dans un enregistrement daté (rejeu).",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: new(bool)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in EntreeRecherche) (*mcp.CallToolResult, SortieRecherche, error) {
		return rechercher(ctx, c, in.Siren)
	})
	return s
}

func rechercher(ctx context.Context, c *rejeu.Client, siren string) (*mcp.CallToolResult, SortieRecherche, error) {
	if err := validerSiren(siren); err != nil {
		e, res := rejeu.EnteteErreur(fmt.Errorf("%w: %v", rejeu.ErrRequeteRefusee, err))
		return res, SortieRecherche{Entete: e}, nil
	}
	q := url.Values{
		"q":        {siren},
		"minimal":  {"true"},
		"include":  {"siege,complements"},
		"page":     {"1"},
		"per_page": {"5"},
	}
	rep, err := c.Obtenir(ctx, rejeu.Requete{
		URL:           APIBase + "?" + q.Encode(),
		Entetes:       map[string][]string{"Accept": {"application/json"}},
		Enregistrable: pasDePersonnePhysique,
	})
	if err != nil {
		e, res := rejeu.EnteteErreur(err)
		return res, SortieRecherche{Entete: e}, nil
	}
	ent, err := extraire(rep.Corps, siren)
	if err != nil {
		e, res := rejeu.EnteteErreur(err)
		e.Source = &rep.Source
		return res, SortieRecherche{Entete: e}, nil
	}
	return nil, SortieRecherche{Entete: rejeu.EnteteOK(rep), Entreprise: ent}, nil
}

// validerSiren : 9 chiffres et clé de Luhn (La Poste, 356000000, fait
// exception).
func validerSiren(s string) error {
	if len(s) != 9 {
		return fmt.Errorf("SIREN %q : 9 chiffres attendus", s)
	}
	somme := 0
	for i, r := range s {
		if r < '0' || r > '9' {
			return fmt.Errorf("SIREN %q : 9 chiffres attendus", s)
		}
		d := int(r - '0')
		if i%2 == 1 {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		somme += d
	}
	if somme%10 != 0 && s != "356000000" {
		return fmt.Errorf("SIREN %q : clé de contrôle invalide", s)
	}
	return nil
}

type reponseAPI struct {
	Results []resultatAPI `json:"results"`
}

type resultatAPI struct {
	Siren                     string  `json:"siren"`
	NomComplet                string  `json:"nom_complet"`
	NomRaisonSociale          *string `json:"nom_raison_sociale"`
	Sigle                     *string `json:"sigle"`
	NatureJuridique           string  `json:"nature_juridique"`
	EtatAdministratif         string  `json:"etat_administratif"`
	DateCreation              *string `json:"date_creation"`
	DateFermeture             *string `json:"date_fermeture"`
	CategorieEntreprise       *string `json:"categorie_entreprise"`
	ActivitePrincipale        string  `json:"activite_principale"`
	SectionActivitePrincipale *string `json:"section_activite_principale"`
	StatutDiffusion           string  `json:"statut_diffusion"`
	DateMiseAJour             *string `json:"date_mise_a_jour"`
	Siege                     *struct {
		Siret          string  `json:"siret"`
		Adresse        string  `json:"adresse"`
		CodePostal     *string `json:"code_postal"`
		LibelleCommune *string `json:"libelle_commune"`
	} `json:"siege"`
	Complements *struct {
		EstEntrepreneurIndividuel bool `json:"est_entrepreneur_individuel"`
	} `json:"complements"`
}

func (r resultatAPI) personnePhysique() bool {
	return (len(r.NatureJuridique) > 0 && r.NatureJuridique[0] == '1') ||
		(r.Complements != nil && r.Complements.EstEntrepreneurIndividuel)
}

// pasDePersonnePhysique refuse d'enregistrer une réponse qui contient un
// entrepreneur individuel ou une unité en diffusion partielle : les
// enregistrements sont versionnés.
func pasDePersonnePhysique(corps []byte) error {
	var r reponseAPI
	if err := json.Unmarshal(corps, &r); err != nil {
		return err
	}
	for _, e := range r.Results {
		if e.personnePhysique() || e.StatutDiffusion != "O" {
			return errors.New("contient une personne physique ou une unité en diffusion partielle")
		}
	}
	return nil
}

func extraire(corps []byte, siren string) (*Entreprise, error) {
	var r reponseAPI
	if err := json.Unmarshal(corps, &r); err != nil {
		return nil, fmt.Errorf("%w: réponse illisible: %v", rejeu.ErrIndisponible, err)
	}
	for _, e := range r.Results {
		if e.Siren != siren {
			continue
		}
		ent := &Entreprise{
			Siren:                 e.Siren,
			Sigle:                 val(e.Sigle),
			FormeJuridiqueCode:    e.NatureJuridique,
			FormeJuridique:        formes[e.NatureJuridique],
			PersonnePhysique:      e.personnePhysique(),
			Etat:                  etat(e.EtatAdministratif),
			DateCreation:          val(e.DateCreation),
			DateFermeture:         val(e.DateFermeture),
			Categorie:             val(e.CategorieEntreprise),
			ActivitePrincipale:    e.ActivitePrincipale,
			SectionActivite:       val(e.SectionActivitePrincipale),
			DateMiseAJourRegistre: val(e.DateMiseAJour),
		}
		// Ni nom ni adresse de personne physique, ni d'unité dont la
		// diffusion est restreinte.
		if !ent.PersonnePhysique && e.StatutDiffusion == "O" {
			ent.Denomination = e.NomComplet
			if n := val(e.NomRaisonSociale); n != "" {
				ent.Denomination = n
			}
			if e.Siege != nil {
				ent.Siege = &Siege{
					Siret:      e.Siege.Siret,
					Adresse:    e.Siege.Adresse,
					CodePostal: val(e.Siege.CodePostal),
					Commune:    val(e.Siege.LibelleCommune),
				}
			}
		}
		return ent, nil
	}
	return nil, fmt.Errorf("%w: aucun résultat pour le SIREN %s", rejeu.ErrIntrouvable, siren)
}

func val(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func etat(code string) string {
	switch code {
	case "A":
		return "active"
	case "C":
		return "cessee"
	}
	return code
}

// formes : libellés des catégories juridiques INSEE les plus courantes.
// Un code absent garde un libellé vide ; le code est toujours rendu.
var formes = map[string]string{
	"1000": "Entrepreneur individuel",
	"5202": "Société en nom collectif",
	"5306": "Société en commandite simple",
	"5308": "Société en commandite par actions",
	"5410": "SARL nationale",
	"5485": "Société d'exercice libéral à responsabilité limitée",
	"5498": "SARL unipersonnelle",
	"5499": "Société à responsabilité limitée (sans autre indication)",
	"5505": "SA à participation ouvrière à conseil d'administration",
	"5560": "SA coopérative de commerçants-détaillants à conseil d'administration",
	"5599": "SA à conseil d'administration (s.a.i.)",
	"5699": "SA à directoire (s.a.i.)",
	"5710": "SAS, société par actions simplifiée",
	"5720": "Société par actions simplifiée à associé unique",
	"5785": "Société d'exercice libéral par action simplifiée",
	"6540": "Société civile immobilière",
	"9220": "Association déclarée",
}
