package dto

type VolumeProfile struct {
	SlotMinutes int     `json:"slotMinutes"`
	Sessions    int     `json:"sessions"`
	Cumulative  []int64 `json:"cumulative"`
}

type VolumeProfilesResponse struct {
	Profiles map[string]VolumeProfile `json:"profiles"`
}
