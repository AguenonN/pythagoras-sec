// Package rejeu enregistre les réponses des API officielles et les rejoue.
//
// Il est distinct de mcprogue/internal/rejeu (le rejeu du journal de
// l'engine) : ici, il s'agit de faire tenir une démo quand une API publique
// tombe, sans jamais cacher que la donnée vient d'un enregistrement.
//
// Trois modes, choisis par la variable d'environnement DONNEES_MODE :
//
//   - DIRECT : appelle l'API et enregistre chaque réponse obtenue ;
//   - REJEU  : ne lit que les enregistrements, aucun accès réseau ;
//   - MIXTE  : appelle l'API et, si elle est indisponible, sert
//     l'enregistrement.
//
// Toute réponse servie depuis un enregistrement porte
// Source.Origine = "rejeu" et la date de l'enregistrement.
package rejeu

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Mode choisit d'où viennent les données.
type Mode string

const (
	Direct Mode = "DIRECT"
	Rejeu  Mode = "REJEU"
	Mixte  Mode = "MIXTE"
)

// VariableMode est la variable d'environnement qui fixe le mode.
const VariableMode = "DONNEES_MODE"

// ModeDefaut s'applique quand la variable est absente : aucun réseau.
const ModeDefaut = Rejeu

// ModeDepuisEnv lit DONNEES_MODE. Absente, elle vaut REJEU ; une valeur
// inconnue est une erreur (un mode mal orthographié ne doit pas ouvrir le
// réseau par surprise).
func ModeDepuisEnv() (Mode, error) {
	v, ok := os.LookupEnv(VariableMode)
	if !ok || v == "" {
		return ModeDefaut, nil
	}
	return ParseMode(v)
}

// ParseMode accepte DIRECT, REJEU ou MIXTE, sans tenir compte de la casse.
func ParseMode(v string) (Mode, error) {
	switch m := Mode(strings.ToUpper(strings.TrimSpace(v))); m {
	case Direct, Rejeu, Mixte:
		return m, nil
	}
	return "", fmt.Errorf("rejeu: mode %q inconnu (DIRECT, REJEU ou MIXTE)", v)
}

// SourceAbsente décrit un résultat sans donnée : origine « aucune », l'URL
// visée et la raison.
func SourceAbsente(urlVisee string, err error) *Source {
	u, e := NettoyerURL(urlVisee)
	if e != nil {
		u = ""
	}
	return &Source{Origine: OrigineAucune, URL: u, Motif: err.Error()}
}

// EcrireSynthetique écrit dans dir un enregistrement fictif pour la requête
// GET url, marqué synthetique. Réservé aux données de test : le rejeu le
// sert avec source.synthetique = true.
func EcrireSynthetique(dir, urlBrute, date string, corps []byte) (string, error) {
	u, err := NettoyerURL(urlBrute)
	if err != nil {
		return "", err
	}
	c := Nouveau(dir, Rejeu)
	cle := Cle(http.MethodGet, u, nil)
	e := Enregistrement{
		Cle:         cle,
		Methode:     http.MethodGet,
		URL:         u,
		Date:        date,
		Statut:      http.StatusOK,
		TypeContenu: "application/json",
		SHA256:      Empreinte(corps),
		Synthetique: true,
		Corps:       string(corps),
	}
	return c.chemin(cle), c.ecrire(cle, &e)
}

// Origines d'une réponse.
const (
	OrigineDirect = "direct"
	OrigineRejeu  = "rejeu"
	// OrigineAucune : aucune donnée n'a été servie (résultat indisponible).
	OrigineAucune = "aucune"
)

