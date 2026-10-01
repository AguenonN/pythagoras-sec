// Package annuaire est la source des habilitations : qui est qui, dans
// quels groupes, avec quels rôles et depuis quand, et la clé publique de
// chacun. L'engine en tire trois choses :
//
//   - le demandeur d'une session (la personne pour qui l'agent travaille),
//     prouvé par un jeton d'identité signé, et ses groupes, résolus à
//     l'ouverture de la session ;
//   - les valideurs d'un appel en attente, prouvés par un jeton
//     d'approbation signé de leur clé, avec les rôles qu'ils détenaient à
//     la date de validation ;
//   - la référence de l'annuaire (identifiant, version, empreinte), écrite
//     au journal pour que le rejeu sache contre quoi tout a été vérifié.
//
// L'engine ne lit l'annuaire que par l'interface Source ; Local est
// l'implémentation de démonstration, un fichier JSON signé Ed25519 par une
// autorité dont la clé publique est fournie à part (jamais lue dans le
// fichier lui-même : une signature vérifiée avec la clé qu'elle apporte ne
// prouve rien).
//
// Aucune clé privée n'est dans ce paquet ni dans le dépôt : les clés de
// démonstration sont générées par mcp-approve dans un répertoire ignoré
// par Git (voir Generer).
package annuaire

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// SourceLocaleDemo est le nom de la source d'identité de démonstration,
// consigné au journal (source_identite).
const SourceLocaleDemo = "annuaire-local-demo"

// Reference identifie l'annuaire chargé : ce que le journal consigne.
type Reference struct {
	ID        string `json:"id"`
	Version   int    `json:"version"`
	Empreinte string `json:"empreinte"`
}

// Fiche est ce que l'annuaire dit d'une identité à une date : ses groupes,
// les rôles qu'elle détient à cette date, sa clé publique.
type Fiche struct {
	Identite string
	Groupes  []string
	Roles    []string
	Cle      ed25519.PublicKey
}

// Source est ce que l'engine attend d'une source d'identité.
type Source interface {
	// Nom est consigné au journal (source_identite).
	Nom() string
	// Reference identifie la version chargée.
	Reference() Reference
	// Resoudre rend la fiche d'une identité à une date ; faux si
	// l'identité est inconnue.
	Resoudre(identite string, a time.Time) (Fiche, bool)
}

// Role est un rôle détenu sur une période : [Depuis, JusquA[ ; JusquA
// absent, sans fin.
type Role struct {
	Role   string     `json:"role"`
	Depuis time.Time  `json:"depuis"`
	JusquA *time.Time `json:"jusqu_a,omitempty"`
}

// Detenu dit si le rôle est détenu à la date.
func (r Role) Detenu(a time.Time) bool {
	return !a.Before(r.Depuis) && (r.JusquA == nil || a.Before(*r.JusquA))
}

// Identite est une entrée de l'annuaire. Toutes les identités de
// démonstration sont fictives.
type Identite struct {
	Identite    string   `json:"identite"`
	Nom         string   `json:"nom"`
	Groupes     []string `json:"groupes,omitempty"`
	Roles       []Role   `json:"roles,omitempty"`
	ClePublique string   `json:"cle_publique"` // Ed25519, base64 standard
}

// Meta identifie et signe le document. Empreinte est sha256 du tableau
// identites[] canonique ; Signature est la signature Ed25519 de
// l'autorité sur la chaîne Empreinte.
type Meta struct {
	ID            string    `json:"id"`
	Version       int       `json:"version"`
	Emis          time.Time `json:"emis_le"`
	Avertissement string    `json:"avertissement,omitempty"`
	Empreinte     string    `json:"empreinte"`
	Signature     string    `json:"signature"`
}

// Document est le fichier de l'annuaire.
type Document struct {
	Meta      Meta       `json:"annuaire"`
	Identites []Identite `json:"identites"`
}

// Local est l'annuaire de démonstration chargé et vérifié.
type Local struct {
	doc   Document
	index map[string]Identite
	cles  map[string]ed25519.PublicKey
}

var (
	// identiteValide : une adresse de forme courriel ou un nom de service,
	// sans espace ni caractère de contrôle.
	identiteValide = regexp.MustCompile(`^[a-z0-9][a-z0-9._@-]{0,127}$`)
	nomCourt       = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
)

// ErrSignature : la signature de l'annuaire ne se vérifie pas avec la clé
// de l'autorité fournie.
var ErrSignature = errors.New("annuaire: signature de l'autorité invalide")

// Charger lit l'annuaire, le valide strictement et vérifie sa signature
// avec la clé publique de l'autorité, fournie à part. Toutes les fautes de
// contenu sont rendues en bloc.
func Charger(donnees []byte, autorite ed25519.PublicKey) (*Local, error) {
	if len(autorite) != ed25519.PublicKeySize {
		return nil, errors.New("annuaire: clé de l'autorité absente ou de taille invalide")
	}
	dec := json.NewDecoder(bytes.NewReader(donnees))
	dec.DisallowUnknownFields()
	var d Document
	if err := dec.Decode(&d); err != nil {
		return nil, fmt.Errorf("annuaire: document illisible: %w", err)
	}
	if dec.More() {
		return nil, errors.New("annuaire: contenu inattendu après le document")
	}
	if err := valider(&d); err != nil {
		return nil, err
	}
	emp, err := Empreinte(d.Identites)
	if err != nil {
		return nil, err
	}
	if d.Meta.Empreinte != emp {
		return nil, fmt.Errorf("annuaire: l'empreinte déclarée %s ne correspond pas au contenu (%s)", d.Meta.Empreinte, emp)
	}
	sig, err := base64.StdEncoding.DecodeString(d.Meta.Signature)
	if err != nil || !ed25519.Verify(autorite, []byte(d.Meta.Empreinte), sig) {
		return nil, ErrSignature
	}
	l := &Local{doc: d, index: map[string]Identite{}, cles: map[string]ed25519.PublicKey{}}
	for _, id := range d.Identites {
		l.index[id.Identite] = id
		cle, _ := base64.StdEncoding.DecodeString(id.ClePublique)
		l.cles[id.Identite] = ed25519.PublicKey(cle)
	}
	return l, nil
}

