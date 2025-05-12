```bash
docker run --name db-staging \
    -e POSTGRES_USER=myuser \
    -e POSTGRES_PASSWORD=mypassword \
    -e POSTGRES_DB=codex \
    -p 5432:5432 \
    -d postgres
```

```bash
#docker exec -it db-staging psql -U myuser -d codex -c "CREATE DATABASE codex;" && \
docker cp codex.sql db-staging:/codex.sql && \
docker exec -i db-staging psql -U myuser -d codex -f /codex.sql
```

```bash
docker exec -it db-staging psql -U myuser -d codex -c "CREATE DATABASE controlstatus;" && \
docker cp controlstatus.sql db-staging:/controlstatus.sql && \
docker exec -i db-staging psql -U myuser -d controlstatus -f /controlstatus.sql
```

```bash
docker exec -it db-staging psql -U myuser -d codex -c "\l"
```

```bash
docker exec -it db-staging psql -U myuser -d postgres -c "DROP DATABASE codex;"
```