// Source décrit d'où vient une réponse. Elle est recopiée telle quelle dans
// le résultat de l'outil.
type Source struct {
	// Origine vaut "direct" (obtenue de l'API pendant ce processus) ou
	// "rejeu" (lue dans un enregistrement).
	Origine string `json:"origine" jsonschema:"direct (obtenue de l'API officielle pendant ce processus), rejeu (lue dans un enregistrement) ou aucune (aucune donnée servie)"`
	// URL appelée, sans secret.
	URL string `json:"url" jsonschema:"URL de l'API officielle appelée"`
	// Date à laquelle l'API a produit cette réponse (RFC 3339, UTC). En
	// rejeu, c'est la date de l'enregistrement.
	Date string `json:"date" jsonschema:"date d'obtention de la réponse auprès de l'API (RFC 3339, UTC) ; en rejeu, date de l'enregistrement"`
	// SHA256 du corps servi.
	SHA256 string `json:"sha256" jsonschema:"empreinte SHA-256 du corps de réponse servi"`
	// Motif du rejeu en mode MIXTE (l'erreur de l'API).
	Motif string `json:"motif,omitempty" jsonschema:"en mode MIXTE, pourquoi l'API n'a pas été utilisée"`
	// Filtre appliqué au corps avant enregistrement, s'il y en a un. Une
	// réponse rejouée filtrée est partielle.
	Filtre string `json:"filtre,omitempty" jsonschema:"filtre appliqué avant enregistrement ; présent, la donnée rejouée est partielle"`
	// Synthetique marque un enregistrement fictif (tests) : jamais une
	// donnée officielle.
	Synthetique bool `json:"synthetique,omitempty" jsonschema:"vrai si la donnée vient d'un enregistrement fictif écrit pour les tests : ce n'est pas une donnée officielle"`
}

// Enregistrement est le contenu d'un fichier du répertoire d'enregistrements.
type Enregistrement struct {
	Cle          string `json:"cle"`
	Methode      string `json:"methode"`
	URL          string `json:"url"`
	CorpsRequete string `json:"corps_requete,omitempty"`
	Date         string `json:"date"`
	Statut       int    `json:"statut"`
	TypeContenu  string `json:"type_contenu,omitempty"`
	// SHA256 du corps enregistré.
	SHA256 string `json:"sha256"`
	// SHA256Origine est l'empreinte du corps reçu, quand un filtre l'a
	// réduit avant enregistrement.
	SHA256Origine string `json:"sha256_origine,omitempty"`
	Filtre        string `json:"filtre,omitempty"`
	Synthetique   bool   `json:"synthetique,omitempty"`
	Corps         string `json:"corps"`
}

// Filtre réduit un corps avant enregistrement, par exemple pour retirer des
// données de personnes physiques. Le corps servi en direct n'est pas filtré.
type Filtre struct {
	Description string
	Appliquer   func([]byte) ([]byte, error)
}

// Requete est un appel à une API.
type Requete struct {
	Methode string // GET par défaut
	URL     string
	Corps   []byte
	// Entetes envoyés mais jamais enregistrés (jetons compris).
	Entetes http.Header
	// Filtre éventuel avant enregistrement.
	Filtre *Filtre
	// NePasEnregistrer empêche l'écriture (réponse portant un secret, ou
	// donnée qu'il ne faut pas versionner). La réponse n'est alors jamais
	// rejouable.
	NePasEnregistrer bool
	// Enregistrable, s'il est fourni, examine le corps reçu : une erreur
	// empêche l'enregistrement (la réponse est servie quand même).
	Enregistrable func([]byte) error
}

// Reponse est ce que le client rend.
type Reponse struct {
	Corps  []byte
	Source Source
}

// Erreurs rendues par Obtenir. Tester avec errors.Is.
var (
	// ErrIndisponible : l'API n'a pas répondu utilement (réseau, 5xx, 429,
	// 401, 403) et aucun enregistrement n'a pu la remplacer, ou le mode
	// REJEU n'a pas d'enregistrement pour cette requête.
	ErrIndisponible = errors.New("indisponible")
	// ErrIntrouvable : l'API a répondu 404.
	ErrIntrouvable = errors.New("introuvable")
	// ErrRequeteRefusee : l'API a répondu 400 ou 422, la requête est en
	// cause, pas l'API.
	ErrRequeteRefusee = errors.New("requete refusee")
)

