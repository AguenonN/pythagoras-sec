package main

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcprogue/servers/internal/rejeu"
)

// Nom est le nom logique du serveur (internal/nommage).
const Nom = "eurlex"

const (
	// URLSparql est le point d'accès SPARQL de CELLAR (Office des
	// publications), sans clé.
	URLSparql = "https://publications.europa.eu/webapi/rdf/sparql"
	// URLCelex résout un numéro CELEX vers la manifestation XHTML de l'acte
	// (négociation de contenu). eur-lex.europa.eu répond 202 à corps vide
	// (protection anti-robots) : on passe par CELLAR.
	URLCelex = "https://publications.europa.eu/resource/celex/"
	// VersionTexte décrit ce que lire_article rend.
	VersionTexte = "texte de l'acte tel que publié au Journal officiel de l'UE (version originale, non consolidée) ; les modifications ultérieures n'y figurent pas, voir rechercher_acte.modifie_par"
)

// DescriptionFiltre figure dans les enregistrements de texte et dans la source
// d'une réponse rejouée.
const DescriptionFiltre = "bloc final de signatures retiré (noms des signataires) ; le texte des articles est intact"

var (
	motifCelex   = regexp.MustCompile(`^[0-9CE][0-9]{4}[A-Z]{1,2}[0-9]{1,6}$`)
	motifArticle = regexp.MustCompile(`^[0-9]{1,4}[a-z]{0,8}$`)
)

// EntreeActe est l'argument de rechercher_acte.
type EntreeActe struct {
	Celex string `json:"celex" jsonschema:"numéro CELEX, par exemple 32022R2554"`
}

// Acte décrit un acte de l'Union.
type Acte struct {
	Celex                string   `json:"celex"`
	Titre                string   `json:"titre"`
	DateDocument         string   `json:"date_document,omitempty"`
	DatePublicationJO    string   `json:"date_publication_jo,omitempty"`
	Statut               string   `json:"statut" jsonschema:"en_vigueur, plus_en_vigueur ou inconnu"`
	DatesEntreeEnVigueur []string `json:"dates_entree_en_vigueur"`
	DateFinValidite      string   `json:"date_fin_validite,omitempty" jsonschema:"9999-12-31 si aucune"`
	ELI                  string   `json:"eli,omitempty"`
	ModifiePar           []string `json:"modifie_par" jsonschema:"numéros CELEX des actes qui le modifient"`
}

// SortieActe est le résultat de rechercher_acte.
type SortieActe struct {
	rejeu.Entete
	Acte *Acte `json:"acte,omitempty"`
}

// EntreeArticle est l'argument de lire_article.
type EntreeArticle struct {
	Celex   string `json:"celex" jsonschema:"numéro CELEX, par exemple 32016R0679"`
	Article string `json:"article" jsonschema:"numéro d'article, par exemple 5 ou 4a"`
}

// SortieArticle est le résultat de lire_article.
type SortieArticle struct {
	rejeu.Entete
	Celex   string `json:"celex,omitempty"`
	Article string `json:"article,omitempty"`
	Titre   string `json:"titre,omitempty"`
	Texte   string `json:"texte,omitempty"`
	Version string `json:"version,omitempty"`
}

// Serveur construit le serveur MCP eurlex.
func Serveur(c *rejeu.Client) *mcp.Server {
	c.Restreindre("publications.europa.eu")
	s := mcp.NewServer(&mcp.Implementation{Name: Nom, Version: "0.1.0"}, nil)
	mcp.AddTool(s, &mcp.Tool{
		Name: "rechercher_acte",
		Description: "Métadonnées d'un acte de l'Union européenne par son numéro CELEX (CELLAR, Office des publications) : " +
			"titre, dates, statut en vigueur, actes modificatifs. " +
			"Le champ source indique si la donnée est obtenue à l'instant (direct) ou lue dans un enregistrement daté (rejeu).",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: new(bool)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in EntreeActe) (*mcp.CallToolResult, SortieActe, error) {
		return rechercher(ctx, c, in.Celex)
	})
	mcp.AddTool(s, &mcp.Tool{
		Name: "lire_article",
		Description: "Texte d'un article d'un acte de l'Union européenne (numéro CELEX et numéro d'article), " +
			"dans la version publiée au Journal officiel, non consolidée. " +
			"Le champ source indique si la donnée est obtenue à l'instant (direct) ou lue dans un enregistrement daté (rejeu).",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: new(bool)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in EntreeArticle) (*mcp.CallToolResult, SortieArticle, error) {
		return lire(ctx, c, in.Celex, in.Article)
	})
	return s
}

func refuse(format string, a ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{rejeu.ErrRequeteRefusee}, a...)...)
}

