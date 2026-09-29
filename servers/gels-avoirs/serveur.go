package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcprogue/servers/internal/rejeu"
)

// Nom est le nom logique du serveur (internal/nommage).
const Nom = "gels-avoirs"

// URLRegistre est la dernière publication du registre national des gels,
// DG Trésor, au format JSON.
const URLRegistre = "https://gels-avoirs.dgtresor.gouv.fr/ApiPublic/api/v1/publication/derniere-publication-flux-json"

// MaxCorrespondances borne la liste rendue ; le total est toujours donné.
const MaxCorrespondances = 20

// DescriptionFiltre figure dans chaque enregistrement et dans la source d'une
// réponse rejouée.
const DescriptionFiltre = "personnes physiques retirées ; seuls nom, alias, fondements juridiques et références (UE, ONU, OMI) des personnes morales et navires sont conservés"

// EntreeVerification est l'argument de verifier_gel.
type EntreeVerification struct {
	Nom string `json:"nom" jsonschema:"nom de la personne morale, de la personne physique ou du navire à filtrer"`
}

// Correspondance est une entrée du registre qui correspond au nom.
type Correspondance struct {
	IDRegistre   int      `json:"id_registre"`
	Nature       string   `json:"nature" jsonschema:"Personne morale, Personne physique ou Navire"`
	Nom          string   `json:"nom"`
	AliasTrouve  string   `json:"alias_trouve,omitempty" jsonschema:"alias du registre qui a correspondu, si ce n'est pas le nom"`
	Type         string   `json:"type" jsonschema:"exacte (mêmes mots) ou partielle (tous les mots cherchés présents)"`
	Fondements   []string `json:"fondements_juridiques"`
	ReferencesUE []string `json:"references_ue,omitempty"`
}

// SortieVerification est le résultat de verifier_gel.
type SortieVerification struct {
	rejeu.Entete
	DatePublicationRegistre string           `json:"date_publication_registre,omitempty" jsonschema:"date de la publication du registre consultée"`
	NombreCorrespondances   int              `json:"nombre_correspondances"`
	Correspondances         []Correspondance `json:"correspondances"`
	Avertissement           string           `json:"avertissement,omitempty"`
}

// Serveur construit le serveur MCP gels-avoirs.
func Serveur(c *rejeu.Client) *mcp.Server {
	c.Restreindre("gels-avoirs.dgtresor.gouv.fr")
	s := mcp.NewServer(&mcp.Implementation{Name: Nom, Version: "0.1.0"}, nil)
	idx := &index{}
	mcp.AddTool(s, &mcp.Tool{
		Name: "verifier_gel",
		Description: "Cherche un nom dans le registre national des gels des avoirs (DG Trésor) : " +
			"correspondances exactes ou partielles, nature, fondements juridiques, et date de publication du registre. " +
			"Une correspondance est un signal à examiner, pas une identification. " +
			"Le champ source indique si le registre est obtenu à l'instant (direct) ou lu dans un enregistrement daté (rejeu).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in EntreeVerification) (*mcp.CallToolResult, SortieVerification, error) {
		return verifier(ctx, c, idx, in.Nom)
	})
	return s
}

func verifier(ctx context.Context, c *rejeu.Client, idx *index, nom string) (*mcp.CallToolResult, SortieVerification, error) {
	cherche := mots(nom)
	if len(cherche) == 0 {
		e, res := rejeu.EnteteErreur(fmt.Errorf("%w: nom vide", rejeu.ErrRequeteRefusee))
		return res, SortieVerification{Entete: e, Correspondances: []Correspondance{}}, nil
	}
	rep, err := c.Obtenir(ctx, rejeu.Requete{
		URL:     URLRegistre,
		Entetes: map[string][]string{"Accept": {"application/json"}},
		Filtre:  &rejeu.Filtre{Description: DescriptionFiltre, Appliquer: filtrer},
	})
	if err != nil {
		e, res := rejeu.EnteteErreur(err)
		return res, SortieVerification{Entete: e, Correspondances: []Correspondance{}}, nil
	}
	reg, err := idx.charger(rep)
	if err != nil {
		e, res := rejeu.EnteteErreur(err)
		e.Source = &rep.Source
		return res, SortieVerification{Entete: e, Correspondances: []Correspondance{}}, nil
	}
	trouve := reg.chercher(cherche)
	out := SortieVerification{
		Entete:                  rejeu.EnteteOK(rep),
		DatePublicationRegistre: reg.date,
		NombreCorrespondances:   len(trouve),
		Correspondances:         trouve,
	}
	if len(trouve) > MaxCorrespondances {
		out.Correspondances = trouve[:MaxCorrespondances]
	}
	if rep.Source.Filtre != "" {
		out.Avertissement = "registre rejoué partiel (" + rep.Source.Filtre + ") : l'absence de correspondance n'est pas probante, en particulier pour une personne physique"
	}
	return nil, out, nil
}

