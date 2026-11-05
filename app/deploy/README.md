# Local demo

Run `make up` from the repository root and open http://localhost:8080.
`FEATUREBAN_PORT=8081 make up` selects another local port. `make down` stops the
containers and keeps the database volume. Removing the volume deletes saved games.

`deploy.sh` starts the current checkout. It does not fetch/reset Git or contact the
original team's VPS. Public hosting and TLS must be configured separately.

The upstream history contains a filled deployment credential file. It is removed
from this fork's current version. If those credentials are still active, the VPS
owner must rotate them; deleting a file does not remove it from Git history.
