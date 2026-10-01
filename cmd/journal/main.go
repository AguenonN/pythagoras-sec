// Commande journal lit un journal de l'engine (mcp-gate) pour un humain.
//
//	journal verifier <journal.jsonl>
//	journal rejouer  <journal.jsonl> -directives <jeu.json> [<jeu2.json>]
//	                 [-coffre <coffre.jsonl>] [-annuaire <annuaire.json> -annuaire-autorite <autorite.pub>]
//
// verifier contrôle la chaîne HMAC et résume le journal : sessions,
// demandeurs, appels par décision et par statut, approbations,
// notifications, alertes.
//
// rejouer vérifie d'abord la chaîne, puis rejoue chaque appel sous le jeu
// d'origine à sa date (le jeu est retrouvé parmi ceux fournis par son
// identifiant et sa version) et affiche, appel par appel, la décision
// journalisée, la décision rejouée et les écarts. Avec un second jeu, il
// ajoute la vue avant / après : ce que chaque appel serait devenu sous ce
// jeu, et le compte des transitions. Le coffre est par défaut coffre.jsonl
// à côté du journal. Avec l'annuaire, les rôles des valideurs sont
// revérifiés à leur date.
//
// La clé HMAC est lue dans MCP_GATE_CLE_JOURNAL (hexadécimal), comme pour
// mcp-gate. Code de sortie : 0 si tout est conforme, 1 si la chaîne est
// rompue ou si un écart est constaté, 2 pour une erreur d'usage ou de
// lecture.
package main

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"mcprogue/internal/annuaire"
	"mcprogue/internal/directive"
	"mcprogue/internal/journal"
	"mcprogue/internal/rejeu"
)

// VarCle porte la clé du journal, comme pour mcp-gate.
const VarCle = "MCP_GATE_CLE_JOURNAL"

func main() {
	os.Exit(executer(os.Args[1:], os.Stdout, os.Stderr, os.Getenv))
}

const usage = `usage :
  journal verifier <journal.jsonl>
  journal rejouer  <journal.jsonl> -directives <jeu.json> [<jeu2.json>] [-coffre <coffre.jsonl>]
                   [-annuaire <annuaire.json> -annuaire-autorite <autorite.pub>]
La clé du journal est lue dans MCP_GATE_CLE_JOURNAL.`

func executer(args []string, out, errw io.Writer, env func(string) string) int {
	if len(args) < 2 {
		fmt.Fprintln(errw, usage)
		return 2
	}
	cle, err := lireCle(env(VarCle))
	if err != nil {
		fmt.Fprintln(errw, "journal:", err)
		return 2
	}
	brut, err := os.ReadFile(args[1])
	if err != nil {
		fmt.Fprintln(errw, "journal:", err)
		return 2
	}
	switch args[0] {
	case "verifier":
		if len(args) != 2 {
			fmt.Fprintln(errw, usage)
			return 2
		}
		return verifier(args[1], brut, cle, out)
	case "rejouer":
		o, err := lireOptions(args[1], args[2:])
		if err != nil {
			fmt.Fprintln(errw, "journal:", err)
			fmt.Fprintln(errw, usage)
			return 2
		}
		code, err := rejouer(args[1], brut, cle, o, out)
		if err != nil {
			fmt.Fprintln(errw, "journal:", err)
			return 2
		}
		return code
	}
	fmt.Fprintln(errw, usage)
	return 2
}

func lireCle(v string) ([]byte, error) {
	if v == "" {
		return nil, fmt.Errorf("%s absente : la chaîne ne se vérifie pas sans clé", VarCle)
	}
	cle, err := hex.DecodeString(v)
	if err != nil || len(cle) < 32 {
		return nil, fmt.Errorf("%s invalide (hexadécimal, 32 octets au moins)", VarCle)
	}
	return cle, nil
}

// chaine vérifie la chaîne et relit les entrées.
func chaine(chemin string, brut, cle []byte, out io.Writer) ([]journal.Entree, bool) {
	n, err := journal.Verifier(bytes.NewReader(brut), cle)
	if err != nil {
		fmt.Fprintf(out, "Journal  : %s\nChaîne   : ROMPUE après %d entrée(s) vérifiée(s) : %v\n", chemin, n, err)
		return nil, false
	}
	es, err := journal.Lire(bytes.NewReader(brut))
	if err != nil {
		fmt.Fprintf(out, "Journal  : %s\nLecture  : %v\n", chemin, err)
		return nil, false
	}
	fmt.Fprintf(out, "Journal  : %s\nChaîne   : intègre, %d entrée(s) vérifiée(s) (HMAC)\n", chemin, n)
	return es, true
}