// requeteActe : la valeur CELEX est validée par motifCelex avant d'être
// insérée, ce qui exclut toute injection SPARQL.
func requeteActe(celex string) string {
	return `PREFIX cdm: <http://publications.europa.eu/ontology/cdm#>
PREFIX xsd: <http://www.w3.org/2001/XMLSchema#>
SELECT ?titre ?date ?enVigueur ?eli ?dateJO ?finValidite
  (GROUP_CONCAT(DISTINCT STR(?eif); separator="|") AS ?entrees)
  (GROUP_CONCAT(DISTINCT STR(?modCelex); separator="|") AS ?modifiePar)
WHERE {
  ?work cdm:resource_legal_id_celex "` + celex + `"^^xsd:string .
  OPTIONAL { ?work cdm:work_date_document ?date }
  OPTIONAL { ?work cdm:resource_legal_in-force ?enVigueur }
  OPTIONAL { ?work cdm:resource_legal_eli ?eli }
  OPTIONAL { ?work cdm:official-journal-act_date_publication ?dateJO }
  OPTIONAL { ?work cdm:resource_legal_date_end-of-validity ?finValidite }
  OPTIONAL { ?work cdm:resource_legal_date_entry-into-force ?eif }
  OPTIONAL { ?mod cdm:resource_legal_amends_resource_legal ?work . ?mod cdm:resource_legal_id_celex ?modCelex }
  OPTIONAL { ?exp cdm:expression_belongs_to_work ?work ;
                  cdm:expression_uses_language <http://publications.europa.eu/resource/authority/language/FRA> ;
                  cdm:expression_title ?titre }
}
GROUP BY ?titre ?date ?enVigueur ?eli ?dateJO ?finValidite
LIMIT 5`
}

func rechercher(ctx context.Context, c *rejeu.Client, celex string) (*mcp.CallToolResult, SortieActe, error) {
	if !motifCelex.MatchString(celex) {
		e, res := rejeu.EnteteErreur(refuse("numéro CELEX %q invalide", celex))
		return res, SortieActe{Entete: e}, nil
	}
	q := url.Values{"query": {requeteActe(celex)}}
	rep, err := c.Obtenir(ctx, rejeu.Requete{
		URL:     URLSparql + "?" + q.Encode(),
		Entetes: map[string][]string{"Accept": {"application/sparql-results+json"}},
	})
	if err != nil {
		e, res := rejeu.EnteteErreur(err)
		return res, SortieActe{Entete: e}, nil
	}
	acte, err := analyserActe(rep.Corps, celex)
	if err != nil {
		e, res := rejeu.EnteteErreur(err)
		e.Source = &rep.Source
		return res, SortieActe{Entete: e}, nil
	}
	return nil, SortieActe{Entete: rejeu.EnteteOK(rep), Acte: acte}, nil
}

func analyserActe(corps []byte, celex string) (*Acte, error) {
	var r struct {
		Results struct {
			Bindings []map[string]struct {
				Value string `json:"value"`
			} `json:"bindings"`
		} `json:"results"`
	}
	if err := json.Unmarshal(corps, &r); err != nil {
		return nil, fmt.Errorf("%w: réponse SPARQL illisible: %v", rejeu.ErrIndisponible, err)
	}
	for _, b := range r.Results.Bindings {
		if b["titre"].Value == "" && b["date"].Value == "" {
			continue // ligne d'agrégat vide : l'acte n'existe pas
		}
		a := &Acte{
			Celex:                celex,
			Titre:                b["titre"].Value,
			DateDocument:         b["date"].Value,
			DatePublicationJO:    b["dateJO"].Value,
			DateFinValidite:      b["finValidite"].Value,
			ELI:                  b["eli"].Value,
			DatesEntreeEnVigueur: liste(b["entrees"].Value),
			ModifiePar:           liste(b["modifiePar"].Value),
		}
		switch b["enVigueur"].Value {
		case "1", "true":
			a.Statut = "en_vigueur"
		case "0", "false":
			a.Statut = "plus_en_vigueur"
		default:
			a.Statut = "inconnu"
		}
		return a, nil
	}
	return nil, fmt.Errorf("%w: aucun acte %s dans CELLAR", rejeu.ErrIntrouvable, celex)
}

