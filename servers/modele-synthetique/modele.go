// Package modele est le moteur commun des serveurs MCP de données
// synthétiques créés par scripts/nouveau-serveur.sh. Un serveur ne contient
// aucun code propre : ses outils sont déclarés dans outils.json, ses données
// FICTIVES dans donnees.json, tous deux embarqués dans le binaire.
//
// Trois genres d'outils :
//
//   - lire   : rend l'enregistrement d'une collection dont les champs valent
//     les arguments (lecture, readOnlyHint vrai) ;
//   - lister : rend les enregistrements qui satisfont les filtres donnés en
//     arguments, tous s'il n'y en a pas (lecture, readOnlyHint vrai) ;
//   - action : enregistre une action EN MÉMOIRE (rien ne survit au processus),
//     rend une référence <préfixe>-0001, <préfixe>-0002… (readOnlyHint faux :
//     l'outil est classé « action » au registre).
//
// Les noms d'outils, d'arguments et les types sont validés au chargement :
// une erreur nomme l'outil et la cause ; rien ne démarre sur un fichier
// invalide.
package modele

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcprogue/internal/nommage"
)

// Genres d'outils.
const (
	GenreLire   = "lire"
	GenreLister = "lister"
	GenreAction = "action"
)

// Statuts rendus.
const (
	StatutOK          = "ok"
	StatutIntrouvable = "introuvable"
	StatutRefuse      = "requete_refusee"
	StatutEnregistre  = "enregistre"
)

// LimiteListe est le nombre maximal d'enregistrements d'un résultat « lister ».
const LimiteListe = 50

// Outils est le contenu d'outils.json.
type Outils struct {
	Description string  `json:"description,omitempty"`
	Outils      []Outil `json:"outils"`
}

// Outil déclare un outil.
type Outil struct {
	Nom         string `json:"nom"`
	Description string `json:"description"`
	Genre       string `json:"genre"`
	// ReadOnlyHint est obligatoire et doit être cohérent avec le genre :
	// vrai pour lire et lister, faux pour action (un outil sans cette
	// annotation vraie est classé « action », fermé par défaut).
	ReadOnlyHint *bool `json:"readOnlyHint"`
	// Collection : lire et lister cherchent dans cette collection ; action
	// peut s'y référer par Verifie.
	Collection string     `json:"collection,omitempty"`
	Arguments  []Argument `json:"arguments,omitempty"`
	// ReferencePrefixe : action seulement, par exemple "DEPOT-FICTIF".
	ReferencePrefixe string `json:"reference_prefixe,omitempty"`
	// Verifie : action seulement ; l'action n'est enregistrée que si chaque
	// argument désigné existe dans la collection.
	Verifie []Verification `json:"verifie,omitempty"`
}

// Argument déclare un argument d'outil.
type Argument struct {
	Nom         string `json:"nom"`
	Type        string `json:"type"` // string, integer, number, boolean, liste (de chaînes)
	Description string `json:"description"`
	Obligatoire bool   `json:"obligatoire,omitempty"`
	// Champ : champ des enregistrements comparé à l'argument (lire, lister) ;
	// défaut : le nom de l'argument.
	Champ string `json:"champ,omitempty"`
}

// Verification : l'argument doit désigner un enregistrement existant.
type Verification struct {
	Argument   string `json:"argument"`
	Collection string `json:"collection"`
	Champ      string `json:"champ"`
}

// Donnees est le contenu de donnees.json.
type Donnees struct {
	Avertissement string                      `json:"avertissement"`
	Collections   map[string][]map[string]any `json:"collections"`
}

var (
	motNom      = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
	typesValide = map[string]bool{"string": true, "integer": true, "number": true, "boolean": true, "liste": true}
)

func decoderStrict(b []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if dec.More() {
		return errors.New("contenu inattendu après le document")
	}
	return nil
}

// LireOutils décode et valide outils.json.
func LireOutils(b []byte) (*Outils, error) {
	var o Outils
	if err := decoderStrict(b, &o); err != nil {
		return nil, fmt.Errorf("outils.json illisible : %w", err)
	}
	if len(o.Outils) == 0 {
		return nil, errors.New("outils.json : aucun outil déclaré")
	}
	var errs []error
	vus := map[string]bool{}
	for i := range o.Outils {
		t := &o.Outils[i]
		if err := t.valider(); err != nil {
			errs = append(errs, fmt.Errorf("outil %q : %w", t.Nom, err))
		}
		if vus[t.Nom] {
			errs = append(errs, fmt.Errorf("outil %q en double", t.Nom))
		}
		vus[t.Nom] = true
	}
	return &o, errors.Join(errs...)
}

