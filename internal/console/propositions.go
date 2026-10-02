package console

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// reIdSur : un identifiant de proposition ou de conversation n'est jamais
// un chemin. Il ne sert à construire un nom de fichier qu'après ce contrôle.
var reIdSur = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// Constat est ce que l'Analyste a relevé dans les exécutions passées.
type Constat struct {
	Type   string `json:"type"`
	Detail string `json:"detail"`
}

// Proposition est écrite par l'Analyste seul (contrats §6).
type Proposition struct {
	ID               string    `json:"id"`
	Date             time.Time `json:"date"`
	Constat          Constat   `json:"constat"`
	Sessions         []string  `json:"sessions"`
	TextePropose     string    `json:"texte_propose"`
	FondementPropose string    `json:"fondement_propose"`
	Explication      string    `json:"explication"`
}

// DecisionProposition est écrite par la console seule, à l'action du juriste.
type DecisionProposition struct {
	Decision                string    `json:"decision"` // rejetee | reprise_modifiee | reprise_telle_quelle
	Motif                   string    `json:"motif"`
	TexteRetenu             string    `json:"texte_retenu"`
	Auteur                  string    `json:"auteur"`
	Date                    time.Time `json:"date"`
	ConversationCompilateur string    `json:"conversation_compilateur"`
}

// LireProposition lit propositions/<id>.json.
func (d *Depot) LireProposition(id string) (*Proposition, error) {
	if !reIdSur.MatchString(id) || strings.HasSuffix(id, ".decision") {
		return nil, errors.New("identifiant de proposition invalide")
	}
	b, err := os.ReadFile(filepath.Join(d.Chemins().Propositions, id+".json"))
	if err != nil {
		return nil, err
	}
	var p Proposition
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, fmt.Errorf("proposition %s illisible : %v", id, err)
	}
	if p.ID == "" {
		p.ID = id
	}
	return &p, nil
}

// LireDecision lit propositions/<id>.decision.json (nil : pas de décision).
func (d *Depot) LireDecision(id string) *DecisionProposition {
	if !reIdSur.MatchString(id) {
		return nil
	}
	b, err := os.ReadFile(filepath.Join(d.Chemins().Propositions, id+".decision.json"))
	if err != nil {
		return nil
	}
	var dp DecisionProposition
	if json.Unmarshal(b, &dp) != nil {
		return nil
	}
	return &dp
}

// PropositionVue est une proposition avec la décision du juriste, si elle existe.
type PropositionVue struct {
	Proposition
	Decision *DecisionProposition `json:"decision,omitempty"`
}

// ListerPropositions rend toutes les propositions, les plus récentes
// d'abord. Une proposition illisible est ignorée (en cours d'écriture).
func (d *Depot) ListerPropositions() []PropositionVue {
	ents, _ := os.ReadDir(d.Chemins().Propositions)
	out := []PropositionVue{}
	for _, e := range ents {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".json") || strings.HasSuffix(n, ".decision.json") {
			continue
		}
		p, err := d.LireProposition(strings.TrimSuffix(n, ".json"))
		if err != nil {
			continue
		}
		out = append(out, PropositionVue{Proposition: *p, Decision: d.LireDecision(p.ID)})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Date.After(out[j].Date) })
	return out
}

// EcrireDecision écrit propositions/<id>.decision.json. Un rejet exige un
// motif ; une proposition déjà décidée n'est pas redécidée en silence : la
// décision existante est écrasée seulement par une reprise après un rejet ou
// une autre reprise, jamais sans trace (l'écriture est atomique).
func (d *Depot) EcrireDecision(id string, dp DecisionProposition) error {
	if _, err := d.LireProposition(id); err != nil {
		return fmt.Errorf("proposition %q introuvable", id)
	}
	switch dp.Decision {
	case "rejetee":
		if strings.TrimSpace(dp.Motif) == "" {
			return errors.New("un rejet exige un motif")
		}
	case "reprise_modifiee", "reprise_telle_quelle":
		if strings.TrimSpace(dp.TexteRetenu) == "" {
			return errors.New("le texte retenu est vide")
		}
	default:
		return fmt.Errorf("décision %q inconnue", dp.Decision)
	}
	b, err := json.MarshalIndent(dp, "", "  ")
	if err != nil {
		return err
	}
	ecritureMu.Lock()
	defer ecritureMu.Unlock()
	return ecrireAtomique(filepath.Join(d.Chemins().Propositions, id+".decision.json"), append(b, '\n'), 0o644)
}
