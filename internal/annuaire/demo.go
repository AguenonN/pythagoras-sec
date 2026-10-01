package annuaire

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FichierSourceDemo est la source versionnée de l'annuaire de
// démonstration : identités fictives, groupes, rôles datés, sans aucune
// clé. Generer en fait un annuaire signé.
const FichierSourceDemo = "demo/annuaire-source.json"

// SourceDemo est le contenu de FichierSourceDemo.
type SourceDemo struct {
	ID            string           `json:"id"`
	Version       int              `json:"version"`
	Avertissement string           `json:"avertissement"`
	Identites     []IdentiteSource `json:"identites"`
}

// IdentiteSource est une identité sans clé.
type IdentiteSource struct {
	Identite string   `json:"identite"`
	Nom      string   `json:"nom"`
	Groupes  []string `json:"groupes,omitempty"`
	Roles    []Role   `json:"roles,omitempty"`
}

// LireSourceDemo lit la source, strictement.
func LireSourceDemo(chemin string) (*SourceDemo, error) {
	b, err := os.ReadFile(chemin)
	if err != nil {
		return nil, fmt.Errorf("annuaire: source: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var s SourceDemo
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("annuaire: source illisible: %w", err)
	}
	return &s, nil
}

// Cles sont les clés privées générées : celle de l'autorité et une par
// identité. Elles ne vont jamais au dépôt.
type Cles struct {
	Autorite  ed25519.PrivateKey
	Identites map[string]ed25519.PrivateKey
}

// Generer tire une clé par identité et une pour l'autorité (alea : nil
// pour crypto/rand), et rend l'annuaire signé.
func Generer(src SourceDemo, emis time.Time, alea io.Reader) (*Document, *Cles, error) {
	_, auto, err := ed25519.GenerateKey(alea)
	if err != nil {
		return nil, nil, err
	}
	cles := &Cles{Autorite: auto, Identites: map[string]ed25519.PrivateKey{}}
	d := &Document{Meta: Meta{ID: src.ID, Version: src.Version, Emis: emis.UTC(), Avertissement: src.Avertissement}}
	for _, is := range src.Identites {
		pub, priv, err := ed25519.GenerateKey(alea)
		if err != nil {
			return nil, nil, err
		}
		cles.Identites[is.Identite] = priv
		d.Identites = append(d.Identites, Identite{Identite: is.Identite, Nom: is.Nom, Groupes: is.Groupes, Roles: is.Roles,
			ClePublique: base64.StdEncoding.EncodeToString(pub)})
	}
	if err := Signer(d, auto); err != nil {
		return nil, nil, err
	}
	return d, cles, nil
}

// Noms des fichiers de clés dans un répertoire de clés.
const (
	FichierAutorite       = "autorite.cle"
	FichierAutoritePublic = "autorite.pub"
)

// FichierCle est le fichier de la clé privée d'une identité.
func FichierCle(identite string) string { return identite + ".cle" }

// EcrireCles écrit les clés privées (0600, graine Ed25519 en base64) et la
// clé publique de l'autorité dans dir, créé en 0700.
func EcrireCles(dir string, c *Cles) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	ecrire := func(nom string, contenu []byte, mode os.FileMode) error {
		return os.WriteFile(filepath.Join(dir, nom), append(contenu, '\n'), mode)
	}
	if err := ecrire(FichierAutorite, []byte(base64.StdEncoding.EncodeToString(c.Autorite.Seed())), 0o600); err != nil {
		return err
	}
	pub := c.Autorite.Public().(ed25519.PublicKey)
	if err := ecrire(FichierAutoritePublic, []byte(base64.StdEncoding.EncodeToString(pub)), 0o644); err != nil {
		return err
	}
	for id, k := range c.Identites {
		if err := ecrire(FichierCle(id), []byte(base64.StdEncoding.EncodeToString(k.Seed())), 0o600); err != nil {
			return err
		}
	}
	return nil
}

// LireCle lit une clé privée écrite par EcrireCles.
func LireCle(chemin string) (ed25519.PrivateKey, error) {
	b, err := os.ReadFile(chemin)
	if err != nil {
		return nil, err
	}
	seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, errors.New("annuaire: clé privée illisible")
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

// LireClePublique lit une clé publique Ed25519 en base64.
func LireClePublique(chemin string) (ed25519.PublicKey, error) {
	b, err := os.ReadFile(chemin)
	if err != nil {
		return nil, err
	}
	k, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
	if err != nil || len(k) != ed25519.PublicKeySize {
		return nil, errors.New("annuaire: clé publique illisible")
	}
	return ed25519.PublicKey(k), nil
}

// Encoder rend le document en JSON indenté.
func Encoder(d *Document) ([]byte, error) {
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}