func (t *Outil) valider() error {
	var errs []error
	ajout := func(f string, a ...any) { errs = append(errs, fmt.Errorf(f, a...)) }
	if err := nommage.ValiderNomOutil(t.Nom); err != nil {
		ajout("nom : %v", err)
	} else if !motNom.MatchString(t.Nom) {
		ajout("nom : minuscules, chiffres et « _ », commençant par une lettre")
	}
	if strings.TrimSpace(t.Description) == "" {
		ajout("description obligatoire (c'est ce que lit le modèle)")
	}
	switch t.Genre {
	case GenreLire, GenreLister, GenreAction:
	default:
		ajout("genre %q inconnu (lire, lister ou action)", t.Genre)
	}
	switch {
	case t.ReadOnlyHint == nil:
		ajout("readOnlyHint obligatoire (true pour une lecture, false pour une action)")
	case t.Genre == GenreAction && *t.ReadOnlyHint:
		ajout("une action ne peut pas être readOnlyHint=true")
	case (t.Genre == GenreLire || t.Genre == GenreLister) && !*t.ReadOnlyHint:
		ajout("un outil %s doit être readOnlyHint=true", t.Genre)
	}
	if t.Genre == GenreAction {
		if !regexp.MustCompile(`^[A-Z][A-Z0-9-]{1,30}$`).MatchString(t.ReferencePrefixe) {
			ajout("reference_prefixe obligatoire pour une action : majuscules, chiffres, tirets (par exemple DEPOT-FICTIF)")
		}
		if t.Collection != "" {
			ajout("collection n'a pas de sens pour une action (utiliser verifie)")
		}
	} else {
		if t.Collection == "" {
			ajout("collection obligatoire pour %s", t.Genre)
		}
		if t.ReferencePrefixe != "" || len(t.Verifie) > 0 {
			ajout("reference_prefixe et verifie ne valent que pour une action")
		}
	}
	noms := map[string]bool{}
	obligatoires := 0
	for _, a := range t.Arguments {
		if !motNom.MatchString(a.Nom) {
			ajout("argument %q : minuscules, chiffres et « _ »", a.Nom)
		}
		if noms[a.Nom] {
			ajout("argument %q en double", a.Nom)
		}
		noms[a.Nom] = true
		if !typesValide[a.Type] {
			ajout("argument %q : type %q inconnu (string, integer, number, boolean, liste)", a.Nom, a.Type)
		}
		if strings.TrimSpace(a.Description) == "" {
			ajout("argument %q : description obligatoire", a.Nom)
		}
		if a.Obligatoire {
			obligatoires++
		}
		if a.Champ != "" && t.Genre == GenreAction {
			ajout("argument %q : champ n'a pas de sens pour une action", a.Nom)
		}
		if a.Type == "liste" && t.Genre != GenreAction {
			ajout("argument %q : une liste ne sert qu'à une action", a.Nom)
		}
	}
	if t.Genre == GenreLire && (len(t.Arguments) == 0 || obligatoires != len(t.Arguments)) {
		ajout("un outil lire a au moins un argument, tous obligatoires")
	}
	for _, v := range t.Verifie {
		if !noms[v.Argument] || v.Collection == "" || v.Champ == "" {
			ajout("verifie : argument (déclaré), collection et champ obligatoires")
		}
	}
	return errors.Join(errs...)
}

// ArgumentsAFaible : noms d'arguments qui invitent à passer des instructions en
// texte libre à un outil d'action, que ni une directive ni le gate ne peuvent
// juger.
var argumentsAFaible = map[string]bool{"instructions": true, "texte": true, "commande": true}

