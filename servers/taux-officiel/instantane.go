package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"mcprogue/servers/internal/rejeu"
)

// Instantane est le contenu de instantane_trimestre_precedent.json : les
// seuils du trimestre qui précède celui en vigueur à la date de référence.
// Il sert de « base interne périmée » dans la démo ; ce n'est jamais un seuil
// applicable.
type Instantane struct {
	Nature              string            `json:"nature"`
	Avertissement       string            `json:"avertissement"`
	DateReference       string            `json:"date_reference"`
	TrimestreEnVigueur  string            `json:"trimestre_en_vigueur_a_la_date_reference"`
	TrimestreInstantane string            `json:"trimestre_instantane"`
	Source              string            `json:"source"`
	Seuils              []SeuilInstantane `json:"seuils"`
}

// SeuilInstantane est le seuil périmé d'une catégorie.
type SeuilInstantane struct {
	Categorie        string `json:"categorie"`
	Libelle          string `json:"libelle"`
	IdentifiantSerie string `json:"identifiant_serie"`
	Seuil
	EnregistrementDate   string `json:"enregistrement_date"`
	EnregistrementSHA256 string `json:"enregistrement_sha256"`
}

// genererInstantane lit les enregistrements (jamais le réseau) et construit
// l'instantané pour la date de référence.
func genererInstantane(ctx context.Context, c *rejeu.Client, date string) (*Instantane, error) {
	inst := &Instantane{
		Nature:        "base interne périmée",
		Avertissement: "Seuils de l'usure du trimestre PRÉCÉDENT celui en vigueur à la date de référence. Valeurs réelles publiées par la Banque de France, volontairement périmées pour la démonstration : ne jamais les appliquer.",
		DateReference: date,
		Source:        "Webstat, Banque de France, export CSV public " + URLExport + "<identifiant_serie>",
	}
	for _, cat := range Categories {
		obs, rep, err := Observations(ctx, c, cat)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", cat.Code, err)
		}
		var prec *Seuil
		for i := range obs {
			if obs[i].Debut <= date && obs[i].Fin >= date {
				if inst.TrimestreEnVigueur == "" {
					inst.TrimestreEnVigueur = obs[i].Trimestre
				} else if inst.TrimestreEnVigueur != obs[i].Trimestre {
					return nil, fmt.Errorf("%s: trimestre en vigueur %s, attendu %s", cat.Code, obs[i].Trimestre, inst.TrimestreEnVigueur)
				}
				if i+1 < len(obs) {
					prec = &obs[i+1]
				}
				break
			}
		}
		if prec == nil {
			return nil, fmt.Errorf("%s: pas de trimestre précédent celui en vigueur au %s", cat.Code, date)
		}
		if inst.TrimestreInstantane == "" {
			inst.TrimestreInstantane = prec.Trimestre
		} else if inst.TrimestreInstantane != prec.Trimestre {
			return nil, fmt.Errorf("%s: trimestre précédent %s, attendu %s", cat.Code, prec.Trimestre, inst.TrimestreInstantane)
		}
		inst.Seuils = append(inst.Seuils, SeuilInstantane{
			Categorie:            cat.Code,
			Libelle:              cat.Libelle,
			IdentifiantSerie:     cat.Serie,
			Seuil:                *prec,
			EnregistrementDate:   rep.Source.Date,
			EnregistrementSHA256: rep.Source.SHA256,
		})
	}
	return inst, nil
}

// commandeInstantane : taux-officiel instantane [-date AAAA-MM-JJ] [-o fichier]
func commandeInstantane(args []string) error {
	fs := flag.NewFlagSet("instantane", flag.ContinueOnError)
	date := fs.String("date", time.Now().In(paris).Format(time.DateOnly), "date de référence")
	dir := fs.String("enregistrements", "servers/"+Nom+"/enregistrements", "répertoire des enregistrements")
	sortie := fs.String("o", "servers/"+Nom+"/instantane_trimestre_precedent.json", "fichier de sortie")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if _, err := time.Parse(time.DateOnly, *date); err != nil {
		return fmt.Errorf("-date %q : AAAA-MM-JJ attendu", *date)
	}
	inst, err := genererInstantane(context.Background(), rejeu.Nouveau(*dir, rejeu.Rejeu), *date)
	if err != nil {
		return err
	}
	b, err := encoderInstantane(inst)
	if err != nil {
		return err
	}
	return os.WriteFile(*sortie, b, 0o644)
}

func encoderInstantane(inst *Instantane) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(inst); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
