// Package pingpresets 提供"全国31省市三网延迟检测"内置节点目录。
//
// 节点数据来自 zstaticcdn.com（https://zstaticcdn.com），经项目所有者
// zonefile 明确同意开源项目长期接入使用（2026年，GitHub issue 沟通确认）。
// 域名规则固定为 {省份代码}-{运营商代码}-v{4|6}.ip.zstaticcdn.com，
// 省级节点使用 :80 端口。本包只维护省份/运营商这两张小表，186 个目标地址
// 由固定模板拼接生成，不需要逐条维护。
package pingpresets

import "fmt"

// Province 表示一个省级行政区（不含港澳台，共 31 个）。
type Province struct {
	Code string `json:"code"` // zstaticcdn 域名里用的两字母省份代码
	Name string `json:"name"` // 中文名，用于生成任务名称和地图标签
}

// Carrier 表示一家运营商。
type Carrier struct {
	Code string `json:"code"` // zstaticcdn 域名里用的运营商代码
	Name string `json:"name"` // 中文名
}

// Node 表示一个具体的内置探测目标：某省份 + 某运营商 + 某 IP 版本。
type Node struct {
	ProvinceCode string `json:"province_code"`
	ProvinceName string `json:"province_name"`
	CarrierCode  string `json:"carrier_code"`
	CarrierName  string `json:"carrier_name"`
	IPVersion    int    `json:"ip_version"` // 4 或 6
	Name         string `json:"name"`       // "省份-运营商"，如 "北京-电信"
	Target       string `json:"target"`     // "bj-ct-v4.ip.zstaticcdn.com:80"
}

const (
	domainPattern = "%s-%s-v%d.ip.zstaticcdn.com"
	provincePort  = 80
)

// Provinces 是全部 31 个省级行政区，顺序按行政区划代码排列。
var Provinces = []Province{
	{Code: "bj", Name: "北京"},
	{Code: "tj", Name: "天津"},
	{Code: "he", Name: "河北"},
	{Code: "sx", Name: "山西"},
	{Code: "nm", Name: "内蒙古"},
	{Code: "ln", Name: "辽宁"},
	{Code: "jl", Name: "吉林"},
	{Code: "hl", Name: "黑龙江"},
	{Code: "sh", Name: "上海"},
	{Code: "js", Name: "江苏"},
	{Code: "zj", Name: "浙江"},
	{Code: "ah", Name: "安徽"},
	{Code: "fj", Name: "福建"},
	{Code: "jx", Name: "江西"},
	{Code: "sd", Name: "山东"},
	{Code: "ha", Name: "河南"},
	{Code: "hb", Name: "湖北"},
	{Code: "hn", Name: "湖南"},
	{Code: "gd", Name: "广东"},
	{Code: "gx", Name: "广西"},
	{Code: "hi", Name: "海南"},
	{Code: "cq", Name: "重庆"},
	{Code: "sc", Name: "四川"},
	{Code: "gz", Name: "贵州"},
	{Code: "yn", Name: "云南"},
	{Code: "xz", Name: "西藏"},
	{Code: "sn", Name: "陕西"},
	{Code: "gs", Name: "甘肃"},
	{Code: "qh", Name: "青海"},
	{Code: "nx", Name: "宁夏"},
	{Code: "xj", Name: "新疆"},
}

// Carriers 是三大运营商，顺序固定，前端展示顺序也按这个来。
var Carriers = []Carrier{
	{Code: "ct", Name: "电信"},
	{Code: "cu", Name: "联通"},
	{Code: "cm", Name: "移动"},
}

// Nodes 生成指定 IP 版本（4 或 6）下的全部 31省 × 3网 节点，
// 顺序为先按 Provinces 顺序、再按 Carriers 顺序。
func Nodes(ipVersion int) []Node {
	nodes := make([]Node, 0, len(Provinces)*len(Carriers))
	for _, p := range Provinces {
		for _, c := range Carriers {
			host := fmt.Sprintf(domainPattern, p.Code, c.Code, ipVersion)
			nodes = append(nodes, Node{
				ProvinceCode: p.Code,
				ProvinceName: p.Name,
				CarrierCode:  c.Code,
				CarrierName:  c.Name,
				IPVersion:    ipVersion,
				Name:         fmt.Sprintf("%s-%s", p.Name, c.Name),
				Target:       fmt.Sprintf("%s:%d", host, provincePort),
			})
		}
	}
	return nodes
}

// AllNodes 返回 v4 + v6 全部节点（最多 186 个）。
func AllNodes() []Node {
	all := make([]Node, 0, len(Provinces)*len(Carriers)*2)
	all = append(all, Nodes(4)...)
	all = append(all, Nodes(6)...)
	return all
}
