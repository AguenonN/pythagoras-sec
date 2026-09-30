package directive

import (
	"embed"
	"sort"
	"sync"
	"time"
)

// Les fuseaux viennent d'une copie figée de la base IANA, versionnée dans le
// dépôt (fuseaux/VERSION), jamais du système ni de TZ : une directive temps
// rend la même décision sur toute machine, aujourd'hui et au rejeu.
//
//go:embed fuseaux
var baseFuseaux embed.FS

// fuseauxAdmis est la liste fermée des fuseaux qu'une directive peut citer.
var fuseauxAdmis = []string{
	"America/Cayenne", "America/Guadeloupe", "America/Martinique", "America/New_York",
	"Asia/Singapore", "Asia/Tokyo",
	"Europe/Amsterdam", "Europe/Berlin", "Europe/Brussels", "Europe/Dublin",
	"Europe/London", "Europe/Luxembourg", "Europe/Madrid", "Europe/Paris",
	"Europe/Rome", "Europe/Zurich",
	"Indian/Mayotte", "Indian/Reunion",
	"Pacific/Noumea", "Pacific/Tahiti",
	"UTC",
}

var (
	fuseauxMu sync.Mutex
	fuseauxCh = map[string]*time.Location{}
)

// Fuseaux rend, triée, la liste des fuseaux admis.
func Fuseaux() []string {
	out := append([]string(nil), fuseauxAdmis...)
	sort.Strings(out)
	return out
}

// Fuseau charge un fuseau admis depuis la base embarquée.
func Fuseau(nom string) (*time.Location, bool) {
	i := sort.SearchStrings(fuseauxAdmis, nom)
	if i >= len(fuseauxAdmis) || fuseauxAdmis[i] != nom {
		return nil, false
	}
	fuseauxMu.Lock()
	defer fuseauxMu.Unlock()
	if l, ok := fuseauxCh[nom]; ok {
		return l, true
	}
	b, err := baseFuseaux.ReadFile("fuseaux/" + nom)
	if err != nil {
		return nil, false
	}
	l, err := time.LoadLocationFromTZData(nom, b)
	if err != nil {
		return nil, false
	}
	fuseauxCh[nom] = l
	return l, true
}
