package directive

import (
	"fmt"
	"strings"
	"time"

	"mcprogue/internal/nommage"
)

// Bornes de l'arbre de condition (docs/schema-directive-v2.md §5.2).
const (
	ProfondeurMax = 6
	NoeudsMax     = 64
	// ElementsMax borne un tableau parcouru par un quantificateur ; au-delà
	// le quantificateur est indéterminé.
	ElementsMax = 10000
)

var joursSemaine = []string{"dimanche", "lundi", "mardi", "mercredi", "jeudi", "vendredi", "samedi"}

// validerDirectiveV2 applique le schéma v2 à une directive. L'enveloppe
// commune (id, version, statut, dates, texte) est contrôlée par
// validerEnveloppe.
func validerDirectiveV2(v *validation, p string, d *Directive) {
	validerEnveloppe(v, p, d)

	if len(d.Fondements) == 0 {
		v.f("%s: fondements manquants (liste non vide)", p)
	}
	for i, f := range d.Fondements {
		if strings.TrimSpace(f.Texte) == "" {
			v.f("%s: fondements[%d].texte manquant", p, i)
		}
	}

	switch d.SiIndetermine {
	case "", IndetermineRefuser:
		if d.SiIndetermineValidePar != "" {
			v.f("%s: si_indetermine_valide_par n'a de sens qu'avec si_indetermine: ignorer", p)
		}
	case IndetermineIgnorer:
		if strings.TrimSpace(d.SiIndetermineValidePar) == "" {
			v.f("%s: si_indetermine: ignorer exige si_indetermine_valide_par (validation nommée du juriste)", p)
		}
	default:
		v.f("%s: si_indetermine %q inconnu", p, d.SiIndetermine)
	}

	nomme := validerDeclencheur(v, p, d)

	if d.Arbre != nil {
		n := 0
		validerNoeud(v, p+": condition", d.Arbre, 1, &n, false)
		if n > NoeudsMax {
			v.f("%s: condition de %d nœuds (au plus %d)", p, n, NoeudsMax)
		}
	}

	validerEffets(v, p, d.Effets, nomme)
}

// validerEnveloppe contrôle ce que v1 et v2 partagent.
func validerEnveloppe(v *validation, p string, d *Directive) {
	if d.ID == "" {
		v.f("%s: id manquant", p)
	}
	if d.Version < 1 {
		v.f("%s: version doit être >= 1", p)
	}
	switch d.Statut {
	case Brouillon, Observation, Active, Retiree:
	default:
		v.f("%s: statut %q inconnu", p, d.Statut)
	}
	if d.DateEffet.IsZero() {
		v.f("%s: date_effet manquante", p)
	}
	if d.RevisionAvant != nil && !d.RevisionAvant.After(d.DateEffet) {
		v.f("%s: revision_avant doit être postérieure à date_effet", p)
	}
	if strings.TrimSpace(d.TexteFR) == "" {
		v.f("%s: texte_fr manquant", p)
	}
	if d.Perimetre != nil {
		if len(d.Perimetre.Appelants) == 0 {
			v.f("%s: perimetre.appelants vide", p)
		}
		for _, a := range d.Perimetre.Appelants {
			if a != "*" && !strings.HasPrefix(a, "spiffe://") {
				v.f("%s: perimetre.appelants %q n'est ni \"*\" ni un SPIFFE ID", p, a)
			}
		}
	}
}

// validerDeclencheur rend vrai si la cible est nommée (serveur explicite ou
// étiquette), ce que caviarder exige.
func validerDeclencheur(v *validation, p string, d *Directive) bool {
	dc := d.Declencheur
	if dc == nil {
		v.f("%s: declencheur manquant", p)
		return false
	}
	p += ": declencheur"
	c := dc.Cible
	switch dc.Evenement {
	case EvAppelOutil:
		if c == nil {
			v.f("%s: cible obligatoire pour appel_outil", p)
			return false
		}
		if *c == (CibleV2{}) {
			v.f("%s: cible vide (écrire \"serveur\": \"*\" pour tout serveur)", p)
		}
	case EvOuvertureSession, EvPurge:
		if c != nil {
			v.f("%s: l'évènement %s n'admet pas de cible", p, dc.Evenement)
		}
	case EvAttestationEchouee:
		if c != nil && (c.Outil != "" || c.Etiquette != "" || c.Acces != "") {
			v.f("%s: l'évènement attestation_echouee n'admet que cible.serveur", p)
		}
	case "":
		v.f("%s: evenement manquant", p)
		return false
	default:
		v.f("%s: evenement %q inconnu", p, dc.Evenement)
		return false
	}
	if dc.Evenement != EvAppelOutil && d.Statut != Brouillon {
		v.f("%s: l'évènement %s est défini par le schéma mais pas encore évalué par l'engine : seul le statut brouillon est admis", p, dc.Evenement)
	}
	if c == nil {
		return false
	}
	if c.Serveur != "" && c.Serveur != "*" {
		if err := nommage.ValiderNomLogique(c.Serveur); err != nil {
			v.f("%s: cible.serveur: %v", p, err)
		}
	}
	if c.Outil != "" && c.Outil != "*" {
		if err := nommage.ValiderNomOutil(c.Outil); err != nil {
			v.f("%s: cible.outil: %v", p, err)
		}
	}
	if c.Etiquette == "*" {
		v.f("%s: cible.etiquette n'admet pas de joker", p)
	}
	switch c.Acces {
	case "", AccesLecture, AccesAction:
	default:
		v.f("%s: cible.acces %q inconnu (read | action)", p, c.Acces)
	}
	return (c.Serveur != "" && c.Serveur != "*") || c.Etiquette != ""
}