func verifier(chemin string, brut, cle []byte, out io.Writer) int {
	es, ok := chaine(chemin, brut, cle, out)
	if !ok {
		return 1
	}
	types := map[journal.Type]int{}
	effets, statuts := map[string]int{}, map[string]int{}
	var sessions []journal.Entree
	var alertes []string
	approbations := map[string]int{}
	notifications := map[string]int{}
	for _, e := range es {
		types[e.Type]++
		switch e.Type {
		case journal.SessionOuverte:
			sessions = append(sessions, e)
		case journal.AppelT:
			if e.Decision != nil {
				effets[e.Decision.Effet]++
			}
			if e.Resultat != nil {
				statuts[e.Resultat.Statut]++
			}
		case journal.AlerteT:
			alertes = append(alertes, e.Message)
		case journal.ApprobationT:
			if e.Approbation != nil {
				approbations[e.Approbation.Statut]++
			}
		case journal.NotificationT:
			if e.Notification != nil {
				notifications[e.Notification.Statut]++
			}
		}
	}
	if len(es) > 0 {
		debut, fin := es[0].TS, es[0].TS
		for _, e := range es {
			if e.TS.Before(debut) {
				debut = e.TS
			}
			if e.TS.After(fin) {
				fin = e.TS
			}
		}
		fmt.Fprintf(out, "Période  : du %s au %s\n", debut.UTC().Format("2006-01-02 15:04:05"), fin.UTC().Format("2006-01-02 15:04:05 MST"))
	}
	fmt.Fprintf(out, "Entrées  : %s\n", compte(types))
	fmt.Fprintf(out, "Sessions : %d\n", len(sessions))
	for _, s := range sessions {
		d := "aucun demandeur"
		if s.Demandeur != nil {
			d = "demandeur " + s.Demandeur.Identite
			if len(s.Demandeur.Groupes) > 0 {
				d += " (" + strings.Join(s.Demandeur.Groupes, ", ") + ")"
			}
		}
		appelant := s.Appelant
		if appelant == "" {
			appelant = "appelant non prouvé"
		}
		fmt.Fprintf(out, "  %s  ouverte le %s par %s, %s\n", court(s.Session), s.TS.UTC().Format("2006-01-02 15:04:05"), appelant, d)
	}
	fmt.Fprintf(out, "Appels   : %d ; décisions : %s ; résultats : %s\n", types[journal.AppelT], compteS(effets), compteS(statuts))
	if len(approbations) > 0 {
		fmt.Fprintf(out, "Approbations : %s\n", compteS(approbations))
	}
	if len(notifications) > 0 {
		fmt.Fprintf(out, "Notifications : %s\n", compteS(notifications))
	}
	if o := rejeu.TransmissionsSansAppel(es); len(o) > 0 {
		fmt.Fprintf(out, "ATTENTION : %d transmission(s) sans entrée appel (appel parti, résultat non journalisé)\n", len(o))
	}
	for _, a := range alertes {
		fmt.Fprintf(out, "Alerte   : %s\n", a)
	}
	return 0
}

// options de rejouer.
type options struct {
	jeux               []string
	coffre             string
	annuaire, autorite string
}

// lireOptions lit -directives (une ou plusieurs valeurs : les arguments
// qui suivent sans tiret), -coffre, -annuaire, -annuaire-autorite.
func lireOptions(cheminJournal string, args []string) (options, error) {
	o := options{coffre: filepath.Join(filepath.Dir(cheminJournal), "coffre.jsonl")}
	for i := 0; i < len(args); i++ {
		valeur := func() (string, error) {
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return "", fmt.Errorf("%s attend une valeur", args[i])
			}
			i++
			return args[i], nil
		}
		var err error
		switch strings.TrimPrefix(args[i], "-") {
		case "directives":
			var v string
			if v, err = valeur(); err == nil {
				o.jeux = append(o.jeux, v)
				for i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
					i++
					o.jeux = append(o.jeux, args[i])
				}
			}
		case "coffre":
			o.coffre, err = valeur()
		case "annuaire":
			o.annuaire, err = valeur()
		case "annuaire-autorite":
			o.autorite, err = valeur()
		default:
			err = fmt.Errorf("option inconnue %s", args[i])
		}
		if err != nil {
			return o, err
		}
	}
	switch {
	case len(o.jeux) == 0:
		return o, errors.New("-directives obligatoire")
	case len(o.jeux) > 2:
		return o, errors.New("-directives : un jeu, ou deux pour la vue avant / après")
	case (o.annuaire == "") != (o.autorite == ""):
		return o, errors.New("-annuaire et -annuaire-autorite vont ensemble")
	}
	return o, nil
}