func valider(d *Document) error {
	var errs []error
	f := func(format string, args ...any) { errs = append(errs, fmt.Errorf("annuaire: "+format, args...)) }
	if !nomCourt.MatchString(d.Meta.ID) {
		f("id %q invalide", d.Meta.ID)
	}
	if d.Meta.Version < 1 {
		f("version doit être >= 1")
	}
	if len(d.Identites) == 0 {
		f("aucune identité")
	}
	vues, clesVues := map[string]bool{}, map[string]string{}
	for i, id := range d.Identites {
		p := fmt.Sprintf("identité %d", i+1)
		if id.Identite != "" {
			p = "identité " + id.Identite
		}
		if !identiteValide.MatchString(id.Identite) {
			f("%s: identifiant invalide (minuscules, chiffres, . _ @ -)", p)
		}
		if vues[id.Identite] {
			f("%s: en double", p)
		}
		vues[id.Identite] = true
		if strings.TrimSpace(id.Nom) == "" {
			f("%s: nom manquant", p)
		}
		for _, g := range id.Groupes {
			if !nomCourt.MatchString(g) {
				f("%s: groupe %q invalide", p, g)
			}
		}
		if dup := doublon(id.Groupes); dup != "" {
			f("%s: groupe %s en double", p, dup)
		}
		for _, r := range id.Roles {
			if !nomCourt.MatchString(r.Role) {
				f("%s: rôle %q invalide", p, r.Role)
			}
			if r.Depuis.IsZero() {
				f("%s: rôle %s sans date de début", p, r.Role)
			}
			if r.JusquA != nil && !r.JusquA.After(r.Depuis) {
				f("%s: rôle %s : jusqu_a doit suivre depuis", p, r.Role)
			}
		}
		cle, err := base64.StdEncoding.DecodeString(id.ClePublique)
		switch {
		case err != nil || len(cle) != ed25519.PublicKeySize:
			f("%s: clé publique Ed25519 invalide", p)
		case clesVues[id.ClePublique] != "":
			// Deux identités sous la même clé : une signature ne dirait
			// plus qui a signé.
			f("%s: même clé publique que %s", p, clesVues[id.ClePublique])
		default:
			clesVues[id.ClePublique] = id.Identite
		}
	}
	return errors.Join(errs...)
}

func doublon(l []string) string {
	vus := map[string]bool{}
	for _, x := range l {
		if vus[x] {
			return x
		}
		vus[x] = true
	}
	return ""
}

// Empreinte calcule sha256 du tableau identites[] en JSON canonique (clés
// triées, dates en RFC 3339).
func Empreinte(ids []Identite) (string, error) {
	b, err := json.Marshal(ids)
	if err != nil {
		return "", fmt.Errorf("annuaire: encoder pour empreinte: %w", err)
	}
	var generique any
	if err := json.Unmarshal(b, &generique); err != nil {
		return "", fmt.Errorf("annuaire: canoniser: %w", err)
	}
	canon, err := json.Marshal(generique)
	if err != nil {
		return "", fmt.Errorf("annuaire: canoniser: %w", err)
	}
	s := sha256.Sum256(canon)
	return "sha256:" + hex.EncodeToString(s[:]), nil
}

// Signer fixe l'empreinte du document et le signe avec la clé privée de
// l'autorité. Le document est validé d'abord.
func Signer(d *Document, autorite ed25519.PrivateKey) error {
	if err := valider(d); err != nil {
		return err
	}
	emp, err := Empreinte(d.Identites)
	if err != nil {
		return err
	}
	d.Meta.Empreinte = emp
	d.Meta.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(autorite, []byte(emp)))
	return nil
}

// Nom est la source d'identité consignée au journal.
func (l *Local) Nom() string { return SourceLocaleDemo }

// Reference identifie l'annuaire chargé.
func (l *Local) Reference() Reference {
	return Reference{ID: l.doc.Meta.ID, Version: l.doc.Meta.Version, Empreinte: l.doc.Meta.Empreinte}
}

// Resoudre rend la fiche d'une identité à une date : groupes triés, rôles
// détenus à cette date triés, clé publique.
func (l *Local) Resoudre(identite string, a time.Time) (Fiche, bool) {
	id, ok := l.index[identite]
	if !ok {
		return Fiche{}, false
	}
	fi := Fiche{Identite: id.Identite, Groupes: trier(id.Groupes), Cle: l.cles[identite]}
	var roles []string
	for _, r := range id.Roles {
		if r.Detenu(a) {
			roles = append(roles, r.Role)
		}
	}
	fi.Roles = trier(roles)
	return fi, true
}

// Identites rend les identifiants de l'annuaire, triés.
func (l *Local) Identites() []string {
	out := make([]string, 0, len(l.index))
	for k := range l.index {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func trier(l []string) []string {
	if len(l) == 0 {
		return nil
	}
	s := append([]string(nil), l...)
	sort.Strings(s)
	out := s[:1]
	for _, x := range s[1:] {
		if x != out[len(out)-1] {
			out = append(out, x)
		}
	}
	return out
}

// DetientRole dit si la fiche porte le rôle.
func (f Fiche) DetientRole(role string) bool {
	for _, r := range f.Roles {
		if r == role {
			return true
		}
	}
	return false
}
