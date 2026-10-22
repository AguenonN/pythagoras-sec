// Commande journal-sql verse un journal de l'engine dans PostgreSQL (Aiven)
// et calcule l'impact rétroactif d'un changement de donnée réglementaire.
//
//	journal-sql schema
//	journal-sql exporter <journal.jsonl> [-id <identifiant>]
//	journal-sql impacts  <journal.jsonl> -directives <jeu.json> -gels <siren,siren…>
//	                     [-evenement <libellé>] [-coffre <coffre.jsonl>] [-id <identifiant>]
//
// La commande n'ouvre aucune connexion : elle écrit du SQL sur la sortie
// standard, à envoyer à psql. Aucune dépendance ajoutée au module.
//
//	journal-sql schema | psql "$AIVEN_PG_URI"
//	journal-sql exporter journal.jsonl | psql "$AIVEN_PG_URI"
//	journal-sql impacts journal.jsonl -directives avec-d009.json \
//	    -gels 405750753 -evenement "gels 2026-10-22" | psql "$AIVEN_PG_URI"
//
// exporter vérifie la chaîne HMAC avant d'écrire quoi que ce soit : un
// journal rompu n'entre pas dans la base. L'import est idempotent (les
// lignes du même journal sont remplacées dans une transaction).
//
// impacts rejoue le journal deux fois avec rejeu.Comparer : sous le jeu
// d'origine, puis sous ce jeu augmenté d'une directive « aucun prêt à une
// entité désormais gelée ». Le moteur n'est pas modifié : la directive est
// une directive v2 ordinaire (feuille argument, op dans), chargée par
// directive.Charger. Les décisions dont l'effet change sont les décisions
// que la mise à jour du registre invalide.
//
// La clé HMAC est lue dans MCP_GATE_CLE_JOURNAL (hexadécimal), comme pour
// mcp-gate et journal.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"mcprogue/internal/directive"
	"mcprogue/internal/journal"
	"mcprogue/internal/rejeu"
)

// VarCle porte la clé du journal, comme pour mcp-gate.
const VarCle = "MCP_GATE_CLE_JOURNAL"

// IDDirectiveGels est l'identifiant de la directive synthétisée par impacts.
const IDDirectiveGels = "D-090"

const usage = `usage :
  journal-sql schema
  journal-sql exporter <journal.jsonl> [-id <identifiant>]
  journal-sql impacts  <journal.jsonl> -directives <jeu.json> -gels <siren,siren…>
                       [-evenement <libellé>] [-coffre <coffre.jsonl>] [-id <identifiant>]
Le SQL est écrit sur la sortie standard (à envoyer à psql).
La clé du journal est lue dans MCP_GATE_CLE_JOURNAL.`

func main() {
	os.Exit(executer(os.Args[1:], os.Stdout, os.Stderr, os.Getenv))
}

func executer(args []string, out, errw io.Writer, env func(string) string) int {
	if len(args) == 0 {
		fmt.Fprintln(errw, usage)
		return 2
	}
	if args[0] == "schema" {
		fmt.Fprint(out, Schema)
		return 0
	}
	if len(args) < 2 || (args[0] != "exporter" && args[0] != "impacts") {
		fmt.Fprintln(errw, usage)
		return 2
	}
	o, err := lireOptions(args[1], args[2:])
	if err != nil {
		fmt.Fprintln(errw, "journal-sql:", err)
		fmt.Fprintln(errw, usage)
		return 2
	}
	cle, err := lireCle(env(VarCle))
	if err != nil {
		fmt.Fprintln(errw, "journal-sql:", err)
		return 2
	}
	brut, err := os.ReadFile(args[1])
	if err != nil {
		fmt.Fprintln(errw, "journal-sql:", err)
		return 2
	}
	n, err := journal.Verifier(bytes.NewReader(brut), cle)
	if err != nil {
		fmt.Fprintf(errw, "journal-sql: chaîne ROMPUE après %d entrée(s) : %v — rien n'est écrit\n", n, err)
		return 1
	}
	es, err := journal.Lire(bytes.NewReader(brut))
	if err != nil {
		fmt.Fprintln(errw, "journal-sql:", err)
		return 2
	}
	if o.id == "" {
		o.id = IdentifiantJournal(brut)
	}
	switch args[0] {
	case "exporter":
		ecrireExport(out, o.id, args[1], es)
		fmt.Fprintf(errw, "journal-sql: %d entrée(s) intègres, journal %s\n", len(es), o.id)
		return 0
	default:
		if err := impacts(out, errw, o, es); err != nil {
			fmt.Fprintln(errw, "journal-sql:", err)
			return 2
		}
		return 0
	}
}

