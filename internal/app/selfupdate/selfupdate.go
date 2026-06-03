package selfupdate

type AimSelfUpdateCheckResult struct {
	CurrentVersion string
	LatestVersion  string
	HasUpdate      bool
	Comparable     bool
	CurrentAhead   bool
}

type InstallerSelfUpdateRequest struct {
	CurrentVersion string
	TargetVersion  string
}

type InstallerSelfUpdateResult struct {
	PreviousVersion  string
	InstalledVersion string
}
