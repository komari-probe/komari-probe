package kv

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
)

// unmarshalToField 将 JSON 字符串反序列化到字段，支持数值类型转换
func unmarshalToField(jsonStr string, fieldVal reflect.Value) error {
	target := reflect.New(fieldVal.Type()).Interface()
	if err := json.Unmarshal([]byte(jsonStr), target); err != nil {
		// 尝试通用解析后转换
		var generic any
		if err := json.Unmarshal([]byte(jsonStr), &generic); err != nil {
			return err
		}
		return convertAndSet(generic, fieldVal)
	}
	fieldVal.Set(reflect.ValueOf(target).Elem())
	return nil
}

// scanIntegerDefault 解析 default tag 里的整数值：优先按整数解析以保留 64 位精度，
// 解析失败（例如写成了 "1.5" 这种小数形式）再退化为按浮点数解析后取整。
// 空字符串返回 0，不算错误。
func scanIntegerDefault(defaultVal string) (int64, error) {
	if defaultVal == "" {
		return 0, nil
	}
	if v, err := strconv.ParseInt(defaultVal, 10, 64); err == nil {
		return v, nil
	}
	f, err := strconv.ParseFloat(defaultVal, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid integer default %q", defaultVal)
	}
	return int64(f), nil
}

// parseDefaultToField 解析 default tag 值到字段
func parseDefaultToField(defaultVal string, fieldVal reflect.Value) error {
	kind := fieldVal.Kind()

	switch kind {
	case reflect.String:
		fieldVal.SetString(defaultVal)
	case reflect.Bool:
		v := false
		if defaultVal != "" {
			parsed, err := strconv.ParseBool(defaultVal)
			if err != nil {
				return fmt.Errorf("invalid bool default %q: %w", defaultVal, err)
			}
			v = parsed
		}
		fieldVal.SetBool(v)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v, err := scanIntegerDefault(defaultVal)
		if err != nil {
			return err
		}
		fieldVal.SetInt(v)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v, err := scanIntegerDefault(defaultVal)
		if err != nil {
			return err
		}
		if v < 0 {
			return fmt.Errorf("default value %q is negative and cannot be stored in an unsigned field", defaultVal)
		}
		fieldVal.SetUint(uint64(v))
	case reflect.Float32, reflect.Float64:
		var v float64
		if defaultVal != "" {
			parsed, err := strconv.ParseFloat(defaultVal, 64)
			if err != nil {
				return fmt.Errorf("invalid float default %q: %w", defaultVal, err)
			}
			v = parsed
		}
		fieldVal.SetFloat(v)
	default:
		// 对于复杂类型，尝试 JSON 解析
		if defaultVal == "" {
			return nil // 保持零值
		}
		target := reflect.New(fieldVal.Type()).Interface()
		if err := json.Unmarshal([]byte(defaultVal), target); err != nil {
			return err
		}
		fieldVal.Set(reflect.ValueOf(target).Elem())
	}
	return nil
}

// convertAndSet 通用类型转换并设置字段值
func convertAndSet(val any, fieldVal reflect.Value) error {
	if val == nil {
		return nil
	}

	targetType := fieldVal.Type()
	v := reflect.ValueOf(val)

	// 直接类型匹配
	if v.Type().AssignableTo(targetType) {
		fieldVal.Set(v)
		return nil
	}

	// 类型可转换
	if v.Type().ConvertibleTo(targetType) {
		fieldVal.Set(v.Convert(targetType))
		return nil
	}

	// 数值类型特殊处理 (JSON 数字默认解析为 float64)
	if f, ok := val.(float64); ok {
		switch fieldVal.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			fieldVal.SetInt(int64(f))
			return nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			fieldVal.SetUint(uint64(f))
			return nil
		case reflect.Float32, reflect.Float64:
			fieldVal.SetFloat(f)
			return nil
		}
	}

	// JSON 回环转换
	b, err := json.Marshal(val)
	if err != nil {
		return err
	}
	target := reflect.New(targetType).Interface()
	if err := json.Unmarshal(b, target); err != nil {
		return err
	}
	fieldVal.Set(reflect.ValueOf(target).Elem())
	return nil
}
