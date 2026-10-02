package directive

import (
	"bytes"
	"encoding/json"
	"errors"
	"time"
)

// Une directive s'écrit sous la forme v2 (docs/schema-directive-v2.md §3).
// Le décodage est strict (champs inconnus refusés, nombres gardés en
// json.Number), parce que les options du décodeur appelant ne traversent
// pas un UnmarshalJSON.

// ErrFormatV1 refuse un document au format de directive v1 (clé verrou,
// ou directive sans déclencheur). Le message ne cite rien du document.
var ErrFormatV1 = errors.New("format de directive v1 non pris en charge : réécrire en v2")

// directiveV2 est la forme écrite (docs/schema-directive-v2.md §3).
type directiveV2 struct {
	ID                     string           `json:"id"`
	Version                int              `json:"version"`
	Statut                 Statut           `json:"statut"`
	DateEffet              time.Time        `json:"date_effet"`
	RevisionAvant          *time.Time       `json:"revision_avant,omitempty"`
	TexteFR                string           `json:"texte_fr"`
	Fondements             []Fondement      `json:"fondements"`
	Perimetre              *Perimetre       `json:"perimetre,omitempty"`
	SiIndetermine          SiIndetermine    `json:"si_indetermine,omitempty"`
	SiIndetermineValidePar string           `json:"si_indetermine_valide_par,omitempty"`
	Declencheur            *Declencheur     `json:"declencheur"`
	Condition              *Noeud           `json:"condition,omitempty"`
	Effets                 []EffetDirective `json:"effets"`
}

// MarshalJSON écrit la directive sous sa forme v2.
func (d Directive) MarshalJSON() ([]byte, error) {
	return json.Marshal(directiveV2{
		ID: d.ID, Version: d.Version, Statut: d.Statut, DateEffet: d.DateEffet,
		RevisionAvant: d.RevisionAvant, TexteFR: d.TexteFR, Fondements: d.Fondements,
		Perimetre: d.Perimetre, SiIndetermine: d.SiIndetermine,
		SiIndetermineValidePar: d.SiIndetermineValidePar,
		Declencheur:            d.Declencheur, Condition: d.Arbre, Effets: d.Effets,
	})
}

// estV1 dit si un objet directive est au format v1 : il porte la clé
// verrou, ou n'a pas de déclencheur.
func estV1(cles map[string]json.RawMessage) bool {
	_, verrou := cles["verrou"]
	_, declencheur := cles["declencheur"]
	return verrou || !declencheur
}

// UnmarshalJSON lit une directive v2, strictement ; une directive v1 est
// refusée par ErrFormatV1.
func (d *Directive) UnmarshalJSON(b []byte) error {
	var cles map[string]json.RawMessage
	if err := json.Unmarshal(b, &cles); err != nil {
		return err
	}
	if estV1(cles) {
		return ErrFormatV1
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	dec.UseNumber()
	var w directiveV2
	if err := dec.Decode(&w); err != nil {
		return err
	}
	*d = Directive{
		ID: w.ID, Version: w.Version, Statut: w.Statut, DateEffet: w.DateEffet,
		RevisionAvant: w.RevisionAvant, TexteFR: w.TexteFR, Fondements: w.Fondements,
		Perimetre: w.Perimetre, SiIndetermine: w.SiIndetermine,
		SiIndetermineValidePar: w.SiIndetermineValidePar,
		Declencheur:            w.Declencheur, Arbre: w.Condition, Effets: w.Effets,
	}
	if d.Declencheur == nil {
		// "declencheur": null : la clé est là, la valeur non.
		d.Declencheur = &Declencheur{}
	}
	return nil
}
