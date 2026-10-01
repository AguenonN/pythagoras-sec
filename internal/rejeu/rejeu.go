// Package rejeu rejoue les décisions du journal. Une entrée appel porte
// tout ce que Decide a lu, sauf les arguments, qui sont au coffre sous leur
// empreinte d'origine : le jeu (id, version, empreinte), l'attestation,
// l'instantané de session, l'appelant, la date. Rejouer remet ces pièces
// ensemble et vérifie qu'elles redonnent ce que le journal affirme.
//
// Deux contrôles internes, toujours faits dans le cadre historique (jeu
// d'origine, date de l'entrée) :
//
//   - les arguments du coffre doivent retrouver empreinte_origine ;
//   - la décision rejouée doit retrouver empreinte_transmise, et l'effet
//     journalisé.
//
// Tout écart est nommé : purge (le coffre a une pierre tombale et le
// journal une entrée purge chaînée qui la couvre, à la même date : l'appel
// n'est plus rejouable, son entrée reste vérifiable dans la chaîne) ou
// altération (tout le reste : arguments modifiés ou disparus sans trace,
// pierre tombale sans entrée purge, jeu différent, décision qui ne se
// retrouve pas). Hors cadre historique, une décision rejouée qui diffère
// de la décision journalisée est un effet_de_regle.
//
// La nature ne s'applique qu'aux appels rejouables : violation quand le
// rejeu se fait à la date historique sous le jeu d'origine, exposition
// quand AuDate force la date ou qu'un autre jeu est imposé. Un appel non
// rejouable, purgé ou altéré, ne porte aucune nature : on ne qualifie pas
// ce qu'on ne peut pas rejouer, et une purge conforme n'est jamais une
// violation.
//
// Le rejeu n'est pas en cascade : un appel est rejoué avec l'état de
// session qu'il a réellement vu, même si, sous un autre jeu, un appel
// antérieur aurait été refusé.
//
// Le journal lu doit avoir été vérifié (journal.Verifier) : le rejeu ne
// refait pas la chaîne.
package rejeu

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"time"

	"mcprogue/internal/annuaire"
	"mcprogue/internal/approval"
	"mcprogue/internal/controle"
	"mcprogue/internal/directive"
	"mcprogue/internal/journal"
)

// Cause d'un écart.
type Cause string

const (
	CausePurge      Cause = "purge"
	CauseAlteration Cause = "alteration"
	// CauseEffetDeRegle : en mode exposition, la décision rejouée diffère
	// de la décision journalisée. C'est ce que la vue après montre.
	CauseEffetDeRegle Cause = "effet_de_regle"
)

// Nature d'un écart : le cadre dans lequel il a été constaté. Vide pour
// un appel non rejouable.
type Nature string

const (
	Violation  Nature = "violation"
	Exposition Nature = "exposition"
)

// Contrôles, nommés dans chaque écart.
const (
	ControleJeu       = "jeu"
	ControleArguments = "arguments"
	ControleTransmis  = "transmis"
	ControleDecision  = "decision"
	ControleEtat      = "etat"
	// ControleStatut : le statut du résultat doit être cohérent avec
	// l'effet (un refus n'est jamais transmis, une autorisation l'est
	// toujours, une attente ne l'est qu'avec une approbation).
	ControleStatut = "statut"
	// ControleTransmission : un appel transmis doit être précédé de son
	// entrée transmission (même session, seq, serveur, outil, empreinte
	// transmise, effet) ; un appel non transmis ne doit pas en avoir.
	ControleTransmission = "transmission"
	// ControleApprobation : un appel en attente transmis porte une
	// approbation acceptée, chaînée avant sa transmission, qui couvre
	// chaque rôle requis par un valideur distinct qui n'est pas le
	// demandeur ; avec un annuaire, chaque valideur détenait son rôle à la
	// date de validation.
	ControleApprobation = "approbation"
)

// NonRejouable remplace l'effet dans les vues quand les arguments manquent.
const NonRejouable = "non_rejouable"

// Ecart est un désaccord entre le journal et le rejeu.
type Ecart struct {
	Cause    Cause  `json:"cause"`
	Nature   Nature `json:"nature,omitempty"`
	Controle string `json:"controle"`
	Detail   string `json:"detail"`
}

