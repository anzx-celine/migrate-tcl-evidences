# Xplore Evidence Migration

This code was written for a one-time migration to transfer evidence from the old PAC to the new TCL. It can be run in all environments.

> **Note:** The code may require manual intervention and is intended to run on a local machine.

## How does it work?

The script reads a mapping file (`mapping.xlsx`) containing source and target control IDs and AC numbers. It connects to the database, retrieves evidence from PostgreSQL, and submits it to the new **Control/AC** using the provided mappings via `xplore-api`.

> **Warning:** Evidence must be submitted through the API rather than directly inserted into the database because `control_status_id` is a foreign key in the `evidence` table. The API ensures the control status exists and handles evidence validation.

## Prerequisites

### Connect to Database

1. Download DB dump files and restore using Docker.  
   Save dump files to `dumpfile` for the codex and controlstatus databases, then run commands from `dumpfile/command.md`.
2. Alternatively, connect to the database using [cloud-sql-proxy](https://cloud.google.com/sql/docs/postgres/connect-auth-proxy).  
   If you use this method, update the DB connection string accordingly.

**Example:**

_Staging:_
```shell
./cloud-sql-proxy --auto-iam-authn --private-ip anz-x-xplore-staging-1bbe6e:australia-southeast1:xp-sql-outcomestore-282f07
```
```shell
gcloud auth application-default login --impersonate-service-account=xp-sa-generator-evidence@anz-x-xplore-staging-1bbe6e.iam.gserviceaccount.com
```

_Production:_
```shell
./cloud-sql-proxy --auto-iam-authn --private-ip anz-x-xplore-prod-44f597:australia-southeast1:xp-sql-outcomestore-2469ef
```
```shell
gcloud auth application-default login --impersonate-service-account=xp-sa-generator-evidence@anz-x-xplore-prod-44f597.iam.gserviceaccount.com
```

### Prepare Mapping
AC mappings need to save under the root directory with file name `mapping.xlsx` with the following columns:
  - `SourceControlID` eg. `PAC-001`
  - `SourceACNumber` eg. `1`
  - `TargetControlID` eg. `CTOB001`
  - `TargetControlACNumber` eg. `1`
> Note: no header row is required in the `mapping.xlsx` file.
>

### Generate Token
To generate the identity token for the service account, you can use the following commands. To Impersonate prod service account you will need to raise a ticket to apply for elevated access, add yourself to `MAC Prod group`. Any futher questions please refer to `xplore-infra` to check access.
>!WARNING: token is only valid for 60 minutes, so make sure each batch of assets run is less than 60 minutes.
>
_Staging:_
```shell
gcloud auth print-identity-token --include-email --impersonate-service-account xp-sa-xplore-tclsync@anz-x-xplore-staging-1bbe6e.iam.gserviceaccount.com
```

_Production:_
```shell
gcloud auth print-identity-token --include-email --impersonate-service-account xp-sa-xplore-tclsync@anz-x-xplore-prod-44f597.iam.gserviceaccount.com
```

## Run the script
Once the prerequisites are met, you can run the script using the following command:
1. update [config.go](./config.go) with the correct values based on your environment.
2. run the script
    ```shell
    go run .
    ```
3. check [migration_results.txt](./migration_results.xlsx) for the results of the migration.

## Performance issue
The xplore-api is not optimized for large data volumes. Run the script in batches by adjusting values in config.go.
Consider increasing the controls-api instance count for parallel processing.

## Summary
- Test in a non-production environment before running in production.
- Review and understand the code before execution.
- No future support is provided; use at your own risk.