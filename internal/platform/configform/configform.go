// Package configform implements the shared configuration-form schema
// resolution used by themes and plugins: interpreting a manifest's declared
// config items, computing their defaults, and resolving submitted values
// against still-live nodes/ping tasks for rendering.
package configform

import (
	"encoding/json"
	"strings"

	"github.com/komari-monitor/komari/internal/platform/dbcore"
	"github.com/komari-monitor/komari/internal/platform/models"
)

const (
	// TypeNodes is the item type for a field whose value is a set of client UUIDs.
	TypeNodes = "nodes"
	// TypePingTasks is the item type for a field whose value is a set of ping task IDs.
	TypePingTasks = "pingtasks"
)

// Items returns the declared managed configuration items. An omitted type is
// managed, matching the theme configuration default.
func Items(configuration models.Configuration) []models.ManagedThemeConfigurationItem {
	if configuration.Type != "" && !strings.EqualFold(configuration.Type, models.ThemeConfigurationManaged) {
		return nil
	}
	raw, err := json.Marshal(configuration.Data)
	if err != nil {
		return nil
	}
	var items []models.ManagedThemeConfigurationItem
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil
	}
	return items
}

// DefaultValue computes the fallback value for a config item that has no
// explicit Default: the first option for a select, a type-appropriate zero
// value otherwise.
func DefaultValue(item models.ManagedThemeConfigurationItem) any {
	value := item.Default
	if item.Type == "select" && (value == nil || value == "") && item.Options != "" {
		options := strings.Split(item.Options, ",")
		if len(options) > 0 {
			return strings.TrimSpace(options[0])
		}
	}
	if value != nil {
		return value
	}
	switch item.Type {
	case "number":
		return float64(0)
	case "switch":
		return false
	case TypeNodes, TypePingTasks:
		return "[]"
	default:
		return ""
	}
}

// ResolveForOutput decodes declared selectors, drops deleted references, and
// returns typed arrays suitable for public theme settings and plugin config.
func ResolveForOutput(values map[string]any, items []models.ManagedThemeConfigurationItem) error {
	hasNodes := false
	hasPingTasks := false
	for _, item := range items {
		switch item.Type {
		case TypeNodes:
			hasNodes = true
		case TypePingTasks:
			hasPingTasks = true
		}
	}

	db := dbcore.GetDBInstance()
	liveNodes := map[string]struct{}{}
	if hasNodes {
		var nodes []models.Client
		if err := db.Select("uuid").Find(&nodes).Error; err != nil {
			return err
		}
		for _, node := range nodes {
			liveNodes[node.UUID] = struct{}{}
		}
	}
	livePingTasks := map[uint]struct{}{}
	if hasPingTasks {
		var tasks []models.PingTask
		if err := db.Select("id").Find(&tasks).Error; err != nil {
			return err
		}
		for _, task := range tasks {
			livePingTasks[task.ID] = struct{}{}
		}
	}

	for _, item := range items {
		if item.Key == "" {
			continue
		}
		switch item.Type {
		case TypeNodes:
			selected := NodeIDs(values[item.Key])
			filtered := make([]string, 0, len(selected))
			for _, id := range selected {
				if _, ok := liveNodes[id]; ok {
					filtered = append(filtered, id)
				}
			}
			values[item.Key] = filtered
		case TypePingTasks:
			selected := PingTaskIDs(values[item.Key])
			filtered := make([]uint, 0, len(selected))
			for _, id := range selected {
				if _, ok := livePingTasks[id]; ok {
					filtered = append(filtered, id)
				}
			}
			values[item.Key] = filtered
		}
	}
	return nil
}

// NodeIDs decodes a TypeNodes field's stored value into client UUIDs.
func NodeIDs(value any) []string {
	return decodeIDs[string](value)
}

// PingTaskIDs decodes a TypePingTasks field's stored value into ping task IDs.
func PingTaskIDs(value any) []uint {
	return decodeIDs[uint](value)
}

// decodeIDs decodes a JSON array of IDs stored as a string value. Any other
// shape (wrong dynamic type, invalid JSON) yields nil instead of an error.
func decodeIDs[T any](value any) []T {
	raw, ok := value.(string)
	if !ok {
		return nil
	}
	var ids []T
	if json.Unmarshal([]byte(raw), &ids) != nil {
		return nil
	}
	return ids
}