// Resolveur rend les arguments d'origine d'une empreinte et dit si elle a
// été purgée. *journal.Coffre l'implémente.
type Resolveur interface {
	Arguments(empreinte string) (map[string]any, bool)
	Purgee(empreinte string) (time.Time, bool)
}

// Jeux retrouve un jeu par identifiant et version.
type Jeux interface {
	Jeu(id string, version int) (directive.Jeu, bool)
}

// Catalogue est une liste de jeux chargés.
type Catalogue []directive.Jeu

func (c Catalogue) Jeu(id string, version int) (directive.Jeu, bool) {
	for _, j := range c {
		if j.Meta.ID == id && j.Meta.Version == version {
			return j, true
		}
	}
	return directive.Jeu{}, false
}

// Options fixent le cadre du rejeu. Zéro : le cadre historique.
type Options struct {
	// Jeu remplace le jeu d'origine.
	Jeu *directive.Jeu
	// AuDate force la date de l'appel.
	AuDate time.Time
	// Annuaire, s'il est fourni, sert à revérifier que chaque valideur
	// d'une approbation détenait son rôle à la date de validation. Il ne
	// change pas le cadre (historique ou non).
	Annuaire annuaire.Source
}

// Historique dit si le cadre est celui de l'entrée.
func (o Options) Historique() bool { return o.Jeu == nil && o.AuDate.IsZero() }

func (o Options) nature() Nature {
	if o.Historique() {
		return Violation
	}
	return Exposition
}

// Rejeu est le résultat du rejeu d'une entrée appel.
type Rejeu struct {
	Session string    `json:"session"`
	Seq     int       `json:"seq"`
	Serveur string    `json:"serveur"`
	Outil   string    `json:"outil"`
	TS      time.Time `json:"ts"`

	// Journalise est l'effet que le journal affirme, établi par la chaîne
	// même quand l'appel n'est plus rejouable.
	Journalise directive.Effet `json:"journalise"`
	// Origine est la décision rejouée sous le jeu d'origine à la date
	// historique ; Rejouee celle du cadre demandé (la même en cadre
	// historique). Nulles si l'appel n'est pas rejouable.
	Origine *directive.Decision `json:"-"`
	Rejouee *directive.Decision `json:"-"`

	Rejouable bool    `json:"rejouable"`
	Nature    Nature  `json:"nature,omitempty"`
	Ecarts    []Ecart `json:"ecarts,omitempty"`
}

// Effet rend l'effet du rejeu dans son cadre, ou non_rejouable.
func (r Rejeu) Effet() string {
	if r.Rejouee == nil {
		return NonRejouable
	}
	return string(r.Rejouee.Effet)
}

// Purges indexe les entrées purge chaînées : empreinte d'origine → dates
// des purges qui la citent.
type Purges map[string][]time.Time

// PurgesChainees relève les entrées purge d'un journal vérifié.
func PurgesChainees(entrees []journal.Entree) Purges {
	p := Purges{}
	for _, en := range entrees {
		if en.Type != journal.PurgeT {
			continue
		}
		for _, emp := range en.Empreintes {
			p[emp] = append(p[emp], en.TS)
		}
	}
	return p
}

// couvre dit si une entrée purge chaînée cite l'empreinte à cette date.
func (p Purges) couvre(empreinte string, le time.Time) bool {
	for _, d := range p[empreinte] {
		if d.Equal(le) {
			return true
		}
	}
	return false
}

// ErrPasUnAppel : l'entrée n'est pas une entrée appel complète.
var ErrPasUnAppel = errors.New("rejeu: l'entrée n'est pas un appel rejouable")

