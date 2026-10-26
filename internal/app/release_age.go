package app

import (
	"errors"
	"strings"
	"time"

	"github.com/slobbe/appimage-manager/internal/domain"
)

func shouldDeferRelease(source domain.UpdateSource, release GitHubRelease, minimumHours int64, ignore bool, now time.Time) (bool, error) {
	if minimumHours <= 0 || ignore || release.Prerelease {
		return false, nil
	}
	if !source.Embedded || strings.TrimSpace(source.ReleaseTag) == "" {
		if source.Prerelease {
			return false, nil
		}
	} else if source.ReleaseTag == "latest-pre" || source.ReleaseTag == "latest-all" {
		return false, nil
	}
	if release.PublishedAt.IsZero() {
		return false, errors.New("release publication time is missing")
	}
	if release.PublishedAt.After(now) {
		return true, nil
	}
	// Compare whole elapsed hours rather than multiplying an unbounded setting
	// into a time.Duration, which would overflow for large minimum ages.
	seconds := now.Unix() - release.PublishedAt.Unix()
	if now.Nanosecond() < release.PublishedAt.Nanosecond() {
		seconds--
	}
	return seconds/3600 < minimumHours, nil
}