// validerNoeud contrôle un nœud et ses descendants ; n compte les nœuds.
// dansQuantif dit si le nœud est sous un quantificateur, ce qui rend les
// pointeurs relatifs à l'élément courant.
func validerNoeud(v *validation, p string, nd *Noeud, prof int, n *int, dansQuantif bool) {
	*n++
	if nd == nil {
		v.f("%s: nœud vide", p)
		return
	}
	if prof > ProfondeurMax {
		v.f("%s: profondeur %d (au plus %d)", p, prof, ProfondeurMax)
		return
	}
	cles := clesNoeud(nd)
	if len(cles) != 1 {
		if len(cles) == 0 {
			v.f("%s: nœud sans clé (une parmi et, ou, non, existe, tous ou une feuille)", p)
		} else {
			v.f("%s: nœud à plusieurs clés %v (une seule admise)", p, cles)
		}
		return
	}
	cle := cles[0]
	p += "." + cle
	switch {
	case nd.Et != nil || nd.Ou != nil:
		enfants := nd.Et
		if nd.Ou != nil {
			enfants = nd.Ou
		}
		if len(enfants) == 0 {
			v.f("%s: liste vide", p)
		}
		for i, e := range enfants {
			validerNoeud(v, fmt.Sprintf("%s[%d]", p, i), e, prof+1, n, dansQuantif)
		}
	case nd.Non != nil:
		validerNoeud(v, p, nd.Non, prof+1, n, dansQuantif)
	case nd.Existe != nil || nd.Tous != nil:
		q := nd.Existe
		if q == nil {
			q = nd.Tous
		}
		validerPointeurV2(v, p+".champ", q.Champ, dansQuantif)
		if q.Condition == nil {
			v.f("%s: condition manquante", p)
			return
		}
		validerNoeud(v, p+".condition", q.Condition, prof+1, n, true)
	case nd.Argument != nil:
		validerArgument(v, p, *nd.Argument, dansQuantif)
	case nd.AConsulte != nil:
		f := nd.AConsulte
		if compter(f.Serveur != "", f.Outil != "", f.Etiquette != "") != 1 {
			v.f("%s: exactement une clé parmi serveur, outil, etiquette", p)
		}
		if f.Serveur != "" {
			if err := nommage.ValiderNomLogique(f.Serveur); err != nil {
				v.f("%s: serveur: %v", p, err)
			}
		}
		if f.Outil != "" {
			s, t, ok := strings.Cut(f.Outil, "/")
			if !ok || nommage.ValiderNomLogique(s) != nil || nommage.ValiderNomOutil(t) != nil {
				v.f("%s: outil %q doit s'écrire serveur/outil", p, f.Outil)
			}
		}
		switch f.Critere {
		case Transmis, DonneesRecues:
		case "":
			v.f("%s: critere manquant (transmis | donnees_recues)", p)
		default:
			v.f("%s: critere %q inconnu", p, f.Critere)
		}
	case nd.Appelant != nil:
		f := nd.Appelant
		if compter(f.Egal != "", f.Prefixe != "", f.Dans != nil) != 1 || f.Dans != nil {
			v.f("%s: exactement une clé parmi egal, prefixe", p)
		}
	case nd.SousAgentDeclare != nil:
		f := nd.SousAgentDeclare
		if compter(f.Egal != "", f.Prefixe != "", f.Dans != nil) != 1 || f.Prefixe != "" {
			v.f("%s: exactement une clé parmi egal, dans", p)
		}
		validerListe(v, p+".dans", f.Dans)
	case nd.Demandeur != nil:
		f := nd.Demandeur
		if compter(f.Identite != nil, f.Groupe != "") != 1 {
			v.f("%s: exactement une forme : identite ou groupe", p)
		}
		if id := f.Identite; id != nil {
			if compter(id.Egal != "", id.Prefixe != "", id.Dans != nil) != 1 || id.Prefixe != "" {
				v.f("%s.identite: exactement une clé parmi egal, dans", p)
			}
			validerListe(v, p+".identite.dans", id.Dans)
		}
	case nd.Attestation != nil:
		f := nd.Attestation
		if f.Verifiee == nil {
			v.f("%s: verifiee obligatoire", p)
		}
		switch f.Mode {
		case "", "spiffe", "tls-pin", "binaire", "aucune":
		default:
			v.f("%s: mode %q inconnu (spiffe | tls-pin | binaire | aucune)", p, f.Mode)
		}
	case nd.Temps != nil:
		validerTemps(v, p, nd.Temps)
	case nd.EtiquetteCible != nil:
		if strings.TrimSpace(nd.EtiquetteCible.Contient) == "" {
			v.f("%s: contient manquant", p)
		}
	case nd.MetadonneeReponse != nil:
		validerMetadonnee(v, p, nd.MetadonneeReponse)
	case nd.Sujet != nil:
		f := nd.Sujet
		if strings.TrimSpace(f.Groupe) == "" {
			v.f("%s: groupe manquant", p)
		}
		switch {
		case f.Champ == nil || *f.Champ == "":
			if !dansQuantif {
				v.f("%s: champ obligatoire hors quantificateur (pointeur vers l'identifiant du sujet)", p)
			}
		default:
			validerPointeurV2(v, p+".champ", *f.Champ, dansQuantif)
		}
	}
}

