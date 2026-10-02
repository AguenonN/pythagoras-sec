package directive

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Évaluation d'un arbre de condition en logique de Kleene forte
// (docs/schema-directive-v2.md §5 et §7). Chaque nœud rend son résultat à
// trois valeurs et une explication qui ne cite que la directive : champs,
// opérateurs, valeurs de seuil, positions d'éléments ; jamais une valeur
// d'argument, une clé reçue ni un identifiant de personne.

// contexte porte les entrées de Decide pour une directive.
type contexte struct {
	etat  *EtatSession
	appel *Appel
	// racine sont les arguments évalués (après les caviardages des
	// directives précédentes) : ce que $racine/… désigne sous un
	// quantificateur.
	racine any
}

// PrefixeRacine, devant un pointeur, le fait lire depuis les arguments et
// non depuis l'élément courant d'un quantificateur.
const PrefixeRacine = "$racine"

// lire résout un pointeur de feuille ou de quantificateur : depuis la
// racine des arguments s'il commence par $racine/, depuis courant sinon.
func (c *contexte) lire(courant any, ptr string) (any, bool) {
	if strings.HasPrefix(ptr, PrefixeRacine+"/") {
		return pointerDans(c.racine, strings.TrimPrefix(ptr, PrefixeRacine))
	}
	return pointerDans(courant, ptr)
}

// noeud évalue nd sur courant : les arguments à la racine, l'élément courant
// sous un quantificateur.
func (c *contexte) noeud(nd *Noeud, courant any) (Trivalent, string) {
	switch {
	case nd == nil:
		return Indetermine, "nœud vide"
	case nd.Et != nil:
		return c.liste(nd.Et, courant, true)
	case nd.Ou != nil:
		return c.liste(nd.Ou, courant, false)
	case nd.Non != nil:
		r, t := c.noeud(nd.Non, courant)
		return Non(r), "non(" + t + ")"
	case nd.Existe != nil:
		return c.quantifier(nd.Existe, courant, true)
	case nd.Tous != nil:
		return c.quantifier(nd.Tous, courant, false)
	case nd.Argument != nil:
		v, trouve := c.lire(courant, nd.Argument.Champ)
		return feuilleArgument(*nd.Argument, v, trouve)
	case nd.AConsulte != nil:
		return c.aConsulte(nd.AConsulte)
	case nd.Appelant != nil:
		return feuilleAppelant(nd.Appelant, c.etat.Appelant)
	case nd.SousAgentDeclare != nil:
		return feuilleSousAgent(nd.SousAgentDeclare, c.appel.SousAgentDeclare)
	case nd.Demandeur != nil:
		return feuilleDemandeur(nd.Demandeur, c.appel.Demandeur)
	case nd.Attestation != nil:
		return feuilleAttestation(nd.Attestation, c.appel.Attestation)
	case nd.Temps != nil:
		return feuilleTemps(nd.Temps, c.appel.Horodatage)
	case nd.EtiquetteCible != nil:
		return c.etiquetteCible(nd.EtiquetteCible)
	case nd.MetadonneeReponse != nil:
		return c.metadonnee(nd.MetadonneeReponse)
	case nd.Sujet != nil:
		return c.sujet(nd.Sujet, courant)
	}
	return Indetermine, "nœud sans feuille connue"
}

// liste évalue tous les enfants (sans court-circuit, pour tous les citer)
// et replie par ET ou OU.
func (c *contexte) liste(enfants []*Noeud, courant any, et bool) (Trivalent, string) {
	acc := Vrai
	if !et {
		acc = Faux
	}
	res := make([]Trivalent, len(enfants))
	txt := make([]string, len(enfants))
	for i, e := range enfants {
		res[i], txt[i] = c.noeud(e, courant)
		if et {
			acc = Et(acc, res[i])
		} else {
			acc = Ou(acc, res[i])
		}
	}
	// On cite les enfants qui font le résultat.
	var cites []string
	for i := range enfants {
		if res[i] == acc {
			cites = append(cites, txt[i])
		}
	}
	switch {
	case et && acc == Vrai:
		return acc, strings.Join(cites, " et ")
	case !et && acc == Vrai:
		return acc, strings.Join(cites, " ou ")
	case !et && acc == Faux:
		return acc, "aucune de : " + strings.Join(cites, " ; ")
	}
	return acc, strings.Join(cites, ", ")
}

