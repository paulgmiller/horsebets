# Horsebets

Run `go run .` to use the local SQLite file `bets.db`. Set `SQLLITE_FILE` to
choose another file. When `DATABASE_URL` is set, the app uses that CockroachDB
PostgreSQL URL instead. SQL Server is no longer used.

The Kubernetes deployment sets `DATABASE_URL` to
`postgresql://root@cockroachdb-public.cockroach.svc.cluster.local:26257/horsebets?sslmode=disable`.
The PostgreSQL driver uses pgx simple protocol, matching the CockroachDB
metadata-query workaround in aggrovites.

Create the `horsebets` database before starting the app. GORM creates or updates
the `races`, `horses`, and `bets` tables on startup. Existing SQLite or SQL Server
data is not copied automatically.

Run `go test ./...` to check the app. Build and push a new container image,
update the image tag in `deploy.yaml`, and apply the deployment to switch the
cluster app to CockroachDB. The Kubernetes DNS name is reachable inside the
cluster; use a reachable database URL when running outside it.
