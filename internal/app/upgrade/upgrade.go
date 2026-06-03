package upgrade

type AimUpgradeCheckResult struct {
	CurrentVersion string
	LatestVersion  string
	HasUpdate      bool
	Comparable     bool
}

type InstallerUpgradeResult struct {
	PreviousVersion  string
	InstalledVersion string
}
