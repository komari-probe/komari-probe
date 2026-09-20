package mask

import "strings"

// maskText 是敏感片段被替换后的占位符文本。
const maskText = "********"

// DataMasking 将 str 中出现的每一个 private 敏感串替换为占位符。
// private 里的值是调用方已知的精确字符串（如 OAuth client secret/id），
// 因此这里做精确子串匹配即可，不需要模糊匹配。
// 连续被遮蔽的片段长度超过 5 时保留首尾字符方便定位，否则整段替换。
func DataMasking(str string, private []string) string {
	if str == "" || len(private) == 0 {
		return str
	}

	words := uniqueNonEmpty(private)
	if len(words) == 0 {
		return str
	}

	runes := []rune(str)
	n := len(runes)
	toMask := make([]bool, n)

	for _, w := range words {
		wRunes := []rune(w)
		wl := len(wRunes)
		if wl == 0 || wl > n {
			continue
		}
		for i := 0; i <= n-wl; i++ {
			if string(runes[i:i+wl]) == w {
				for k := 0; k < wl; k++ {
					toMask[i+k] = true
				}
			}
		}
	}

	var b strings.Builder
	i := 0
	for i < n {
		if !toMask[i] {
			b.WriteRune(runes[i])
			i++
			continue
		}
		start := i
		for i < n && toMask[i] {
			i++
		}
		end := i // 不包含
		if end-start > 5 {
			b.WriteRune(runes[start])
			b.WriteString(maskText)
			b.WriteRune(runes[end-1])
		} else {
			b.WriteString(maskText)
		}
	}
	return b.String()
}

// uniqueNonEmpty 去掉 values 中的空白项和重复项，并裁剪首尾空白。
func uniqueNonEmpty(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		result = append(result, v)
	}
	return result
}
