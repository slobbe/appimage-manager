package upgrade

import (
	"net/http"
	"time"
)

var upgradeHTTPClient = &http.Client{Timeout: 30 * time.Second}

func SetSelfUpdater(updater SelfUpdater) { defaultSelfUpdater = updater }
func SetPaths(paths Paths)               { defaultPaths = paths }
