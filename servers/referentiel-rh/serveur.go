package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Nom est le nom logique du serveur (internal/nommage). Tout ce que sert ce
// serveur est fictif.
const Nom = "referentiel-rh"

// AvertissementFictif accompagne chaque résultat.
const AvertissementFictif = "données fictives : groupes et appartenances inventés pour la démonstration"

//go:embed groupes_fictifs.json
var groupesFictifs []byte

// Referentiel : groupes connus et appartenances par salarié.
type Referentiel struct {
	Avertissement string              `json:"avertissement"`
	Groupes       map[string]string   `json:"groupes"`
	Appartenances map[string][]string `json:"appartenances"`
}

// Charger lit et contrôle les données embarquées : tout groupe cité est
// défini.
func Charger() (*Referentiel, error) {
	var r Referentiel
	if err := json.Unmarshal(groupesFictifs, &r); err != nil {
		return nil, err
	}
	for id, gs := range r.Appartenances {
		for _, g := range gs {
			if _, ok := r.Groupes[g]; !ok {
				return nil, fmt.Errorf("salarié %s : groupe %q non défini", id, g)
			}
		}
	}
	return &r, nil
}

// Statuts.
const (
	StatutOK          = "ok"
	StatutIntrouvable = "introuvable"
)

// EntreeGroupes est l'argument de groupes_salarie.
type EntreeGroupes struct {
	SalarieID string `json:"salarie_id" jsonschema:"identifiant du salarié, par exemple SAL-FICTIF-002"`
}

// Groupe est un groupe du référentiel.
type Groupe struct {
	Groupe      string `json:"groupe"`
	Description string `json:"description"`
}

// SortieGroupes est le résultat de groupes_salarie.
//
// Un salarié inconnu donne statut introuvable et isError, sans champ
// groupes : il ne se lit jamais comme « aucun groupe ».
type SortieGroupes struct {
	Statut          string    `json:"statut"`
	Message         string    `json:"message,omitempty"`
	DonneesFictives string    `json:"donnees_fictives" jsonschema:"rappel : tout ce que sert ce serveur est fictif"`
	SalarieID       string    `json:"salarie_id"`
	Groupes         *[]Groupe `json:"groupes,omitempty" jsonschema:"groupes du salarié ; présent seulement si le statut est ok (liste vide s'il n'appartient à aucun groupe)"`
}

// Serveur construit le serveur MCP referentiel-rh.
func Serveur(r *Referentiel) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: Nom, Version: "0.1.0"}, nil)
	faux := false
	mcp.AddTool(s, &mcp.Tool{
		Name:        "groupes_salarie",
		Description: "Groupes d'un salarié de l'entreprise synthétique dans le référentiel RH (données fictives), par exemple salaries-proteges ou contentieux-en-cours.",
		Annotations: &mcp.ToolAnnotations{Title: "Groupes d'un salarié (lecture)", ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: &faux},
	}, func(_ context.Context, _ *mcp.CallToolRequest, in EntreeGroupes) (*mcp.CallToolResult, SortieGroupes, error) {
		return r.groupes(in)
	})
	return s
}

func (r *Referentiel) groupes(in EntreeGroupes) (*mcp.CallToolResult, SortieGroupes, error) {
	out := SortieGroupes{DonneesFictives: AvertissementFictif, SalarieID: in.SalarieID}
	noms, ok := r.Appartenances[in.SalarieID]
	if !ok {
		out.Statut = StatutIntrouvable
		out.Message = fmt.Sprintf("salarié %q absent du référentiel : ses groupes sont inconnus (ce n'est pas « aucun groupe »)", in.SalarieID)
		return &mcp.CallToolResult{IsError: true}, out, nil
	}
	gs := make([]Groupe, 0, len(noms))
	for _, g := range noms {
		gs = append(gs, Groupe{Groupe: g, Description: r.Groupes[g]})
	}
	sort.Slice(gs, func(i, j int) bool { return gs[i].Groupe < gs[j].Groupe })
	out.Statut = StatutOK
	out.Groupes = &gs
	return nil, out, nil
}