// Rejouer rejoue une entrée appel. purges vient de PurgesChainees sur le
// même journal. Une erreur signale une entrée structurellement
// inutilisable ou un jeu introuvable ; les désaccords sont des écarts, pas
// des erreurs.
func Rejouer(en journal.Entree, res Resolveur, jeux Jeux, purges Purges, opts Options) (Rejeu, error) {
	if en.Type != journal.AppelT || en.Appel == nil || en.Decision == nil || en.Jeu == nil {
		return Rejeu{}, ErrPasUnAppel
	}
	r := Rejeu{
		Session: en.Session, Seq: en.Seq, Serveur: en.Serveur, Outil: en.Outil, TS: en.TS,
		Journalise: directive.Effet(en.Decision.Effet),
	}
	// La nature n'est posée qu'une fois l'appel établi rejouable : les
	// écarts d'un appel non rejouable n'en ont pas.
	ecart := func(c Cause, controle, format string, args ...any) {
		r.Ecarts = append(r.Ecarts, Ecart{Cause: c, Nature: r.Nature, Controle: controle, Detail: fmt.Sprintf(format, args...)})
	}
	// Statut et effet se contrôlent sans les arguments : même un appel
	// purgé ne peut pas affirmer avoir transmis un refus.
	if detail, ok := statutCoherent(en); !ok {
		ecart(CauseAlteration, ControleStatut, "%s", detail)
	}

	origine, ok := jeux.Jeu(en.Jeu.ID, en.Jeu.Version)
	if !ok {
		return r, fmt.Errorf("rejeu: jeu %s v%d introuvable", en.Jeu.ID, en.Jeu.Version)
	}
	if origine.Meta.Empreinte != en.Jeu.Empreinte {
		ecart(CauseAlteration, ControleJeu, "le jeu %s v%d fourni a l'empreinte %s, le journal cite %s",
			en.Jeu.ID, en.Jeu.Version, origine.Meta.Empreinte, en.Jeu.Empreinte)
		return r, nil
	}

	// Contrôle 1 : les arguments du coffre retrouvent empreinte_origine.
	args, ok := res.Arguments(en.Appel.EmpreinteOrigine)
	if !ok {
		le, purge := res.Purgee(en.Appel.EmpreinteOrigine)
		switch {
		case purge && purges.couvre(en.Appel.EmpreinteOrigine, le):
			ecart(CausePurge, ControleArguments, "arguments purgés du coffre le %s ; l'entrée reste vérifiable dans la chaîne",
				le.UTC().Format(time.RFC3339))
		case purge:
			ecart(CauseAlteration, ControleArguments, "pierre tombale du %s sans entrée purge chaînée correspondante",
				le.UTC().Format(time.RFC3339))
		default:
			ecart(CauseAlteration, ControleArguments, "arguments absents du coffre sans trace de purge")
		}
		return r, nil
	}
	if got := approval.Digest(en.Outil, args); got != en.Appel.EmpreinteOrigine {
		ecart(CauseAlteration, ControleArguments, "les arguments du coffre hachent vers %s, le journal cite %s",
			got, en.Appel.EmpreinteOrigine)
		return r, nil
	}
	r.Rejouable, r.Nature = true, opts.nature()

	etat := directive.EtatSession{ID: en.Session, Appelant: en.Appelant, Consultations: journal.VersConsultations(en.EtatSession)}
	appel := AppelDepuis(en, args)

	// Contrôle 2 : sous le jeu d'origine, à la date historique, la décision
	// retrouve empreinte_transmise et l'effet journalisé.
	// Le contrôle du registre s'applique après Decide, comme dans l'engine,
	// sur l'attestation que l'entrée porte.
	dec, parRegistre := controle.Registre(directive.Decide(origine, etat, appel), en.Serveur, appel.Attestation)
	r.Origine = &dec
	switch origineJ := en.Decision.Origine; {
	case origineJ != "" && origineJ != controle.OrigineRegistre:
		ecart(CauseAlteration, ControleDecision, "origine de décision %q inconnue", origineJ)
	case parRegistre && origineJ != controle.OrigineRegistre:
		ecart(CauseAlteration, ControleDecision, "le serveur n'est pas attesté : le refus est d'origine registre, le journal ne le dit pas")
	case !parRegistre && origineJ == controle.OrigineRegistre:
		ecart(CauseAlteration, ControleDecision, "le journal attribue la décision au registre, le rejeu à une directive")
	}
	if dec.EmpreinteTransmise != en.Appel.EmpreinteTransmise {
		ecart(CauseAlteration, ControleTransmis, "la décision rejouée transmet %s, le journal cite %s",
			dec.EmpreinteTransmise, en.Appel.EmpreinteTransmise)
	}
	if dec.Effet != r.Journalise {
		ecart(CauseAlteration, ControleDecision, "la décision rejouée est %s, le journal affirme %s",
			dec.Effet, r.Journalise)
	}
	if detail, ok := sortiesV2(en.Decision, dec); !ok {
		ecart(CauseAlteration, ControleDecision, "%s", detail)
	}

	if opts.Historique() {
		r.Rejouee = r.Origine
		return r, nil
	}

	jeu := origine
	if opts.Jeu != nil {
		jeu = *opts.Jeu
	}
	if !opts.AuDate.IsZero() {
		appel.Horodatage = opts.AuDate
	}
	autre, _ := controle.Registre(directive.Decide(jeu, etat, appel), en.Serveur, appel.Attestation)
	r.Rejouee = &autre
	if autre.Effet != r.Journalise {
		ecart(CauseEffetDeRegle, ControleDecision, "sous %s v%d au %s, la décision serait %s ; le journal affirme %s",
			jeu.Meta.ID, jeu.Meta.Version, appel.Horodatage.UTC().Format(time.RFC3339), autre.Effet, r.Journalise)
	}
	return r, nil
}