// clesNoeud rend, dans l'ordre du schéma, les clés posées d'un nœud.
func clesNoeud(nd *Noeud) []string {
	var out []string
	ajouter := func(pose bool, cle string) {
		if pose {
			out = append(out, cle)
		}
	}
	ajouter(nd.Et != nil, "et")
	ajouter(nd.Ou != nil, "ou")
	ajouter(nd.Non != nil, "non")
	ajouter(nd.Existe != nil, "existe")
	ajouter(nd.Tous != nil, "tous")
	ajouter(nd.Argument != nil, "argument")
	ajouter(nd.AConsulte != nil, "session.a_consulte")
	ajouter(nd.Appelant != nil, "appelant")
	ajouter(nd.SousAgentDeclare != nil, "sous_agent_declare")
	ajouter(nd.Demandeur != nil, "demandeur")
	ajouter(nd.Attestation != nil, "attestation")
	ajouter(nd.Temps != nil, "temps")
	ajouter(nd.EtiquetteCible != nil, "etiquette_cible")
	ajouter(nd.MetadonneeReponse != nil, "metadonnee_reponse")
	ajouter(nd.Sujet != nil, "sujet")
	return out
}

func compter(b ...bool) int {
	n := 0
	for _, x := range b {
		if x {
			n++
		}
	}
	return n
}

func validerListe(v *validation, p string, l []string) {
	if l == nil {
		return
	}
	if len(l) == 0 {
		v.f("%s: liste vide", p)
	}
	for _, s := range l {
		if strings.TrimSpace(s) == "" {
			v.f("%s: élément vide", p)
		}
	}
}

// validerPointeurV2 : hors quantificateur un pointeur commence par "/" ; sous
// un quantificateur, "" désigne l'élément courant.
func validerPointeurV2(v *validation, p, ptr string, dansQuantif bool) {
	if ptr == "" && dansQuantif {
		return
	}
	if err := validerPointeur(ptr); err != nil {
		v.f("%s: %v", p, err)
	}
}

