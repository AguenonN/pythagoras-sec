package gate

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcprogue/internal/annuaire"
	"mcprogue/internal/directive"
	"mcprogue/internal/journal"
)

// Reprise d'un appel en attente de validation (docs/contrat-engine.md §6).
// L'agent renvoie le même appel avec, dans _meta.approvalToken, un jeton
// d'approbation par rôle requis (une chaîne, ou une liste de chaînes),
// chacun signé par un valideur de l'annuaire et lié à l'empreinte
// transmise. Decide est réévalué : si l'appel attend toujours une
// validation et que les jetons la satisfont, l'entrée approbation est
// écrite, puis l'appel est transmis comme une autorisation.

// CleJetonApprobation est la clé de _meta qui porte les jetons.
const CleJetonApprobation = "approvalToken"

// Bornes des jetons lus dans _meta : ce qui vient de l'agent est borné.
const (
	JetonsMax        = 8
	LongueurMaxJeton = 8 << 10
)

// MecanismeAnnuaire nomme, au journal, la validation par jetons signés
// par des identités de l'annuaire.
const MecanismeAnnuaire = "jeton-annuaire"

// jetonsApprobation lit _meta.approvalToken : absent, aucun jeton ; une
// chaîne ou une liste de chaînes non vides, bornées. Toute autre forme est
// illisible.
func jetonsApprobation(meta mcp.Meta) ([]string, bool) {
	v, ok := meta[CleJetonApprobation]
	if !ok || v == nil {
		return nil, true
	}
	var brut []any
	switch x := v.(type) {
	case string:
		brut = []any{x}
	case []any:
		brut = x
	default:
		return nil, false
	}
	if len(brut) > JetonsMax {
		return nil, false
	}
	out := make([]string, 0, len(brut))
	for _, e := range brut {
		s, ok := e.(string)
		if !ok || s == "" || len(s) > LongueurMaxJeton {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}

// verifierValidation confronte les jetons à la validation que demande la
// décision, à la date ts de l'appel :
//
//   - chaque jeton se vérifie (signature du valideur, période, appel visé,
//     rôle détenu à la date de validation) ;
//   - un jeton par rôle requis, ni plus ni moins ; sans rôle requis, un
//     seul jeton, de tout valideur de l'annuaire ;
//   - des valideurs distincts, dont aucun n'est le demandeur ;
//   - quatre yeux demandés sans demandeur résolu : refus, la règle ne peut
//     pas être vérifiée.
//
// Rend toujours une approbation, acceptée ou refusée avec son motif.
func (g *Gate) verifierValidation(dec directive.Decision, jetons []string, demandeur *directive.Demandeur, ts time.Time) *journal.Approbation {
	var roles []string
	quatre := false
	if v := dec.Validation; v != nil {
		roles, quatre = v.Roles, v.QuatreYeux
	}
	a := &journal.Approbation{Mecanisme: MecanismeAnnuaire, Statut: journal.ApprobationRefusee,
		EmpreinteTransmise: dec.EmpreinteTransmise, Roles: roles, QuatreYeux: quatre}
	refus := func(format string, args ...any) *journal.Approbation {
		a.Motif = fmt.Sprintf(format, args...)
		return a
	}
	if g.cfg.Annuaire == nil {
		return refus("aucun annuaire : les jetons d'approbation ne peuvent pas être vérifiés")
	}
	if quatre && demandeur == nil {
		return refus("quatre yeux demandés, mais le demandeur n'est pas résolu : la règle ne peut pas être vérifiée")
	}
	vus := map[string]bool{}
	parRole := map[string]int{}
	for i, j := range jetons {
		ap, err := annuaire.VerifierApprobation(g.cfg.Annuaire, j, dec.EmpreinteTransmise, ts)
		if err != nil {
			return refus("jeton %d refusé : %v", i+1, err)
		}
		if demandeur != nil && ap.Valideur == demandeur.Identite {
			return refus("jeton %d : le valideur %s est le demandeur", i+1, ap.Valideur)
		}
		if vus[ap.Valideur] {
			return refus("jeton %d : %s a déjà validé cet appel ; chaque rôle demande un valideur distinct", i+1, ap.Valideur)
		}
		vus[ap.Valideur] = true
		parRole[ap.Role]++
		a.Valideurs = append(a.Valideurs, journal.Valideur{Identite: ap.Valideur, Role: ap.Role, Date: ap.Date, EmpreinteJeton: ap.EmpreinteJeton})
	}
	if len(roles) == 0 {
		if len(jetons) != 1 {
			return refus("un seul jeton attendu, %d reçus", len(jetons))
		}
	} else {
		var manquants []string
		for _, r := range roles {
			if parRole[r] == 0 {
				manquants = append(manquants, r)
			}
		}
		if len(manquants) > 0 {
			return refus("rôle(s) sans validation : %s", strings.Join(manquants, ", "))
		}
		if len(jetons) != len(roles) {
			return refus("un jeton par rôle requis (%d), %d reçus", len(roles), len(jetons))
		}
	}
	// Usage unique : un jeton déjà accepté ne vaut plus, même pour un
	// appel identique (sinon une validation autoriserait deux prêts).
	g.jetonsMu.Lock()
	defer g.jetonsMu.Unlock()
	for i, v := range a.Valideurs {
		if g.jetonsUtilises[v.EmpreinteJeton] {
			return refus("jeton %d déjà utilisé : une validation ne vaut que pour un appel", i+1)
		}
	}
	for _, v := range a.Valideurs {
		g.jetonsUtilises[v.EmpreinteJeton] = true
	}
	sort.Slice(a.Valideurs, func(i, j int) bool { return a.Valideurs[i].Role < a.Valideurs[j].Role })
	var noms []string
	for _, v := range a.Valideurs {
		noms = append(noms, v.Identite)
	}
	a.ApprouvePar = strings.Join(noms, ", ")
	a.Statut, a.Motif = journal.ApprobationAcceptee, ""
	return a
}
