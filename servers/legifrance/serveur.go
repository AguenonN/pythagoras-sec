package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcprogue/servers/internal/rejeu"
)

// Nom est le nom logique du serveur (internal/nommage).
const Nom = "legifrance"

const (
	// URLJeton : OAuth client_credentials de PISTE (production).
	URLJeton = "https://oauth.piste.gouv.fr/api/oauth/token"
	// URLArticle : consultation d'un article par identifiant, API
	// Légifrance (DILA) via PISTE, point d'accès documenté
	// POST /consult/getArticle.
	URLArticle = "https://api.piste.gouv.fr/dila/legifrance/lf-engine-app/consult/getArticle"
)

var motifIdentifiant = regexp.MustCompile(`^LEGIARTI[0-9]{12}$`)

// EntreeArticle est l'argument de lire_article.
type EntreeArticle struct {
	Identifiant   string `json:"identifiant" jsonschema:"identifiant Légifrance d'une version d'article, LEGIARTI suivi de 12 chiffres"`
	DateReference string `json:"date_reference,omitempty" jsonschema:"date AAAA-MM-JJ à laquelle le texte doit être en vigueur ; aujourd'hui (Europe/Paris) par défaut"`
}

// Version est une version d'article.
type Version struct {
	Identifiant string `json:"identifiant"`
	Version     string `json:"version"`
	Etat        string `json:"etat" jsonschema:"état Légifrance : VIGUEUR, ABROGE_DIFF (en vigueur, abrogé à une date future), VIGUEUR_DIFF (en vigueur à une date future), MODIFIE, ABROGE…"`
	DateDebut   string `json:"date_debut"`
	DateFin     string `json:"date_fin" jsonschema:"2999-01-01 si aucune fin prévue"`
}

// SortieArticle est le résultat de lire_article.
type SortieArticle struct {
	rejeu.Entete
	IdentifiantDemande string    `json:"identifiant_demande,omitempty"`
	DateReference      string    `json:"date_reference,omitempty"`
	Numero             string    `json:"numero,omitempty" jsonschema:"numéro de l'article, par exemple L314-6"`
	Texte              string    `json:"texte_source,omitempty" jsonschema:"code ou texte qui contient l'article"`
	EnVigueur          *Version  `json:"en_vigueur,omitempty" jsonschema:"version servie : celle en vigueur à la date de référence"`
	Contenu            string    `json:"contenu,omitempty" jsonschema:"texte de la version servie"`
	Suivante           *Version  `json:"suivante,omitempty" jsonschema:"version postérieure déjà publiée, s'il y en a une"`
	Versions           []Version `json:"versions,omitempty"`
}

// Jeton obtient et garde le jeton OAuth PISTE. Il n'est jamais enregistré ni
// journalisé.
type Jeton struct {
	HTTP       *http.Client
	Maintenant func() time.Time

	mu     sync.Mutex
	valeur string
	expire time.Time
}

// Obtenir rend un jeton valide, en le renouvelant une minute avant expiration.
func (j *Jeton) Obtenir(ctx context.Context) (string, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.valeur != "" && j.Maintenant().Before(j.expire) {
		return j.valeur, nil
	}
	id, secret := os.Getenv("PISTE_CLIENT_ID"), os.Getenv("PISTE_CLIENT_SECRET")
	if id == "" || secret == "" {
		return "", errors.New("PISTE_CLIENT_ID ou PISTE_CLIENT_SECRET absent de l'environnement")
	}
	corps := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {id},
		"client_secret": {secret},
		"scope":         {"openid"},
	}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, URLJeton, strings.NewReader(corps))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := j.HTTP.Do(req)
	if err != nil {
		// L'erreur de transport cite l'URL, jamais le corps.
		return "", fmt.Errorf("OAuth PISTE: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// Le corps d'une erreur OAuth n'est pas repris : il pourrait
		// refléter la requête.
		return "", fmt.Errorf("OAuth PISTE: HTTP %d", resp.StatusCode)
	}
	var r struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil || r.AccessToken == "" {
		return "", errors.New("OAuth PISTE: réponse sans jeton")
	}
	if r.ExpiresIn <= 120 {
		r.ExpiresIn = 120
	}
	j.valeur = r.AccessToken
	j.expire = j.Maintenant().Add(time.Duration(r.ExpiresIn-60) * time.Second)
	return j.valeur, nil
}