// Avertissements rend les avertissements (non bloquants) sur la gouvernabilité
// des outils : un argument « instructions », « texte » ou « commande » sur un
// outil d'action.
func Avertissements(o *Outils) []string {
	var l []string
	for _, t := range o.Outils {
		if t.Genre != GenreAction {
			continue
		}
		for _, a := range t.Arguments {
			if argumentsAFaible[a.Nom] {
				l = append(l, fmt.Sprintf("outil d'action %q : l'argument %q est un texte libre que les directives ne peuvent pas juger ; préférer des arguments explicites (qui, combien, vers où) — voir « Concevoir des outils gouvernables » dans docs/demo/etendre.md", t.Nom, a.Nom))
			}
		}
	}
	return l
}

// LireDonnees décode et valide donnees.json. L'avertissement « fictif » est
// obligatoire : il est répété dans chaque résultat.
func LireDonnees(b []byte) (*Donnees, error) {
	var d Donnees
	if err := decoderStrict(b, &d); err != nil {
		return nil, fmt.Errorf("donnees.json illisible : %w", err)
	}
	if a := strings.ToLower(d.Avertissement); !strings.Contains(a, "fictif") && !strings.Contains(a, "fictive") {
		return nil, errors.New(`donnees.json : "avertissement" obligatoire et doit dire que les données sont fictives (mot « fictif » ou « fictives »)`)
	}
	if len(d.Collections) == 0 {
		return nil, errors.New("donnees.json : aucune collection")
	}
	for nom, liste := range d.Collections {
		if !motNom.MatchString(nom) {
			return nil, fmt.Errorf("donnees.json : collection %q : minuscules, chiffres et « _ »", nom)
		}
		if len(liste) == 0 {
			return nil, fmt.Errorf("donnees.json : collection %q vide", nom)
		}
	}
	return &d, nil
}

// Verifier contrôle la cohérence des deux fichiers : collections et champs
// cités par les outils.
func Verifier(o *Outils, d *Donnees) error {
	var errs []error
	existe := func(coll, champ string) bool {
		l, ok := d.Collections[coll]
		if !ok {
			return false
		}
		for _, e := range l {
			if _, ok := e[champ]; ok {
				return true
			}
		}
		return false
	}
	for _, t := range o.Outils {
		if t.Collection != "" {
			if _, ok := d.Collections[t.Collection]; !ok {
				errs = append(errs, fmt.Errorf("outil %q : collection %q absente de donnees.json (collections : %s)",
					t.Nom, t.Collection, strings.Join(nomsCollections(d), ", ")))
				continue
			}
			for _, a := range t.Arguments {
				if c := a.champ(); !existe(t.Collection, c) {
					errs = append(errs, fmt.Errorf("outil %q : aucun enregistrement de %q n'a le champ %q (argument %q)", t.Nom, t.Collection, c, a.Nom))
				}
			}
		}
		for _, v := range t.Verifie {
			if !existe(v.Collection, v.Champ) {
				errs = append(errs, fmt.Errorf("outil %q : verifie : aucun enregistrement de %q n'a le champ %q", t.Nom, v.Collection, v.Champ))
			}
		}
	}
	return errors.Join(errs...)
}

func nomsCollections(d *Donnees) []string {
	var l []string
	for n := range d.Collections {
		l = append(l, n)
	}
	sort.Strings(l)
	return l
}

func (a Argument) champ() string {
	if a.Champ != "" {
		return a.Champ
	}
	return a.Nom
}

// LireFichiers lit outils.json et donnees.json depuis le disque et les valide.
func LireFichiers(outils, donnees string) (*Outils, *Donnees, error) {
	bo, err := os.ReadFile(outils)
	if err != nil {
		return nil, nil, err
	}
	bd, err := os.ReadFile(donnees)
	if err != nil {
		return nil, nil, err
	}
	return charger(bo, bd)
}

func charger(bo, bd []byte) (*Outils, *Donnees, error) {
	o, err := LireOutils(bo)
	if err != nil {
		return nil, nil, err
	}
	d, err := LireDonnees(bd)
	if err != nil {
		return nil, nil, err
	}
	if err := Verifier(o, d); err != nil {
		return nil, nil, err
	}
	return o, d, nil
}

// Resultat est le contenu structuré de tout appel.
type Resultat struct {
	Statut          string `json:"statut"`
	Message         string `json:"message,omitempty"`
	DonneesFictives string `json:"donnees_fictives"`
	Reference       string `json:"reference,omitempty"`
	Enregistrement  any    `json:"enregistrement,omitempty"`
	Enregistrements any    `json:"enregistrements,omitempty"`
	Nombre          *int   `json:"nombre,omitempty"`
	Enregistre      any    `json:"enregistre,omitempty"`
}

