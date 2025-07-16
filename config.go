package main

// update these constants as needed
const (
	token             = "<<put your token here>>"
	startingRow       = 11   // starting row in the asset table order by asset_id
	rowLimits         = 20   // number of rows to fetch in each query
	env               = "np" // env for xplore-api, allowed values: prod, staging, np
	dryRun            = true // if true, no evidences will be created, just logged
	concurrencyLimits = 10   // number of concurrent goroutines to run
)