// AppelDepuis reconstitue l'appel que Decide a reçu, à partir de l'entrée
// appel et des arguments d'origine : tous les champs, v2 compris (registre,
// demandeur, personnes concernées, sous-agent déclaré).
func AppelDepuis(en journal.Entree, args map[string]any) directive.Appel {
	return directive.Appel{
		Serveur: en.Serveur, Outil: en.Outil, Arguments: args,
		Attestation: en.Attestation.VersDirective(), Horodatage: en.TS,
		Acces: directive.Acces(en.Acces), EtiquettesCible: en.EtiquettesCible, VersionRegistre: en.VersionRegistre,
		Demandeur: en.Demandeur.VersDirective(), Sujets: journal.VersSujets(en.Sujets),
		SousAgentDeclare: en.SousAgentDeclare,
	}
}

// sortiesV2 confronte le détail de la décision à ce que le journal
// affirme : motif, applications (résultat à trois valeurs compris),
// alertes, et ce que la décision demande en plus de son effet
// (notifications, marques, validation). Un rôle de validation retiré, une
// notification effacée ou une condition indéterminée présentée comme
// vraie est une altération, même si l'effet est le même. Un journal
// antérieur à la v2 ne porte pas de résultat d'application : il n'est
// alors pas comparé.
func sortiesV2(j *journal.Decision, dec directive.Decision) (string, bool) {
	r := journal.DepuisDecision(dec)
	if len(j.Applications) == len(r.Applications) {
		for i := range j.Applications {
			if j.Applications[i].Resultat == "" {
				r.Applications[i].Resultat = ""
			}
		}
	}
	switch {
	case j.Motif != r.Motif:
		return "le motif rejoué diffère du motif journalisé", false
	case !reflect.DeepEqual(j.Applications, r.Applications):
		return fmt.Sprintf("les applications rejouées (%d) diffèrent de celles du journal (%d)",
			len(r.Applications), len(j.Applications)), false
	case !reflect.DeepEqual(j.Alertes, r.Alertes):
		return "les alertes rejouées diffèrent de celles du journal", false
	case !reflect.DeepEqual(j.Notifications, r.Notifications):
		return fmt.Sprintf("la décision rejouée demande %d notification(s), le journal en consigne %d autrement",
			len(r.Notifications), len(j.Notifications)), false
	case !reflect.DeepEqual(j.Marques, r.Marques):
		return fmt.Sprintf("la décision rejouée pose %d marque(s), le journal en consigne %d autrement",
			len(r.Marques), len(j.Marques)), false
	case !reflect.DeepEqual(j.Validation, r.Validation):
		return "la validation demandée (rôles, quatre yeux) diffère entre le rejeu et le journal", false
	}
	return "", true
}

