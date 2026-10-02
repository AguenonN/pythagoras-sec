package console

import "embed"

// Les fichiers de l'interface sont embarqués : aucune ressource n'est
// chargée depuis le réseau. Avec l'option -dev, ils sont servis depuis le
// disque (voir Serveur.Dev).
//
//go:embed web
var webFS embed.FS
