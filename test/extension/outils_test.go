// Package extension prouve que le kit d'extension (docs/demo/etendre.md) tient
// ses promesses : un serveur synthétique, un sous-agent, un scénario et une
// directive ajoutés par des fichiers et des gabarits, sans toucher à
// l'engine, passent devant le gate.
//
// Les tests travaillent sur une COPIE du dépôt (jamais sur le dépôt) et
// exécutent à la lettre les blocs de commandes de docs/demo/etendre.md.
// Aucun réseau, aucun appel Mistral.
package extension

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// racine est la racine du dépôt qui contient ce paquet de test.
func racine(t testing.TB) string {
	t.Helper()
	r, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// copierDepot copie les fichiers suivis (et non ignorés) du dépôt dans un
// répertoire temporaire, bits d'exécution compris : la copie est un dépôt
// neuf, tel qu'une équipe le récupère.
func copierDepot(t testing.TB) string {
	t.Helper()
	src := racine(t)
	cmd := exec.Command("git", "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	cmd.Dir = src
	liste, err := cmd.Output()
	if err != nil {
		t.Fatalf("git ls-files : %v", err)
	}
	dst := t.TempDir()
	for _, f := range strings.Split(string(liste), "\x00") {
		if f == "" {
			continue
		}
		info, err := os.Lstat(filepath.Join(src, f))
		if err != nil {
			continue // supprimé de l'arbre de travail, pas encore de l'index
		}
		cible := filepath.Join(dst, f)
		if err := os.MkdirAll(filepath.Dir(cible), 0o755); err != nil {
			t.Fatal(err)
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			lien, _ := os.Readlink(filepath.Join(src, f))
			if err := os.Symlink(lien, cible); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if !info.Mode().IsRegular() {
			continue
		}
		in, err := os.Open(filepath.Join(src, f))
		if err != nil {
			t.Fatal(err)
		}
		out, err := os.OpenFile(cible, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(out, in); err != nil {
			t.Fatal(err)
		}
		in.Close()
		out.Close()
	}
	return dst
}

// environnement : celui du test, sans aucune clé d'API, plus extra.
func environnement(extra ...string) []string {
	var env []string
	for _, v := range os.Environ() {
		if strings.HasPrefix(v, "MISTRAL_API_KEY=") || strings.HasPrefix(v, "PISTE_") || strings.HasPrefix(v, "MCP_GATE_") {
			continue
		}
		env = append(env, v)
	}
	return append(env, extra...)
}

// bash exécute un script dans dir (bash strict) et rend sa sortie combinée ;
// il échoue le test si le script échoue.
func bash(t testing.TB, dir, script string, extra ...string) string {
	t.Helper()
	sortie, err := essayer(dir, script, extra...)
	if err != nil {
		t.Fatalf("échec de :\n%s\n%v\n%s", script, err, sortie)
	}
	return sortie
}

// essayer est bash sans échec automatique.
func essayer(dir, script string, extra ...string) (string, error) {
	cmd := exec.Command("bash", "-euo", "pipefail", "-c", script)
	cmd.Dir = dir
	cmd.Env = environnement(append([]string{"GOFLAGS=-mod=readonly", "GOTOOLCHAIN=local"}, extra...)...)
	var sortie bytes.Buffer
	cmd.Stdout, cmd.Stderr = &sortie, &sortie
	err := cmd.Run()
	return sortie.String(), err
}

var reBloc = regexp.MustCompile("(?s)```sh ([a-z0-9-]+)\n(.*?)```")

// blocs rend les blocs de commandes de docs/demo/etendre.md, par étiquette :
// ```sh recette-serveur … ```. C'est le texte que l'équipe suivra : le test
// l'exécute tel quel.
func blocs(t testing.TB, dir string) map[string]string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "docs", "demo", "etendre.md"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, m := range reBloc.FindAllStringSubmatch(string(b), -1) {
		if _, dup := out[m[1]]; dup {
			t.Fatalf("étiquette de bloc %q en double dans etendre.md", m[1])
		}
		out[m[1]] = m[2]
	}
	return out
}

func lire(t testing.TB, chemin string) string {
	t.Helper()
	b, err := os.ReadFile(chemin)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// bashCodes est bash qui admet certains codes de sortie (le lanceur rend 2
// quand la demande est bloquée par l'engine, ce que certains scénarios
// attendent).
func bashCodes(t testing.TB, dir, script string, codes []int, extra ...string) string {
	t.Helper()
	sortie, err := essayer(dir, script, extra...)
	if err == nil {
		return sortie
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		for _, c := range codes {
			if ee.ExitCode() == c {
				return sortie
			}
		}
	}
	t.Fatalf("échec de :\n%s\n%v\n%s", script, err, sortie)
	return ""
}