func (c *contexte) quantifier(q *Quantificateur, courant any, existe bool) (Trivalent, string) {
	nom := "tous"
	if existe {
		nom = "existe"
	}
	v, trouve := c.lire(courant, q.Champ)
	tab, ok := v.([]any)
	switch {
	case !trouve || !ok:
		return Indetermine, fmt.Sprintf("%s %s (champ absent ou qui n'est pas un tableau)", nom, q.Champ)
	case len(tab) > ElementsMax:
		return Indetermine, fmt.Sprintf("%s %s (plus de %d éléments)", nom, q.Champ, ElementsMax)
	}
	acc := Vrai
	if existe {
		acc = Faux
	}
	premier, texte := -1, ""
	for i, e := range tab {
		r, t := c.noeud(q.Condition, e)
		if existe {
			acc = Ou(acc, r)
		} else {
			acc = Et(acc, r)
		}
		// L'élément qui fixe le résultat : le premier vrai pour existe, le
		// premier faux pour tous ; à défaut, le premier indéterminé.
		if (existe && r == Vrai) || (!existe && r == Faux) {
			return acc, fmt.Sprintf("%s %s : élément %d : %s", nom, q.Champ, i, t)
		}
		if r == Indetermine && premier < 0 {
			premier, texte = i, t
		}
	}
	if acc == Indetermine {
		return acc, fmt.Sprintf("%s %s : élément %d : %s", nom, q.Champ, premier, texte)
	}
	if existe {
		return acc, fmt.Sprintf("aucun élément de %s (%d) ne satisfait la condition", q.Champ, len(tab))
	}
	return acc, fmt.Sprintf("tous les éléments de %s (%d) satisfont la condition", q.Champ, len(tab))
}

// feuilleArgument : le texte cite le champ, l'opérateur et la valeur de la
// directive, jamais celle de l'argument.
func feuilleArgument(cd Condition, v any, trouve bool) (Trivalent, string) {
	base := cd.Champ + " " + string(cd.Op) + " " + valeurTexte(cd.Valeur)
	switch r := evaluerOp(cd.Op, cd.Valeur, v, trouve); r {
	case Vrai:
		return r, base
	case Faux:
		return r, base + " faux"
	default:
		return r, base + " (champ absent ou type inattendu)"
	}
}

// evaluerOp applique un opérateur de feuille argument.
func evaluerOp(op Op, seuil, v any, trouve bool) Trivalent {
	switch op {
	case OpPresent:
		return vf(trouve)
	case OpAbsent:
		return vf(!trouve)
	}
	if !trouve {
		return Indetermine
	}
	switch op {
	case OpSup, OpSupEgal, OpInf, OpInfEgal:
		a, okA := decimal(v)
		b, okB := decimal(seuil)
		if !okA || !okB {
			return Indetermine
		}
		cmp := a.Cmp(b)
		switch op {
		case OpSup:
			return vf(cmp > 0)
		case OpSupEgal:
			return vf(cmp >= 0)
		case OpInf:
			return vf(cmp < 0)
		}
		return vf(cmp <= 0)
	case OpEgal, OpDiff:
		eq, ok := egauxExacts(v, seuil)
		if !ok {
			return Indetermine
		}
		return vf(eq == (op == OpEgal))
	case OpDansV2:
		liste, _ := seuil.([]any)
		for _, e := range liste {
			if eq, ok := egauxExacts(v, e); ok && eq {
				return Vrai
			}
		}
		return Faux
	case OpPrefixe:
		s, ok1 := v.(string)
		p, ok2 := seuil.(string)
		if !ok1 || !ok2 {
			return Indetermine
		}
		return vf(strings.HasPrefix(s, p))
	}
	return Indetermine
}