// statutCoherent confronte le statut du résultat à l'effet journalisé.
func statutCoherent(en journal.Entree) (string, bool) {
	if en.Resultat == nil {
		return "entrée appel sans résultat", false
	}
	_, transmis := consultation(en)
	switch effet := directive.Effet(en.Decision.Effet); effet {
	case directive.Refuser:
		if transmis || en.Resultat.Statut != journal.StatutNonTransmis {
			return fmt.Sprintf("effet refuser, statut %s : un refus n'est jamais transmis", en.Resultat.Statut), false
		}
	case directive.AttendreValidation:
		if transmis && en.Approbation == nil {
			return fmt.Sprintf("effet attendre_validation, statut %s, sans approbation", en.Resultat.Statut), false
		}
		if !transmis && en.Resultat.Statut != journal.StatutNonTransmis {
			return fmt.Sprintf("statut %s inconnu", en.Resultat.Statut), false
		}
	case directive.Autoriser, directive.Caviarder:
		if !transmis {
			return fmt.Sprintf("effet %s, statut %s : une autorisation est toujours transmise", effet, en.Resultat.Statut), false
		}
	default:
		return fmt.Sprintf("effet %q inconnu", effet), false
	}
	return "", true
}

// consultation rend la consultation qu'une entrée appel a produite : un
// appel transmis, quel que soit son résultat. Un refus de l'engine
// (non_transmis) n'en est pas une.
func consultation(en journal.Entree) (journal.Consultation, bool) {
	if en.Type != journal.AppelT || en.Resultat == nil || en.Appel == nil {
		return journal.Consultation{}, false
	}
	switch en.Resultat.Statut {
	case journal.StatutOK, journal.StatutErreur, journal.StatutIndisponible, journal.StatutRefuse:
	default:
		return journal.Consultation{}, false
	}
	// Les compléments v2 sont ceux de l'entrée qui a produit la
	// consultation : étiquettes et version du registre au moment de
	// l'appel, source déclarée telle que Decide la lit.
	return journal.Consultation{
		Seq: en.Seq, Serveur: en.Serveur, Outil: en.Outil, TS: en.TS, Resultat: en.Resultat.Statut,
		Empreinte: en.Appel.EmpreinteTransmise, EmpreinteReponse: en.Resultat.EmpreinteReponse,
		Etiquettes: en.EtiquettesCible, VersionRegistre: en.VersionRegistre,
		Source: journal.DepuisSourceReponse(en.Resultat.Source.VersDirective()),
	}, true
}

// ReconstituerEtat recompose, à partir du journal seul, l'état de la
// session tel qu'il était avant l'appel de rang avantSeq : l'appelant fixé
// à l'ouverture et les consultations des appels transmis. L'horodatage
// d'une consultation est celui de l'entrée, pas celui de la transmission.
func ReconstituerEtat(entrees []journal.Entree, session string, avantSeq int) (directive.EtatSession, error) {
	etat := directive.EtatSession{ID: session}
	ouverte := false
	var cs []journal.Consultation
	for _, en := range entrees {
		if en.Session != session {
			continue
		}
		switch en.Type {
		case journal.SessionOuverte:
			ouverte = true
			etat.Appelant = en.Appelant
		case journal.AppelT:
			if en.Seq >= avantSeq {
				continue
			}
			if c, ok := consultation(en); ok {
				cs = append(cs, c)
			}
		}
	}
	if !ouverte {
		return etat, fmt.Errorf("rejeu: session %s sans entrée session_ouverte", session)
	}
	sort.SliceStable(cs, func(i, j int) bool { return cs[i].Seq < cs[j].Seq })
	etat.Consultations = journal.VersConsultations(cs)
	return etat, nil
}

// DemandeurSession rend le demandeur consigné à l'ouverture de la session
// (entrée session_ouverte), nil s'il n'a pas été résolu. C'est le seul que
// les appels de la session peuvent porter.
func DemandeurSession(entrees []journal.Entree, session string) *journal.Demandeur {
	for _, en := range entrees {
		if en.Type == journal.SessionOuverte && en.Session == session {
			return en.Demandeur
		}
	}
	return nil
}

// cleAppel identifie un appel dans le journal.
type cleAppel struct {
	session string
	seq     int
}

// transmissions indexe les entrées transmission par appel, avec leur rang
// dans le journal.
func transmissions(entrees []journal.Entree) map[cleAppel][]int {
	out := map[cleAppel][]int{}
	for i, en := range entrees {
		if en.Type == journal.TransmissionT {
			k := cleAppel{en.Session, en.Seq}
			out[k] = append(out[k], i)
		}
	}
	return out
}

