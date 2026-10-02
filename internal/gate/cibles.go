package gate

import (
	"errors"
	"fmt"
	"strings"

	"mcprogue/internal/directive"
	"mcprogue/internal/registre"
)

// ErrCibleHorsRegistre : une directive vise, ou cite dans sa condition,
// un serveur, un outil ou une étiquette que le registre ne connaît pas.
var ErrCibleHorsRegistre = errors.New("gate: directive visant une cible absente du registre")

// verifierCibles refuse un jeu dont une directive, quel que soit son
// statut, vise un serveur ou un outil absent du registre, ou en nomme un
// dans sa condition. Une telle directive ne se déclencherait jamais, sans
// que rien ne le dise : c'est une erreur de rédaction, pas une règle.
// Toutes les fautes sont rendues en bloc. Une directive sans déclencheur
// (que le chargeur refuse déjà) est une faute : fermé par défaut.
func verifierCibles(jeu directive.Jeu, reg *registre.Registre) error {
	var errs []error
	for _, d := range jeu.Directives {
		if d.Declencheur == nil {
			errs = append(errs, fmt.Errorf("%w : %s n'a pas de déclencheur", ErrCibleHorsRegistre, d.ID))
			continue
		}
		errs = append(errs, verifierCiblesV2(d, reg)...)
	}
	return errors.Join(errs...)
}

// verifierCiblesV2 applique la même règle à une directive v2 : la cible du
// déclencheur (serveur, outil, étiquette) et les noms que citent les
// feuilles (session.a_consulte, metadonnee_reponse, etiquette_cible)
// doivent exister au registre. Une étiquette que le registre ne pose
// nulle part ne correspondrait jamais.
func verifierCiblesV2(d directive.Directive, reg *registre.Registre) []error {
	var errs []error
	faute := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("%w : %s "+format, append([]any{ErrCibleHorsRegistre, d.ID}, args...)...))
	}
	etiquettes := map[string]bool{}
	for _, e := range reg.Etiquettes() {
		etiquettes[e] = true
	}
	etiquette := func(quoi, e string) {
		if e != "" && !etiquettes[e] {
			faute("%s l'étiquette %s, que le registre ne pose sur aucun serveur ni outil", quoi, e)
		}
	}
	serveur := func(quoi, nom string) {
		if nom == "" || nom == "*" {
			return
		}
		if _, ok := reg.Serveur(nom); !ok {
			faute("%s le serveur %s, absent du registre", quoi, nom)
		}
	}
	outil := func(quoi, so string) {
		if so == "" {
			return
		}
		nom, o, _ := strings.Cut(so, "/")
		s, ok := reg.Serveur(nom)
		if ok {
			_, ok = s.Outil(o)
		}
		if !ok {
			faute("%s l'outil %s, absent du registre", quoi, so)
		}
	}
	if c := d.Declencheur.Cible; c != nil {
		serveur("vise", c.Serveur)
		etiquette("vise", c.Etiquette)
		switch {
		case c.Outil == "" || c.Outil == "*":
		case c.Serveur == "" || c.Serveur == "*":
			trouve := false
			for _, s := range reg.Serveurs {
				if _, ok := s.Outil(c.Outil); ok {
					trouve = true
				}
			}
			if !trouve {
				faute("vise l'outil %s, qu'aucun serveur du registre n'expose", c.Outil)
			}
		default:
			if _, ok := reg.Serveur(c.Serveur); ok {
				outil("vise", c.Serveur+"/"+c.Outil)
			}
		}
	}
	var parcourir func(*directive.Noeud)
	parcourir = func(n *directive.Noeud) {
		if n == nil {
			return
		}
		for _, e := range append(append([]*directive.Noeud{}, n.Et...), n.Ou...) {
			parcourir(e)
		}
		parcourir(n.Non)
		if n.Existe != nil {
			parcourir(n.Existe.Condition)
		}
		if n.Tous != nil {
			parcourir(n.Tous.Condition)
		}
		if f := n.AConsulte; f != nil {
			serveur("attend dans sa condition", f.Serveur)
			outil("attend dans sa condition", f.Outil)
			etiquette("attend dans sa condition", f.Etiquette)
		}
		if f := n.MetadonneeReponse; f != nil {
			serveur("lit dans sa condition", f.Serveur)
			etiquette("lit dans sa condition", f.Etiquette)
		}
		if f := n.EtiquetteCible; f != nil {
			etiquette("teste dans sa condition", f.Contient)
		}
	}
	parcourir(d.Arbre)
	return errs
}
