package models

import "slices"

type TorrentSettings struct {
	DefaultInstanceID int                  `json:"defaultInstanceId"`
	Categories        []InstanceCategories `json:"categories"`
}

type InstanceCategories struct {
	InstanceID int      `json:"instanceId"`
	Enabled    []string `json:"enabled"`
}

func (v TorrentSettings) AllowsCategory(instance int, category string) bool {
	if category == "" {
		return true
	}
	for _, rule := range v.Categories {
		if rule.InstanceID == instance {
			return slices.Contains(rule.Enabled, category)
		}
	}
	return true
}