type etat struct {
	mu      sync.Mutex
	donnees *Donnees
	actions map[string]int // compteur par préfixe
}

// Serveur construit le serveur MCP de nom logique nom depuis les contenus
// de outils.json et donnees.json.
func Serveur(nom string, outilsJSON, donneesJSON []byte) (*mcp.Server, error) {
	if err := nommage.ValiderNomLogique(nom); err != nil {
		return nil, err
	}
	o, d, err := charger(outilsJSON, donneesJSON)
	if err != nil {
		return nil, err
	}
	version := "0.1.0"
	s := mcp.NewServer(&mcp.Implementation{Name: nom, Version: version}, nil)
	st := &etat{donnees: d, actions: map[string]int{}}
	for _, t := range o.Outils {
		s.AddTool(t.outilMCP(), st.gestionnaire(t))
	}
	return s, nil
}

func (a Argument) schema() map[string]any {
	m := map[string]any{"description": a.Description}
	if a.Type == "liste" {
		m["type"] = "array"
		m["items"] = map[string]any{"type": "string"}
	} else {
		m["type"] = a.Type
	}
	return m
}

func (t Outil) outilMCP() *mcp.Tool {
	props := map[string]any{}
	var requis []string
	for _, a := range t.Arguments {
		props[a.Nom] = a.schema()
		if a.Obligatoire {
			requis = append(requis, a.Nom)
		}
	}
	schema := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(requis) > 0 {
		schema["required"] = requis
	}
	faux := false
	titre := t.Nom
	return &mcp.Tool{
		Name:        t.Nom,
		Description: t.Description,
		InputSchema: schema,
		Annotations: &mcp.ToolAnnotations{
			Title:          titre,
			ReadOnlyHint:   *t.ReadOnlyHint,
			IdempotentHint: t.Genre != GenreAction,
			OpenWorldHint:  &faux,
		},
	}
}

func rendre(r Resultat, erreur bool) (*mcp.CallToolResult, error) {
	b, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	var sc map[string]any
	if err := json.Unmarshal(b, &sc); err != nil {
		return nil, err
	}
	return &mcp.CallToolResult{
		IsError:           erreur,
		Content:           []mcp.Content{&mcp.TextContent{Text: string(b)}},
		StructuredContent: sc,
	}, nil
}

func (st *etat) gestionnaire(t Outil) mcp.ToolHandler {
	avert := st.donnees.Avertissement
	refus := func(msg string) (*mcp.CallToolResult, error) {
		return rendre(Resultat{Statut: StatutRefuse, Message: msg, DonneesFictives: avert}, true)
	}
	return func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, err := lireArguments(t, req.Params.Arguments)
		if err != nil {
			return refus(err.Error())
		}
		switch t.Genre {
		case GenreLire:
			return st.lire(t, args)
		case GenreLister:
			return st.lister(t, args)
		default:
			return st.agir(t, args)
		}
	}
}

// lireArguments décode et contrôle les arguments : aucun inconnu, types
// respectés, obligatoires présents.
func lireArguments(t Outil, brut json.RawMessage) (map[string]any, error) {
	args := map[string]any{}
	if len(bytes.TrimSpace(brut)) > 0 {
		dec := json.NewDecoder(bytes.NewReader(brut))
		dec.UseNumber()
		if err := dec.Decode(&args); err != nil {
			return nil, fmt.Errorf("arguments illisibles : %v", err)
		}
	}
	decl := map[string]Argument{}
	for _, a := range t.Arguments {
		decl[a.Nom] = a
	}
	for n := range args {
		if _, ok := decl[n]; !ok {
			return nil, fmt.Errorf("argument %q inconnu", n)
		}
	}
	for _, a := range t.Arguments {
		v, ok := args[a.Nom]
		if !ok || v == nil {
			if a.Obligatoire {
				return nil, fmt.Errorf("argument %q obligatoire", a.Nom)
			}
			delete(args, a.Nom)
			continue
		}
		if err := verifierType(a, v); err != nil {
			return nil, err
		}
	}
	return args, nil
}