type options struct {
	id        string
	jeu       string
	coffre    string
	gels      []string
	evenement string
}

func lireOptions(cheminJournal string, args []string) (options, error) {
	o := options{coffre: filepath.Join(filepath.Dir(cheminJournal), "coffre.jsonl")}
	for i := 0; i < len(args); i++ {
		if i+1 >= len(args) {
			return o, fmt.Errorf("%s attend une valeur", args[i])
		}
		v := args[i+1]
		switch strings.TrimPrefix(args[i], "-") {
		case "id":
			o.id = v
		case "directives":
			o.jeu = v
		case "coffre":
			o.coffre = v
		case "evenement":
			o.evenement = v
		case "gels":
			for _, s := range strings.Split(v, ",") {
				if s = strings.TrimSpace(s); s != "" {
					o.gels = append(o.gels, s)
				}
			}
		default:
			return o, fmt.Errorf("option inconnue %s", args[i])
		}
		i++
	}
	return o, nil
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

// IdentifiantJournal : 12 premiers caractères hexadécimaux du sha256 du
// fichier. Réimporter le même journal remplace ses lignes.
func IdentifiantJournal(brut []byte) string {
	h := sha256.Sum256(brut)
	return hex.EncodeToString(h[:])[:12]
}

// ---------------------------------------------------------------------------
// Schéma
// ---------------------------------------------------------------------------

// Schema crée les tables et la vue « detective ». Rejouable.
const Schema = `-- Pythagoras : journal de l'engine dans PostgreSQL.
CREATE TABLE IF NOT EXISTS journaux (
  id          text PRIMARY KEY,
  chemin      text NOT NULL,
  nb_entrees  integer NOT NULL,
  importe_le  timestamptz NOT NULL DEFAULT now()
);

-- Une ligne par entrée du journal ; brut garde l'entrée chaînée entière.
CREATE TABLE IF NOT EXISTS entrees (
  journal      text NOT NULL REFERENCES journaux(id) ON DELETE CASCADE,
  rang         integer NOT NULL,
  ts           timestamptz NOT NULL,
  type         text NOT NULL,
  session      text,
  seq          integer,
  appelant     text,
  sous_agent   text,
  serveur      text,
  outil        text,
  jeu_id       text,
  jeu_version  integer,
  effet        text,
  motif        text,
  statut       text,
  mac          text,
  brut         jsonb NOT NULL,
  PRIMARY KEY (journal, rang)
);

-- Ce que la décision a lu : l'état de session consigné sur chaque appel.
CREATE TABLE IF NOT EXISTS consultations (
  journal           text NOT NULL REFERENCES journaux(id) ON DELETE CASCADE,
  rang              integer NOT NULL,
  i                 integer NOT NULL,
  seq               integer,
  serveur           text,
  outil             text,
  ts                timestamptz,
  resultat          text,
  source_origine    text,
  source_date       timestamptz,
  version_registre  text,
  PRIMARY KEY (journal, rang, i)
);

-- Les directives évaluées sur chaque appel.
CREATE TABLE IF NOT EXISTS applications (
  journal     text NOT NULL REFERENCES journaux(id) ON DELETE CASCADE,
  rang        integer NOT NULL,
  directive   text NOT NULL,
  version     integer,
  effet       text,
  declenchee  boolean,
  appliquee   boolean,
  resultat    text,
  motif       text,
  PRIMARY KEY (journal, rang, directive)
);

-- Impact rétroactif d'un changement de donnée (journal-sql impacts).
CREATE TABLE IF NOT EXISTS impacts (
  evenement   text NOT NULL,
  journal     text NOT NULL,
  session     text NOT NULL,
  seq         integer NOT NULL,
  serveur     text,
  outil       text,
  avant       text,
  apres       text,
  change      boolean NOT NULL,
  calcule_le  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (evenement, journal, session, seq)
);

-- Pour une décision : ce que l'agent a lu, d'où, de quand.
CREATE OR REPLACE VIEW detective AS
SELECT e.journal, e.session, e.seq, e.ts,
       e.serveur || '/' || e.outil AS action,
       e.sous_agent, e.effet, e.motif,
       c.serveur || '/' || c.outil AS donnee_lue,
       c.resultat, c.source_origine, c.source_date
FROM entrees e
LEFT JOIN consultations c ON c.journal = e.journal AND c.rang = e.rang
WHERE e.type = 'appel'
ORDER BY e.journal, e.session, e.seq, c.i;
`

// ---------------------------------------------------------------------------
// Export
// ---------------------------------------------------------------------------

func ecrireExport(w io.Writer, id, chemin string, es []journal.Entree) {
	fmt.Fprintln(w, "BEGIN;")
	fmt.Fprintf(w, "DELETE FROM journaux WHERE id = %s;\n", Lit(id))
	fmt.Fprintf(w, "INSERT INTO journaux (id, chemin, nb_entrees) VALUES (%s, %s, %d);\n", Lit(id), Lit(chemin), len(es))
	for rang, en := range es {
		ecrireEntree(w, id, rang, en)
	}
	fmt.Fprintln(w, "COMMIT;")
}

func ecrireEntree(w io.Writer, id string, rang int, en journal.Entree) {
	var effet, motif, statut, jeuID string
	jeuVersion := 0
	if en.Decision != nil {
		effet, motif = en.Decision.Effet, en.Decision.Motif
	}
	if en.Resultat != nil {
		statut = en.Resultat.Statut
	}
	if en.Jeu != nil {
		jeuID, jeuVersion = en.Jeu.ID, en.Jeu.Version
	}
	brut, _ := json.Marshal(en)
	fmt.Fprintf(w, "INSERT INTO entrees (journal, rang, ts, type, session, seq, appelant, sous_agent, serveur, outil, jeu_id, jeu_version, effet, motif, statut, mac, brut) VALUES (%s, %d, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s::jsonb);\n",
		Lit(id), rang, Instant(en.TS), Lit(string(en.Type)), LitOuNull(en.Session), EntierOuNull(en.Seq),
		LitOuNull(en.Appelant), LitOuNull(en.SousAgentDeclare), LitOuNull(en.Serveur), LitOuNull(en.Outil),
		LitOuNull(jeuID), EntierOuNull(jeuVersion), LitOuNull(effet), LitOuNull(motif), LitOuNull(statut),
		LitOuNull(en.MAC), Lit(string(JSONSansNul(brut))))
	if en.Type != journal.AppelT {
		return
	}
	for i, c := range en.EtatSession {
		origine, date := "", ""
		if c.Source != nil {
			origine, date = c.Source.Origine, c.Source.Date
		}
		fmt.Fprintf(w, "INSERT INTO consultations (journal, rang, i, seq, serveur, outil, ts, resultat, source_origine, source_date, version_registre) VALUES (%s, %d, %d, %d, %s, %s, %s, %s, %s, %s, %s);\n",
			Lit(id), rang, i, c.Seq, LitOuNull(c.Serveur), LitOuNull(c.Outil), Instant(c.TS), LitOuNull(c.Resultat),
			LitOuNull(origine), InstantTexte(date), LitOuNull(c.VersionRegistre))
	}
	if en.Decision == nil {
		return
	}
	for _, a := range en.Decision.Applications {
		fmt.Fprintf(w, "INSERT INTO applications (journal, rang, directive, version, effet, declenchee, appliquee, resultat, motif) VALUES (%s, %d, %s, %d, %s, %t, %t, %s, %s) ON CONFLICT DO NOTHING;\n",
			Lit(id), rang, Lit(a.ID), a.Version, LitOuNull(a.Effet), a.Declenchee, a.Appliquee, LitOuNull(a.Resultat), LitOuNull(a.Motif))
	}
}

// ---------------------------------------------------------------------------
// Impacts rétroactifs
// ---------------------------------------------------------------------------

func impacts(out, errw io.Writer, o options, es []journal.Entree) error {
	if o.jeu == "" {
		return errors.New("-directives obligatoire : le jeu d'origine du journal")
	}
	if len(o.gels) == 0 {
		return errors.New("-gels obligatoire : les SIREN ajoutés au registre des gels")
	}
	if o.evenement == "" {
		o.evenement = "gels " + strings.Join(o.gels, ",")
	}
	brutJeu, err := os.ReadFile(o.jeu)
	if err != nil {
		return err
	}
	origine, err := directive.Charger(brutJeu)
	if err != nil {
		return fmt.Errorf("%s : %w", o.jeu, err)
	}
	brutApres, err := JeuAvecGels(brutJeu, o.gels)
	if err != nil {
		return err
	}
	apres, err := directive.Charger(brutApres)
	if err != nil {
		return fmt.Errorf("jeu augmenté de %s : %w", IDDirectiveGels, err)
	}
	coffre, err := journal.OuvrirCoffre(o.coffre)
	if err != nil {
		return err
	}
	defer coffre.Fermer()
	cmp, err := rejeu.Comparer(es, coffre, rejeu.Catalogue{*origine, *apres}, rejeu.Options{}, rejeu.Options{Jeu: apres})
	if err != nil {
		return err
	}
	ecrireImpacts(out, o.id, o.evenement, cmp)
	changes := 0
	for _, l := range cmp.Lignes {
		if l.Avant != l.Apres {
			changes++
			fmt.Fprintf(errw, "  invalidée : session %s #%d %s/%s  %s → %s\n", court(l.Session), l.Seq, l.Serveur, l.Outil, l.Avant, l.Apres)
		}
	}
	fmt.Fprintf(errw, "journal-sql: %q : %d décision(s) rejouée(s), %d invalidée(s)\n", o.evenement, len(cmp.Lignes), changes)
	return nil
}

func ecrireImpacts(w io.Writer, id, evenement string, cmp rejeu.Comparaison) {
	fmt.Fprintln(w, "BEGIN;")
	fmt.Fprintf(w, "DELETE FROM impacts WHERE evenement = %s AND journal = %s;\n", Lit(evenement), Lit(id))
	for _, l := range cmp.Lignes {
		fmt.Fprintf(w, "INSERT INTO impacts (evenement, journal, session, seq, serveur, outil, avant, apres, change) VALUES (%s, %s, %s, %d, %s, %s, %s, %s, %t);\n",
			Lit(evenement), Lit(id), Lit(l.Session), l.Seq, LitOuNull(l.Serveur), LitOuNull(l.Outil),
			LitOuNull(l.Avant), LitOuNull(l.Apres), l.Avant != l.Apres)
	}
	fmt.Fprintln(w, "COMMIT;")
}

// JeuAvecGels renvoie le jeu d'origine, version + 1, augmenté d'une
// directive qui refuse attribuer_pret quand l'argument siren figure parmi
// les entités nouvellement gelées. Sa date d'effet est celle du jeu
// d'origine : le rejeu évalue « si nous avions su », cadre d'exposition.
// L'empreinte et la signature du jeu d'origine sont retirées, le
// chargeur recalcule l'empreinte.
func JeuAvecGels(brutJeu []byte, sirens []string) ([]byte, error) {
	var jeu map[string]any
	dec := json.NewDecoder(bytes.NewReader(brutJeu))
	dec.UseNumber()
	if err := dec.Decode(&jeu); err != nil {
		return nil, fmt.Errorf("jeu illisible : %w", err)
	}
	meta, ok := jeu["jeu"].(map[string]any)
	if !ok {
		return nil, errors.New("jeu sans en-tête « jeu »")
	}
	version, err := meta["version"].(json.Number).Int64()
	if err != nil {
		return nil, errors.New("version du jeu illisible")
	}
	dateEffet, _ := meta["date_effet"].(string)
	if dateEffet == "" {
		return nil, errors.New("date_effet du jeu absente")
	}
	directives, _ := jeu["directives"].([]any)
	for _, d := range directives {
		if m, _ := d.(map[string]any); m != nil && m["id"] == IDDirectiveGels {
			return nil, fmt.Errorf("le jeu contient déjà %s", IDDirectiveGels)
		}
	}
	tries := append([]string(nil), sirens...)
	sort.Strings(tries)
	valeurs := make([]any, len(tries))
	for i, s := range tries {
		valeurs[i] = s
	}
	directives = append(directives, map[string]any{
		"id":         IDDirectiveGels,
		"version":    1,
		"statut":     "active",
		"date_effet": dateEffet,
		"texte_fr": "Mise à jour du registre des gels : aucun prêt n'est attribué à une entité " +
			"inscrite au registre national des gels des avoirs lors de sa dernière mise à jour.",
		"fondements": []any{map[string]any{"texte": "Code monétaire et financier — à valider par le juriste", "article": "L562-4"}},
		"declencheur": map[string]any{
			"evenement": "appel_outil",
			"cible":     map[string]any{"serveur": "banque-synthetique", "outil": "attribuer_pret"},
		},
		"condition": map[string]any{"argument": map[string]any{"champ": "/siren", "op": "dans", "valeur": valeurs}},
		"effets":    []any{map[string]any{"effet": "refuser"}},
	})
	delete(meta, "empreinte")
	delete(meta, "signature")
	meta["version"] = version + 1
	jeu["directives"] = directives
	return json.MarshalIndent(jeu, "", "  ")
}

// ---------------------------------------------------------------------------
// Littéraux SQL (standard_conforming_strings, défaut de PostgreSQL)
// ---------------------------------------------------------------------------

// Lit est un littéral chaîne SQL : apostrophes doublées. Les octets NUL,
// refusés par PostgreSQL dans text, sont retirés.
func Lit(s string) string {
	s = strings.ReplaceAll(s, "\x00", "")
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// LitOuNull : NULL pour la chaîne vide.
func LitOuNull(s string) string {
	if s == "" {
		return "NULL"
	}
	return Lit(s)
}

// EntierOuNull : NULL pour zéro (seq et version commencent à 1).
func EntierOuNull(n int) string {
	if n == 0 {
		return "NULL"
	}
	return fmt.Sprintf("%d", n)
}

// Instant : timestamptz en UTC, RFC 3339 ; NULL pour l'instant nul.
func Instant(t time.Time) string {
	if t.IsZero() {
		return "NULL"
	}
	return Lit(t.UTC().Format(time.RFC3339Nano)) + "::timestamptz"
}

// InstantTexte : une date RFC 3339 consignée en texte ; NULL si vide ou
// illisible (le journal n'en garde que de lisibles).
func InstantTexte(s string) string {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return "NULL"
	}
	return Instant(t)
}

// JSONSansNul retire les \u0000 que jsonb refuse.
func JSONSansNul(b []byte) []byte {
	return bytes.ReplaceAll(b, []byte(`\u0000`), nil)
}

func court(session string) string {
	if len(session) > 8 {
		return session[:8]
	}
	return session
}
