package console

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Suggestion est une demande proposée au Poste conseiller : un clic remplit
// la demande et le demandeur, rien n'est lancé sans le bouton « Lancer les
// agents ».
type Suggestion struct {
	Fichier   string `json:"fichier"`
	Titre     string `json:"titre"`
	Demande   string `json:"demande"`
	Demandeur string `json:"demandeur"`
}

// Suggestions lit demo/scenarios/*.json (en lecture seule) : titre, demande
// et demandeur de chaque scénario. Un scénario sans titre, sans demande ou
// illisible est ignoré. Triées par nom de fichier.
func (d *Depot) Suggestions() []Suggestion {
	out := []Suggestion{}
	fichiers, _ := filepath.Glob(filepath.Join(d.Chemins().Scenarios, "*.json"))
	sort.Strings(fichiers)
	for _, f := range fichiers {
		brut, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var s struct {
			Titre     string `json:"titre"`
			Demande   string `json:"demande"`
			Demandeur string `json:"demandeur"`
		}
		if json.Unmarshal(brut, &s) != nil {
			continue
		}
		titre, demande := strings.TrimSpace(s.Titre), strings.TrimSpace(s.Demande)
		if titre == "" || demande == "" {
			continue
		}
		out = append(out, Suggestion{Fichier: strings.TrimSuffix(filepath.Base(f), ".json"), Titre: titre, Demande: demande,
			Demandeur: strings.TrimSpace(s.Demandeur)})
	}
	return out
}
