# Maquettes validées de la console (référence visuelle)

Six écrans, validés par Manu. Ils font foi pour la disposition, les libellés, les couleurs, la typographie et les badges.

| Fichier | Écran |
|---|---|
| Main.dc.html | 1 · Journal et graphe de chaîne |
| Conseiller.dc.html | 2 · Poste conseiller |
| Regles.dc.html | 3 · Vitrine des règles |
| AvantApres.dc.html | 4 · Avant / après la directive du juriste |
| Superviseur.dc.html | 5 · File du superviseur |
| Redaction.dc.html | 6 · Juriste — rédaction (deux onglets : écrire / propositions de l'Analyste) |

## Comment les lire

- Ce sont des fichiers d'un outil de maquettage. Le contenu utile est le HTML entre `<x-dc>` et `</x-dc>` : du HTML ordinaire avec des styles en ligne. Ignore `<helmet>`, `support.js` et le bloc `<script type="text/x-dc">`.
- Les données affichées (sessions, montants, D-101…, horodatages) sont **d'exemple**. La console affiche les vraies données des journaux et de `demo/directives/demo.json` ; les identifiants réels sont D-001 à D-010 (contrats).
- Les valeurs entre crochets (`[x] ms`, `[n]`, `[Nom du produit]`) sont des emplacements.

## Système visuel à reprendre tel quel

- Fond `#F6F5F1`, surfaces `#FFFFFF`, traits `#DAD7CE`, texte `#17191C`, texte secondaire `#5B6068`, accent `#1D4F7A`.
- Badges (fond / texte) : autorisé `#E4F1E8`/`#1E6B3C` ; caviardé `#FBF0D9`/`#7A5200` ; en attente `#E3ECF7`/`#1D4F7A` ; refusé `#FBE4E1`/`#A1261B` ; indisponible `#ECECEA`/`#4A4E55` ; source non attestée `#F1E6F3`/`#6E2A7A`.
- Chaîne rompue : fond `#FBE4E1`, bord `#A1261B`.
- Polices : Source Serif 4 (titres), IBM Plex Sans (texte), IBM Plex Mono (empreintes, horodatages). **Aucun CDN** : les maquettes chargent Google Fonts, la console doit embarquer les fichiers de police (paquets npm `@fontsource/source-serif-4`, `@fontsource/ibm-plex-sans`, `@fontsource/ibm-plex-mono`, licence OFL, récupérés depuis le registre npm, fichiers woff2 copiés dans les ressources embarquées). À défaut, pile système équivalente, signalée.
- Vocabulaire : jamais « v1 / v2 » ni « jeu » à l'écran ; « règles en vigueur depuis le … », « sans la directive D-00n » / « avec la directive D-00n ».
