```bash
docker run --name db-prod \
    -e POSTGRES_USER=myuser \
    -e POSTGRES_PASSWORD=mypassword \
    -e POSTGRES_DB=codex \
    -p 5432:5432 \
    -d postgres
```

```bash
#docker exec -it db-prod psql -U myuser -d controlstatus -c "CREATE DATABASE codex;" && \
docker cp codex.sql db-prod:/codex.sql && \
docker exec -i db-prod psql -U myuser -d codex -f /codex.sql
```

```bash
docker exec -it db-prod psql -U myuser -d codex -c "CREATE DATABASE controlstatus;" && \
docker cp controlstatus.sql db-prod:/controlstatus.sql && \
docker exec -i db-prod psql -U myuser -d controlstatus -f /controlstatus.sql
```

```bash
docker exec -it db-prod psql -U myuser -d codex -c "CREATE DATABASE outcomestore;" && \
docker cp outcomestore.sql db-prod:/outcomestore.sql && \
docker exec -i db-prod psql -U myuser -d outcomestore -f /outcomestore.sql
```

```bash
docker exec -it db-prod psql -U myuser -d codex -c "\l"
```

```bash
docker exec -it db-prod psql -U myuser -d postgres -c "DROP DATABASE codex;"
```