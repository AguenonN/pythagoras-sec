package gate

import (
	"errors"
	"fmt"

	"mcprogue/internal/directive"
	"mcprogue/internal/registre"
)

// ErrCibleHorsRegistre : une directive vise un serveur ou un outil que le
// registre ne connaît pas.
var ErrCibleHorsRegistre = errors.New("gate: directive visant une cible absente du registre")

// verifierCibles refuse un jeu dont une directive, quel que soit son
// statut, vise un serveur ou un outil absent du registre. Une directive
// dont la cible n'existe pas ne s'appliquerait jamais, sans que rien ne le
// dise : c'est une erreur de rédaction, pas une règle. Les jokers sont
// admis ; un outil nommé sous un serveur joker doit exister sur au moins
// un serveur. Toutes les cibles fautives sont rendues en bloc.
func verifierCibles(jeu directive.Jeu, reg *registre.Registre) error {
	var errs []error
	for _, d := range jeu.Directives {
		c := d.Cible
		switch {
		case c.Serveur == "*" && c.Outil == "*":
		case c.Serveur == "*":
			trouve := false
			for _, s := range reg.Serveurs {
				if _, ok := s.Outil(c.Outil); ok {
					trouve = true
				}
			}
			if !trouve {
				errs = append(errs, fmt.Errorf("%w : %s vise l'outil %s, qu'aucun serveur du registre n'expose", ErrCibleHorsRegistre, d.ID, c.Outil))
			}
		default:
			s, ok := reg.Serveur(c.Serveur)
			if !ok {
				errs = append(errs, fmt.Errorf("%w : %s vise le serveur %s, absent du registre", ErrCibleHorsRegistre, d.ID, c.Serveur))
				continue
			}
			if c.Outil == "*" {
				continue
			}
			if _, ok := s.Outil(c.Outil); !ok {
				errs = append(errs, fmt.Errorf("%w : %s vise l'outil %s/%s, absent du registre", ErrCibleHorsRegistre, d.ID, c.Serveur, c.Outil))
			}
		}
	}
	return errors.Join(errs...)
}
