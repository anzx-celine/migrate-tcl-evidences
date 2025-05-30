# Xplore Evidence Migration
# ===========================
This is a one time migration script to migrate evidence from the old Xplore system to the new one. It is designed to be run in both staging and production environments.

Code is not perfect and may require some manual intervention as this only runs on my local machine.

# migrate-tcl-evidences
resolves https://jira.anzx.service.anz/browse/ANZX-222305

staging token
```shell
gcloud auth print-identity-token --include-email --impersonate-service-account xp-sa-xplore-tclsync@anz-x-xplore-staging-1bbe6e.iam.gserviceaccount.com
```

prod token
```shell
gcloud auth print-identity-token --include-email --impersonate-service-account xp-sa-xplore-tclsync@anz-x-xplore-prod-44f597.iam.gserviceaccount.com
```

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