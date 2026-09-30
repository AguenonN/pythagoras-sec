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

// HoteRegistre est le seul hôte que le client peut joindre.
const HoteRegistre = "gels-avoirs.dgtresor.gouv.fr"

// URLRegistre est la dernière publication du registre national des gels,
// DG Trésor, au format JSON.
const URLRegistre = "https://gels-avoirs.dgtresor.gouv.fr/ApiPublic/api/v1/publication/derniere-publication-flux-json"

// MaxCorrespondances borne la liste rendue ; le total est toujours donné.
const MaxCorrespondances = 20

// DossierLocal est le répertoire par défaut du registre complet enregistré
// (personnes physiques comprises). Il est ignoré par Git : seul
// enregistrements/ (registre synthétique des tests) est versionné.
const DossierLocal = "servers/gels-avoirs/enregistrements_locaux"

// EntreeVerification est l'argument de verifier_gel.
type EntreeVerification struct {
	Nom string `json:"nom" jsonschema:"nom de la personne morale, de la personne physique ou du navire à filtrer"`
}

// Correspondance est une entrée du registre qui correspond au nom.
//
// Pour une personne physique, seuls la correspondance, l'identifiant du
// registre, la nature, les fondements juridiques et les références UE / ONU
// sont rendus : ni nom, ni alias, ni aucune autre donnée d'identité.
type Correspondance struct {
	IDRegistre    int      `json:"id_registre"`
	Nature        string   `json:"nature" jsonschema:"Personne morale, Personne physique ou Navire"`
	Nom           string   `json:"nom,omitempty" jsonschema:"nom au registre ; jamais rendu pour une personne physique"`
	AliasTrouve   string   `json:"alias_trouve,omitempty" jsonschema:"alias du registre qui a correspondu, si ce n'est pas le nom ; jamais rendu pour une personne physique"`
	Type          string   `json:"type" jsonschema:"exacte (mêmes mots) ou partielle (tous les mots cherchés présents)"`
	Fondements    []string `json:"fondements_juridiques"`
	ReferencesUE  []string `json:"references_ue,omitempty"`
	ReferencesONU []string `json:"references_onu,omitempty"`
}

// natureNommee : les seules natures dont le nom est rendu. Toute autre
// nature, connue ou non, est traitée comme une personne physique (fermé par
// défaut).
var natureNommee = map[string]bool{"Personne morale": true, "Navire": true}

// SortieVerification est le résultat de verifier_gel.
//
// date_publication_registre et source figurent dans tout résultat. Hors du
// statut ok, nombre_correspondances et correspondances sont absents : un
// registre indisponible ne se lit jamais comme « aucune correspondance ».
type SortieVerification struct {
	rejeu.Entete
	DatePublicationRegistre string            `json:"date_publication_registre" jsonschema:"date de la publication du registre consultée ; vide si aucun registre n'a pu être lu"`
	NombreCorrespondances   *int              `json:"nombre_correspondances,omitempty" jsonschema:"présent seulement si le statut est ok"`
	Correspondances         *[]Correspondance `json:"correspondances,omitempty" jsonschema:"présent seulement si le statut est ok (liste vide s'il n'y a aucune correspondance)"`
	Avertissement           string            `json:"avertissement,omitempty"`
}

// Serveur construit le serveur MCP gels-avoirs.
func Serveur(c *rejeu.Client) *mcp.Server {
	c.Restreindre(HoteRegistre)
	s := mcp.NewServer(&mcp.Implementation{Name: Nom, Version: "0.1.0"}, nil)
	idx := &index{}
	mcp.AddTool(s, &mcp.Tool{
		Name: "verifier_gel",
		Description: "Cherche un nom dans le registre national des gels des avoirs (DG Trésor) : " +
			"correspondances exactes ou partielles, nature, fondements juridiques, et date de publication du registre. " +
			"Une correspondance est un signal à examiner, pas une identification. " +
			"Le champ source indique si le registre est obtenu à l'instant (direct) ou lu dans un enregistrement daté (rejeu).",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: new(bool)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in EntreeVerification) (*mcp.CallToolResult, SortieVerification, error) {
		return verifier(ctx, c, idx, in.Nom)
	})
	return s
}

func verifier(ctx context.Context, c *rejeu.Client, idx *index, nom string) (*mcp.CallToolResult, SortieVerification, error) {
	echec := func(err error, rep *rejeu.Reponse) (*mcp.CallToolResult, SortieVerification, error) {
		e, res := rejeu.EnteteErreur(err)
		if rep != nil {
			e.Source = &rep.Source
		} else {
			e.Source = rejeu.SourceAbsente(URLRegistre, err)
		}
		return res, SortieVerification{Entete: e}, nil
	}
	cherche := mots(nom)
	if len(cherche) == 0 {
		return echec(fmt.Errorf("%w: nom vide", rejeu.ErrRequeteRefusee), nil)
	}
	rep, err := c.Obtenir(ctx, rejeu.Requete{
		URL:     URLRegistre,
		Entetes: map[string][]string{"Accept": {"application/json"}},
	})
	if err != nil {
		return echec(err, nil)
	}
	reg, err := idx.charger(rep)
	if err != nil {
		return echec(err, rep)
	}
	trouve := reg.chercher(cherche)
	n := len(trouve)
	out := SortieVerification{
		Entete:                  rejeu.EnteteOK(rep),
		DatePublicationRegistre: reg.date,
		NombreCorrespondances:   &n,
	}
	if n > MaxCorrespondances {
		trouve = trouve[:MaxCorrespondances]
	}
	out.Correspondances = &trouve
	if rep.Source.Synthetique {
		out.Avertissement = "registre SYNTHÉTIQUE (données fictives de test) : ni une correspondance ni son absence ne disent quoi que ce soit du registre officiel"
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
				case "REFERENCE_ONU":
					if l := chaine(v["ReferenceOnu"]); l != "" {
						f.refsONU = append(f.refsONU, l)
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
	refsONU    []string
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
		c := Correspondance{
			IDRegistre:    f.IDRegistre,
			Nature:        f.Nature,
			Type:          meilleur,
			Fondements:    fond,
			ReferencesUE:  f.refsUE,
			ReferencesONU: f.refsONU,
		}
		if natureNommee[f.Nature] {
			c.Nom, c.AliasTrouve = f.Nom, alias
		}
		out = append(out, c)
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
