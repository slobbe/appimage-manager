package models

type App struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Version     string `json:"version"`
	AppImage    string `json:"appimage"`
	Icon        string `json:"icon"`
	Desktop     string `json:"desktop"`
	DesktopLink string `json:"desktopLink"`
	AddedAt     string `json:"addedAt"`
	UpdatedAt   string `json:"updatedAt"`
	SHA256      string `json:"sha256"`
	SHA1        string `json:"sha1"`
	Type        string `json:"type"`
	UpdateInfo  string `json:"updateInfo"`
}