// Format de l'API.
type publication struct {
	Publications struct {
		DatePublication   string   `json:"DatePublication"`
		PublicationDetail []entree `json:"PublicationDetail"`
	} `json:"Publications"`
}

type entree struct {
	IDRegistre     int      `json:"IdRegistre"`
	Nature         string   `json:"Nature"`
	Nom            string   `json:"Nom"`
	RegistreDetail []detail `json:"RegistreDetail"`
}

type detail struct {
	TypeChamp string                       `json:"TypeChamp"`
	Valeur    []map[string]json.RawMessage `json:"Valeur"`
}

// champsConserves : ce qu'un enregistrement garde d'une personne morale ou
// d'un navire. Motifs, adresses, contacts et identifiants sont retirés : ils
// peuvent nommer des personnes physiques.
var champsConserves = map[string]bool{
	"ALIAS":               true,
	"FONDEMENT_JURIDIQUE": true,
	"REFERENCE_UE":        true,
	"REFERENCE_ONU":       true,
	"NUMERO_OMI":          true,
}

// clesConservees : dans ces champs, les clés gardées. Le commentaire d'un
// alias peut citer une personne ; il part.
var clesConservees = map[string]bool{
	"Alias":                   true,
	"FondementJuridique":      true,
	"FondementJuridiqueLabel": true,
	"ReferenceUe":             true,
	"ReferenceOnu":            true,
	"NumeroOMI":               true,
	"NumeroOmi":               true,
}

// filtrer retire du registre toute personne physique et ne garde que les
// champs utiles au filtrage des autres entrées.
func filtrer(corps []byte) ([]byte, error) {
	var p publication
	if err := json.Unmarshal(corps, &p); err != nil {
		return nil, err
	}
	if len(p.Publications.PublicationDetail) == 0 {
		return nil, fmt.Errorf("registre vide")
	}
	garde := make([]entree, 0, len(p.Publications.PublicationDetail))
	for _, e := range p.Publications.PublicationDetail {
		if e.Nature != "Personne morale" && e.Nature != "Navire" {
			continue
		}
		var det []detail
		for _, d := range e.RegistreDetail {
			if !champsConserves[d.TypeChamp] {
				continue
			}
			var vals []map[string]json.RawMessage
			for _, v := range d.Valeur {
				m := map[string]json.RawMessage{}
				for k, x := range v {
					if clesConservees[k] {
						m[k] = x
					}
				}
				if len(m) > 0 {
					vals = append(vals, m)
				}
			}
			if len(vals) > 0 {
				det = append(det, detail{TypeChamp: d.TypeChamp, Valeur: vals})
			}
		}
		e.RegistreDetail = det
		garde = append(garde, e)
	}
	p.Publications.PublicationDetail = garde
	return json.Marshal(p)
}

// index garde le dernier registre analysé, par empreinte : le corps fait
// plusieurs mégaoctets et la même réponse est resservie par le client.
type index struct {
	mu     sync.Mutex
	sha256 string
	reg    *registre
}

