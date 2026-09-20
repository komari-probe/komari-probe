package kv

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	logger "github.com/komari-monitor/komari/pkg/log"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ConfigItem struct {
	Key   string `gorm:"primaryKey;column:key;type:text"`
	Value string `gorm:"column:value;type:text"` // 存 JSON 字符串
}

func (ConfigItem) TableName() string {
	return "configs"
}

var (
	db    *gorm.DB
	SetDb = func(gdb *gorm.DB) {
		db = gdb
		if err := db.AutoMigrate(&ConfigItem{}); err != nil {
			panic("failed to migrate config item table: " + err.Error())
		}
	}
)

// GetAs 获取并转换为指定类型 (泛型)，支持数值类型自动转换
func GetAs[T any](key string, defaults ...any) (T, error) {
	var t T
	var item ConfigItem

	err := db.First(&item, "key = ?", key).Error
	if err != nil {
		if len(defaults) > 0 {
			// 尝试直接类型断言
			if v, ok := defaults[0].(T); ok {
				err = Set(key, v)
				return v, err
			}
			// 尝试类型转换
			val := reflect.ValueOf(&t).Elem()
			if err := convertAndSet(defaults[0], val); err != nil {
				return t, fmt.Errorf("default value type mismatch: expected %T, got %T", t, defaults[0])
			}
			err = Set(key, t)
			return t, err
		}
		return t, err
	}

	if err := unmarshalToField(item.Value, reflect.ValueOf(&t).Elem()); err != nil {
		return t, err
	}
	return t, nil
}

// GetMany 获取多个配置项，keys 为 map[key]defaultValue
// 如果 defaultValue 为 nil，则数据库不存在时不写入
// 如果 defaultValue 不为 nil，则数据库不存在时写入默认值
func GetMany(keys map[string]any) (map[string]any, error) {
	result := make(map[string]any)
	keyList := make([]string, 0, len(keys))
	for k := range keys {
		keyList = append(keyList, k)
	}
	if len(keyList) == 0 {
		return result, nil
	}
	items, err := fetchConfigItems(keyList)
	if err != nil {
		return nil, err
	}

	foundKeys := make(map[string]bool)
	for _, item := range items {
		if parsed, ok := decodeConfigValue(item.Value); ok {
			result[item.Key] = parsed
			foundKeys[item.Key] = true
		}
	}

	// 收集需要写入数据库的默认值
	var toInsert []ConfigItem
	for k, def := range keys {
		if _, found := foundKeys[k]; !found {
			if def != nil {
				result[k] = def
				// 序列化后加入待写入列表
				jsonBytes, err := json.Marshal(def)
				if err != nil {
					logger.Warn("config", "marshal default value failed", "key", k, "error", err)
					continue
				}
				toInsert = append(toInsert, ConfigItem{
					Key:   k,
					Value: string(jsonBytes),
				})
			}
		}
	}

	if err := upsertConfigItems(toInsert); err != nil {
		logger.Warn("config", "batch insert default config failed", "error", err)
	}

	return result, nil
}

