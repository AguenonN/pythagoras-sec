package directive

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"
)

// Une directive s'écrit sous la forme de sa version. Le discriminant est la
// clé "declencheur" : présente, la directive est v2 ; sinon v1. Le décodage
// est strict dans les deux cas (champs inconnus refusés, nombres gardés en
// json.Number), parce que les options du décodeur appelant ne traversent
// pas un UnmarshalJSON.

// directiveV1 est la forme écrite v1, champ pour champ celle d'avant la v2 :
// l'empreinte d'un jeu v1 ne change pas.
type directiveV1 struct {
	ID                     string        `json:"id"`
	Version                int           `json:"version"`
	Statut                 Statut        `json:"statut"`
	DateEffet              time.Time     `json:"date_effet"`
	RevisionAvant          *time.Time    `json:"revision_avant,omitempty"`
	TexteFR                string        `json:"texte_fr"`
	Fondement              Fondement     `json:"fondement"`
	Verrou                 Verrou        `json:"verrou"`
	Cible                  Cible         `json:"cible"`
	Perimetre              *Perimetre    `json:"perimetre,omitempty"`
	Condition              []Condition   `json:"condition,omitempty"`
	SiIndetermine          SiIndetermine `json:"si_indetermine,omitempty"`
	SiIndetermineValidePar string        `json:"si_indetermine_valide_par,omitempty"`
	Provenance             *Provenance   `json:"provenance,omitempty"`
	Champs                 *Champs       `json:"champs,omitempty"`
	Effet                  Effet         `json:"effet"`
}

// directiveV2 est la forme écrite v2 (docs/schema-directive-v2.md §3).
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

// MarshalJSON écrit la directive sous la forme de sa version.
func (d Directive) MarshalJSON() ([]byte, error) {
	if d.EstV2() {
		return json.Marshal(directiveV2{
			ID: d.ID, Version: d.Version, Statut: d.Statut, DateEffet: d.DateEffet,
			RevisionAvant: d.RevisionAvant, TexteFR: d.TexteFR, Fondements: d.Fondements,
			Perimetre: d.Perimetre, SiIndetermine: d.SiIndetermine,
			SiIndetermineValidePar: d.SiIndetermineValidePar,
			Declencheur:            d.Declencheur, Condition: d.Arbre, Effets: d.Effets,
		})
	}
	return json.Marshal(directiveV1{
		ID: d.ID, Version: d.Version, Statut: d.Statut, DateEffet: d.DateEffet,
		RevisionAvant: d.RevisionAvant, TexteFR: d.TexteFR, Fondement: d.Fondement,
		Verrou: d.Verrou, Cible: d.Cible, Perimetre: d.Perimetre, Condition: d.Condition,
		SiIndetermine: d.SiIndetermine, SiIndetermineValidePar: d.SiIndetermineValidePar,
		Provenance: d.Provenance, Champs: d.Champs, Effet: d.Effet,
	})
}

// UnmarshalJSON lit une directive v1 ou v2, strictement.
func (d *Directive) UnmarshalJSON(b []byte) error {
	var cles map[string]json.RawMessage
	if err := json.Unmarshal(b, &cles); err != nil {
		return err
	}
	_, v2 := cles["declencheur"]
	_, v1 := cles["verrou"]
	if v1 && v2 {
		return fmt.Errorf("directive: porte à la fois verrou (v1) et declencheur (v2)")
	}
	strict := func(dst any) error {
		dec := json.NewDecoder(bytes.NewReader(b))
		dec.DisallowUnknownFields()
		dec.UseNumber()
		return dec.Decode(dst)
	}
	if v2 {
		var w directiveV2
		if err := strict(&w); err != nil {
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
	var w directiveV1
	if err := strict(&w); err != nil {
		return err
	}
	*d = Directive{
		ID: w.ID, Version: w.Version, Statut: w.Statut, DateEffet: w.DateEffet,
		RevisionAvant: w.RevisionAvant, TexteFR: w.TexteFR, Fondement: w.Fondement,
		Verrou: w.Verrou, Cible: w.Cible, Perimetre: w.Perimetre, Condition: w.Condition,
		SiIndetermine: w.SiIndetermine, SiIndetermineValidePar: w.SiIndetermineValidePar,
		Provenance: w.Provenance, Champs: w.Champs, Effet: w.Effet,
	}
	return nil
}
