package console

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Serveur est le serveur HTTP de la console. Il lit les journaux et les
// règles ; il n'écrit que ce que la liste de Depot permet (règles, origines,
// approbations, décisions sur les propositions), sur action explicite.
type Serveur struct {
	Depot *Depot
	// Produit est le nom affiché dans l'en-tête (la maquette le laisse en
	// emplacement) ; vide, « Console de gouvernance ».
	Produit string
	// Dev : répertoire des fichiers HTML, CSS et JS servis depuis le disque
	// (rechargement sans recompiler) ; vide, ils sont servis depuis le
	// binaire.
	Dev string

	lanceur Lanceur
}

// Handler assemble les routes. Toutes les requêtes doivent viser 127.0.0.1 ou
// localhost : un nom de domaine qui résoudrait vers la machine (rebinding)
// est refusé. Une écriture exige un corps JSON et, si le navigateur envoie
// une origine, que ce soit la console elle-même.
func (s *Serveur) Handler() http.Handler {
	mux := http.NewServeMux()
	var statiques fs.FS
	if s.Dev != "" {
		statiques = dirFS(s.Dev)
	} else {
		statiques, _ = fs.Sub(webFS, "web")
	}
	mux.Handle("GET /", http.FileServerFS(statiques))
	mux.HandleFunc("GET /api/etat", s.etat)
	mux.HandleFunc("GET /api/journal/{id}/session/{sid}/chaine", s.chaine)
	mux.HandleFunc("GET /api/journal/{id}/session/{sid}/poste", s.poste)
	mux.HandleFunc("GET /api/journal/{id}/chaine", s.chaine)
	mux.HandleFunc("GET /api/journal/{id}/poste", s.poste)
	mux.HandleFunc("GET /api/identites", s.identites)
	mux.HandleFunc("POST /api/lancer", s.lancer)
	mux.HandleFunc("GET /api/lancements", s.lancements)
	mux.HandleFunc("GET /api/regles", s.regles)
	mux.HandleFunc("POST /api/regles/statut", s.statut)
	mux.HandleFunc("GET /api/avant-apres", s.avantApres)
	mux.HandleFunc("GET /api/file", s.file)
	mux.HandleFunc("POST /api/validation/valider", s.valider)
	mux.HandleFunc("POST /api/validation/rejeter", s.rejeter)
	mux.HandleFunc("GET /api/notifications", s.notifications)
	mux.HandleFunc("GET /api/redaction", s.redaction)
	mux.HandleFunc("POST /api/redaction/compiler", s.compiler)
	mux.HandleFunc("POST /api/redaction/essai", s.essai)
	mux.HandleFunc("POST /api/redaction/mettre-en-vigueur", s.mettreEnVigueur)
	mux.HandleFunc("POST /api/propositions/{id}/decision", s.decision)
	return gardes(mux)
}

func dirFS(dir string) fs.FS { return os.DirFS(dir) }

func gardes(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host
		}
		if host != "127.0.0.1" && host != "localhost" && host != "[::1]" && host != "::1" {
			http.Error(w, "hôte refusé", http.StatusForbidden)
			return
		}
		hd := w.Header()
		hd.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'none'; form-action 'none'")
		hd.Set("X-Content-Type-Options", "nosniff")
		hd.Set("Cache-Control", "no-store")
		if r.Method == http.MethodPost {
			if ct, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); ct != "application/json" {
				erreur(w, http.StatusUnsupportedMediaType, "corps JSON attendu")
				return
			}
			if o := r.Header.Get("Origin"); o != "" {
				u, err := url.Parse(o)
				if err != nil || u.Host != r.Host {
					erreur(w, http.StatusForbidden, "origine refusée")
					return
				}
			}
		}
		h.ServeHTTP(w, r)
	})
}

func ecrire(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	_ = enc.Encode(v)
}

func erreur(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"erreur": msg})
}

// echec rend l'erreur d'une action : le message français, avec son code.
func echec(w http.ResponseWriter, err error) {
	var ea *ErreurAction
	if errors.As(err, &ea) {
		erreur(w, ea.Code, ea.Message)
		return
	}
	erreur(w, http.StatusInternalServerError, err.Error())
}

