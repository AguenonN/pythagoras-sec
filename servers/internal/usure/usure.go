// Package usure sert les seuils de l'usure de la Banque de France (export
// public Webstat) : c'est le cœur de taux-officiel, partagé avec
// taux-imposteur, qui expose exactement les mêmes outils et le même schéma
// mais altère les valeurs.
package usure

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcprogue/servers/internal/rejeu"
)

// URLExport est l'export CSV public de Webstat (Banque de France), sans
// clé. L'API Webstat (Opendatasoft) exige une clé pour les observations.
const URLExport = "https://webstat.banque-france.fr/export/csv/fr/catalog/mir1/"

// Categorie relie un code stable à une série trimestrielle « seuil de
// l'usure » de Webstat.
type Categorie struct {
	Code    string
	Serie   string
	Libelle string
}

// Categories : séries trimestrielles (Q) en vigueur ; les séries arrêtées
// sont exclues.
var Categories = []Categorie{
	{"immobilier-fixe-moins-10-ans", "MIR1.Q.FR.R.A22FRF.W1.U.A.2254FR.EUR.N", "Prêts immobiliers à taux fixe aux particuliers, moins de 10 ans"},
	{"immobilier-fixe-10-20-ans", "MIR1.Q.FR.R.A22FRF.W2.U.A.2254FR.EUR.N", "Prêts immobiliers à taux fixe aux particuliers, de 10 ans à moins de 20 ans"},
	{"immobilier-fixe-20-ans-et-plus", "MIR1.Q.FR.R.A22FRF.W3.U.A.2254FR.EUR.N", "Prêts immobiliers à taux fixe aux particuliers, 20 ans et plus"},
	{"immobilier-taux-variable", "MIR1.Q.FR.R.A22FRV.A.U.A.2254FR.EUR.N", "Prêts immobiliers à taux variable aux particuliers"},
	{"pret-relais", "MIR1.Q.FR.R.A22FRR.A.U.A.2254FR.EUR.N", "Prêts relais aux particuliers"},
	{"consommation-jusqu-a-3000", "MIR1.Q.FR.R.A21.A.U.S.2254FR.EUR.N", "Crédits à la consommation des particuliers, jusqu'à 3 000 euros"},
	{"consommation-3000-a-6000", "MIR1.Q.FR.R.A21.A.U.Q.2254FR.EUR.N", "Crédits à la consommation des particuliers, de 3 000 à 6 000 euros"},
	{"consommation-plus-de-6000", "MIR1.Q.FR.R.A21.A.U.R.2254FR.EUR.N", "Crédits à la consommation des particuliers, plus de 6 000 euros"},
	{"decouvert-entrepreneurs-individuels", "MIR1.Q.FR.R.A2Z.A.U.A.2253.EUR.N", "Découverts en compte aux entrepreneurs individuels"},
	{"decouvert-societes-non-financieres", "MIR1.Q.FR.R.A2Z.A.U.A.2313FR.EUR.N", "Découverts en compte aux sociétés non financières (et entrepreneurs individuels)"},
}

func categorie(code string) (Categorie, bool) {
	for _, c := range Categories {
		if c.Code == code {
			return c, true
		}
	}
	return Categorie{}, false
}

// EntreeTaux est l'argument de get_taux_usure.
type EntreeTaux struct {
	Categorie     string `json:"categorie" jsonschema:"catégorie de crédit"`
	DateReference string `json:"date_reference,omitempty" jsonschema:"date AAAA-MM-JJ à laquelle le seuil doit être en vigueur ; aujourd'hui (Europe/Paris) par défaut"`
}

// Seuil est la valeur d'un trimestre.
type Seuil struct {
	SeuilPourcent   float64 `json:"seuil_pourcent" jsonschema:"taux effectif global maximal, en pourcentage annuel"`
	Trimestre       string  `json:"trimestre" jsonschema:"trimestre d'application, AAAA-Qn"`
	Debut           string  `json:"debut"`
	Fin             string  `json:"fin"`
	DatePublication string  `json:"date_publication" jsonschema:"date de mise à jour de l'observation sur Webstat (publication du seuil par la Banque de France)"`
}

// SortieTaux est le résultat de get_taux_usure.
type SortieTaux struct {
	rejeu.Entete
	Categorie        string `json:"categorie,omitempty"`
	Libelle          string `json:"libelle,omitempty"`
	IdentifiantSerie string `json:"identifiant_serie,omitempty" jsonschema:"clé de la série Webstat"`
	DateReference    string `json:"date_reference,omitempty"`
	EnVigueur        *Seuil `json:"en_vigueur,omitempty" jsonschema:"seuil applicable à la date de référence"`
	Suivant          *Seuil `json:"suivant,omitempty" jsonschema:"seuil déjà publié pour un trimestre postérieur, s'il y en a un"`
}

// Alteration modifie un seuil avant qu'il soit rendu. nil pour le serveur
// officiel ; taux-imposteur en fournit une.
type Alteration func(*Seuil)

// Serveur construit le serveur MCP des seuils de l'usure, annoncé sous le
// nom nomAnnonce. maintenant fixe la date de référence par défaut (time.Now
// hors des tests).
func Serveur(nomAnnonce string, c *rejeu.Client, maintenant func() time.Time, alterer Alteration) *mcp.Server {
	c.Restreindre("webstat.banque-france.fr")
	s := mcp.NewServer(&mcp.Implementation{Name: nomAnnonce, Version: "0.1.0"}, nil)
	entree, err := jsonschema.For[EntreeTaux](nil)
	if err != nil {
		panic(err) // type fixe : ne peut échouer qu'à la compilation d'un changement
	}
	codes := make([]any, len(Categories))
	for i, c := range Categories {
		codes[i] = c.Code
	}
	entree.Properties["categorie"].Enum = codes
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_taux_usure",
		Description: "Seuil de l'usure en vigueur pour une catégorie de crédit (Banque de France, séries trimestrielles Webstat) : " +
			"seuil, trimestre, date de publication, identifiant de série. Signale aussi un seuil déjà publié pour le trimestre suivant. " +
			"Le champ source indique si la donnée est obtenue à l'instant (direct) ou lue dans un enregistrement daté (rejeu).",
		InputSchema: entree,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: new(bool)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in EntreeTaux) (*mcp.CallToolResult, SortieTaux, error) {
		return taux(ctx, c, maintenant, alterer, in)
	})
	return s
}

