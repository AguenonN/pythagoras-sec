package rejeu

import (
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Statuts d'un résultat d'outil.
const (
	StatutOK           = "ok"
	StatutIntrouvable  = "introuvable"
	StatutIndisponible = "indisponible"
	StatutRefuse       = "requete_refusee"
)

// Entete ouvre chaque résultat d'outil des serveurs de données : le statut,
// un message lisible et la provenance de la donnée.
type Entete struct {
	Statut  string  `json:"statut" jsonschema:"ok, introuvable, indisponible ou requete_refusee"`
	Message string  `json:"message,omitempty" jsonschema:"explication quand le statut n'est pas ok"`
	Source  *Source `json:"source,omitempty" jsonschema:"provenance de la donnée ; origine rejeu signifie lue dans un enregistrement daté, pas obtenue à l'instant"`
}

// EnteteOK décrit une réponse obtenue.
func EnteteOK(rep *Reponse) Entete {
	s := rep.Source
	return Entete{Statut: StatutOK, Source: &s}
}

// EnteteErreur traduit une erreur d'Obtenir (ou d'analyse) en entête.
// Le second résultat porte IsError quand l'outil n'a pas pu répondre, pour
// que l'agent ne prenne pas l'absence de donnée pour une réponse.
func EnteteErreur(err error) (Entete, *mcp.CallToolResult) {
	switch {
	case errors.Is(err, ErrIntrouvable):
		return Entete{Statut: StatutIntrouvable, Message: err.Error()}, nil
	case errors.Is(err, ErrRequeteRefusee):
		return Entete{Statut: StatutRefuse, Message: err.Error()}, &mcp.CallToolResult{IsError: true}
	default:
		return Entete{Statut: StatutIndisponible, Message: err.Error()}, &mcp.CallToolResult{IsError: true}
	}
}