// Client appelle une API, enregistre et rejoue. Il est sûr en concurrence.
// Un Client par API : le débit est limité par Client.
type Client struct {
	mode Mode
	dir  string

	HTTP *http.Client
	// Intervalle minimal entre deux requêtes réseau (1 s par défaut).
	Intervalle time.Duration
	// Tentatives réseau au plus (3 par défaut), avec attente croissante à
	// partir de Attente (1 s par défaut).
	Tentatives int
	Attente    time.Duration
	// DureeMemo : durée pendant laquelle une réponse obtenue est resservie
	// sans nouvel appel (10 min par défaut).
	DureeMemo time.Duration
	// UserAgent envoyé à l'API.
	UserAgent string
	// Maintenant remplace l'horloge dans les tests.
	Maintenant func() time.Time
	// Journal reçoit les avertissements (échec d'écriture d'un
	// enregistrement). os.Stderr par défaut ; jamais stdout, qui porte MCP.
	Journal io.Writer

	hotes map[string]bool

	mu      sync.Mutex
	dernier time.Time
	memo    map[string]memo
}

type memo struct {
	rep    Reponse
	expire time.Time
}

// TailleMax borne le corps lu.
const TailleMax = 64 << 20

// Nouveau crée un client. dir est le répertoire d'enregistrements du serveur.
func Nouveau(dir string, mode Mode) *Client {
	return &Client{
		mode:       mode,
		dir:        dir,
		HTTP:       &http.Client{Timeout: 30 * time.Second},
		Intervalle: time.Second,
		Tentatives: 3,
		Attente:    time.Second,
		DureeMemo:  10 * time.Minute,
		UserAgent:  "mcp_rogue-donnees/0.1",
		Maintenant: time.Now,
		Journal:    os.Stderr,
		memo:       map[string]memo{},
	}
}

// Restreindre limite le client à ces hôtes, redirections comprises ; une
// redirection vers http:// sur un hôte autorisé est passée en https://. Il
// est obligatoire : sans appel à Restreindre, le client ne joint aucun hôte
// (le mode REJEU, qui n'accède pas au réseau, n'est pas concerné).
func (c *Client) Restreindre(hotes ...string) {
	c.hotes = map[string]bool{}
	for _, h := range hotes {
		c.hotes[strings.ToLower(h)] = true
	}
	c.HTTP.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("trop de redirections")
		}
		if !c.hotes[strings.ToLower(req.URL.Hostname())] {
			return fmt.Errorf("redirection vers %s refusée (hôte non autorisé)", req.URL.Hostname())
		}
		if req.URL.Scheme == "http" {
			req.URL.Scheme = "https"
		}
		return nil
	}
}

// Mode rend le mode du client.
func (c *Client) Mode() Mode { return c.mode }

// Obtenir sert la requête selon le mode.
func (c *Client) Obtenir(ctx context.Context, r Requete) (*Reponse, error) {
	if r.Methode == "" {
		r.Methode = http.MethodGet
	}
	urlPropre, err := NettoyerURL(r.URL)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRequeteRefusee, err)
	}
	cle := Cle(r.Methode, urlPropre, r.Corps)

	if c.mode == Rejeu {
		return c.lire(cle, "")
	}
	// Fermé par défaut : sans Restreindre, aucun hôte n'est joignable.
	u, _ := url.Parse(r.URL)
	if !c.hotes[strings.ToLower(u.Hostname())] {
		if c.hotes == nil {
			return nil, fmt.Errorf("%w: client sans liste d'hôtes (Restreindre non appelé), aucun hôte n'est joignable", ErrRequeteRefusee)
		}
		return nil, fmt.Errorf("%w: hôte %s non autorisé", ErrRequeteRefusee, u.Hostname())
	}

	if rep, ok := c.memoire(cle); ok {
		return rep, nil
	}
	rep, err := c.appeler(ctx, r, urlPropre, cle)
	if err == nil {
		c.retenir(cle, rep)
		return rep, nil
	}
	if c.mode == Mixte && errors.Is(err, ErrIndisponible) {
		if rej, errRejeu := c.lire(cle, err.Error()); errRejeu == nil {
			return rej, nil
		}
	}
	return nil, err
}