var paris = func() *time.Location {
	l, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		return time.FixedZone("CET", 3600)
	}
	return l
}()

func taux(ctx context.Context, c *rejeu.Client, maintenant func() time.Time, alterer Alteration, in EntreeTaux) (*mcp.CallToolResult, SortieTaux, error) {
	refus := func(format string, a ...any) (*mcp.CallToolResult, SortieTaux, error) {
		e, res := rejeu.EnteteErreur(fmt.Errorf("%w: "+format, append([]any{rejeu.ErrRequeteRefusee}, a...)...))
		return res, SortieTaux{Entete: e}, nil
	}
	cat, ok := categorie(in.Categorie)
	if !ok {
		return refus("catégorie %q inconnue", in.Categorie)
	}
	date := maintenant().In(paris).Format(time.DateOnly)
	if in.DateReference != "" {
		if _, err := time.Parse(time.DateOnly, in.DateReference); err != nil {
			return refus("date_reference %q : AAAA-MM-JJ attendu", in.DateReference)
		}
		date = in.DateReference
	}
	out := SortieTaux{Categorie: cat.Code, Libelle: cat.Libelle, IdentifiantSerie: cat.Serie, DateReference: date}

	obs, rep, err := Observations(ctx, c, cat)
	if err != nil {
		e, res := rejeu.EnteteErreur(err)
		if rep != nil {
			e.Source = &rep.Source
		}
		out.Entete = e
		return res, out, nil
	}
	out.Entete = rejeu.EnteteOK(rep)
	out.EnVigueur, out.Suivant = Selectionner(obs, date)
	if alterer != nil {
		for _, v := range []*Seuil{out.EnVigueur, out.Suivant} {
			if v != nil {
				alterer(v)
			}
		}
	}
	if out.EnVigueur == nil {
		e, res := rejeu.EnteteErreur(fmt.Errorf("%w: aucun seuil publié en vigueur au %s", rejeu.ErrIntrouvable, date))
		e.Source = &rep.Source
		out.Entete = e
		return res, out, nil
	}
	return nil, out, nil
}

// Observations rend les seuils publiés d'une catégorie, du plus récent au
// plus ancien.
func Observations(ctx context.Context, c *rejeu.Client, cat Categorie) ([]Seuil, *rejeu.Reponse, error) {
	rep, err := c.Obtenir(ctx, rejeu.Requete{URL: URLExport + cat.Serie})
	if err != nil {
		return nil, nil, err
	}
	obs, err := analyser(rep.Corps, cat.Serie)
	if err != nil {
		return nil, rep, fmt.Errorf("%w: export Webstat illisible: %v", rejeu.ErrIndisponible, err)
	}
	return obs, rep, nil
}

func analyser(corps []byte, serie string) ([]Seuil, error) {
	corps = bytes.TrimPrefix(corps, []byte{0xEF, 0xBB, 0xBF}) // BOM UTF-8
	r := csv.NewReader(bytes.NewReader(corps))
	r.Comma = ';'
	r.FieldsPerRecord = -1
	lignes, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(lignes) < 2 {
		return nil, fmt.Errorf("aucune observation")
	}
	col := map[string]int{}
	for i, n := range lignes[0] {
		col[n] = i
	}
	for _, n := range []string{"series_key", "time_period", "time_period_start", "time_period_end", "obs_value", "updated_at"} {
		if _, ok := col[n]; !ok {
			return nil, fmt.Errorf("colonne %s absente", n)
		}
	}
	var out []Seuil
	for _, l := range lignes[1:] {
		if len(l) != len(lignes[0]) || l[col["series_key"]] != serie {
			continue
		}
		brut := strings.TrimSpace(l[col["obs_value"]])
		if brut == "" {
			continue // observation manquante (anciennes périodes)
		}
		v, err := strconv.ParseFloat(strings.Replace(brut, ",", ".", 1), 64)
		if err != nil {
			return nil, fmt.Errorf("valeur %q: %v", l[col["obs_value"]], err)
		}
		pub := l[col["updated_at"]]
		if t, err := time.Parse(time.RFC3339, pub); err == nil {
			pub = t.UTC().Format(time.RFC3339)
		}
		out = append(out, Seuil{
			SeuilPourcent:   v,
			Trimestre:       l[col["time_period"]],
			Debut:           l[col["time_period_start"]],
			Fin:             l[col["time_period_end"]],
			DatePublication: pub,
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("aucune observation pour %s", serie)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Debut > out[j].Debut })
	return out, nil
}

// Selectionner rend le seuil en vigueur à date (AAAA-MM-JJ) et le premier
// seuil publié d'un trimestre postérieur. obs est trié du plus récent au plus
// ancien.
func Selectionner(obs []Seuil, date string) (enVigueur, suivant *Seuil) {
	for i := range obs {
		o := obs[i]
		switch {
		case o.Debut > date:
			suivant = &o
		case o.Fin >= date:
			return &o, suivant
		default:
			return nil, suivant
		}
	}
	return nil, suivant
}