// controlerTransmission confronte l'entrée appel de rang i aux entrées
// transmission du même appel.
func controlerTransmission(entrees []journal.Entree, i int, tr map[cleAppel][]int) (string, bool) {
	en := entrees[i]
	rangs := tr[cleAppel{en.Session, en.Seq}]
	_, transmis := consultation(en)
	if !transmis {
		if len(rangs) > 0 {
			return "une transmission est journalisée pour un appel déclaré non transmis", false
		}
		return "", true
	}
	if len(rangs) != 1 {
		return fmt.Sprintf("appel transmis avec %d entrée(s) transmission au lieu d'une", len(rangs)), false
	}
	t := entrees[rangs[0]]
	switch {
	case rangs[0] > i:
		return "l'entrée transmission suit l'entrée appel au lieu de la précéder", false
	case t.Serveur != en.Serveur || t.Outil != en.Outil:
		return fmt.Sprintf("la transmission vise %s/%s, l'appel %s/%s", t.Serveur, t.Outil, en.Serveur, en.Outil), false
	case t.Appel == nil || t.Appel.EmpreinteTransmise != en.Appel.EmpreinteTransmise:
		return "l'empreinte transmise diffère entre la transmission et l'appel", false
	case t.Decision == nil || t.Decision.Effet != en.Decision.Effet:
		return "l'effet diffère entre la transmission et l'appel", false
	}
	return "", true
}

// controlerApprobation confronte l'entrée appel de rang i à son
// approbation. Seuls les appels en attente de validation sont concernés.
// Une approbation d'un autre mécanisme (sans statut) n'est contrôlée que
// par sa présence (statutCoherent).
func controlerApprobation(entrees []journal.Entree, i int, tr map[cleAppel][]int, src annuaire.Source) (string, bool) {
	en := entrees[i]
	_, transmis := consultation(en)
	if en.Decision == nil || directive.Effet(en.Decision.Effet) != directive.AttendreValidation {
		if en.Approbation != nil {
			return "approbation consignée pour un appel qui n'attendait pas de validation", false
		}
		return "", true
	}
	a := en.Approbation
	if !transmis {
		if a != nil {
			return "approbation consignée pour un appel en attente non transmis", false
		}
		return "", true
	}
	if a == nil || a.Statut == "" {
		return "", true // statutCoherent signale l'absence
	}
	if a.Statut != journal.ApprobationAcceptee {
		return "appel transmis sous une approbation " + a.Statut, false
	}
	// L'entrée approbation chaînée, identique, avant la transmission.
	rangs := tr[cleAppel{en.Session, en.Seq}]
	trouvee := false
	for j := 0; j < i; j++ {
		e := entrees[j]
		if e.Type == journal.ApprobationT && e.Session == en.Session && e.Seq == en.Seq && reflect.DeepEqual(e.Approbation, a) &&
			(len(rangs) == 0 || j < rangs[0]) {
			trouvee = true
		}
	}
	if !trouvee {
		return "aucune entrée approbation chaînée avant la transmission ne correspond à l'approbation de l'appel", false
	}
	if a.EmpreinteTransmise != en.Appel.EmpreinteTransmise {
		return "l'approbation vise une autre empreinte transmise que l'appel", false
	}
	var roles []string
	quatre := false
	if v := en.Decision.Validation; v != nil {
		roles, quatre = v.Roles, v.QuatreYeux
	}
	if !reflect.DeepEqual(a.Roles, roles) || a.QuatreYeux != quatre {
		return "l'approbation ne porte pas les rôles ou les quatre yeux que la décision demande", false
	}
	if quatre && en.Demandeur == nil {
		return "quatre yeux approuvés sans demandeur résolu", false
	}
	vus, parRole := map[string]bool{}, map[string]int{}
	for _, v := range a.Valideurs {
		if vus[v.Identite] {
			return "le même valideur apparaît deux fois", false
		}
		vus[v.Identite] = true
		parRole[v.Role]++
		if en.Demandeur != nil && v.Identite == en.Demandeur.Identite {
			return "le demandeur a validé son propre appel", false
		}
		if src != nil {
			f, ok := src.Resoudre(v.Identite, v.Date)
			if !ok || !f.DetientRole(v.Role) {
				return fmt.Sprintf("%s ne détenait pas le rôle %s le %s selon l'annuaire", v.Identite, v.Role, v.Date.UTC().Format(time.RFC3339)), false
			}
		}
	}
	if len(roles) == 0 {
		if len(a.Valideurs) != 1 {
			return fmt.Sprintf("un valideur attendu, %d consignés", len(a.Valideurs)), false
		}
		return "", true
	}
	if len(a.Valideurs) != len(roles) {
		return fmt.Sprintf("%d valideur(s) pour %d rôle(s) requis", len(a.Valideurs), len(roles)), false
	}
	for _, r := range roles {
		if parRole[r] != 1 {
			return "le rôle " + r + " n'est pas validé exactement une fois", false
		}
	}
	return "", true
}

