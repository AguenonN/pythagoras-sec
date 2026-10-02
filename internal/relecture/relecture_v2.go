package relecture

import (
	"strconv"
	"strings"
	"time"

	"mcprogue/internal/directive"
)

// Relecture d'une directive v2 (docs/schema-directive-v2.md) : déclencheur,
// arbre de condition, liste d'effets, règle de l'indéterminé. Même
// principe que pour la v1 : un gabarit fixe par nœud, par feuille et par
// effet, aucune horloge, aucun fuseau local, aucun parcours de map. Les
// dates de l'enveloppe sont écrites en UTC ; celles d'une feuille temps le
// sont dans le fuseau de la feuille, qui est toujours nommé.

// Indentation d'un niveau de l'arbre.
const retrait = "  "

// directiveV2 rend une directive v2 : identité et fondements, validité,
// puis une ligne par partie de la règle.
func directiveV2(d directive.Directive) string {
	var b strings.Builder
	b.WriteString("Directive " + d.ID + ", version " + strconv.Itoa(d.Version) + ", " +
		statut(d.Statut) + ", fondée sur " + fondements(d.Fondements) + ".\n")
	ev := evenementDe(d)
	vise := "aux appels"
	if ev != directive.EvAppelOutil {
		vise = "aux évènements"
	}
	b.WriteString("Applicable " + vise + " à partir du " + date(d.DateEffet) + ", sans date de fin ; " +
		revision(d.RevisionAvant) + ".\n")
	b.WriteString(perimetre(d.Perimetre) + ".\n")
	b.WriteString("Quand : " + declencheur(d.Declencheur) + ".\n")
	if d.Arbre == nil {
		b.WriteString("Condition : aucune, la directive s'applique à chaque fois.\n")
	} else {
		si := bloc(d.Arbre, 0, false)
		if ev != directive.EvAppelOutil {
			// La feuille temps date l'évènement, et non un appel.
			si = strings.ReplaceAll(si, "l'appel a lieu ", "l'évènement a lieu ")
		}
		b.WriteString("Si " + si)
	}
	b.WriteString("Alors :\n")
	for _, e := range d.Effets {
		b.WriteString(retrait + "- " + effetV2(e, ev) + "\n")
	}
	if l := indetermineV2(d); l != "" {
		b.WriteString(l + "\n")
	}
	return b.String()
}

// fondements cite chaque fondement, le premier étant celui que citent les
// motifs.
func fondements(fs []directive.Fondement) string {
	if len(fs) == 0 {
		return "aucun fondement"
	}
	parts := make([]string, len(fs))
	for i, f := range fs {
		parts[i] = fondement(f)
	}
	out := parts[0]
	if len(parts) > 1 {
		out += " (fondement cité par les motifs), puis sur " + strings.Join(parts[1:], ", puis sur ")
	}
	return out
}

func declencheur(dc *directive.Declencheur) string {
	if dc == nil {
		return "déclencheur absent"
	}
	switch dc.Evenement {
	case directive.EvAppelOutil:
		return cibleV2(dc.Cible)
	case directive.EvOuvertureSession:
		return "à l'ouverture de chaque session"
	case directive.EvPurge:
		return "à chaque purge du coffre"
	case directive.EvAttestationEchouee:
		s := "à chaque attestation en échec (connexion ou changement)"
		if dc.Cible != nil && dc.Cible.Serveur != "" && dc.Cible.Serveur != "*" {
			s = "à chaque attestation en échec du serveur " + dc.Cible.Serveur + " (connexion ou changement)"
		}
		return s
	}
	return "à l'évènement inconnu " + strconv.Quote(string(dc.Evenement))
}

// cibleV2 combine les clés présentes de la cible en une phrase ; chaque clé
// resserre la cible (ET).
func cibleV2(c *directive.CibleV2) string {
	if c == nil {
		return "tout appel d'outil (cible absente)"
	}
	s := "tout appel "
	switch {
	case c.Outil != "" && c.Outil != "*":
		s += "à l'outil " + c.Outil
	default:
		s += "d'outil"
	}
	switch {
	case c.Serveur != "" && c.Serveur != "*":
		if c.Outil != "" && c.Outil != "*" {
			s += " sur le serveur " + c.Serveur
		} else {
			s += " du serveur " + c.Serveur
		}
	case c.Etiquette == "":
		s += ", sur tout serveur"
	}
	switch {
	case c.Etiquette != "" && c.Serveur != "" && c.Serveur != "*":
		s += ", si ce serveur porte l'étiquette « " + c.Etiquette + " » au registre"
	case c.Etiquette != "":
		s += ", vers un serveur portant l'étiquette « " + c.Etiquette + " » au registre"
	}
	switch c.Acces {
	case directive.AccesAction:
		s += ", pour un outil classé « action » au registre"
	case directive.AccesLecture:
		s += ", pour un outil classé « lecture » au registre"
	case "":
	default:
		s += ", pour l'accès inconnu " + strconv.Quote(string(c.Acces))
	}
	return s
}

