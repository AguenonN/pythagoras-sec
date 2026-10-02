package relecture

import (
	"fmt"
	"strconv"
	"strings"

	"mcprogue/internal/directive"
)

// Ajout rend la relecture des seules directives ids d'un jeu chargé : ce
// que le juriste ajoute à un jeu existant (version n+1 du même jeu). Les
// directives existantes ne sont pas relues ; l'en-tête dit où les ajoutées
// prennent place. Un identifiant absent du jeu est une erreur.
func Ajout(j *directive.Jeu, ids []string) (string, error) {
	if len(ids) == 0 {
		return "", fmt.Errorf("relecture: aucune directive à relire")
	}
	pos := make(map[string]int, len(j.Directives))
	for i, d := range j.Directives {
		pos[d.ID] = i
	}
	vues := make(map[string]bool, len(ids))
	for _, id := range ids {
		if _, ok := pos[id]; !ok {
			return "", fmt.Errorf("relecture: directive %s absente du jeu", strconv.Quote(id))
		}
		if vues[id] {
			return "", fmt.Errorf("relecture: directive %s citée deux fois", strconv.Quote(id))
		}
		vues[id] = true
	}
	var b strings.Builder
	m := j.Meta
	n := len(j.Directives) - len(ids)
	fmt.Fprintf(&b, "Ajout au jeu %s, version %d : %s (%s), évaluée%s après %s, dans l'ordre du jeu ; "+
		"si plusieurs s'appliquent au même appel, l'effet le plus contraignant l'emporte "+
		"(refus, puis validation humaine, puis caviardage).\n",
		m.ID, m.Version, compte(len(ids), "directive ajoutée", "directives ajoutées"), strings.Join(ids, ", "),
		s(len(ids)), compte(n, "directive existante", "directives existantes"))
	b.WriteString("Les notifications et les marques des directives appliquées se cumulent, quel que soit l'effet retenu.\n")
	for i, d := range j.Directives {
		if vues[d.ID] {
			b.WriteString("\n")
			b.WriteString(Directive(j.Directives[i]))
		}
	}
	return b.String(), nil
}