// Serveur construit le serveur MCP legifrance. jeton peut être nil en mode
// REJEU.
func Serveur(c *rejeu.Client, jeton *Jeton, maintenant func() time.Time) *mcp.Server {
	c.Restreindre("api.piste.gouv.fr")
	s := mcp.NewServer(&mcp.Implementation{Name: Nom, Version: "0.1.0"}, nil)
	mcp.AddTool(s, &mcp.Tool{
		Name: "lire_article",
		Description: "Texte d'un article de loi ou de règlement français (Légifrance, DILA) par son identifiant LEGIARTI : " +
			"texte de la version en vigueur à la date de référence, numéro de version, dates de début et de fin, " +
			"et version suivante déjà publiée. " +
			"Le champ source indique si la donnée est obtenue à l'instant (direct) ou lue dans un enregistrement daté (rejeu).",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: new(bool)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in EntreeArticle) (*mcp.CallToolResult, SortieArticle, error) {
		return lire(ctx, c, jeton, maintenant, in)
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

func lire(ctx context.Context, c *rejeu.Client, jeton *Jeton, maintenant func() time.Time, in EntreeArticle) (*mcp.CallToolResult, SortieArticle, error) {
	out := SortieArticle{IdentifiantDemande: in.Identifiant}
	echec := func(err error, src *rejeu.Source) (*mcp.CallToolResult, SortieArticle, error) {
		e, res := rejeu.EnteteErreur(err)
		if src != nil {
			e.Source = src
		}
		out.Entete = e
		return res, out, nil
	}
	if !motifIdentifiant.MatchString(in.Identifiant) {
		return echec(fmt.Errorf("%w: identifiant %q : LEGIARTI suivi de 12 chiffres attendu", rejeu.ErrRequeteRefusee, in.Identifiant), nil)
	}
	date := maintenant().In(paris).Format(time.DateOnly)
	if in.DateReference != "" {
		if _, err := time.Parse(time.DateOnly, in.DateReference); err != nil {
			return echec(fmt.Errorf("%w: date_reference %q : AAAA-MM-JJ attendu", rejeu.ErrRequeteRefusee, in.DateReference), nil)
		}
		date = in.DateReference
	}
	out.DateReference = date

	art, rep, err := article(ctx, c, jeton, in.Identifiant)
	if err != nil {
		var src *rejeu.Source
		if rep != nil {
			src = &rep.Source
		}
		return echec(err, src)
	}
	out.Versions = art.versions()
	enVigueur, suivante := choisir(out.Versions, date)
	if enVigueur == nil {
		return echec(fmt.Errorf("%w: aucune version de l'article en vigueur au %s", rejeu.ErrIntrouvable, date), &rep.Source)
	}
	out.Suivante = suivante
	// La version demandée n'est pas celle en vigueur : lire la bonne.
	if enVigueur.Identifiant != art.ID {
		art, rep, err = article(ctx, c, jeton, enVigueur.Identifiant)
		if err != nil {
			var src *rejeu.Source
			if rep != nil {
				src = &rep.Source
			}
			return echec(fmt.Errorf("version en vigueur %s: %w", enVigueur.Identifiant, err), src)
		}
	}
	out.Entete = rejeu.EnteteOK(rep)
	out.Numero = art.Num
	if len(art.TextTitles) > 0 {
		out.Texte = art.TextTitles[0].Titre
	}
	v := art.version()
	out.EnVigueur = &v
	out.Contenu = texteBrut(art.TexteHTML)
	if out.Contenu == "" {
		out.Contenu = strings.TrimSpace(art.Texte)
	}
	return nil, out, nil
}

// Format de la réponse getArticle (champs utilisés).
type articleAPI struct {
	ID             string `json:"id"`
	Num            string `json:"num"`
	Etat           string `json:"etat"`
	VersionArticle string `json:"versionArticle"`
	DateDebut      int64  `json:"dateDebut"`
	DateFin        int64  `json:"dateFin"`
	Texte          string `json:"texte"`
	TexteHTML      string `json:"texteHtml"`
	TextTitles     []struct {
		Titre string `json:"titre"`
	} `json:"textTitles"`
	ArticleVersions []struct {
		ID        string `json:"id"`
		Etat      string `json:"etat"`
		Version   string `json:"version"`
		DateDebut int64  `json:"dateDebut"`
		DateFin   int64  `json:"dateFin"`
	} `json:"articleVersions"`
}

func (a *articleAPI) version() Version {
	return Version{Identifiant: a.ID, Version: a.VersionArticle, Etat: a.Etat, DateDebut: jour(a.DateDebut), DateFin: jour(a.DateFin)}
}

func (a *articleAPI) versions() []Version {
	out := make([]Version, 0, len(a.ArticleVersions))
	for _, v := range a.ArticleVersions {
		out = append(out, Version{Identifiant: v.ID, Version: v.Version, Etat: v.Etat, DateDebut: jour(v.DateDebut), DateFin: jour(v.DateFin)})
	}
	if len(out) == 0 {
		out = append(out, a.version())
	}
	return out
}

// jour : Légifrance date en millisecondes depuis l'époque, minuit UTC.
func jour(ms int64) string {
	return time.UnixMilli(ms).UTC().Format(time.DateOnly)
}

// etatsApplicables : états d'une version qui peut s'appliquer sur sa période
// [debut, fin[. MODIFIE_MORT_NE, ANNULE, etc. ne s'appliquent jamais.
var etatsApplicables = map[string]bool{
	"VIGUEUR": true, "VIGUEUR_DIFF": true, "ABROGE_DIFF": true,
	"MODIFIE": true, "ABROGE": true, "PERIME": true, "TRANSFERE": true,
	"VIGUEUR_ETEN": true, "VIGUEUR_NON_ETEN": true,
}

// choisir rend la version applicable à date (debut <= date < fin) et la
// première version applicable qui commence après date.
func choisir(vs []Version, date string) (enVigueur, suivante *Version) {
	for i := range vs {
		v := vs[i]
		if !etatsApplicables[v.Etat] || v.DateFin <= v.DateDebut {
			continue
		}
		switch {
		case v.DateDebut <= date && date < v.DateFin:
			enVigueur = &v
		case v.DateDebut > date && (suivante == nil || v.DateDebut < suivante.DateDebut):
			suivante = &v
		}
	}
	return enVigueur, suivante
}

func article(ctx context.Context, c *rejeu.Client, jeton *Jeton, id string) (*articleAPI, *rejeu.Reponse, error) {
	req := rejeu.Requete{
		Methode: http.MethodPost,
		URL:     URLArticle,
		Corps:   []byte(`{"id":"` + id + `"}`), // id validé par motifIdentifiant
		Entetes: http.Header{"Content-Type": {"application/json"}, "Accept": {"application/json"}},
	}
	var rep *rejeu.Reponse
	var err error
	switch {
	case c.Mode() == rejeu.Rejeu:
		rep, err = c.Obtenir(ctx, req)
	case jeton == nil:
		err = fmt.Errorf("%w: aucun jeton OAuth configuré", rejeu.ErrIndisponible)
	default:
		var j string
		j, err = jeton.Obtenir(ctx)
		if err != nil {
			err = fmt.Errorf("%w: %v", rejeu.ErrIndisponible, err)
			if c.Mode() == rejeu.Mixte {
				if r, e := c.Relire(req, err.Error()); e == nil {
					rep, err = r, nil
				}
			}
			break
		}
		req.Entetes.Set("Authorization", "Bearer "+j)
		rep, err = c.Obtenir(ctx, req)
	}
	if err != nil {
		return nil, nil, err
	}
	var r struct {
		Article *articleAPI `json:"article"`
	}
	if err := json.Unmarshal(rep.Corps, &r); err != nil {
		return nil, rep, fmt.Errorf("%w: réponse Légifrance illisible: %v", rejeu.ErrIndisponible, err)
	}
	if r.Article == nil {
		return nil, rep, fmt.Errorf("%w: aucun article %s", rejeu.ErrIntrouvable, id)
	}
	return r.Article, rep, nil
}

var (
	baliseFinBloc = regexp.MustCompile(`(?i)</p>|<br\s*/?>|</li>|</div>`)
	balise        = regexp.MustCompile(`<[^>]*>`)
)

// texteBrut convertit le HTML Légifrance en texte : un paragraphe par ligne.
func texteBrut(h string) string {
	h = baliseFinBloc.ReplaceAllString(h, "\n")
	h = html.UnescapeString(balise.ReplaceAllString(h, ""))
	var lignes []string
	for _, l := range strings.Split(h, "\n") {
		if l = strings.Join(strings.Fields(l), " "); l != "" {
			lignes = append(lignes, l)
		}
	}
	return strings.Join(lignes, "\n")
}