// bloc rend un nœud qui suit « Si » (ou un tiret) : une phrase terminée par
// un saut de ligne, ou une annonce suivie de la liste de ses enfants,
// indentée d'un cran. dansQuantif dit si les pointeurs sont relatifs à
// l'élément courant.
func bloc(nd *directive.Noeud, niveau int, dansQuantif bool) string {
	switch {
	case nd == nil:
		return "(nœud vide)\n"
	case len(nd.Et) == 1:
		return bloc(nd.Et[0], niveau, dansQuantif)
	case len(nd.Ou) == 1:
		return bloc(nd.Ou[0], niveau, dansQuantif)
	case nd.Et != nil:
		return "toutes les conditions suivantes sont vraies :\n" + enfants(nd.Et, niveau+1, dansQuantif)
	case nd.Ou != nil:
		return "au moins une des conditions suivantes est vraie :\n" + enfants(nd.Ou, niveau+1, dansQuantif)
	case nd.Non != nil:
		if s, ok := negation(nd.Non, dansQuantif); ok {
			return s + "\n"
		}
		return "la condition suivante est fausse :\n" + enfants([]*directive.Noeud{nd.Non}, niveau+1, dansQuantif)
	case nd.Existe != nil:
		return "au moins un élément de " + liste(nd.Existe.Champ, dansQuantif) +
			" vérifie (faux si la liste est vide) :\n" + enfants([]*directive.Noeud{nd.Existe.Condition}, niveau+1, true)
	case nd.Tous != nil:
		return "chaque élément de " + liste(nd.Tous.Champ, dansQuantif) +
			" vérifie (vrai si la liste est vide) :\n" + enfants([]*directive.Noeud{nd.Tous.Condition}, niveau+1, true)
	}
	return feuille(nd, dansQuantif) + "\n"
}

func enfants(ns []*directive.Noeud, niveau int, dansQuantif bool) string {
	var b strings.Builder
	for _, n := range ns {
		b.WriteString(strings.Repeat(retrait, niveau) + "- " + bloc(n, niveau, dansQuantif))
	}
	return b.String()
}

// negation rend « non » en une phrase quand la feuille a une forme niée
// naturelle ; sinon ok est faux et « non » est écrit comme un nœud.
func negation(nd *directive.Noeud, dansQuantif bool) (string, bool) {
	switch {
	case nd.AConsulte != nil:
		return consultation(nd.AConsulte, false), true
	case nd.Demandeur != nil && nd.Demandeur.Groupe != "":
		return "le demandeur (la personne pour qui l'agent travaille, selon l'annuaire) n'est pas membre du groupe « " +
			nd.Demandeur.Groupe + " »", true
	case nd.Sujet != nil:
		return personne(nd.Sujet, dansQuantif) + " n'appartient pas au groupe « " + nd.Sujet.Groupe + " »", true
	case nd.EtiquetteCible != nil:
		return "le serveur cible ne porte pas l'étiquette « " + nd.EtiquetteCible.Contient + " » au registre", true
	}
	return "", false
}

// prefixeRacine marque, sous un quantificateur, un pointeur lu depuis les
// arguments de l'appel et non depuis l'élément courant.
const prefixeRacine = "$racine"

// racine rend le pointeur d'un champ « $racine/… » et vrai, ou faux.
func racine(champ string) (string, bool) {
	if p, ok := strings.CutPrefix(champ, prefixeRacine); ok && strings.HasPrefix(p, "/") {
		return p, true
	}
	return "", false
}

const lueALaRacine = " de l'appel (lu dans les arguments, et non dans l'élément courant)"

func liste(champ string, dansQuantif bool) string {
	if p, ok := racine(champ); ok {
		return "la liste en argument " + p + lueALaRacine
	}
	if champ == "" {
		return "l'élément courant, qui est une liste,"
	}
	if dansQuantif {
		return "la liste " + champ + " de l'élément courant"
	}
	return "la liste en argument " + champ
}

