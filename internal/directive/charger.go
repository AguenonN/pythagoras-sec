package directive

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"mcprogue/internal/nommage"
)

// Charger lit un jeu depuis ses octets et le valide strictement. Toute
// erreur de validation est rendue en bloc, une ligne par défaut, pour que le
// juriste corrige en une passe plutôt qu'une erreur à la fois.
//
// Le paquet ne lit aucun fichier lui-même : la lecture appartient à
// l'appelant, ce qui garde ce paquet sans os, io ni réseau (voir
// purete_test.go).
func Charger(donnees []byte) (*Jeu, error) {
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

// Valider applique le schéma strict, normalise les défauts (critère de
// provenance) et fixe l'empreinte. Un jeu qui passe Valider est exactement
// ce que Decide attend.
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
		if d.EstV2() {
			validerDirectiveV2(&v, prefixe, d)
		} else {
			validerDirective(&v, prefixe, d)
		}
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
// triées à tous les niveaux), après normalisation.
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

// exigences décrit, par verrou, les champs facultatifs exigés et l'ensemble
// des effets admis. Tout champ facultatif non exigé est refusé : le juriste
// relit une directive dont chaque champ compte, pas une directive où un
// champ ignoré aurait l'air de compter.
type exigences struct {
	condition, provenance, champs bool
	serveurNomme                  bool
	effets                        []Effet
}

var parVerrou = map[Verrou]exigences{
	Interdiction:       {effets: []Effet{Refuser}},
	SeuilArgument:      {condition: true, effets: []Effet{Refuser, AttendreValidation}},
	ProvenanceSession:  {provenance: true, effets: []Effet{Refuser, AttendreValidation}},
	ApprobationHumaine: {effets: []Effet{AttendreValidation}},
	ChampsAutorises:    {champs: true, serveurNomme: true, effets: []Effet{Refuser, Caviarder}},
	SourceAttestee:     {serveurNomme: true, effets: []Effet{Refuser}},
}

func validerDirective(v *validation, p string, d *Directive) {
	if d.ID == "" {
		v.f("%s: id manquant", p)
	}
	if d.Version < 1 {
		v.f("%s: version doit être >= 1", p)
	}
	switch d.Statut {
	case Brouillon, Observation, Active, Retiree:
	default:
		v.f("%s: statut %q inconnu", p, d.Statut)
	}
	if d.DateEffet.IsZero() {
		v.f("%s: date_effet manquante", p)
	}
	if d.RevisionAvant != nil && !d.RevisionAvant.After(d.DateEffet) {
		v.f("%s: revision_avant doit être postérieure à date_effet", p)
	}
	if strings.TrimSpace(d.TexteFR) == "" {
		v.f("%s: texte_fr manquant", p)
	}
	if strings.TrimSpace(d.Fondement.Texte) == "" {
		v.f("%s: fondement.texte manquant", p)
	}
	ex, connu := parVerrou[d.Verrou]
	if !connu {
		v.f("%s: verrou %q inconnu", p, d.Verrou)
		return
	}

	// Cible.
	if d.Cible.Serveur == "" {
		v.f("%s: cible.serveur manquant", p)
	} else if d.Cible.Serveur != "*" {
		if err := nommage.ValiderNomLogique(d.Cible.Serveur); err != nil {
			v.f("%s: cible.serveur: %v", p, err)
		}
	} else if ex.serveurNomme {
		v.f("%s: le verrou %s exige un serveur nommé dans cible.serveur", p, d.Verrou)
	}
	if d.Cible.Outil == "" {
		v.f("%s: cible.outil manquant", p)
	} else if d.Cible.Outil != "*" {
		if err := nommage.ValiderNomOutil(d.Cible.Outil); err != nil {
			v.f("%s: cible.outil: %v", p, err)
		}
	}

	// Périmètre, admis partout.
	if d.Perimetre != nil {
		if len(d.Perimetre.Appelants) == 0 {
			v.f("%s: perimetre.appelants vide", p)
		}
		for _, a := range d.Perimetre.Appelants {
			if a != "*" && !strings.HasPrefix(a, "spiffe://") {
				v.f("%s: perimetre.appelants %q n'est ni \"*\" ni un SPIFFE ID", p, a)
			}
		}
	}

	// Champs facultatifs : exigés ou interdits, jamais tolérés.
	if ex.condition && len(d.Condition) == 0 {
		v.f("%s: le verrou %s exige au moins une condition", p, d.Verrou)
	}
	if !ex.condition && len(d.Condition) > 0 {
		v.f("%s: le verrou %s n'admet pas de condition", p, d.Verrou)
	}
	if ex.provenance && d.Provenance == nil {
		v.f("%s: le verrou %s exige une provenance", p, d.Verrou)
	}
	if !ex.provenance && d.Provenance != nil {
		v.f("%s: le verrou %s n'admet pas de provenance", p, d.Verrou)
	}
	if ex.champs && d.Champs == nil {
		v.f("%s: le verrou %s exige champs", p, d.Verrou)
	}
	if !ex.champs && d.Champs != nil {
		v.f("%s: le verrou %s n'admet pas champs", p, d.Verrou)
	}

	for i, c := range d.Condition {
		validerCondition(v, fmt.Sprintf("%s: condition %d", p, i+1), c)
	}
	switch d.SiIndetermine {
	case "":
		if d.SiIndetermineValidePar != "" {
			v.f("%s: si_indetermine_valide_par sans si_indetermine: ignorer", p)
		}
	case IndetermineRefuser:
		if len(d.Condition) == 0 {
			v.f("%s: si_indetermine sans condition", p)
		}
		if d.SiIndetermineValidePar != "" {
			v.f("%s: si_indetermine_valide_par n'a de sens qu'avec si_indetermine: ignorer", p)
		}
	case IndetermineIgnorer:
		if len(d.Condition) == 0 {
			v.f("%s: si_indetermine sans condition", p)
		}
		if strings.TrimSpace(d.SiIndetermineValidePar) == "" {
			v.f("%s: si_indetermine: ignorer exige si_indetermine_valide_par (validation nommée du juriste)", p)
		}
	default:
		v.f("%s: si_indetermine %q inconnu", p, d.SiIndetermine)
	}

	if pr := d.Provenance; pr != nil {
		if len(pr.Serveurs) == 0 && len(pr.Outils) == 0 {
			v.f("%s: provenance sans serveur ni outil", p)
		}
		for _, s := range pr.Serveurs {
			if err := nommage.ValiderNomLogique(s); err != nil {
				v.f("%s: provenance.serveurs: %v", p, err)
			}
		}
		for _, o := range pr.Outils {
			s, t, ok := strings.Cut(o, "/")
			if !ok || nommage.ValiderNomLogique(s) != nil || nommage.ValiderNomOutil(t) != nil {
				v.f("%s: provenance.outils %q doit s'écrire serveur/outil", p, o)
			}
		}
		switch pr.Critere {
		case "":
			pr.Critere = DonneesRecues
		case Transmis, DonneesRecues:
		default:
			v.f("%s: provenance.critere %q inconnu", p, pr.Critere)
		}
	}

	if ch := d.Champs; ch != nil {
		if len(ch.Autorises) == 0 {
			v.f("%s: champs.autorises vide", p)
		}
		for _, ptr := range ch.Autorises {
			if err := validerPointeur(ptr); err != nil {
				v.f("%s: champs.autorises: %v", p, err)
			}
		}
		switch ch.Mode {
		case ModeRefuser:
			if d.Effet != Refuser {
				v.f("%s: champs.mode refuser exige effet refuser (effet %q)", p, d.Effet)
			}
		case ModeCaviarder:
			if d.Effet != Caviarder {
				v.f("%s: champs.mode caviarder exige effet caviarder (effet %q)", p, d.Effet)
			}
		default:
			v.f("%s: champs.mode %q inconnu", p, ch.Mode)
		}
	}

	admis := false
	for _, e := range ex.effets {
		if e == d.Effet {
			admis = true
		}
	}
	if !admis {
		v.f("%s: effet %q non admis pour le verrou %s (admis : %v)", p, d.Effet, d.Verrou, ex.effets)
	}
}

func validerCondition(v *validation, p string, c Condition) {
	if err := validerPointeur(c.Champ); err != nil {
		v.f("%s: %v", p, err)
	}
	switch c.Op {
	case OpPresent, OpAbsent:
		if c.Valeur != nil {
			v.f("%s: l'opérateur %s ne prend pas de valeur", p, c.Op)
		}
	case OpSup, OpSupEgal, OpInf, OpInfEgal:
		if _, ok := nombre(c.Valeur); !ok {
			v.f("%s: l'opérateur %s exige une valeur numérique", p, c.Op)
		}
	case OpEgal, OpDiff:
		switch c.Valeur.(type) {
		case json.Number, float64, string, bool:
		default:
			v.f("%s: l'opérateur %s exige un nombre, une chaîne ou un booléen", p, c.Op)
		}
	case OpDans:
		liste, ok := c.Valeur.([]any)
		if !ok || len(liste) == 0 {
			v.f("%s: l'opérateur in exige une liste non vide", p)
		}
	default:
		v.f("%s: opérateur %q inconnu", p, c.Op)
	}
}

func validerPointeur(ptr string) error {
	if ptr == "" || ptr[0] != '/' {
		return fmt.Errorf("pointeur JSON %q invalide (doit commencer par /)", ptr)
	}
	if strings.HasSuffix(ptr, "/") {
		return fmt.Errorf("pointeur JSON %q invalide (se termine par /)", ptr)
	}
	return nil
}