func lireCorps(w http.ResponseWriter, r *http.Request, v any) bool {
	corps, err := io.ReadAll(io.LimitReader(r.Body, 1<<20+1))
	if err != nil || len(corps) > 1<<20 {
		erreur(w, http.StatusRequestEntityTooLarge, "corps trop long")
		return false
	}
	dec := json.NewDecoder(strings.NewReader(string(corps)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		erreur(w, http.StatusBadRequest, "requête illisible")
		return false
	}
	return true
}

type jeuListe struct {
	Index      int       `json:"index"`
	Libelle    string    `json:"libelle"`
	Empreinte  string    `json:"empreinte"`
	Source     string    `json:"source"`
	Directives int       `json:"directives"`
	Version    int       `json:"version"`
	Courant    bool      `json:"courant"`
	DateEffet  time.Time `json:"date_effet"`
}

type journalListe struct {
	ID       string          `json:"id"`
	Verif    Verification    `json:"verification"`
	Entrees  int             `json:"entrees"`
	Ecart    int             `json:"premier_ecart"`
	Sessions []SessionResume `json:"sessions"`
	Debut    *time.Time      `json:"debut,omitempty"`  // etat.json, sinon première session
	Statut   string          `json:"statut,omitempty"` // etat.json
	Demande  string          `json:"demande,omitempty"`
}

func (s *Serveur) etat(w http.ResponseWriter, r *http.Request) {
	out := struct {
		Journaux     []journalListe `json:"journaux"`
		Jeux         []jeuListe     `json:"jeux"`
		Refus        []string       `json:"jeux_refuses"`
		Cle          bool           `json:"cle_presente"`
		Regles       *Regles        `json:"regles,omitempty"`
		Produit      string         `json:"produit,omitempty"`
		Juriste      string         `json:"juriste"`
		JuristeNom   string         `json:"juriste_nom"`
		Attentes     int            `json:"attentes"`
		Propositions int            `json:"propositions"`
	}{Journaux: []journalListe{}, Jeux: []jeuListe{}, Cle: len(s.Depot.Cle) > 0}
	for _, st := range s.tousEtats() {
		jl := journalListe{ID: st.Journal.ID, Verif: st.Verif, Entrees: len(st.Entrees), Ecart: st.Verif.PremierEcart(), Sessions: Sessions(st)}
		if len(jl.Sessions) > 0 {
			t := jl.Sessions[0].Ouverte
			jl.Debut = &t
		}
		if st.Dossier != nil {
			if st.Dossier.Debut != nil {
				jl.Debut = st.Dossier.Debut
			}
			jl.Statut, jl.Demande = st.Dossier.Statut, st.Dossier.Demande
			if st.Dossier.Statut == StatutEnAttente {
				out.Attentes++
			}
		}
		out.Journaux = append(out.Journaux, jl)
	}
	jeux, refus := s.Depot.ChargerJeux()
	for i, j := range jeux {
		out.Jeux = append(out.Jeux, jeuListe{Index: i, Libelle: j.Libelle(), Empreinte: j.Jeu.Meta.Empreinte, Source: j.Source,
			Directives: len(j.Jeu.Directives), Version: j.Jeu.Meta.Version, Courant: j.Courant, DateEffet: j.Jeu.Meta.DateEffet})
	}
	out.Produit = s.Produit
	out.Juriste, out.JuristeNom = s.Depot.Juriste, s.Depot.NomIdentite(s.Depot.Juriste)
	for _, p := range s.Depot.ListerPropositions() {
		if p.Decision == nil {
			out.Propositions++
		}
	}
	out.Refus = append([]string{}, refus...)
	if jc := EnVigueur(jeux); jc != nil {
		out.Regles = reglesDe(&jc.Jeu)
	}
	ecrire(w, out)
}

func (s *Serveur) ouvrir(w http.ResponseWriter, id string) (*Etat, bool) {
	j, ok := s.Depot.Journal(id)
	if !ok {
		erreur(w, http.StatusNotFound, "journal inconnu")
		return nil, false
	}
	st, err := s.Depot.Lire(j)
	if err != nil {
		erreur(w, http.StatusInternalServerError, "journal illisible : "+err.Error())
		return nil, false
	}
	return st, true
}

func (s *Serveur) chaine(w http.ResponseWriter, r *http.Request) {
	st, ok := s.ouvrir(w, r.PathValue("id"))
	if !ok {
		return
	}
	jeux, _ := s.Depot.ChargerJeux()
	v, ok := s.Depot.Chaine(st, jeux, r.PathValue("sid"))
	if !ok {
		erreur(w, http.StatusNotFound, "session inconnue")
		return
	}
	ecrire(w, v)
}

func (s *Serveur) poste(w http.ResponseWriter, r *http.Request) {
	st, ok := s.ouvrir(w, r.PathValue("id"))
	if !ok {
		return
	}
	jeux, _ := s.Depot.ChargerJeux()
	v, ok := s.Depot.Poste(st, jeux, r.PathValue("sid"))
	if !ok {
		erreur(w, http.StatusNotFound, "session inconnue")
		return
	}
	ecrire(w, v)
}

func (s *Serveur) tousEtats() []*Etat {
	var out []*Etat
	for _, j := range s.Depot.Journaux() {
		if st, err := s.Depot.Lire(j); err == nil {
			out = append(out, st)
		}
	}
	return out
}

func (s *Serveur) identites(w http.ResponseWriter, r *http.Request) {
	_, doc, err := s.Depot.Annuaire()
	out := struct {
		Identites []FicheDemandeur `json:"identites"`
		Erreur    string           `json:"erreur,omitempty"`
	}{Identites: []FicheDemandeur{}}
	if err != nil {
		out.Erreur = err.Error()
	} else {
		now := time.Now()
		for _, i := range doc.Identites {
			if f, ok := s.Depot.Fiche(i.Identite, now); ok {
				out.Identites = append(out.Identites, f)
			}
		}
	}
	ecrire(w, out)
}

func (s *Serveur) lancer(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Demande   string `json:"demande"`
		Demandeur string `json:"demandeur"`
	}
	if !lireCorps(w, r, &in) {
		return
	}
	l, err := s.Depot.Lancer(&s.lanceur, in.Demande, in.Demandeur)
	if err != nil {
		echec(w, err)
		return
	}
	ecrire(w, l)
}