// argument nomme le champ d'une feuille argument.
func argument(champ string, dansQuantif bool) string {
	if p, ok := racine(champ); ok {
		return "l'argument " + p + lueALaRacine
	}
	switch {
	case dansQuantif && champ == "":
		return "l'élément"
	case dansQuantif:
		return "le champ " + champ + " de l'élément"
	}
	return "l'argument " + champ
}

func feuille(nd *directive.Noeud, dansQuantif bool) string {
	switch {
	case nd.Argument != nil:
		return argument(nd.Argument.Champ, dansQuantif) + " " + operationV2(*nd.Argument)
	case nd.AConsulte != nil:
		return consultation(nd.AConsulte, true)
	case nd.Appelant != nil:
		f := nd.Appelant
		if f.Prefixe != "" {
			return "l'identité de l'appelant, prouvée par le transport, commence par " + f.Prefixe
		}
		return "l'identité de l'appelant, prouvée par le transport, est " + f.Egal
	case nd.SousAgentDeclare != nil:
		f := nd.SousAgentDeclare
		s := "le sous-agent que l'agent déclare (déclaration non prouvée) "
		if f.Dans != nil {
			return s + "est l'un de " + chaines(f.Dans)
		}
		return s + "est « " + f.Egal + " »"
	case nd.Demandeur != nil:
		f := nd.Demandeur
		s := "le demandeur (la personne pour qui l'agent travaille, selon l'annuaire) "
		switch {
		case f.Groupe != "":
			return s + "est membre du groupe « " + f.Groupe + " »"
		case f.Identite != nil && f.Identite.Dans != nil:
			return s + "est l'une des personnes " + chaines(f.Identite.Dans)
		case f.Identite != nil:
			return s + "est « " + f.Identite.Egal + " »"
		}
		return s + "(forme absente)"
	case nd.Attestation != nil:
		return attestation(nd.Attestation)
	case nd.Temps != nil:
		return temps(nd.Temps)
	case nd.EtiquetteCible != nil:
		return "le serveur cible porte l'étiquette « " + nd.EtiquetteCible.Contient + " » au registre"
	case nd.MetadonneeReponse != nil:
		return metadonnee(nd.MetadonneeReponse)
	case nd.Sujet != nil:
		return personne(nd.Sujet, dansQuantif) + " appartient au groupe « " + nd.Sujet.Groupe + " »"
	}
	return "(nœud sans feuille connue)"
}

func operationV2(c directive.Condition) string {
	switch c.Op {
	case directive.OpDansV2:
		return "vaut l'une des valeurs " + valeur(c.Valeur)
	case directive.OpPrefixe:
		return "commence par " + valeur(c.Valeur)
	}
	return operation(c)
}

// consultation rend session.a_consulte, affirmée ou niée.
func consultation(f *directive.FeuilleConsultation, oui bool) string {
	var source string
	switch {
	case f.Serveur != "":
		source = "du serveur " + f.Serveur
	case f.Outil != "":
		sv, outil, _ := strings.Cut(f.Outil, "/")
		source = "de l'outil " + outil + " du serveur " + sv
	default:
		source = "d'un serveur portant l'étiquette « " + f.Etiquette + " » au registre"
	}
	if f.Critere == directive.Transmis {
		if oui {
			return "la session a déjà transmis un appel à destination " + source + " (quel qu'en soit le résultat)"
		}
		return "la session n'a transmis aucun appel à destination " + source
	}
	if oui {
		return "la session a reçu des données " + source
	}
	return "la session n'a reçu aucune donnée " + source
}

func attestation(f *directive.FeuilleAttestation) string {
	if f.Verifiee == nil {
		return "l'attestation du serveur cible (valeur absente)"
	}
	s := "l'attestation du serveur cible au registre (identité, schémas épinglés) a échoué"
	if *f.Verifiee {
		s = "l'attestation du serveur cible au registre (identité, schémas épinglés) est vérifiée"
	}
	if f.Mode != "" {
		s += ", et le mode d'attestation de ce serveur au registre est " + f.Mode
	}
	return s
}

