package discovery

import "github.com/slobbe/appimage-manager/internal/domain"

func ParseGitHubRepoValue(value string) (domain.PackageRef, error) {
	return domain.ParseGitHubRepoValue(value)
}

func ParsePackageRefURL(input string) (domain.PackageRef, error) {
	return domain.ParsePackageRefURL(input)
}

func DisplayNameFromRef(value string) string {
	return domain.DisplayNameFromRef(value)
}
