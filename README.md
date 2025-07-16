# Xplore Evidence Migration
This was a one time migration script to migrate evidence from the old Xplore system to the new one. It is designed to be run in both staging and production environments.

Code is not perfect and may require some manual intervention as this only runs on my local machine.

## How does it work?
The script reads a mapping file `mapping.xlsx` that contains the source and target control IDs and AC numbers. It then connects to the database, retrieves the evidence from postgress, and submits it to the new **Control/AC** using the provided mappings through `xplore-api`.
> !WARNING: why submitting through API rather than insert into db directly.?
> 
> `control_status_id` is a foreign key in the `evidence` table, which means that it must exist in the `control_status` table before inserting evidence. This is why we need to submit through the API, as it will create the control status if it does not exist.
> 
> Addition, the API will also handle the validation of the evidence and ensure that it is submitted correctly.

## Prerequisites
### Connect to Database
1. Download DB dumpfiles and restore using docker
Save dump files to [dumpfile](./dumpfile) for the codex and controlstatus databases, then run commands from [command.md](./dumpfile/command.md).
2. Connect to the database using [cloud-sql-proxy](https://cloud.google.com/sql/docs/postgres/connect-auth-proxy), if choose this method, the db connection string need to be updated as well.

example:

db connection staging
```shell
./cloud-sql-proxy --auto-iam-authn --private-ip anz-x-xplore-staging-1bbe6e:australia-southeast1:xp-sql-outcomestore-282f07
```
```shell
gcloud auth application-default login --impersonate-service-account=xp-sa-generator-evidence@anz-x-xplore-staging-1bbe6e.iam.gserviceaccount.com
```

db connection prod
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
staging token
```shell
gcloud auth print-identity-token --include-email --impersonate-service-account xp-sa-xplore-tclsync@anz-x-xplore-staging-1bbe6e.iam.gserviceaccount.com
```

prod token
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
- xplore-api is not designed to handle large amount of data, so it is recommended to run the script in batches but adjusting the value in [config.go](./config.go).
- you may also need to increase controls-api mini instance number to process these requests in parallel.

## Summary
Please test in non prod env before running in prod.

Please try to understand the code before running it.

No future support is provided, use at your own risk.