// TransmissionsSansAppel rend les entrées transmission qu'aucune entrée
// appel du même (session, seq) ne suit : des appels partis dont le
// résultat n'a pas été journalisé (panne du journal ou arrêt entre les
// deux écritures). Elles ne se rejouent pas, faute de décision complète ;
// elles se signalent.
func TransmissionsSansAppel(entrees []journal.Entree) []journal.Entree {
	appels := map[cleAppel]bool{}
	for _, en := range entrees {
		if en.Type == journal.AppelT {
			appels[cleAppel{en.Session, en.Seq}] = true
		}
	}
	var out []journal.Entree
	for _, en := range entrees {
		if en.Type == journal.TransmissionT && !appels[cleAppel{en.Session, en.Seq}] {
			out = append(out, en)
		}
	}
	return out
}

// RejouerJournal rejoue chaque entrée appel et confronte, en plus des deux
// contrôles, l'instantané de session enregistré à l'état reconstitué à
// partir des entrées précédentes de la même session, et l'appel à son
// entrée transmission.
func RejouerJournal(entrees []journal.Entree, res Resolveur, jeux Jeux, opts Options) ([]Rejeu, error) {
	var out []Rejeu
	purges := PurgesChainees(entrees)
	tr := transmissions(entrees)
	for i, en := range entrees {
		if en.Type != journal.AppelT {
			continue
		}
		r, err := Rejouer(en, res, jeux, purges, opts)
		if err != nil {
			return out, fmt.Errorf("session %s seq %d: %w", en.Session, en.Seq, err)
		}
		etat, err := ReconstituerEtat(entrees, en.Session, en.Seq)
		if err != nil {
			r.Ecarts = append(r.Ecarts, Ecart{Cause: CauseAlteration, Nature: r.Nature, Controle: ControleEtat, Detail: err.Error()})
		} else if !memesConsultations(etat.Consultations, journal.VersConsultations(en.EtatSession)) {
			r.Ecarts = append(r.Ecarts, Ecart{Cause: CauseAlteration, Nature: r.Nature, Controle: ControleEtat,
				Detail: fmt.Sprintf("l'instantané enregistré porte %d consultation(s), le journal en reconstitue %d",
					len(en.EtatSession), len(etat.Consultations))})
		} else if etat.Appelant != en.Appelant {
			r.Ecarts = append(r.Ecarts, Ecart{Cause: CauseAlteration, Nature: r.Nature, Controle: ControleEtat,
				Detail: fmt.Sprintf("l'appel est attribué à %q, la session a été ouverte par %q", en.Appelant, etat.Appelant)})
		} else if d := DemandeurSession(entrees, en.Session); !reflect.DeepEqual(d, en.Demandeur) {
			r.Ecarts = append(r.Ecarts, Ecart{Cause: CauseAlteration, Nature: r.Nature, Controle: ControleEtat,
				Detail: "le demandeur de l'appel n'est pas celui résolu à l'ouverture de la session"})
		}
		if detail, ok := controlerTransmission(entrees, i, tr); !ok {
			r.Ecarts = append(r.Ecarts, Ecart{Cause: CauseAlteration, Nature: r.Nature, Controle: ControleTransmission, Detail: detail})
		}
		if detail, ok := controlerApprobation(entrees, i, tr, opts.Annuaire); !ok {
			r.Ecarts = append(r.Ecarts, Ecart{Cause: CauseAlteration, Nature: r.Nature, Controle: ControleApprobation, Detail: detail})
		}
		out = append(out, r)
	}
	return out, nil
}