func validerArgument(v *validation, p string, c Condition, dansQuantif bool) {
	validerPointeurV2(v, p+".champ", c.Champ, dansQuantif)
	switch c.Op {
	case OpPresent, OpAbsent:
		if c.Valeur != nil {
			v.f("%s: l'opérateur %s ne prend pas de valeur", p, c.Op)
		}
	case OpSup, OpSupEgal, OpInf, OpInfEgal:
		if _, ok := decimal(c.Valeur); !ok {
			v.f("%s: l'opérateur %s exige une valeur numérique (exposant et mantisse bornés)", p, c.Op)
		}
	case OpEgal, OpDiff:
		if !scalaireValide(c.Valeur) {
			v.f("%s: l'opérateur %s exige un nombre, une chaîne ou un booléen", p, c.Op)
		}
	case OpDansV2:
		liste, ok := c.Valeur.([]any)
		if !ok || len(liste) == 0 {
			v.f("%s: l'opérateur dans exige une liste non vide", p)
		}
		for _, e := range liste {
			if !scalaireValide(e) {
				v.f("%s: l'opérateur dans exige une liste de nombres, chaînes ou booléens", p)
				break
			}
		}
	case OpPrefixe:
		if s, ok := c.Valeur.(string); !ok || s == "" {
			v.f("%s: l'opérateur prefixe exige une chaîne non vide", p)
		}
	case OpDans:
		v.f("%s: opérateur \"in\" : écrire \"dans\" en v2", p)
	default:
		v.f("%s: opérateur %q inconnu", p, c.Op)
	}
}

func scalaireValide(x any) bool {
	switch x.(type) {
	case string, bool:
		return true
	}
	_, ok := decimal(x)
	return ok
}

func validerTemps(v *validation, p string, t *FeuilleTemps) {
	if t.Fuseau == "" {
		v.f("%s: fuseau obligatoire (nom IANA, par exemple Europe/Paris)", p)
	} else if _, ok := Fuseau(t.Fuseau); !ok {
		v.f("%s: fuseau %q hors de la base embarquée (admis : %s)", p, t.Fuseau, strings.Join(Fuseaux(), ", "))
	}
	if t.JourSemaine == nil && t.Heure == nil && t.Date == nil {
		v.f("%s: au moins un critère parmi jour_semaine, heure, date", p)
	}
	if t.JourSemaine != nil && len(t.JourSemaine) == 0 {
		v.f("%s: jour_semaine vide", p)
	}
	for _, j := range t.JourSemaine {
		if jourIndex(j) < 0 {
			v.f("%s: jour_semaine %q inconnu (lundi … dimanche)", p, j)
		}
	}
	if h := t.Heure; h != nil {
		de, okDe := minutes(h.De)
		a, okA := minutes(h.A)
		if !okDe {
			v.f("%s: heure.de %q invalide (HH:MM)", p, h.De)
		}
		if !okA {
			v.f("%s: heure.a %q invalide (HH:MM)", p, h.A)
		}
		if okDe && okA && de == a {
			v.f("%s: heure.de et heure.a égales : intervalle vide", p)
		}
	}
	if dt := t.Date; dt != nil {
		switch dt.Op {
		case OpSup, OpSupEgal, OpInf, OpInfEgal, OpEgal, OpDiff:
		default:
			v.f("%s: date.op %q inconnu", p, dt.Op)
		}
		if _, err := time.Parse(time.DateOnly, dt.Valeur); err != nil {
			v.f("%s: date.valeur %q invalide (AAAA-MM-JJ)", p, dt.Valeur)
		}
	}
}

func jourIndex(j string) int {
	for i, x := range joursSemaine {
		if x == j {
			return i
		}
	}
	return -1
}

