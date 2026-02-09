package mediavault

type Config struct {
	Providers []ProviderConfig `json:"providers"`
}

type ProviderConfig struct {
	ProviderID string           `json:"providerID"`
	Databases  []DatabaseConfig `json:"databases"`
}

type DatabaseConfig struct {
	DatabaseID string       `json:"databaseID"`
	Users      []UserConfig `json:"users"`
}

type UserConfig struct {
	UserID string        `json:"userID"`
	Albums []AlbumConfig `json:"albums"`
}

type AlbumConfig struct {
	AlbumUID string   `json:"albumUID"`
	Videos   []string `json:"videos"`
}
