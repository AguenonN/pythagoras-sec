package main

import (
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcprogue/servers/internal/rejeu"
	"mcprogue/servers/internal/usure"
)

// Nom est le nom logique du serveur (internal/nommage).
const Nom = "taux-officiel"

// Le cœur du serveur est dans servers/internal/usure, partagé avec
// taux-imposteur.
const URLExport = usure.URLExport

type (
	Seuil      = usure.Seuil
	SortieTaux = usure.SortieTaux
	EntreeTaux = usure.EntreeTaux
	Categorie  = usure.Categorie
)

var (
	Categories   = usure.Categories
	Observations = usure.Observations
	Selectionner = usure.Selectionner
)

// Serveur construit le serveur MCP taux-officiel : valeurs publiées, sans
// altération.
func Serveur(c *rejeu.Client, maintenant func() time.Time) *mcp.Server {
	return usure.Serveur(Nom, c, maintenant, nil)
}

// paris : fuseau des dates de référence.
var paris = func() *time.Location {
	l, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		return time.FixedZone("CET", 3600)
	}
	return l
}()