// minutes lit "HH:MM" strictement.
func minutes(s string) (int, bool) {
	if len(s) != 5 || s[2] != ':' {
		return 0, false
	}
	chiffres := func(a, b byte) (int, bool) {
		if a < '0' || a > '9' || b < '0' || b > '9' {
			return 0, false
		}
		return int(a-'0')*10 + int(b-'0'), true
	}
	h, ok1 := chiffres(s[0], s[1])
	m, ok2 := chiffres(s[3], s[4])
	if !ok1 || !ok2 || h > 23 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

func validerMetadonnee(v *validation, p string, f *FeuilleMetadonnee) {
	if compter(f.Serveur != "", f.Etiquette != "") != 1 {
		v.f("%s: exactement une clé parmi serveur, etiquette", p)
	}
	if f.Serveur != "" {
		if err := nommage.ValiderNomLogique(f.Serveur); err != nil {
			v.f("%s: serveur: %v", p, err)
		}
	}
	switch f.Champ {
	case MetaSourceOrigine:
		switch f.Op {
		case OpEgal, OpDiff, OpPrefixe:
			if s, ok := f.Valeur.(string); !ok || s == "" {
				v.f("%s: l'opérateur %s sur source.origine exige une chaîne non vide", p, f.Op)
			}
		case OpDansV2:
			liste, ok := f.Valeur.([]any)
			if !ok || len(liste) == 0 {
				v.f("%s: l'opérateur dans exige une liste non vide de chaînes", p)
			}
			for _, e := range liste {
				if s, ok := e.(string); !ok || s == "" {
					v.f("%s: l'opérateur dans exige une liste non vide de chaînes", p)
					break
				}
			}
		default:
			v.f("%s: opérateur %q non admis sur source.origine (== != dans prefixe)", p, f.Op)
		}
	case MetaSourceDate:
		switch f.Op {
		case OpSup, OpSupEgal, OpInf, OpInfEgal, OpEgal, OpDiff:
		default:
			v.f("%s: opérateur %q non admis sur source.date (> >= < <= == !=)", p, f.Op)
		}
		s, _ := f.Valeur.(string)
		if _, err := time.Parse(time.RFC3339, s); err != nil {
			v.f("%s: source.date exige un instant RFC 3339", p)
		}
	default:
		v.f("%s: champ %q inconnu (source.origine | source.date)", p, f.Champ)
	}
}

func validerEffets(v *validation, p string, effets []EffetDirective, cibleNommee bool) {
	if len(effets) == 0 {
		v.f("%s: effets manquants (liste non vide)", p)
		return
	}
	vus := map[Effet]int{}
	identiques := map[string]bool{}
	for i, e := range effets {
		q := fmt.Sprintf("%s: effets[%d] (%s)", p, i, e.Effet)
		vus[e.Effet]++
		cle := fmt.Sprintf("%q %q %q %q %q", e.Effet, e.DestinataireRole, e.Rythme, e.SiEchec, e.Etiquette)
		if (e.Effet == Notifier || e.Effet == Marquer) && identiques[cle] {
			v.f("%s: effet en double", q)
		}
		identiques[cle] = true

		param := func(pose bool, nom string, admis ...Effet) {
			if !pose {
				return
			}
			for _, a := range admis {
				if a == e.Effet {
					return
				}
			}
			v.f("%s: le paramètre %s n'est pas admis pour cet effet", q, nom)
		}
		param(e.ParRole != "", "par_role", AttendreValidation)
		param(e.QuatreYeux != nil, "quatre_yeux", AttendreValidation)
		param(e.ChampsAutorises != nil, "champs_autorises", Caviarder)
		param(e.Mode != "", "mode", Caviarder)
		param(e.DestinataireRole != "", "destinataire_role", Notifier)
		param(e.Rythme != "", "rythme", Notifier)
		param(e.SiEchec != "", "si_echec", Notifier)
		param(e.Etiquette != "", "etiquette", Marquer)

		switch e.Effet {
		case Refuser, AttendreValidation:
		case Caviarder:
			if !cibleNommee {
				v.f("%s: caviarder exige une cible nommée (serveur différent de * ou etiquette)", q)
			}
			if len(e.ChampsAutorises) == 0 {
				v.f("%s: champs_autorises vide", q)
			}
			for _, ptr := range e.ChampsAutorises {
				if err := validerPointeur(ptr); err != nil {
					v.f("%s: champs_autorises: %v", q, err)
				}
			}
			switch e.Mode {
			case ModeCaviarder, ModeRefuser:
			case "":
				v.f("%s: mode manquant (caviarder | refuser)", q)
			default:
				v.f("%s: mode %q inconnu", q, e.Mode)
			}
		case Notifier:
			if strings.TrimSpace(e.DestinataireRole) == "" {
				v.f("%s: destinataire_role manquant", q)
			}
			switch e.Rythme {
			case TempsReel, Synthese:
			default:
				v.f("%s: rythme %q inconnu (temps_reel | synthese)", q, e.Rythme)
			}
			switch e.SiEchec {
			case EchecRefuser, EchecContinuer:
			default:
				v.f("%s: si_echec %q inconnu (refuser | continuer)", q, e.SiEchec)
			}
		case Marquer:
			if strings.TrimSpace(e.Etiquette) == "" {
				v.f("%s: etiquette manquante", q)
			}
		case Autoriser:
			v.f("%s: autoriser n'est pas un effet : c'est l'absence d'effet", q)
		default:
			v.f("%s: effet inconnu (refuser | attendre_validation | caviarder | notifier | marquer)", q)
		}
	}
	for _, e := range []Effet{Refuser, AttendreValidation, Caviarder} {
		if vus[e] > 1 {
			v.f("%s: effet %s en double", p, e)
		}
	}
	if vus[Refuser] > 0 && (vus[AttendreValidation] > 0 || vus[Caviarder] > 0) {
		v.f("%s: refuser ne se combine ni avec attendre_validation ni avec caviarder (ils seraient sans effet)", p)
	}
}