func temps(t *directive.FeuilleTemps) string {
	var parts []string
	if t.JourSemaine != nil {
		jours := make([]string, len(t.JourSemaine))
		for i, j := range t.JourSemaine {
			jours[i] = "un " + j
		}
		parts = append(parts, enumerer(jours, "ou"))
	}
	if h := t.Heure; h != nil {
		p := "entre " + h.De + " (inclus) et " + h.A + " (exclu)"
		if h.De > h.A {
			p += ", à cheval sur minuit"
		}
		parts = append(parts, p)
	}
	if dt := t.Date; dt != nil {
		parts = append(parts, "à une date "+comparaisonDate(dt.Op)+" "+jourCalendaire(dt.Valeur))
	}
	return "l'appel a lieu " + strings.Join(parts, ", et ") + ", en heure locale du fuseau " + t.Fuseau
}

func comparaisonDate(op directive.Op) string {
	switch op {
	case directive.OpSup:
		return "postérieure au"
	case directive.OpSupEgal:
		return "égale ou postérieure au"
	case directive.OpInf:
		return "antérieure au"
	case directive.OpInfEgal:
		return "égale ou antérieure au"
	case directive.OpEgal:
		return "égale au"
	case directive.OpDiff:
		return "différente du"
	}
	return "comparée par l'opérateur inconnu " + strconv.Quote(string(op)) + " au"
}

// jourCalendaire écrit AAAA-MM-JJ en JJ/MM/AAAA, sans passer par une heure.
func jourCalendaire(s string) string {
	if len(s) == 10 && s[4] == '-' && s[7] == '-' {
		return s[8:10] + "/" + s[5:7] + "/" + s[0:4]
	}
	return strconv.Quote(s)
}

func metadonnee(f *directive.FeuilleMetadonnee) string {
	source := "du serveur " + f.Serveur
	if f.Serveur == "" {
		source = "d'un serveur portant l'étiquette « " + f.Etiquette + " » au registre"
	}
	s := "une réponse reçue dans la session " + source + " déclare "
	switch f.Champ {
	case directive.MetaSourceOrigine:
		s += "une origine (source.origine) qui "
		switch f.Op {
		case directive.OpEgal:
			return s + "est " + valeur(f.Valeur)
		case directive.OpDiff:
			return s + "n'est pas " + valeur(f.Valeur)
		case directive.OpDansV2:
			return s + "est l'une de " + valeur(f.Valeur)
		case directive.OpPrefixe:
			return s + "commence par " + valeur(f.Valeur)
		}
		return s + "vérifie l'opérateur inconnu " + strconv.Quote(string(f.Op)) + " " + valeur(f.Valeur)
	case directive.MetaSourceDate:
		v, _ := f.Valeur.(string)
		instant := strconv.Quote(v)
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			instant = date(t)
		}
		return s + "une date de source (source.date) " + comparaisonDate(f.Op) + " " + instant
	}
	return s + "le champ inconnu " + strconv.Quote(f.Champ)
}

// personne nomme la personne concernée d'une feuille sujet.
func personne(f *directive.FeuilleSujet, dansQuantif bool) string {
	if f.Champ == nil || *f.Champ == "" {
		return "la personne désignée par l'élément"
	}
	if p, ok := racine(*f.Champ); ok {
		return "la personne désignée par l'argument " + p + lueALaRacine
	}
	if dansQuantif {
		return "la personne désignée par le champ " + *f.Champ + " de l'élément"
	}
	return "la personne désignée par l'argument " + *f.Champ
}

// evenementDe rend l'évènement du déclencheur ; appel_outil s'il manque.
func evenementDe(d directive.Directive) directive.Evenement {
	if d.Declencheur == nil {
		return directive.EvAppelOutil
	}
	return d.Declencheur.Evenement
}

// objetDe nomme ce sur quoi portent les effets : « l'appel » pour
// appel_outil, « l'évènement » sinon.
func objetDe(ev directive.Evenement) string {
	if ev == directive.EvAppelOutil {
		return "l'appel"
	}
	return "l'évènement"
}

// refus dit ce que refuser veut dire pour l'évènement : l'appel n'est pas
// transmis ; à l'ouverture d'une session, la session n'est pas ouverte.
func refus(ev directive.Evenement) string {
	if ev == directive.EvOuvertureSession {
		return "la session n'est pas ouverte"
	}
	return objetDe(ev) + " est refusé"
}

