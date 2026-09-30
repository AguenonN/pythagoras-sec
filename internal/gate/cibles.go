package gate

import (
	"errors"
	"fmt"
	"strings"

	"mcprogue/internal/directive"
	"mcprogue/internal/registre"
)

// ErrCibleHorsRegistre : une directive vise, ou attend dans sa provenance,
// un serveur ou un outil que le registre ne connaît pas.
var ErrCibleHorsRegistre = errors.New("gate: directive visant une cible absente du registre")

// verifierCibles refuse un jeu dont une directive, quel que soit son
// statut, vise un serveur ou un outil absent du registre, ou en nomme un
// dans sa provenance (provenance.serveurs, provenance.outils). Une telle
// directive ne se déclencherait jamais, sans que rien ne le dise : c'est
// une erreur de rédaction, pas une règle. Les jokers de cible sont admis ;
// un outil nommé sous un serveur joker doit exister sur au moins un
// serveur. Toutes les fautes sont rendues en bloc.
func verifierCibles(jeu directive.Jeu, reg *registre.Registre) error {
	var errs []error
	for _, d := range jeu.Directives {
		if p := d.Provenance; p != nil {
			for _, nom := range p.Serveurs {
				if _, ok := reg.Serveur(nom); !ok {
					errs = append(errs, fmt.Errorf("%w : %s attend dans sa provenance le serveur %s, absent du registre", ErrCibleHorsRegistre, d.ID, nom))
				}
			}
			for _, so := range p.Outils {
				nom, outil, _ := strings.Cut(so, "/")
				s, ok := reg.Serveur(nom)
				if ok {
					_, ok = s.Outil(outil)
				}
				if !ok {
					errs = append(errs, fmt.Errorf("%w : %s attend dans sa provenance l'outil %s, absent du registre", ErrCibleHorsRegistre, d.ID, so))
				}
			}
		}
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
