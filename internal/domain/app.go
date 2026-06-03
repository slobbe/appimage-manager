package domain

type App struct {
	Name    string `json:"name"`    // display name
	ID      string `json:"id"`      // unique app id
	Version string `json:"version"` // current app version

	ReplacesID string `json:"-"`

	ExecPath         string `json:"exec_path"`
	IconPath         string `json:"icon_path"`
	DesktopEntryPath string `json:"desktop_entry_path"`
	DesktopEntryLink string `json:"desktop_entry_link"`

	AddedAt   string `json:"added_at"`
	UpdatedAt string `json:"updated_at"`

	LastCheckedAt   string `json:"last_checked_at,omitempty"`
	UpdateAvailable bool   `json:"update_available"`
	LatestVersion   string `json:"latest_version,omitempty"`

	SHA256 string `json:"sha256"`
	SHA1   string `json:"sha1"`

	Source Source        `json:"source"`
	Update *UpdateSource `json:"update,omitempty"` // optional: how to update from here
}
