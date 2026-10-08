package directive

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Charger lit un jeu depuis ses octets et le valide strictement. Toute
// erreur de validation est rendue en bloc, une ligne par défaut, pour que le
// l'auteur corrige en une passe plutôt qu'une erreur à la fois.
//
// Le paquet ne lit aucun fichier lui-même : la lecture appartient à
// l'appelant, ce qui garde ce paquet sans os, io ni réseau (voir
// purete_test.go).
func Charger(donnees []byte) (*Jeu, error) {
	if err := refuserV1(donnees); err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(donnees))
	dec.DisallowUnknownFields()
	dec.UseNumber()
	var jeu Jeu
	if err := dec.Decode(&jeu); err != nil {
		return nil, fmt.Errorf("directive: document illisible: %w", err)
	}
	if dec.More() {
		return nil, fmt.Errorf("directive: contenu inattendu après le jeu")
	}
	if err := Valider(&jeu); err != nil {
		return nil, err
	}
	return &jeu, nil
}

// refuserV1 rend, en bloc et par position, les directives au format v1 d'un
// document par ailleurs lisible ; le reste du contrôle est celui de Charger.
// Le message ne cite rien du document.
func refuserV1(donnees []byte) error {
	var brut struct {
		Directives []map[string]json.RawMessage `json:"directives"`
	}
	if json.Unmarshal(donnees, &brut) != nil {
		return nil // illisible : Charger le dira
	}
	var v validation
	for i, cles := range brut.Directives {
		if estV1(cles) {
			v.f("directive %d: %w", i+1, ErrFormatV1)
		}
	}
	return v.err()
}

// Valider applique le schéma strict et fixe l'empreinte. Un jeu qui passe
// Valider est exactement ce que Decide attend.
func Valider(jeu *Jeu) error {
	var v validation
	m := &jeu.Meta
	if m.ID == "" {
		v.f("jeu: id manquant")
	}
	if m.Version < 1 {
		v.f("jeu: version doit être >= 1")
	}
	if m.DateEffet.IsZero() {
		v.f("jeu: date_effet manquante")
	}
	if m.Auteur == "" {
		v.f("jeu: auteur manquant")
	}
	if m.Signature != "" {
		v.f("jeu: signature présente mais la vérification n'est pas implémentée (P1) ; retirer le champ")
	}
	if len(jeu.Directives) == 0 {
		v.f("jeu: aucune directive")
	}

	vus := map[string]bool{}
	for i := range jeu.Directives {
		d := &jeu.Directives[i]
		prefixe := fmt.Sprintf("directive %d", i+1)
		if d.ID != "" {
			prefixe = "directive " + d.ID
		}
		if vus[d.ID] {
			v.f("%s: identifiant en double", prefixe)
		}
		vus[d.ID] = true
		if !d.EstV2() {
			// Construite à la main sans déclencheur : la forme v1.
			v.f("directive %d: %w", i+1, ErrFormatV1)
			continue
		}
		validerDirectiveV2(&v, prefixe, d)
	}
	if err := v.err(); err != nil {
		return err
	}

	empreinte, err := Empreinte(jeu.Directives)
	if err != nil {
		return err
	}
	if m.Empreinte != "" && m.Empreinte != empreinte {
		return fmt.Errorf("directive: jeu: l'empreinte déclarée %s ne correspond pas au contenu (%s)", m.Empreinte, empreinte)
	}
	m.Empreinte = empreinte
	return nil
}

// Empreinte calcule sha256 du tableau directives[] en JSON canonique (clés
// triées à tous les niveaux).
func Empreinte(directives []Directive) (string, error) {
	b, err := json.Marshal(directives)
	if err != nil {
		return "", fmt.Errorf("directive: encoder pour empreinte: %w", err)
	}
	var generique any
	if err := json.Unmarshal(b, &generique); err != nil {
		return "", fmt.Errorf("directive: canoniser pour empreinte: %w", err)
	}
	canon, err := json.Marshal(generique)
	if err != nil {
		return "", fmt.Errorf("directive: canoniser pour empreinte: %w", err)
	}
	sum := sha256.Sum256(canon)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

type validation struct{ errs []error }

func (v *validation) f(format string, args ...any) {
	v.errs = append(v.errs, fmt.Errorf("directive: "+format, args...))
}

func (v *validation) err() error { return errors.Join(v.errs...) }

func validerPointeur(ptr string) error {
	if ptr == "" || ptr[0] != '/' {
		return fmt.Errorf("pointeur JSON %q invalide (doit commencer par /)", ptr)
	}
	if strings.HasSuffix(ptr, "/") {
		return fmt.Errorf("pointeur JSON %q invalide (se termine par /)", ptr)
	}
	return nil
}