func (s *Serveur) lancements(w http.ResponseWriter, r *http.Request) {
	ecrire(w, map[string]any{"lancements": s.lanceur.Liste()})
}

func (s *Serveur) regles(w http.ResponseWriter, r *http.Request) {
	jeux, _ := s.Depot.ChargerJeux()
	jc := EnVigueur(jeux)
	if jc == nil {
		erreur(w, http.StatusNotFound, "aucune directive disponible")
		return
	}
	ecrire(w, s.Depot.ReglesEnVigueur(*jc, s.tousEtats(), jeux, r.URL.Query().Get("directive")))
}

func (s *Serveur) statut(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Directive string `json:"directive"`
		Statut    string `json:"statut"`
	}
	if !lireCorps(w, r, &in) {
		return
	}
	res, err := s.Depot.ChangerStatut(in.Directive, in.Statut)
	if err != nil {
		echec(w, err)
		return
	}
	ecrire(w, res)
}

func (s *Serveur) avantApres(w http.ResponseWriter, r *http.Request) {
	jeux, _ := s.Depot.ChargerJeux()
	choisies := map[string]bool{}
	if q := r.URL.Query().Get("sessions"); q != "" {
		for _, x := range strings.Split(q, ",") {
			choisies[x] = true
		}
	}
	ecrire(w, s.Depot.AvantApres(s.tousEtats(), jeux, r.URL.Query().Get("directive"), choisies))
}

func (s *Serveur) file(w http.ResponseWriter, r *http.Request) {
	jeux, _ := s.Depot.ChargerJeux()
	ecrire(w, s.Depot.File(s.tousEtats(), jeux, time.Now()))
}

func (s *Serveur) valider(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Journal  string `json:"journal"`
		Valideur string `json:"valideur"`
		Role     string `json:"role"`
	}
	if !lireCorps(w, r, &in) {
		return
	}
	a, err := s.Depot.Valider(in.Journal, in.Valideur, in.Role)
	if err != nil {
		echec(w, err)
		return
	}
	ecrire(w, a)
}

func (s *Serveur) rejeter(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Journal  string `json:"journal"`
		Valideur string `json:"valideur"`
		Motif    string `json:"motif"`
	}
	if !lireCorps(w, r, &in) {
		return
	}
	rj, err := s.Depot.Rejeter(in.Journal, in.Valideur, in.Motif)
	if err != nil {
		echec(w, err)
		return
	}
	ecrire(w, rj)
}

func (s *Serveur) notifications(w http.ResponseWriter, r *http.Request) {
	jeux, _ := s.Depot.ChargerJeux()
	ecrire(w, map[string]any{"notifications": Notifications(s.tousEtats(), jeux)})
}

