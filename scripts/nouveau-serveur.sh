#!/usr/bin/env bash
# Crée le squelette d'un serveur MCP de démonstration dans servers/<nom>/, sur
# le modèle de banque-synthetique : données FICTIVES embarquées (un fichier
# JSON), un outil de lecture « lire_fiche », un test, et son schema.json
# (tools/list exporté du binaire).
#
#	scripts/nouveau-serveur.sh <nom>        # nom logique : minuscules, chiffres, tirets
#
# Ensuite, suivre docs/equipe/ajouter-un-serveur.md (manifeste, étiquettes,
# registre). Rien n'est ajouté au registre par ce script.
set -euo pipefail

racine="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
nom="${1:-}"
nom_valide='^[a-z0-9]([a-z0-9-]*[a-z0-9])?$' # internal/nommage.ValiderNomLogique
if [[ -z "$nom" ]] || ! [[ "$nom" =~ $nom_valide ]] || [[ "$nom" == *__* ]]; then
	echo "usage : scripts/nouveau-serveur.sh <nom>   (minuscules, chiffres, tirets ; pas de « __ »)" >&2
	exit 2
fi
dir="$racine/servers/$nom"
if [[ -e "$dir" ]]; then
	echo "nouveau-serveur: servers/$nom existe déjà" >&2
	exit 1
fi
mkdir -p "$dir"

cat >"$dir/main.go" <<EOF
// Commande $nom : serveur MCP de démonstration, données FICTIVES embarquées
// (donnees_fictives.json). Transports : stdio, ou HTTP streamable TLS.
package main

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcprogue/servers/internal/lanceur"
)

func main() {
	lanceur.Lancer(Nom, func(lanceur.Config) (*mcp.Server, error) {
		fiches, err := ChargerFiches()
		if err != nil {
			return nil, err
		}
		return Serveur(fiches), nil
	})
}
EOF

cat >"$dir/serveur.go" <<EOF
package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Nom est le nom logique du serveur (internal/nommage). Tout ce qu'il sert
// est fictif.
const Nom = "$nom"

//go:embed donnees_fictives.json
var donneesFictives []byte

// Fiche est un enregistrement fictif. Ajouter ici les champs du jeu de données.
type Fiche struct {
	ID          string \`json:"id"\`
	Libelle     string \`json:"libelle"\`
	Commentaire string \`json:"commentaire"\`
}

type donnees struct {
	Avertissement string  \`json:"avertissement"\`
	Fiches        []Fiche \`json:"fiches"\`
}

// ChargerFiches lit les données embarquées.
func ChargerFiches() (map[string]Fiche, error) {
	var d donnees
	if err := json.Unmarshal(donneesFictives, &d); err != nil {
		return nil, err
	}
	out := map[string]Fiche{}
	for _, f := range d.Fiches {
		if _, dup := out[f.ID]; dup {
			return nil, fmt.Errorf("fiche %s en double", f.ID)
		}
		out[f.ID] = f
	}
	return out, nil
}

// EntreeFiche est l'argument de lire_fiche.
type EntreeFiche struct {
	ID string \`json:"id" jsonschema:"identifiant de la fiche, par exemple FICHE-FICTIVE-001"\`
}

// SortieFiche est le résultat de lire_fiche.
type SortieFiche struct {
	Statut          string \`json:"statut"\`
	Message         string \`json:"message,omitempty"\`
	DonneesFictives string \`json:"donnees_fictives"\`
	Fiche           *Fiche \`json:"fiche,omitempty"\`
}

const avertissement = "données fictives inventées pour la démonstration"

// Serveur construit le serveur MCP. Un outil de lecture (readOnlyHint :
// accès « read » au registre) ; un outil d'action n'a pas readOnlyHint et
// sera classé « action ».
func Serveur(fiches map[string]Fiche) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: Nom, Version: "0.1.0"}, nil)
	faux := false
	mcp.AddTool(s, &mcp.Tool{
		Name:        "lire_fiche",
		Description: "Lit une fiche du serveur $nom (données fictives).",
		Annotations: &mcp.ToolAnnotations{Title: "Lire une fiche (lecture)", ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: &faux},
	}, func(_ context.Context, _ *mcp.CallToolRequest, in EntreeFiche) (*mcp.CallToolResult, SortieFiche, error) {
		f, ok := fiches[in.ID]
		if !ok {
			ids := make([]string, 0, len(fiches))
			for id := range fiches {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			return &mcp.CallToolResult{IsError: true}, SortieFiche{Statut: "introuvable", DonneesFictives: avertissement,
				Message: fmt.Sprintf("fiche %q inconnue ; fiches : %v", in.ID, ids)}, nil
		}
		return nil, SortieFiche{Statut: "ok", DonneesFictives: avertissement, Fiche: &f}, nil
	})
	return s
}
EOF

cat >"$dir/donnees_fictives.json" <<EOF
{
  "avertissement": "DONNÉES FICTIVES inventées pour la démonstration ; aucune personne réelle.",
  "fiches": [
    {"id": "FICHE-FICTIVE-001", "libelle": "Première fiche (fictive)", "commentaire": "À remplacer par les données de la démo."}
  ]
}
EOF

cat >"$dir/serveur_test.go" <<EOF
package main

import (
	"testing"

	"mcprogue/internal/nommage"
	"mcprogue/servers/internal/essai"
)

func TestNom(t *testing.T) {
	if err := nommage.ValiderNomLogique(Nom); err != nil {
		t.Fatal(err)
	}
}

func TestLireFiche(t *testing.T) {
	f, err := ChargerFiches()
	if err != nil {
		t.Fatal(err)
	}
	cs := essai.Connecter(t, Serveur(f))
	var out SortieFiche
	if res := essai.Appeler(t, cs, "lire_fiche", map[string]any{"id": "FICHE-FICTIVE-001"}, &out); res.IsError || out.Fiche == nil {
		t.Fatalf("%+v", out)
	}
	if res := essai.Appeler(t, cs, "lire_fiche", map[string]any{"id": "absente"}, &out); !res.IsError || out.Statut != "introuvable" {
		t.Fatalf("%+v", out)
	}
}

// schema.json est la sortie brute de tools/list : à régénérer après toute
// modification des outils (voir docs/equipe/ajouter-un-serveur.md).
func TestSchema(t *testing.T) {
	f, err := ChargerFiches()
	if err != nil {
		t.Fatal(err)
	}
	essai.VerifierSchema(t, essai.Connecter(t, Serveur(f)), "schema.json")
}
EOF

cd "$racine"
gofmt -w "$dir"/*.go
travail="$(mktemp -d)"
trap 'rm -rf "$travail"' EXIT
CGO_ENABLED=0 go build -trimpath -buildvcs=false -o "$travail/$nom" "./servers/$nom"
go build -o "$travail/exporter-schema" ./servers/exporter-schema
DONNEES_MODE=REJEU "$travail/exporter-schema" -o "$dir/schema.json" "$travail/$nom"
go test "./servers/$nom/"
echo "nouveau-serveur: servers/$nom créé (main.go, serveur.go, donnees_fictives.json, serveur_test.go, schema.json)." >&2
echo "  Suite : docs/equipe/ajouter-un-serveur.md" >&2
