package web

func StatusLabel(v string) string {
	switch v {
	case "DRAFT":
		return "草稿"
	case "BASELINED":
		return "基线已冻结"
	case "DATA_CHECKED":
		return "数据已校验"
	case "TRIAGED":
		return "异常已分诊"
	case "REVIEW_PENDING":
		return "待独立复核"
	case "REVIEWED":
		return "复核通过"
	case "DECIDED":
		return "已裁定"
	case "ARCHIVED":
		return "已封存"
	default:
		return v
	}
}
func SeverityLabel(v string) string {
	switch v {
	case "BLOCKING":
		return "阻断"
	case "MAJOR":
		return "重要"
	case "MINOR":
		return "一般"
	default:
		return v
	}
}