func liste(s string) []string {
	out := []string{}
	for _, v := range strings.Split(s, "|") {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func lire(ctx context.Context, c *rejeu.Client, celex, article string) (*mcp.CallToolResult, SortieArticle, error) {
	out := SortieArticle{Celex: celex, Article: article}
	if !motifCelex.MatchString(celex) {
		e, res := rejeu.EnteteErreur(refuse("numéro CELEX %q invalide", celex))
		out.Entete = e
		return res, out, nil
	}
	if !motifArticle.MatchString(article) {
		e, res := rejeu.EnteteErreur(refuse("numéro d'article %q invalide", article))
		out.Entete = e
		return res, out, nil
	}
	rep, err := c.Obtenir(ctx, rejeu.Requete{
		URL:    URLCelex + celex,
		Filtre: &rejeu.Filtre{Description: DescriptionFiltre, Appliquer: retirerSignatures},
		Entetes: map[string][]string{
			"Accept":          {"application/xhtml+xml, text/html;q=0.9"},
			"Accept-Language": {"fra"},
		},
	})
	if err != nil {
		e, res := rejeu.EnteteErreur(err)
		out.Entete = e
		return res, out, nil
	}
	titre, texte, err := extraireArticle(rep.Corps, article)
	if err != nil {
		e, res := rejeu.EnteteErreur(err)
		e.Source = &rep.Source
		out.Entete = e
		return res, out, nil
	}
	out.Entete = rejeu.EnteteOK(rep)
	out.Titre, out.Texte, out.Version = titre, texte, VersionTexte
	return nil, out, nil
}

// extraireArticle lit le XHTML de l'Office des publications et rend le titre
// et le texte de l'élément id="art_<n>" : un paragraphe par ligne, les
// cellules d'une ligne de tableau (« a) », texte) jointes par une espace.
func extraireArticle(corps []byte, article string) (titre, texte string, err error) {
	d := xml.NewDecoder(bytes.NewReader(corps))
	d.Strict = false
	d.AutoClose = xml.HTMLAutoClose
	d.Entity = xml.HTMLEntity
	cible := "art_" + article

	profondeur := 0 // profondeur dans l'article ; 0 = hors de l'article
	var (
		lignes, cellules []string
		para             strings.Builder
		dansP, dansTr    int
		classeP          string
	)
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", "", fmt.Errorf("%w: XHTML illisible: %v", rejeu.ErrIndisponible, err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if profondeur == 0 {
				if attr(t, "id") == cible {
					profondeur = 1
				}
				continue
			}
			profondeur++
			switch t.Name.Local {
			case "p":
				dansP++
				if dansP == 1 {
					para.Reset()
					classeP = attr(t, "class")
				}
			case "tr":
				// Tableau imbriqué : les cellules déjà lues de la ligne
				// englobante (« 1) », consigne) forment une ligne à part,
				// avant les points qu'elle introduit.
				if len(cellules) > 0 {
					lignes = append(lignes, strings.Join(cellules, " "))
					cellules = cellules[:0]
				}
				dansTr++
			case "br":
				para.WriteByte(' ')
			}
		case xml.EndElement:
			if profondeur == 0 {
				continue
			}
			profondeur--
			switch t.Name.Local {
			case "p":
				if dansP == 0 {
					break
				}
				dansP--
				if dansP > 0 {
					break
				}
				l := strings.Join(strings.Fields(para.String()), " ")
				if l == "" {
					break
				}
				if classeP == "oj-sti-art" && titre == "" {
					titre = l
				}
				if dansTr > 0 {
					cellules = append(cellules, l)
				} else {
					lignes = append(lignes, l)
				}
			case "tr":
				if dansTr > 0 {
					dansTr--
				}
				if len(cellules) > 0 {
					lignes = append(lignes, strings.Join(cellules, " "))
					cellules = cellules[:0]
				}
			}
			if profondeur == 0 {
				return titre, strings.Join(lignes, "\n"), nil
			}
		case xml.CharData:
			if profondeur > 0 && dansP > 0 {
				para.Write(t)
			}
		}
	}
	return "", "", fmt.Errorf("%w: article %s absent du texte", rejeu.ErrIntrouvable, article)
}

func attr(t xml.StartElement, nom string) string {
	for _, a := range t.Attr {
		if a.Name.Local == nom {
			return a.Value
		}
	}
	return ""
}

// retirerSignatures ôte du XHTML l'élément <div class="oj-final"> (formule
// finale, lieu, date et noms des signataires) : les enregistrements ne
// versionnent aucun nom de personne physique.
func retirerSignatures(corps []byte) ([]byte, error) {
	ouvre := []byte(`<div class="oj-final"`)
	debut := bytes.Index(corps, ouvre)
	if debut < 0 {
		return nil, errors.New("bloc oj-final introuvable : texte non enregistré par prudence")
	}
	if bytes.Index(corps[debut+len(ouvre):], ouvre) >= 0 {
		return nil, errors.New("plusieurs blocs oj-final : texte non enregistré par prudence")
	}
	niveau := 0
	for i := debut; i < len(corps); {
		switch {
		case bytes.HasPrefix(corps[i:], []byte("<div")):
			niveau++
			i += 4
		case bytes.HasPrefix(corps[i:], []byte("</div>")):
			niveau--
			i += 6
			if niveau == 0 {
				out := make([]byte, 0, len(corps)-(i-debut))
				out = append(out, corps[:debut]...)
				return append(out, corps[i:]...), nil
			}
		default:
			i++
		}
	}
	return nil, errors.New("bloc oj-final non fermé")
}