// egauxExacts compare deux scalaires JSON : nombres en décimal exact,
// chaînes, booléens. Types différents : indéterminé.
func egauxExacts(a, b any) (bool, bool) {
	if x, ok := decimal(a); ok {
		y, ok := decimal(b)
		if !ok {
			return false, false
		}
		return x.Cmp(y) == 0, true
	}
	switch x := a.(type) {
	case string:
		y, ok := b.(string)
		return x == y, ok
	case bool:
		y, ok := b.(bool)
		return x == y, ok
	}
	return false, false
}

func vf(b bool) Trivalent {
	if b {
		return Vrai
	}
	return Faux
}

// suffixe ajoute au texte d'une feuille ce qui n'est pas vrai.
func suffixe(r Trivalent, base, raison string) (Trivalent, string) {
	switch r {
	case Vrai:
		return r, base
	case Faux:
		return r, base + " faux"
	}
	return r, base + " (indéterminé : " + raison + ")"
}

func (c *contexte) aConsulte(f *FeuilleConsultation) (Trivalent, string) {
	quoi := f.Serveur
	switch {
	case f.Outil != "":
		quoi = f.Outil
	case f.Etiquette != "":
		quoi = "un serveur étiqueté " + f.Etiquette
	}
	inconnu := false
	for _, cs := range c.etat.Consultations {
		if f.Critere != Transmis && cs.Resultat != ResultatOK {
			continue
		}
		switch {
		case f.Serveur != "" && cs.Serveur == f.Serveur,
			f.Outil != "" && cs.Serveur+"/"+cs.Outil == f.Outil:
			return Vrai, "la session a consulté " + quoi + " (" + string(cs.Resultat) + ")"
		case f.Etiquette != "":
			if cs.VersionRegistre == "" {
				inconnu = true
			} else if contient(cs.Etiquettes, f.Etiquette) {
				return Vrai, "la session a consulté " + quoi + " (" + string(cs.Resultat) + ")"
			}
		}
	}
	if inconnu {
		return Indetermine, "consultation de " + quoi + " (indéterminé : étiquettes d'une consultation inconnues)"
	}
	return Faux, "la session n'a pas consulté " + quoi + " (" + string(f.Critere) + ")"
}

func feuilleAppelant(f *FeuilleChaine, appelant string) (Trivalent, string) {
	base := "appelant = " + f.Egal
	if f.Prefixe != "" {
		base = "appelant commence par " + f.Prefixe
	}
	if appelant == "" {
		return suffixe(Indetermine, base, "appelant non prouvé")
	}
	if f.Prefixe != "" {
		return suffixe(vf(strings.HasPrefix(appelant, f.Prefixe)), base, "")
	}
	return suffixe(vf(appelant == f.Egal), base, "")
}

func feuilleSousAgent(f *FeuilleChaine, declare string) (Trivalent, string) {
	if f.Dans != nil {
		return suffixe(vf(contient(f.Dans, declare)), "sous-agent déclaré dans ["+strings.Join(f.Dans, ", ")+"]", "")
	}
	return suffixe(vf(declare == f.Egal), "sous-agent déclaré = "+f.Egal, "")
}

func feuilleDemandeur(f *FeuilleDemandeur, d *Demandeur) (Trivalent, string) {
	var base string
	switch {
	case f.Groupe != "":
		base = "demandeur du groupe " + f.Groupe
	case f.Identite != nil && f.Identite.Dans != nil:
		base = "demandeur dans [" + strings.Join(f.Identite.Dans, ", ") + "]"
	case f.Identite != nil:
		base = "demandeur = " + f.Identite.Egal
	default:
		// Feuille vide, que le chargeur refuse : jamais vraie ni fausse.
		return Indetermine, "demandeur (indéterminé : feuille sans critère)"
	}
	if d == nil {
		return suffixe(Indetermine, base, "demandeur non résolu")
	}
	switch {
	case f.Groupe != "":
		return suffixe(vf(contient(d.Groupes, f.Groupe)), base, "")
	case d.Identite == "":
		return suffixe(Indetermine, base, "identité du demandeur vide")
	case f.Identite.Dans != nil:
		return suffixe(vf(contient(f.Identite.Dans, d.Identite)), base, "")
	}
	return suffixe(vf(d.Identite == f.Identite.Egal), base, "")
}