func rejouer(chemin string, brut, cle []byte, o options, out io.Writer) (int, error) {
	var jeux rejeu.Catalogue
	for _, c := range o.jeux {
		b, err := os.ReadFile(c)
		if err != nil {
			return 2, err
		}
		j, err := directive.Charger(b)
		if err != nil {
			return 2, fmt.Errorf("%s : %w", c, err)
		}
		jeux = append(jeux, *j)
	}
	coffre, err := journal.OuvrirCoffre(o.coffre)
	if err != nil {
		return 2, err
	}
	defer coffre.Fermer()
	opts := rejeu.Options{}
	if o.annuaire != "" {
		pub, err := annuaire.LireClePublique(o.autorite)
		if err != nil {
			return 2, err
		}
		b, err := os.ReadFile(o.annuaire)
		if err != nil {
			return 2, err
		}
		if opts.Annuaire, err = annuaire.Charger(b, pub); err != nil {
			return 2, err
		}
	}

	es, ok := chaine(chemin, brut, cle, out)
	if !ok {
		return 1, nil
	}
	fmt.Fprintf(out, "Coffre   : %s\n", o.coffre)
	for _, j := range jeux {
		fmt.Fprintf(out, "Jeu      : %s v%d %s (%d directive(s))\n", j.Meta.ID, j.Meta.Version, j.Meta.Empreinte, len(j.Directives))
	}
	rs, err := rejeu.RejouerJournal(es, coffre, jeux, opts)
	if err != nil {
		return 2, err
	}
	fmt.Fprintln(out)
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "SESSION\tSEQ\tDATE (UTC)\tSERVEUR/OUTIL\tJOURNALISÉ\tREJOUÉ\tÉCARTS")
	n := 0
	for _, r := range rs {
		etat := "aucun"
		if len(r.Ecarts) > 0 {
			etat = fmt.Sprintf("%d", len(r.Ecarts))
		}
		fmt.Fprintf(tw, "%s\t%d\t%s\t%s/%s\t%s\t%s\t%s\n", court(r.Session), r.Seq, r.TS.UTC().Format("2006-01-02 15:04:05"),
			r.Serveur, r.Outil, r.Journalise, r.Effet(), etat)
		n += len(r.Ecarts)
	}
	tw.Flush()
	// Motif de chaque décision, pour qui veut savoir pourquoi.
	fmt.Fprintln(out, "\nDécisions :")
	for _, en := range es {
		if en.Type == journal.AppelT && en.Decision != nil {
			fmt.Fprintf(out, "  %s #%d %s/%s : %s\n", court(en.Session), en.Seq, en.Serveur, en.Outil, en.Decision.Motif)
		}
	}
	if n == 0 {
		fmt.Fprintf(out, "\nÉcarts   : aucun sur %d appel(s) rejoué(s)\n", len(rs))
	} else {
		fmt.Fprintf(out, "\nÉcarts   : %d\n", n)
		for _, r := range rs {
			for _, e := range r.Ecarts {
				nature := ""
				if e.Nature != "" {
					nature = ", " + string(e.Nature)
				}
				fmt.Fprintf(out, "  %s #%d [%s, contrôle %s%s] %s\n", court(r.Session), r.Seq, e.Cause, e.Controle, nature, e.Detail)
			}
		}
	}
	if o := rejeu.TransmissionsSansAppel(es); len(o) > 0 {
		fmt.Fprintf(out, "ATTENTION : %d transmission(s) sans entrée appel\n", len(o))
		n += len(o)
	}

	if len(jeux) == 2 {
		apres := jeux[1]
		cmp, err := rejeu.Comparer(es, coffre, jeux, rejeu.Options{Annuaire: opts.Annuaire}, rejeu.Options{Jeu: &apres, Annuaire: opts.Annuaire})
		if err != nil {
			return 2, err
		}
		fmt.Fprintf(out, "\nAvant / après : journal tel quel, puis sous %s v%d\n", apres.Meta.ID, apres.Meta.Version)
		tw = tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
		fmt.Fprintln(tw, "SESSION\tSEQ\tSERVEUR/OUTIL\tAVANT\tAPRÈS")
		for _, l := range cmp.Lignes {
			marque := ""
			if l.Avant != l.Apres {
				marque = "  ←"
			}
			fmt.Fprintf(tw, "%s\t%d\t%s/%s\t%s\t%s%s\n", court(l.Session), l.Seq, l.Serveur, l.Outil, l.Avant, l.Apres, marque)
		}
		tw.Flush()
		fmt.Fprintln(out, "Transitions :")
		for _, t := range cmp.Transitions {
			fmt.Fprintf(out, "  %s → %s : %d\n", t.Avant, t.Apres, t.Nombre)
		}
	}
	if n > 0 {
		return 1, nil
	}
	return 0, nil
}

func court(session string) string {
	if len(session) > 8 {
		return session[:8]
	}
	return session
}

func compte(m map[journal.Type]int) string {
	s := map[string]int{}
	for k, v := range m {
		s[string(k)] = v
	}
	return compteS(s)
}

func compteS(m map[string]int) string {
	if len(m) == 0 {
		return "aucun"
	}
	cles := make([]string, 0, len(m))
	for k := range m {
		cles = append(cles, k)
	}
	sort.Strings(cles)
	var out []string
	for _, k := range cles {
		out = append(out, fmt.Sprintf("%s %d", k, m[k]))
	}
	return strings.Join(out, ", ")
}