func (x *index) charger(rep *rejeu.Reponse) (*registre, error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.reg != nil && x.sha256 == rep.Source.SHA256 {
		return x.reg, nil
	}
	var p publication
	if err := json.Unmarshal(rep.Corps, &p); err != nil {
		return nil, fmt.Errorf("%w: registre illisible: %v", rejeu.ErrIndisponible, err)
	}
	if len(p.Publications.PublicationDetail) == 0 {
		return nil, fmt.Errorf("%w: registre vide", rejeu.ErrIndisponible)
	}
	r := &registre{date: p.Publications.DatePublication}
	for _, e := range p.Publications.PublicationDetail {
		f := fiche{entree: e, noms: []nomIndexe{{texte: e.Nom, mots: mots(e.Nom)}}}
		for _, d := range e.RegistreDetail {
			for _, v := range d.Valeur {
				switch d.TypeChamp {
				case "ALIAS":
					if a := chaine(v["Alias"]); a != "" {
						f.noms = append(f.noms, nomIndexe{texte: a, alias: true, mots: mots(a)})
					}
				case "FONDEMENT_JURIDIQUE":
					if l := chaine(v["FondementJuridiqueLabel"]); l != "" {
						f.fondements = append(f.fondements, l)
					}
				case "REFERENCE_UE":
					if l := chaine(v["ReferenceUe"]); l != "" {
						f.refsUE = append(f.refsUE, l)
					}
				}
			}
		}
		r.fiches = append(r.fiches, f)
	}
	x.sha256, x.reg = rep.Source.SHA256, r
	return r, nil
}

func chaine(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return ""
	}
	return s
}

type registre struct {
	date   string
	fiches []fiche
}

type fiche struct {
	entree
	noms       []nomIndexe
	fondements []string
	refsUE     []string
}

type nomIndexe struct {
	texte string
	alias bool
	mots  []string
}

func (r *registre) chercher(cherche []string) []Correspondance {
	var out []Correspondance
	for _, f := range r.fiches {
		meilleur, alias := "", ""
		for _, n := range f.noms {
			t := comparer(cherche, n.mots)
			if t == "" || meilleur == "exacte" {
				continue
			}
			if meilleur == "" || t == "exacte" {
				meilleur = t
				alias = ""
				if n.alias {
					alias = n.texte
				}
			}
		}
		if meilleur == "" {
			continue
		}
		fond := f.fondements
		if fond == nil {
			fond = []string{}
		}
		out = append(out, Correspondance{
			IDRegistre:   f.IDRegistre,
			Nature:       f.Nature,
			Nom:          f.Nom,
			AliasTrouve:  alias,
			Type:         meilleur,
			Fondements:   fond,
			ReferencesUE: f.refsUE,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Type == "exacte" && out[j].Type != "exacte"
	})
	if out == nil {
		out = []Correspondance{}
	}
	return out
}

// comparer rend "exacte" si les mots sont les mêmes (formes juridiques
// comprises), "partielle" si tous les mots cherchés figurent dans le nom,
// "" sinon.
func comparer(cherche, nom []string) string {
	if len(nom) == 0 {
		return ""
	}
	present := make(map[string]bool, len(nom))
	for _, m := range nom {
		present[m] = true
	}
	for _, m := range cherche {
		if !present[m] {
			return ""
		}
	}
	if strings.Join(cherche, " ") == strings.Join(nom, " ") {
		return "exacte"
	}
	return "partielle"
}

// mots normalise un nom : minuscules, sans accents, découpé sur tout ce qui
// n'est ni lettre ni chiffre.
func mots(s string) []string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if r2, ok := sansAccent[r]; ok {
			b.WriteString(r2)
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	return strings.Fields(b.String())
}

var sansAccent = map[rune]string{
	'à': "a", 'â': "a", 'ä': "a", 'á': "a", 'ã': "a", 'å': "a",
	'ç': "c", 'č': "c", 'ć': "c",
	'é': "e", 'è': "e", 'ê': "e", 'ë': "e", 'ě': "e",
	'î': "i", 'ï': "i", 'í': "i", 'ì': "i",
	'ô': "o", 'ö': "o", 'ó': "o", 'ò': "o", 'õ': "o", 'ø': "o",
	'û': "u", 'ü': "u", 'ú': "u", 'ù': "u",
	'ÿ': "y", 'ý': "y", 'ñ': "n", 'š': "s", 'ž': "z", 'ř': "r",
	'œ': "oe", 'æ': "ae", 'ß': "ss",
}