func (s *Serveur) redaction(w http.ResponseWriter, r *http.Request) {
	jeux, refus := s.Depot.ChargerJeux()
	out := struct {
		Propositions []PropositionVue  `json:"propositions"`
		Juriste      string            `json:"juriste"`
		JuristeNom   string            `json:"juriste_nom"`
		Regles       *VueRegles        `json:"regles,omitempty"`
		Origines     map[string]string `json:"origines"`
		Refus        []string          `json:"jeux_refuses"`
	}{Propositions: s.Depot.ListerPropositions(), Juriste: s.Depot.Juriste, JuristeNom: s.Depot.NomIdentite(s.Depot.Juriste),
		Origines: map[string]string{}, Refus: append([]string{}, refus...)}
	if jc := EnVigueur(jeux); jc != nil {
		out.Regles = s.Depot.ReglesEnVigueur(*jc, nil, jeux, "")
		act, orig := Activations(jeux), s.Depot.LireOrigines()
		for _, dd := range jc.Jeu.Directives {
			out.Origines[dd.ID] = s.Depot.LibelleOrigine(dd.ID, act[dd.ID].Auteur, orig)
		}
	}
	ecrire(w, out)
}

func (s *Serveur) compiler(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Texte        string            `json:"texte"`
		Fondement    string            `json:"fondement"`
		Conversation string            `json:"conversation"`
		Reponses     []ReponseQuestion `json:"reponses"`
		Proposition  string            `json:"proposition"`
	}
	if !lireCorps(w, r, &in) {
		return
	}
	c, err := s.Depot.Compiler(in.Texte, in.Fondement, in.Conversation, in.Reponses, in.Proposition)
	if err != nil {
		echec(w, err)
		return
	}
	ecrire(w, c)
}

func (s *Serveur) essai(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Conversation string `json:"conversation"`
	}
	if !lireCorps(w, r, &in) {
		return
	}
	jeux, _ := s.Depot.ChargerJeux()
	e, err := s.Depot.EssaiSurJournaux(in.Conversation, s.tousEtats(), jeux)
	if err != nil {
		echec(w, err)
		return
	}
	ecrire(w, e)
}

func (s *Serveur) mettreEnVigueur(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Conversation string `json:"conversation"`
		Statut       string `json:"statut"`
	}
	if !lireCorps(w, r, &in) {
		return
	}
	res, err := s.Depot.MettreEnVigueur(in.Conversation, in.Statut)
	if err != nil {
		echec(w, err)
		return
	}
	ecrire(w, res)
}

// decision enregistre la décision du juriste sur une proposition. Une
// reprise « telle quelle » lance aussitôt la compilation du texte proposé ;
// une reprise « modifiée » rend le texte à modifier, qui sera compilé à
// l'action suivante du juriste.
func (s *Serveur) decision(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Decision string `json:"decision"`
		Motif    string `json:"motif"`
		Texte    string `json:"texte"`
	}
	if !lireCorps(w, r, &in) {
		return
	}
	id := r.PathValue("id")
	p, err := s.Depot.LireProposition(id)
	if err != nil {
		erreur(w, http.StatusNotFound, "proposition inconnue")
		return
	}
	texte := in.Texte
	if in.Decision == "reprise_telle_quelle" || strings.TrimSpace(texte) == "" {
		texte = p.TextePropose
	}
	dp := DecisionProposition{Decision: in.Decision, Motif: strings.TrimSpace(in.Motif), TexteRetenu: texte,
		Auteur: s.Depot.Juriste, Date: time.Now().UTC()}
	if in.Decision == "rejetee" {
		dp.TexteRetenu = ""
	}
	if err := s.Depot.EcrireDecision(id, dp); err != nil {
		erreur(w, http.StatusBadRequest, err.Error())
		return
	}
	out := struct {
		Decision    DecisionProposition `json:"decision"`
		Compilation *Compilation        `json:"compilation,omitempty"`
		Erreur      string              `json:"erreur_compilation,omitempty"`
	}{Decision: dp}
	if in.Decision == "reprise_telle_quelle" {
		c, err := s.Depot.Compiler(p.TextePropose, p.FondementPropose, "", nil, id)
		if err != nil {
			out.Erreur = err.Error()
		} else {
			out.Compilation = c
		}
	}
	ecrire(w, out)
}