// feuilleAttestation recalcule la vérification (Attestation.Valide), sans
// croire Verifiee.
func feuilleAttestation(f *FeuilleAttestation, a Attestation) (Trivalent, string) {
	valide := a.Valide()
	texte := "serveur attesté (" + decrireAttestation(a) + ")"
	if !valide {
		texte = "attestation du serveur en échec (" + decrireAttestation(a) + ")"
	}
	veut := f.Verifiee != nil && *f.Verifiee
	r := vf(valide == veut)
	if f.Mode != "" {
		mode := a.Mode
		if mode == "" {
			mode = "aucune"
		}
		r = Et(r, vf(mode == f.Mode))
		texte += " ; mode attendu " + f.Mode
	}
	return r, texte
}

func feuilleTemps(f *FeuilleTemps, h time.Time) (Trivalent, string) {
	var crit []string
	if f.JourSemaine != nil {
		crit = append(crit, "jour "+strings.Join(f.JourSemaine, "|"))
	}
	if f.Heure != nil {
		crit = append(crit, "heure ["+f.Heure.De+", "+f.Heure.A+"[")
	}
	if f.Date != nil {
		crit = append(crit, "date "+string(f.Date.Op)+" "+f.Date.Valeur)
	}
	base := "temps " + strings.Join(crit, " et ") + " (" + f.Fuseau + ")"
	if h.IsZero() {
		return suffixe(Indetermine, base, "horodatage absent")
	}
	loc, ok := Fuseau(f.Fuseau)
	if !ok {
		return suffixe(Indetermine, base, "fuseau hors de la base embarquée")
	}
	t := h.In(loc)
	r := Vrai
	if f.JourSemaine != nil {
		j := false
		for _, nom := range f.JourSemaine {
			j = j || jourIndex(nom) == int(t.Weekday())
		}
		r = Et(r, vf(j))
	}
	if f.Heure != nil {
		de, ok1 := minutes(f.Heure.De)
		a, ok2 := minutes(f.Heure.A)
		if !ok1 || !ok2 {
			return suffixe(Indetermine, base, "heure illisible")
		}
		m := t.Hour()*60 + t.Minute()
		dans := de <= m && m < a
		if de > a { // passe minuit
			dans = m >= de || m < a
		}
		r = Et(r, vf(dans))
	}
	if f.Date != nil {
		// AAAA-MM-JJ : l'ordre lexicographique est l'ordre des dates.
		cmp := strings.Compare(t.Format(time.DateOnly), f.Date.Valeur)
		r = Et(r, comparer(f.Date.Op, cmp))
	}
	return suffixe(r, base, "")
}

// comparer applique un opérateur d'ordre ou d'égalité à un résultat de
// comparaison.
func comparer(op Op, cmp int) Trivalent {
	switch op {
	case OpSup:
		return vf(cmp > 0)
	case OpSupEgal:
		return vf(cmp >= 0)
	case OpInf:
		return vf(cmp < 0)
	case OpInfEgal:
		return vf(cmp <= 0)
	case OpEgal:
		return vf(cmp == 0)
	case OpDiff:
		return vf(cmp != 0)
	}
	return Indetermine
}

func (c *contexte) etiquetteCible(f *FeuilleEtiquette) (Trivalent, string) {
	base := "cible étiquetée " + f.Contient
	if c.appel.VersionRegistre == "" {
		return suffixe(Indetermine, base, "pas de version de registre")
	}
	return suffixe(vf(contient(c.appel.EtiquettesCible, f.Contient)), base, "")
}