func verifierType(a Argument, v any) error {
	faux := func() error { return fmt.Errorf("argument %q : type %s attendu", a.Nom, a.Type) }
	switch a.Type {
	case "string":
		if _, ok := v.(string); !ok {
			return faux()
		}
	case "boolean":
		if _, ok := v.(bool); !ok {
			return faux()
		}
	case "integer":
		n, ok := v.(json.Number)
		if !ok {
			return faux()
		}
		if _, err := n.Int64(); err != nil {
			return faux()
		}
	case "number":
		n, ok := v.(json.Number)
		if !ok {
			return faux()
		}
		if _, err := n.Float64(); err != nil {
			return faux()
		}
	case "liste":
		l, ok := v.([]any)
		if !ok {
			return faux()
		}
		for _, x := range l {
			if _, ok := x.(string); !ok {
				return faux()
			}
		}
	}
	return nil
}

// egal compare un champ d'enregistrement (décodé de donnees.json) à un
// argument (décodé avec UseNumber).
func egal(champ, arg any) bool {
	switch a := arg.(type) {
	case json.Number:
		f, err := a.Float64()
		c, ok := champ.(float64)
		return err == nil && ok && f == c
	default:
		return champ == arg
	}
}

func (st *etat) filtrer(coll string, t Outil, args map[string]any) []map[string]any {
	var out []map[string]any
	for _, e := range st.donnees.Collections[coll] {
		ok := true
		for _, a := range t.Arguments {
			v, donne := args[a.Nom]
			if donne && !egal(e[a.champ()], v) {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, e)
		}
	}
	return out
}

func (st *etat) lire(t Outil, args map[string]any) (*mcp.CallToolResult, error) {
	avert := st.donnees.Avertissement
	trouves := st.filtrer(t.Collection, t, args)
	if len(trouves) == 0 {
		// Les valeurs existantes du premier argument, comme les autres serveurs.
		champ := t.Arguments[0].champ()
		var connus []string
		for _, e := range st.donnees.Collections[t.Collection] {
			if v, ok := e[champ]; ok {
				connus = append(connus, fmt.Sprint(v))
			}
		}
		sort.Strings(connus)
		return rendre(Resultat{Statut: StatutIntrouvable, DonneesFictives: avert,
			Message: fmt.Sprintf("aucun enregistrement ne correspond ; %s connus : %v", champ, connus)}, true)
	}
	return rendre(Resultat{Statut: StatutOK, DonneesFictives: avert, Enregistrement: trouves[0]}, false)
}

func (st *etat) lister(t Outil, args map[string]any) (*mcp.CallToolResult, error) {
	trouves := st.filtrer(t.Collection, t, args)
	n := len(trouves)
	r := Resultat{Statut: StatutOK, DonneesFictives: st.donnees.Avertissement, Nombre: &n}
	if n > LimiteListe {
		trouves = trouves[:LimiteListe]
		r.Message = fmt.Sprintf("%d enregistrements au total, les %d premiers sont rendus", n, LimiteListe)
	}
	if trouves == nil {
		trouves = []map[string]any{}
	}
	r.Enregistrements = trouves
	return rendre(r, false)
}

func (st *etat) agir(t Outil, args map[string]any) (*mcp.CallToolResult, error) {
	avert := st.donnees.Avertissement
	for _, v := range t.Verifie {
		val := args[v.Argument]
		if val == nil {
			continue // argument facultatif absent : rien à vérifier
		}
		vals := []any{val}
		if l, ok := val.([]any); ok {
			vals = l
		}
		for _, x := range vals {
			trouve := false
			for _, e := range st.donnees.Collections[v.Collection] {
				if egal(e[v.Champ], x) {
					trouve = true
					break
				}
			}
			if !trouve {
				return rendre(Resultat{Statut: StatutIntrouvable, DonneesFictives: avert,
					Message: fmt.Sprintf("%s %v inconnu dans %s : action non enregistrée", v.Argument, x, v.Collection)}, true)
			}
		}
	}
	st.mu.Lock()
	st.actions[t.ReferencePrefixe]++
	ref := fmt.Sprintf("%s-%04d", t.ReferencePrefixe, st.actions[t.ReferencePrefixe])
	st.mu.Unlock()
	return rendre(Resultat{Statut: StatutEnregistre, DonneesFictives: avert, Reference: ref, Enregistre: args,
		Message: "action enregistrée en mémoire (fictive)"}, false)
}