// GetManyAs 将多个配置项映射到一个结构体中，json tag 作为 Key
// 支持 default tag 作为默认值，如果数据库中不存在且有 default tag 则写入数据库
// 没有 default tag 的字段使用零值，不写入数据库
func GetManyAs[T any]() (*T, error) {
	var t T
	val := reflect.ValueOf(&t).Elem()
	typ := val.Type()

	type fieldInfo struct {
		index      int
		key        string
		hasDefault bool
		defaultVal string
	}

	fields := make([]fieldInfo, 0)
	keys := make([]string, 0)

	for i := 0; i < val.NumField(); i++ {
		field := typ.Field(i)
		jsonTag := field.Tag.Get("json")
		if jsonTag == "" || jsonTag == "-" {
			continue
		}
		// 解析 json tag，处理 "key,omitempty" 格式
		key, _, _ := strings.Cut(jsonTag, ",")
		if key == "" || key == "-" {
			continue
		}

		defaultTag := field.Tag.Get("default")
		// 检查是否显式定义了 default tag (即使值为空)
		_, hasDefault := field.Tag.Lookup("default")

		fields = append(fields, fieldInfo{
			index:      i,
			key:        key,
			hasDefault: hasDefault,
			defaultVal: defaultTag,
		})
		keys = append(keys, key)
	}

	if len(keys) == 0 {
		return &t, nil
	}

	items, err := fetchConfigItems(keys)
	if err != nil {
		return nil, err
	}

	// 建立数据库中存在的 key 映射
	foundItems := make(map[string]string) // key -> value
	for _, item := range items {
		foundItems[item.Key] = item.Value
	}

	// 需要写入数据库的新配置项
	var toInsert []ConfigItem

	for _, fi := range fields {
		fieldVal := val.Field(fi.index)
		if !fieldVal.CanSet() {
			continue
		}

		if dbValue, found := foundItems[fi.key]; found {
			// 数据库中存在，使用数据库值
			if err := unmarshalToField(dbValue, fieldVal); err != nil {
				logger.Warn("config", "unmarshal config failed", "key", fi.key, "error", err)
			}
		} else if fi.hasDefault {
			// 数据库中不存在，但有 default tag，解析默认值并写入数据库
			if err := parseDefaultToField(fi.defaultVal, fieldVal); err != nil {
				logger.Warn("config", "parse default value failed", "key", fi.key, "error", err)
				continue
			}
			// 序列化后写入数据库
			jsonBytes, err := json.Marshal(fieldVal.Interface())
			if err != nil {
				logger.Warn("config", "marshal default value failed", "key", fi.key, "error", err)
				continue
			}
			toInsert = append(toInsert, ConfigItem{
				Key:   fi.key,
				Value: string(jsonBytes),
			})
		}
		// 没有 default tag 且数据库中不存在，保持零值，不写入数据库
	}

	if err := upsertConfigItems(toInsert); err != nil {
		logger.Warn("config", "batch insert default config failed", "error", err)
	}

	return &t, nil
}

// decodeConfigValue 把一个 ConfigItem 存的 JSON 字符串解析成 any，
// 解析失败时返回 ok=false，调用方按"当作没有这个值"处理。
func decodeConfigValue(raw string) (any, bool) {
	var parsed any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, false
	}
	return parsed, true
}

// upsertConfigItems 按 key 做 upsert（存在则更新 value，不存在则插入）。
// items 为空时不做任何操作。
func upsertConfigItems(items []ConfigItem) error {
	if len(items) == 0 {
		return nil
	}
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value"}),
	}).Create(&items).Error
}

// fetchConfigItems 按 key 列表批量查询已存在的配置项。
func fetchConfigItems(keys []string) ([]ConfigItem, error) {
	var items []ConfigItem
	if len(keys) == 0 {
		return items, nil
	}
	if err := db.Where("key IN ?", keys).Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func GetAll() (map[string]any, error) {
	var items []ConfigItem
	result := make(map[string]any)
	if err := db.Find(&items).Error; err != nil {
		return nil, err
	}

	for _, item := range items {
		if parsed, ok := decodeConfigValue(item.Value); ok {
			result[item.Key] = parsed
		}
	}
	return result, nil
}

// Set 设置单个配置，是 SetMany 只传一个 key 的简写。
func Set(key string, value any) error {
	return SetMany(map[string]any{key: value})
}

func SetMany(cst map[string]any) error {
	items := make([]ConfigItem, 0, len(cst))
	for k, v := range cst {
		bytes, err := json.Marshal(v)
		if err != nil {
			return fmt.Errorf("marshal key %s failed: %w", k, err)
		}
		items = append(items, ConfigItem{
			Key:   k,
			Value: string(bytes),
		})
	}
	if len(items) == 0 {
		return nil
	}

	keys := make([]string, 0, len(items))
	newVal := make(map[string]any, len(items))
	for _, it := range items {
		keys = append(keys, it.Key)
		if parsed, ok := decodeConfigValue(it.Value); ok {
			newVal[it.Key] = parsed
		}
	}

	oldVal := map[string]any{}
	if oldItems, err := fetchConfigItems(keys); err == nil {
		for _, oi := range oldItems {
			if parsed, ok := decodeConfigValue(oi.Value); ok {
				oldVal[oi.Key] = parsed
			}
		}
	}

	if err := upsertConfigItems(items); err != nil {
		return err
	}

	publishEvent(oldVal, newVal)
	return nil
}
