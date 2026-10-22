# Detective (Aiven Runtime)

Page en lecture seule sur le journal versé dans PostgreSQL par
`journal-sql` : pour chaque décision d'agent, ce qu'elle a lu, d'où, de
quand, quelles directives ont joué ; pour chaque rejeu (`journal-sql
impacts`), les décisions passées qu'il change.

Le service ne connaît pas le mot de passe de la base : Aiven Runtime injecte
`DATABASE_URL` par l'intégration de service. Il n'écrit rien (transactions
en lecture seule).

## Déployer (console Aiven)

1. Projet → **Runtime** → **Deploy application**.
2. Connecter GitHub, choisir `AguenonN/pythagoras-sec`, branche `hackathon/aiven`.
3. Manifeste : `apps/detective/Dockerfile` (le dépôt en contient d'autres, pour le lab : ne pas les prendre). Contexte de build : `apps/detective`.
4. Port : 8080 (lu dans `EXPOSE`).
5. Intégration : le service PostgreSQL existant → cette application, variable **`DATABASE_URL`**.
6. **Deploy**, puis ouvrir l'URL publique : `/` et `/healthz`.

Un push ne redéploie pas : *Overview* → *Actions* → *Deploy latest commit*.

## En local

```bash
pip install -r requirements.txt
DATABASE_URL="$AIVEN_PG_URI" PORT=8080 python app.py    # http://127.0.0.1:8080
```