// memesConsultations compare sans l'horodatage, que Decide ne lit pas et
// que l'instantané prend à la transmission, pas à l'écriture.
func memesConsultations(a, b []directive.Consultation) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		x, y := a[i], b[i]
		x.Horodatage, y.Horodatage = time.Time{}, time.Time{}
		if !reflect.DeepEqual(x, y) {
			return false
		}
	}
	return true
}

// Ligne est un appel dans la vue avant / après.
type Ligne struct {
	Session string  `json:"session"`
	Seq     int     `json:"seq"`
	Serveur string  `json:"serveur"`
	Outil   string  `json:"outil"`
	Avant   string  `json:"avant"`
	Apres   string  `json:"apres"`
	Ecarts  []Ecart `json:"ecarts,omitempty"`
}

// Transition compte les appels passés d'un effet à un autre.
type Transition struct {
	Avant  string `json:"avant"`
	Apres  string `json:"apres"`
	Nombre int    `json:"nombre"`
}

// Comparaison est la vue avant / après.
type Comparaison struct {
	Lignes      []Ligne      `json:"lignes"`
	Transitions []Transition `json:"transitions"`
}

// Comparer rejoue le journal dans deux cadres et produit le tableau des
// transitions d'effet. Dans le cadre historique, l'effet avant est celui
// que le journal affirme : il reste établi par la chaîne même quand les
// arguments ont été purgés. Dans tout autre cadre, un appel sans arguments
// est non_rejouable. Les écarts des deux cadres sont joints à la ligne.
func Comparer(entrees []journal.Entree, res Resolveur, jeux Jeux, avant, apres Options) (Comparaison, error) {
	ra, err := RejouerJournal(entrees, res, jeux, avant)
	if err != nil {
		return Comparaison{}, err
	}
	rb, err := RejouerJournal(entrees, res, jeux, apres)
	if err != nil {
		return Comparaison{}, err
	}
	effet := func(r Rejeu, o Options) string {
		if o.Historique() {
			return string(r.Journalise)
		}
		return r.Effet()
	}
	var cmp Comparaison
	compte := map[[2]string]int{}
	for i := range ra {
		a, b := ra[i], rb[i]
		l := Ligne{Session: a.Session, Seq: a.Seq, Serveur: a.Serveur, Outil: a.Outil,
			Avant: effet(a, avant), Apres: effet(b, apres)}
		l.Ecarts = fusionner(a.Ecarts, b.Ecarts)
		cmp.Lignes = append(cmp.Lignes, l)
		compte[[2]string{l.Avant, l.Apres}]++
	}
	for k, n := range compte {
		cmp.Transitions = append(cmp.Transitions, Transition{Avant: k[0], Apres: k[1], Nombre: n})
	}
	sort.Slice(cmp.Transitions, func(i, j int) bool {
		x, y := cmp.Transitions[i], cmp.Transitions[j]
		if x.Avant != y.Avant {
			return rang(x.Avant) < rang(y.Avant)
		}
		return rang(x.Apres) < rang(y.Apres)
	})
	return cmp, nil
}

// fusionner joint les écarts des deux cadres sans répéter ceux que les
// deux ont constatés dans les mêmes termes (les contrôles internes).
func fusionner(a, b []Ecart) []Ecart {
	out := append([]Ecart(nil), a...)
	vus := map[[3]string]bool{}
	for _, e := range a {
		vus[[3]string{string(e.Cause), e.Controle, e.Detail}] = true
	}
	for _, e := range b {
		if !vus[[3]string{string(e.Cause), e.Controle, e.Detail}] {
			out = append(out, e)
		}
	}
	return out
}

func rang(effet string) int {
	switch directive.Effet(effet) {
	case directive.Autoriser:
		return 0
	case directive.Caviarder:
		return 1
	case directive.AttendreValidation:
		return 2
	case directive.Refuser:
		return 3
	}
	return 4 // non_rejouable en dernier
}
