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