// Relire sert l'enregistrement de la requête sans appeler l'API, avec le
// motif donné. Pour le mode MIXTE quand une étape préalable (jeton OAuth) a
// échoué avant même l'appel.
func (c *Client) Relire(r Requete, motif string) (*Reponse, error) {
	if r.Methode == "" {
		r.Methode = http.MethodGet
	}
	urlPropre, err := NettoyerURL(r.URL)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRequeteRefusee, err)
	}
	return c.lire(Cle(r.Methode, urlPropre, r.Corps), motif)
}

// Cle identifie une requête : méthode, URL sans secret et corps.
func Cle(methode, urlPropre string, corps []byte) string {
	h := sha256.New()
	h.Write([]byte(methode))
	h.Write([]byte{'\n'})
	h.Write([]byte(urlPropre))
	h.Write([]byte{'\n'})
	h.Write(corps)
	return hex.EncodeToString(h.Sum(nil))
}

// Empreinte est le SHA-256 hexadécimal d'un corps.
func Empreinte(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// motsSecrets repèrent les paramètres de requête à ne jamais enregistrer.
var motsSecrets = []string{"key", "token", "secret", "password", "passwd", "auth", "signature", "sig"}

// NettoyerURL retire l'identité et les paramètres portant un secret.
func NettoyerURL(brute string) (string, error) {
	u, err := url.Parse(brute)
	if err != nil {
		return "", err
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", fmt.Errorf("schéma %q refusé", u.Scheme)
	}
	u.User = nil
	u.Fragment = ""
	q := u.Query()
	for nom := range q {
		n := strings.ToLower(nom)
		for _, m := range motsSecrets {
			if strings.Contains(n, m) {
				q.Del(nom)
				break
			}
		}
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (c *Client) chemin(cle string) string {
	return filepath.Join(c.dir, cle[:24]+".json")
}

func (c *Client) lire(cle, motif string) (*Reponse, error) {
	b, err := os.ReadFile(c.chemin(cle))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if motif != "" {
				return nil, fmt.Errorf("%w: %s ; aucun enregistrement pour cette requête", ErrIndisponible, motif)
			}
			return nil, fmt.Errorf("%w: mode %s et aucun enregistrement pour cette requête", ErrIndisponible, c.mode)
		}
		return nil, fmt.Errorf("%w: enregistrement illisible: %v", ErrIndisponible, err)
	}
	var e Enregistrement
	if err := json.Unmarshal(b, &e); err != nil {
		return nil, fmt.Errorf("%w: enregistrement corrompu: %v", ErrIndisponible, err)
	}
	corps := []byte(e.Corps)
	if e.Cle != cle || Empreinte(corps) != e.SHA256 {
		return nil, fmt.Errorf("%w: enregistrement %s altéré (clé ou empreinte)", ErrIndisponible, filepath.Base(c.chemin(cle)))
	}
	return &Reponse{
		Corps: corps,
		Source: Source{
			Origine:     OrigineRejeu,
			URL:         e.URL,
			Date:        e.Date,
			SHA256:      e.SHA256,
			Motif:       motif,
			Filtre:      e.Filtre,
			Synthetique: e.Synthetique,
		},
	}, nil
}

func (c *Client) memoire(cle string) (*Reponse, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	m, ok := c.memo[cle]
	if !ok || c.Maintenant().After(m.expire) {
		return nil, false
	}
	rep := m.rep
	return &rep, true
}

func (c *Client) retenir(cle string, rep *Reponse) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.memo[cle] = memo{rep: *rep, expire: c.Maintenant().Add(c.DureeMemo)}
}

// attendreTour respecte l'intervalle minimal entre deux requêtes.
func (c *Client) attendreTour(ctx context.Context) error {
	c.mu.Lock()
	prochain := c.dernier.Add(c.Intervalle)
	now := c.Maintenant()
	if prochain.Before(now) {
		prochain = now
	}
	c.dernier = prochain
	c.mu.Unlock()
	return dormir(ctx, prochain.Sub(now))
}