// effetV2 rend un effet de la directive déclenchée par l'évènement ev.
func effetV2(e directive.EffetDirective, ev directive.Evenement) string {
	objet := objetDe(ev)
	switch e.Effet {
	case directive.Refuser:
		return refus(ev)
	case directive.AttendreValidation:
		s := objet + " attend une validation humaine"
		if e.ParRole != "" {
			s += " par le rôle « " + e.ParRole + " »"
		} else {
			s += " par tout validateur"
		}
		if e.QuatreYeux != nil && *e.QuatreYeux {
			s += ", par deux personnes distinctes (quatre yeux)"
		}
		return s
	case directive.Caviarder:
		liste := pluriel(len(e.ChampsAutorises), "le champ", "les champs") + " " + strings.Join(e.ChampsAutorises, ", ")
		const couvre = " (un champ autorisé couvre aussi tout son contenu)"
		if e.Mode == directive.ModeRefuser {
			return objet + " ne peut contenir que " + liste + couvre + " ; s'il en contient un autre, il est refusé"
		}
		return "seul" + s(len(e.ChampsAutorises)) + " " + liste + " " + pluriel(len(e.ChampsAutorises), "est transmis", "sont transmis") +
			couvre + " ; tout autre champ est retiré avant transmission"
	case directive.Notifier:
		s := "le rôle « " + e.DestinataireRole + " » est notifié"
		switch e.Rythme {
		case directive.TempsReel:
			s += " en temps réel"
		case directive.Synthese:
			s += " dans une synthèse périodique"
		default:
			s += " au rythme inconnu " + strconv.Quote(string(e.Rythme))
		}
		switch e.SiEchec {
		case directive.EchecRefuser:
			s += " ; si la notification ne part pas, " + refus(ev)
		case directive.EchecContinuer:
			s += " ; si la notification ne part pas, " + objet + " suit son cours"
		}
		return s
	case directive.Marquer:
		return objet + " est marqué « " + e.Etiquette + " » au journal"
	}
	return "effet inconnu " + strconv.Quote(string(e.Effet))
}

// indetermineV2 suit Decide (§7.2) : la règle ne vaut qu'à la racine, et
// seulement si quelque chose peut être indéterminé (condition, ou cible
// portant sur une donnée du registre).
func indetermineV2(d directive.Directive) string {
	c := d.Declencheur
	registre := c != nil && c.Cible != nil && (c.Cible.Etiquette != "" || c.Cible.Acces != "")
	if d.Arbre == nil && !registre {
		return ""
	}
	ev := evenementDe(d)
	if ev != directive.EvAppelOutil {
		return indetermineEvenement(d, ev)
	}
	si := "Si la règle ne peut pas être évaluée pour un appel (information absente, illisible ou inconnue du registre), "
	if d.SiIndetermine == directive.IndetermineIgnorer {
		return si + "la directive ne s'applique pas à cet appel (choix validé par " + d.SiIndetermineValidePar + ")."
	}
	for _, e := range d.Effets {
		if e.Effet == directive.Notifier || e.Effet == directive.Marquer {
			return si + "l'appel est refusé ; les notifications et marques de la directive s'appliquent aussi."
		}
	}
	return si + "l'appel est refusé."
}

// indetermineEvenement suit DecideEvenement : l'indéterminé déclenche la
// directive (effet forcé à refuser) ; seule l'ouverture d'une session peut
// être refusée, les autres évènements ne sont pas arrêtés et seules les
// notifications et marques s'appliquent.
func indetermineEvenement(d directive.Directive, ev directive.Evenement) string {
	si := "Si la règle ne peut pas être évaluée pour cet évènement (information absente ou illisible), "
	if d.SiIndetermine == directive.IndetermineIgnorer {
		return si + "la directive ne s'applique pas (choix validé par " + d.SiIndetermineValidePar + ")."
	}
	suite := ""
	for _, e := range d.Effets {
		if e.Effet == directive.Notifier || e.Effet == directive.Marquer {
			suite = " ; les notifications et marques de la directive s'appliquent aussi"
			break
		}
	}
	if ev == directive.EvOuvertureSession {
		return si + "la session n'est pas ouverte" + suite + "."
	}
	if suite == "" {
		return si + "la directive est tenue pour déclenchée, sans effet sur l'évènement, qui suit son cours."
	}
	return si + "la directive est tenue pour déclenchée : l'évènement suit son cours" + suite + "."
}

// chaines écrit une liste de chaînes entre guillemets français.
func chaines(l []string) string {
	parts := make([]string, len(l))
	for i, x := range l {
		parts[i] = "« " + x + " »"
	}
	return strings.Join(parts, ", ")
}

// enumerer joint « a, b ou c ».
func enumerer(l []string, conj string) string {
	if len(l) <= 1 {
		return strings.Join(l, "")
	}
	return strings.Join(l[:len(l)-1], ", ") + " " + conj + " " + l[len(l)-1]
}