func (c *contexte) metadonnee(f *FeuilleMetadonnee) (Trivalent, string) {
	quoi := f.Serveur
	if f.Etiquette != "" {
		quoi = "serveurs étiquetés " + f.Etiquette
	}
	base := "réponse de " + quoi + " : " + f.Champ + " " + string(f.Op) + " " + valeurTexte(f.Valeur)
	inconnu := false
	for _, cs := range c.etat.Consultations {
		if cs.Resultat != ResultatOK {
			continue
		}
		if f.Serveur != "" && cs.Serveur != f.Serveur {
			continue
		}
		if f.Etiquette != "" {
			if cs.VersionRegistre == "" {
				inconnu = true
				continue
			}
			if !contient(cs.Etiquettes, f.Etiquette) {
				continue
			}
		}
		r := evaluerMetadonnee(f, cs.Source)
		if r == Vrai {
			return Vrai, base
		}
		if r == Indetermine {
			inconnu = true
		}
	}
	if inconnu {
		return suffixe(Indetermine, base, "métadonnée absente ou illisible")
	}
	return suffixe(Faux, base, "")
}

func evaluerMetadonnee(f *FeuilleMetadonnee, s *SourceReponse) Trivalent {
	if s == nil {
		return Indetermine
	}
	switch f.Champ {
	case MetaSourceOrigine:
		if s.Origine == "" {
			return Indetermine
		}
		switch f.Op {
		case OpEgal, OpDiff:
			v, _ := f.Valeur.(string)
			return vf((s.Origine == v) == (f.Op == OpEgal))
		case OpPrefixe:
			v, _ := f.Valeur.(string)
			return vf(strings.HasPrefix(s.Origine, v))
		case OpDansV2:
			liste, _ := f.Valeur.([]any)
			for _, e := range liste {
				if e == any(s.Origine) {
					return Vrai
				}
			}
			return Faux
		}
	case MetaSourceDate:
		d, err := time.Parse(time.RFC3339, s.Date)
		v, _ := f.Valeur.(string)
		seuil, err2 := time.Parse(time.RFC3339, v)
		if err != nil || err2 != nil {
			return Indetermine
		}
		return comparer(f.Op, d.Compare(seuil))
	}
	return Indetermine
}

// sujet : l'identifiant est haché, jamais cité.
func (c *contexte) sujet(f *FeuilleSujet, courant any) (Trivalent, string) {
	base := "sujet du groupe " + f.Groupe
	ptr := ""
	if f.Champ != nil {
		ptr = *f.Champ
	}
	v, trouve := c.lire(courant, ptr)
	if !trouve {
		return suffixe(Indetermine, base, "identifiant du sujet absent")
	}
	id, ok := identifiant(v)
	if !ok {
		return suffixe(Indetermine, base, "identifiant du sujet illisible")
	}
	emp := EmpreinteSujet(id)
	for _, s := range c.appel.Sujets {
		if s.Empreinte == emp {
			return suffixe(vf(contient(s.Groupes, f.Groupe)), base, "")
		}
	}
	return suffixe(Indetermine, base, "sujet non résolu")
}

// identifiant lit un identifiant de personne : une chaîne non vide ou un
// entier JSON (sous sa forme décimale canonique).
func identifiant(v any) (string, bool) {
	if s, ok := v.(string); ok {
		return s, s != ""
	}
	r, ok := decimal(v)
	if !ok || !r.IsInt() {
		return "", false
	}
	return r.Num().String(), true
}

func contient(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// pointerDans résout un pointeur JSON (RFC 6901) dans une valeur ; "" est
// la valeur elle-même. Les tableaux sont indexés par entier.
func pointerDans(racine any, ptr string) (any, bool) {
	if ptr == "" {
		return racine, true
	}
	if ptr[0] != '/' {
		return nil, false
	}
	cur := racine
	for _, seg := range strings.Split(ptr[1:], "/") {
		seg = strings.ReplaceAll(strings.ReplaceAll(seg, "~1", "/"), "~0", "~")
		switch n := cur.(type) {
		case map[string]any:
			v, ok := n[seg]
			if !ok {
				return nil, false
			}
			cur = v
		case []any:
			i, err := strconv.Atoi(seg)
			if err != nil || i < 0 || i >= len(n) {
				return nil, false
			}
			cur = n[i]
		default:
			return nil, false
		}
	}
	return cur, true
}
