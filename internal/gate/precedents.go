package gate

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"mcprogue/internal/journal"
)

// JetonsConsommes relit les journaux précédents désignés par un motif
// (filepath.Glob) et rend les empreintes des jetons d'approbation qu'ils
// ont acceptés. Chaque processus mcp-gate de la démo tient son propre
// journal : sans cette relecture, un jeton encore valide resservirait dans
// le processus suivant.
//
// La chaîne de chaque journal est vérifiée avec la clé donnée : un journal
// dont la chaîne est rompue (ou illisible) est une erreur, et le gate ne
// démarre pas, puisqu'il ne peut plus savoir quels jetons ont servi. Un
// motif qui ne désigne aucun fichier n'est pas une erreur (premier
// lancement). Rend aussi les chemins relus, triés.
func JetonsConsommes(motif string, cle []byte) (map[string]bool, []string, error) {
	chemins, err := filepath.Glob(motif)
	if err != nil {
		return nil, nil, fmt.Errorf("journaux précédents : motif %q invalide : %w", motif, err)
	}
	sort.Strings(chemins)
	out := map[string]bool{}
	for _, c := range chemins {
		b, err := os.ReadFile(c)
		if err != nil {
			return nil, nil, fmt.Errorf("journal précédent %s illisible : %w ; démarrage refusé", c, err)
		}
		if n, err := journal.Verifier(bytes.NewReader(b), cle); err != nil {
			return nil, nil, fmt.Errorf("journal précédent %s : chaîne rompue après %d entrée(s) vérifiée(s) (%v) ; "+
				"les jetons d'approbation déjà consommés ne peuvent pas être établis, démarrage refusé", c, n, err)
		}
		es, err := journal.Lire(bytes.NewReader(b))
		if err != nil {
			return nil, nil, fmt.Errorf("journal précédent %s illisible : %w ; démarrage refusé", c, err)
		}
		for _, en := range es {
			if en.Type == journal.ApprobationT && en.Approbation != nil && en.Approbation.Statut == journal.ApprobationAcceptee {
				for _, v := range en.Approbation.Valideurs {
					out[v.EmpreinteJeton] = true
				}
			}
		}
	}
	return out, chemins, nil
}
