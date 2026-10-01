package gate

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcprogue/internal/directive"
)

// Contrat de réponse à l'agent (docs/contrat-engine.md §6). Quatre cas
// que l'agent distingue :
//
//   - refus : isError: true, structuredContent Refus ;
//   - attente : isError: true, structuredContent Attente ;
//   - échec (source indisponible, ou erreur de protocole dont l'engine ne
//     transmet rien) : isError: true, structuredContent Echec ;
//   - réponse de l'outil, y compris quand l'outil décline (isError) :
//     contenu scellé, sans structuredContent.
//
// Le texte du résultat reprend le motif, pour un agent qui ne lit que le
// texte.

// Statuts du contrat.
const (
	StatutRefuse       = "refuse"
	StatutEnAttente    = "en_attente"
	StatutIndisponible = "indisponible"
)

// Origines d'un refus.
const (
	OrigineRefusDirective = "directive" // une directive du jeu, qui cite son fondement
	OrigineRefusRegistre  = "registre"  // le contrôle du registre : serveur non attesté
	OrigineRefusEngine    = "engine"    // l'engine lui-même : journal, coffre, session, appel mal formé
)

// Refus est le structuredContent d'un refus.
type Refus struct {
	Statut string `json:"statut"`
	// Origine précise qui refuse ; hors directive, Directive et Version
	// désignent le registre (son identifiant et sa version) ou restent
	// vides pour l'engine.
	Origine   string              `json:"origine"`
	Directive string              `json:"directive"`
	Version   int                 `json:"version"`
	Fondement directive.Fondement `json:"fondement"`
	Motif     string              `json:"motif"`
}

// Attente est le structuredContent d'un appel mis en attente de
// validation humaine. EmpreinteTransmise est ce qu'un jeton d'approbation
// devra viser.
//
// Roles sont les rôles dont un valideur doit chacun signer un jeton (vide :
// tout valideur de l'annuaire) ; QuatreYeux exige qu'aucun ne soit le
// demandeur. MotifJeton dit pourquoi les jetons d'une reprise ont été
// refusés : l'appel reste en attente.
type Attente struct {
	Statut             string   `json:"statut"`
	EmpreinteTransmise string   `json:"empreinte_transmise"`
	Directive          string   `json:"directive"`
	Version            int      `json:"version"`
	Roles              []string `json:"roles,omitempty"`
	QuatreYeux         bool     `json:"quatre_yeux,omitempty"`
	MotifJeton         string   `json:"motif_jeton,omitempty"`
}

func resultatErreur(texte string, structure any) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: texte}}, StructuredContent: structure}
}

// refusEngine : un refus de l'engine lui-même, sans directive.
func refusEngine(motif string) *mcp.CallToolResult {
	return resultatErreur(motif, Refus{Statut: StatutRefuse, Origine: OrigineRefusEngine,
		Fondement: directive.Fondement{Texte: "Contrat de l'engine"}, Motif: motif})
}

// premiere rend la première directive appliquée avec l'effet donné, dans
// l'ordre du jeu.
func premiere(dec directive.Decision, effet directive.Effet) (directive.Application, bool) {
	for _, a := range dec.Applications {
		if a.Appliquee && a.Effet == effet {
			return a, true
		}
	}
	return directive.Application{}, false
}

// refusDirective : un refus par une directive, qui cite son fondement.
func refusDirective(dec directive.Decision) *mcp.CallToolResult {
	texte := "Appel refusé par l'engine : " + dec.Motif + "."
	a, _ := premiere(dec, directive.Refuser)
	return resultatErreur(texte, Refus{Statut: StatutRefuse, Origine: OrigineRefusDirective,
		Directive: a.ID, Version: a.Version, Fondement: a.Fondement, Motif: dec.Motif})
}

// refusRegistre : un refus par le contrôle du registre.
func (g *Gate) refusRegistre(dec directive.Decision) *mcp.CallToolResult {
	r := g.cfg.Registre.Meta
	return resultatErreur("Appel "+dec.Motif+".", Refus{Statut: StatutRefuse, Origine: OrigineRefusRegistre,
		Directive: r.ID, Version: r.Version,
		Fondement: directive.Fondement{Texte: "Registre des serveurs : attestation du serveur"}, Motif: dec.Motif})
}

// enAttente : l'appel n'est pas transmis. La réponse dit ce qu'un jeton
// d'approbation doit viser (empreinte transmise) et quels rôles doivent
// valider ; motifJeton, s'il n'est pas vide, dit pourquoi les jetons de
// cette reprise ont été refusés.
func enAttente(dec directive.Decision, motifJeton string) *mcp.CallToolResult {
	a, _ := premiere(dec, directive.AttendreValidation)
	at := Attente{Statut: StatutEnAttente, EmpreinteTransmise: dec.EmpreinteTransmise, Directive: a.ID, Version: a.Version,
		MotifJeton: motifJeton}
	if v := dec.Validation; v != nil {
		at.Roles, at.QuatreYeux = v.Roles, v.QuatreYeux
	}
	texte := "Appel non transmis : " + dec.Motif + ". Reprendre le même appel avec, dans _meta." + CleJetonApprobation +
		", un jeton d'approbation par rôle requis, lié à l'empreinte transmise " + dec.EmpreinteTransmise + "."
	if motifJeton != "" {
		texte = "Jeton(s) d'approbation refusé(s) : " + motifJeton + ". " + texte
	}
	return resultatErreur(texte, at)
}

// Echec est le structuredContent d'un appel transmis dont rien
// d'utilisable n'est revenu. Serveur et Outil sont les noms logiques du
// registre.
type Echec struct {
	Statut  string `json:"statut"`
	Serveur string `json:"serveur"`
	Outil   string `json:"outil"`
	Motif   string `json:"motif"`
}

// echec : l'appel est parti, mais aucune donnée utilisable n'est revenue.
// Ce n'est ni un refus ni un outil qui décline. Le motif est de l'engine,
// jamais du serveur.
func echec(serveur, outil, motif string) *mcp.CallToolResult {
	return resultatErreur(motif, Echec{Statut: StatutIndisponible, Serveur: serveur, Outil: outil, Motif: motif})
}
