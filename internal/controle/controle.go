// Package controle tient les contrôles que l'engine applique après
// Decide, sans directive : aujourd'hui, celui du registre. L'engine et le
// rejeu appellent la même fonction, pour que ce qu'affirme le journal se
// recalcule à l'identique.
package controle

import "mcprogue/internal/directive"

// OrigineRegistre marque, dans la décision journalisée, un effet imposé
// par le contrôle du registre et non par une directive.
const OrigineRegistre = "registre"

// Registre applique le contrôle du registre, toujours actif : un appel
// vers un serveur dont l'attestation n'est pas valide est refusé, même si
// aucune directive source_attestee ne le cible. Une décision déjà refusée
// par une directive reste celle de la directive, qui cite son fondement.
// Rend la décision finale et true si le registre l'a imposée.
//
// La fonction est pure : l'attestation est celle que le journal porte, et
// Valide la recalcule, donc le rejeu retrouve le même effet et le même
// motif.
func Registre(dec directive.Decision, serveur string, att directive.Attestation) (directive.Decision, bool) {
	if dec.Effet == directive.Refuser || att.Valide() {
		return dec, false
	}
	dec.Effet = directive.Refuser
	dec.Motif = "refusé par le registre : le serveur " + serveur + " n'est pas attesté (" + decrire(att) + ")"
	return dec, true
}

// decrire résume une attestation en échec ; son détail ne contient jamais
// de texte venu du serveur (voir internal/backend).
func decrire(a directive.Attestation) string {
	if a.Detail != "" {
		return a.Detail
	}
	return a.Mode + " attendu " + a.Attendu + ", observé " + a.Observe
}
