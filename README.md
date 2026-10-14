# Horsebets

Run `go run .` to use the local SQLite file `bets.db`. Set `SQLLITE_FILE` to
choose another file. When `DATABASE_URL` is set, the app uses that CockroachDB
PostgreSQL URL instead. SQL Server is no longer used.

The Kubernetes deployment reads `DATABASE_URL` from key `DATABASE_URL` in
Secret `cockroachdb-connection` in namespace `horsebets`. The Secret contains:

```text
postgresql://root:PASSWORD@cockroachdb-secure-public.cockroach.svc.cluster.local:26257/horsebets?sslmode=require
```

The Secret was provisioned during the secure database migration in `zebu`.
The generated root password is saved in `cockroach/cockroachdb-root` and reused
by subsequent database deployments. Keep passwords out of Git. The application
needs no certificate mounts; `sslmode=require` encrypts the connection but does
not verify the database server's identity.

The PostgreSQL driver uses pgx simple protocol, matching the CockroachDB
metadata-query workaround in aggrovites.

Create the `horsebets` database before starting the app. GORM creates or updates
the `races`, `horses`, and `bets` tables on startup.

Run `go test ./...` to check the app. Build and push a new container image,
update the image tag in `deploy.yaml`, and apply the deployment to switch the
cluster app to CockroachDB. The Kubernetes DNS name is reachable inside the
cluster; use a reachable database URL when running outside it.