func dormir(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (c *Client) appeler(ctx context.Context, r Requete, urlPropre, cle string) (*Reponse, error) {
	var derniere error
	for i := 0; i < c.Tentatives; i++ {
		if i > 0 {
			if err := dormir(ctx, c.Attente*time.Duration(1<<(i-1))); err != nil {
				return nil, fmt.Errorf("%w: %v", ErrIndisponible, err)
			}
		}
		if err := c.attendreTour(ctx); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrIndisponible, err)
		}
		statut, typ, corps, err := c.une(ctx, r)
		switch {
		case err != nil:
			derniere = err
			continue
		case statut >= 200 && statut < 300:
			return c.recu(r, urlPropre, cle, statut, typ, corps), nil
		case statut == http.StatusNotFound:
			return nil, fmt.Errorf("%w: HTTP 404", ErrIntrouvable)
		case statut == http.StatusBadRequest || statut == http.StatusUnprocessableEntity:
			return nil, fmt.Errorf("%w: HTTP %d", ErrRequeteRefusee, statut)
		case statut == http.StatusTooManyRequests || statut >= 500:
			derniere = fmt.Errorf("HTTP %d", statut)
			continue
		default:
			// 401, 403 et le reste : réessayer n'y changera rien.
			return nil, fmt.Errorf("%w: HTTP %d", ErrIndisponible, statut)
		}
	}
	return nil, fmt.Errorf("%w: %v après %d tentatives", ErrIndisponible, derniere, c.Tentatives)
}

func (c *Client) une(ctx context.Context, r Requete) (int, string, []byte, error) {
	var corps io.Reader
	if r.Corps != nil {
		corps = bytes.NewReader(r.Corps)
	}
	req, err := http.NewRequestWithContext(ctx, r.Methode, r.URL, corps)
	if err != nil {
		return 0, "", nil, err
	}
	for k, vs := range r.Entetes {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, "", nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, TailleMax+1))
	if err != nil {
		return 0, "", nil, err
	}
	if len(b) > TailleMax {
		return 0, "", nil, fmt.Errorf("corps de plus de %d octets", TailleMax)
	}
	return resp.StatusCode, resp.Header.Get("Content-Type"), b, nil
}

// recu construit la réponse directe et l'enregistre. Un échec d'écriture
// n'empêche pas de servir la réponse ; il est signalé sur le journal.
func (c *Client) recu(r Requete, urlPropre, cle string, statut int, typ string, corps []byte) *Reponse {
	date := c.Maintenant().UTC().Format(time.RFC3339)
	rep := &Reponse{
		Corps: corps,
		Source: Source{
			Origine: OrigineDirect,
			URL:     urlPropre,
			Date:    date,
			SHA256:  Empreinte(corps),
		},
	}
	if r.NePasEnregistrer {
		return rep
	}
	if r.Enregistrable != nil {
		if err := r.Enregistrable(corps); err != nil {
			fmt.Fprintf(c.Journal, "rejeu: réponse de %s non enregistrée: %v\n", urlPropre, err)
			return rep
		}
	}
	e := Enregistrement{
		Cle:          cle,
		Methode:      r.Methode,
		URL:          urlPropre,
		CorpsRequete: string(r.Corps),
		Date:         date,
		Statut:       statut,
		TypeContenu:  typ,
	}
	stocke := corps
	if r.Filtre != nil {
		f, err := r.Filtre.Appliquer(corps)
		if err != nil {
			fmt.Fprintf(c.Journal, "rejeu: filtre %q en échec, réponse non enregistrée: %v\n", r.Filtre.Description, err)
			return rep
		}
		stocke = f
		e.Filtre = r.Filtre.Description
		e.SHA256Origine = Empreinte(corps)
	}
	e.Corps = string(stocke)
	e.SHA256 = Empreinte(stocke)
	if err := c.ecrire(cle, &e); err != nil {
		fmt.Fprintf(c.Journal, "rejeu: enregistrement de %s impossible: %v\n", urlPropre, err)
	}
	return rep
}

func (c *Client) ecrire(cle string, e *Enregistrement) error {
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	tmp, err := os.CreateTemp(c.dir, ".enregistrement-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), c.chemin(cle))
}
