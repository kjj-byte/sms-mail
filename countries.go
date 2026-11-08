package identity

import "strings"

// countryCN maps ISO-3166 alpha-2 codes and common English spellings/aliases
// (lower-cased) to Chinese country names. It is used to display and filter
// countries consistently regardless of how a provider spells them.
var countryISO = map[string]string{
	"AF": "阿富汗", "AL": "阿尔巴尼亚", "DZ": "阿尔及利亚", "AD": "安道尔", "AO": "安哥拉",
	"AI": "安圭拉", "AG": "安提瓜和巴布达", "AR": "阿根廷", "AM": "亚美尼亚", "AW": "阿鲁巴",
	"AU": "澳大利亚", "AT": "奥地利", "AZ": "阿塞拜疆", "BS": "巴哈马", "BH": "巴林",
	"BD": "孟加拉国", "BB": "巴巴多斯", "BY": "白俄罗斯", "BE": "比利时", "BZ": "伯利兹",
	"BJ": "贝宁", "BM": "百慕大", "BT": "不丹", "BO": "玻利维亚", "BA": "波黑",
	"BW": "博茨瓦纳", "BR": "巴西", "BN": "文莱", "BG": "保加利亚", "BF": "布基纳法索",
	"BI": "布隆迪", "KH": "柬埔寨", "CM": "喀麦隆", "CA": "加拿大", "CV": "佛得角",
	"KY": "开曼群岛", "CF": "中非", "TD": "乍得", "CL": "智利", "CN": "中国",
	"CO": "哥伦比亚", "KM": "科摩罗", "CG": "刚果(布)", "CD": "刚果(金)", "CR": "哥斯达黎加",
	"CI": "科特迪瓦", "HR": "克罗地亚", "CU": "古巴", "CW": "库拉索", "CY": "塞浦路斯",
	"CZ": "捷克", "DK": "丹麦", "DJ": "吉布提", "DM": "多米尼克", "DO": "多米尼加",
	"EC": "厄瓜多尔", "EG": "埃及", "SV": "萨尔瓦多", "GQ": "赤道几内亚", "ER": "厄立特里亚",
	"EE": "爱沙尼亚", "ET": "埃塞俄比亚", "FO": "法罗群岛", "FJ": "斐济", "FI": "芬兰",
	"FR": "法国", "GF": "法属圭亚那", "GA": "加蓬", "GM": "冈比亚", "GE": "格鲁吉亚",
	"DE": "德国", "GH": "加纳", "GI": "直布罗陀", "GR": "希腊", "GL": "格陵兰",
	"GD": "格林纳达", "GP": "瓜德罗普", "GT": "危地马拉", "GN": "几内亚", "GW": "几内亚比绍",
	"GY": "圭亚那", "HT": "海地", "HN": "洪都拉斯", "HK": "中国香港", "HU": "匈牙利",
	"IS": "冰岛", "IN": "印度", "ID": "印度尼西亚", "IR": "伊朗", "IQ": "伊拉克",
	"IE": "爱尔兰", "IL": "以色列", "IT": "意大利", "JM": "牙买加", "JP": "日本",
	"JO": "约旦", "KZ": "哈萨克斯坦", "KE": "肯尼亚", "KG": "吉尔吉斯斯坦", "KN": "圣基茨和尼维斯",
	"KP": "朝鲜", "KR": "韩国", "XK": "科索沃", "KW": "科威特", "LA": "老挝",
	"LV": "拉脱维亚", "LB": "黎巴嫩", "LS": "莱索托", "LR": "利比里亚", "LY": "利比亚",
	"LI": "列支敦士登", "LT": "立陶宛", "LU": "卢森堡", "MO": "中国澳门", "MK": "北马其顿",
	"MG": "马达加斯加", "MW": "马拉维", "MY": "马来西亚", "MV": "马尔代夫", "ML": "马里",
	"MT": "马耳他", "MQ": "马提尼克", "MR": "毛里塔尼亚", "MU": "毛里求斯", "MX": "墨西哥",
	"MD": "摩尔多瓦", "MC": "摩纳哥", "MN": "蒙古", "ME": "黑山", "MS": "蒙特塞拉特",
	"MA": "摩洛哥", "MZ": "莫桑比克", "MM": "缅甸", "NA": "纳米比亚", "NR": "瑙鲁",
	"NP": "尼泊尔", "NL": "荷兰", "NC": "新喀里多尼亚", "NZ": "新西兰", "NI": "尼加拉瓜",
	"NE": "尼日尔", "NG": "尼日利亚", "NU": "纽埃", "NO": "挪威", "OM": "阿曼",
	"PK": "巴基斯坦", "PS": "巴勒斯坦", "PA": "巴拿马", "PG": "巴布亚新几内亚", "PY": "巴拉圭",
	"PE": "秘鲁", "PH": "菲律宾", "PL": "波兰", "PT": "葡萄牙", "PR": "波多黎各",
	"QA": "卡塔尔", "RE": "留尼汪", "RO": "罗马尼亚", "RU": "俄罗斯", "RW": "卢旺达",
	"LC": "圣卢西亚", "PM": "圣皮埃尔和密克隆", "VC": "圣文森特和格林纳丁斯",
	"WS": "萨摩亚", "ST": "圣多美和普林西比", "SA": "沙特阿拉伯", "SN": "塞内加尔", "RS": "塞尔维亚",
	"SC": "塞舌尔", "SL": "塞拉利昂", "SG": "新加坡", "SX": "荷属圣马丁", "SK": "斯洛伐克",
	"SI": "斯洛文尼亚", "SB": "所罗门群岛", "SO": "索马里", "ZA": "南非",
	"SS": "南苏丹", "ES": "西班牙", "LK": "斯里兰卡", "SD": "苏丹", "SR": "苏里南",
	"SZ": "斯威士兰", "SE": "瑞典", "CH": "瑞士", "SY": "叙利亚", "TW": "中国台湾",
	"TJ": "塔吉克斯坦", "TZ": "坦桑尼亚", "TH": "泰国", "TL": "东帝汶", "TG": "多哥",
	"TO": "汤加", "TT": "特立尼达和多巴哥", "TN": "突尼斯", "TR": "土耳其", "TM": "土库曼斯坦",
	"TC": "特克斯和凯科斯群岛", "UG": "乌干达", "UA": "乌克兰", "AE": "阿联酋", "GB": "英国",
	"US": "美国", "UY": "乌拉圭", "UZ": "乌兹别克斯坦", "VU": "瓦努阿图", "VE": "委内瑞拉",
	"VN": "越南", "YE": "也门", "ZM": "赞比亚", "ZW": "津巴布韦",
}

// GrizzlySMS labels its virtual USA line "USA (2)" (Russian: США (виртуальные)).
var countryAlias = map[string]string{
	"afghanistan": "阿富汗", "albania": "阿尔巴尼亚", "algeria": "阿尔及利亚", "american samoa": "美属萨摩亚",
	"andorra": "安道尔", "angola": "安哥拉", "anguilla": "安圭拉", "antigua and barbuda": "安提瓜和巴布达",
	"argentina": "阿根廷", "argentinas": "阿根廷", "armenia": "亚美尼亚", "aruba": "阿鲁巴",
	"australia": "澳大利亚", "austria": "奥地利", "azerbaijan": "阿塞拜疆",
	"bahamas": "巴哈马", "bahrain": "巴林", "bangladesh": "孟加拉国", "barbados": "巴巴多斯",
	"belarus": "白俄罗斯", "belgium": "比利时", "belize": "伯利兹", "benin": "贝宁", "bermuda": "百慕大",
	"bhutan": "不丹", "bolivia": "玻利维亚", "bos. and herz.": "波黑", "bosnia": "波黑",
	"bosnia and herzegovina": "波黑", "botswana": "博茨瓦纳", "brazil": "巴西", "britain": "英国",
	"brunei": "文莱", "brunei darussalam": "文莱", "bulgaria": "保加利亚", "burkina faso": "布基纳法索",
	"burundi": "布隆迪", "cabo verde": "佛得角", "cambodia": "柬埔寨", "cameroon": "喀麦隆",
	"canada": "加拿大", "cape verde": "佛得角", "car": "中非",
	"cayman islands": "开曼群岛", "central african republic": "中非", "chad": "乍得", "chile": "智利",
	"china": "中国", "colombia": "哥伦比亚", "comoros": "科摩罗", "congo": "刚果(布)",
	"congo, the democratic republic of the": "刚果(金)", "dem. congo": "刚果(金)", "dr congo": "刚果(金)",
	"republic of the congo": "刚果(布)", "costa rica": "哥斯达黎加", "côte d'ivoire": "科特迪瓦",
	"cote d'ivoire/ivory coast": "科特迪瓦", "ivory coast": "科特迪瓦", "croatia": "克罗地亚",
	"cuba": "古巴", "curaçao": "库拉索", "cyprus": "塞浦路斯", "czech": "捷克", "czech republic": "捷克",
	"czechia": "捷克", "denmark": "丹麦", "djibouti": "吉布提", "dominica": "多米尼克",
	"dominican republic": "多米尼加", "dominicana": "多米尼加", "ecuador": "厄瓜多尔", "egypt": "埃及",
	"el salvador": "萨尔瓦多", "salvador": "萨尔瓦多", "england (uk)": "英国", "equatorial guinea": "赤道几内亚",
	"eritrea": "厄立特里亚", "estonia": "爱沙尼亚", "ethiopia": "埃塞俄比亚", "faroe islands": "法罗群岛",
	"fiji": "斐济", "finland": "芬兰", "france": "法国", "french guiana": "法属圭亚那", "gabon": "加蓬",
	"gambia": "冈比亚", "georgia": "格鲁吉亚", "germany": "德国", "ghana": "加纳", "gibraltar": "直布罗陀",
	"greece": "希腊", "greenland": "格陵兰", "grenada": "格林纳达", "guadeloupe": "瓜德罗普",
	"guatemala": "危地马拉", "guinea": "几内亚", "guinea bissau": "几内亚比绍", "guinea-bissau": "几内亚比绍",
	"guyana": "圭亚那", "haiti": "海地", "honduras": "洪都拉斯", "hong kong": "中国香港",
	"hong kong (china)": "中国香港", "hungary": "匈牙利", "iceland": "冰岛", "india": "印度",
	"indonesia": "印度尼西亚", "iran": "伊朗", "iraq": "伊拉克", "ireland": "爱尔兰", "israel": "以色列",
	"italy": "意大利", "jamaica": "牙买加", "japan": "日本", "jordan": "约旦", "kazakhstan": "哈萨克斯坦",
	"kenya": "肯尼亚", "kyrgyzstan": "吉尔吉斯斯坦", "korea": "韩国", "south korea": "韩国",
	"north korea": "朝鲜", "kosovo": "科索沃", "republic of kosovo": "科索沃", "kuwait": "科威特",
	"lao": "老挝", "lao people's": "老挝", "laos": "老挝", "latvia": "拉脱维亚", "lebanon": "黎巴嫩",
	"liban": "黎巴嫩", "lesotho": "莱索托", "liberia": "利比里亚", "libya": "利比亚",
	"liechtenstein": "列支敦士登", "lithuania": "立陶宛", "luxembourg": "卢森堡", "macao": "中国澳门",
	"macau": "中国澳门", "macedonia": "北马其顿", "north macedonia": "北马其顿",
	"northern macedonia": "北马其顿", "madagascar": "马达加斯加", "malawi": "马拉维", "malaysia": "马来西亚",
	"maldives": "马尔代夫", "mali": "马里", "malta": "马耳他", "martinique": "马提尼克",
	"mauritania": "毛里塔尼亚", "mauritius": "毛里求斯", "mexico": "墨西哥", "moldova": "摩尔多瓦",
	"monaco": "摩纳哥", "mongolia": "蒙古", "montenegro": "黑山", "montserrat": "蒙特塞拉特",
	"morocco": "摩洛哥", "mozambique": "莫桑比克", "myanmar": "缅甸", "nambia": "纳米比亚",
	"namibia": "纳米比亚", "nauru": "瑙鲁", "nepal": "尼泊尔", "netherlands": "荷兰",
	"new caledonia": "新喀里多尼亚", "new guinea": "巴布亚新几内亚", "papua": "巴布亚新几内亚",
	"papua new guinea": "巴布亚新几内亚", "papua new gvineya": "巴布亚新几内亚", "new zealand": "新西兰",
	"nicaragua": "尼加拉瓜", "niger": "尼日尔", "nigeria": "尼日利亚", "niue": "纽埃", "norway": "挪威",
	"oman": "阿曼", "pakistan": "巴基斯坦", "palestine": "巴勒斯坦", "panama": "巴拿马",
	"paraguay": "巴拉圭", "peru": "秘鲁", "philippines": "菲律宾", "poland": "波兰", "portugal": "葡萄牙",
	"puerto rico": "波多黎各", "qatar": "卡塔尔", "réunion": "留尼汪", "reunion": "留尼汪",
	"romania": "罗马尼亚", "russia": "俄罗斯", "russian federation": "俄罗斯", "rwanda": "卢旺达",
	"saint kitts and nevis": "圣基茨和尼维斯", "st. kitts and nevis": "圣基茨和尼维斯",
	"saint lucia": "圣卢西亚", "st. lucia": "圣卢西亚", "saint pierre and miquelon": "圣皮埃尔和密克隆",
	"saint vincent": "圣文森特和格林纳丁斯", "saint vincent and the grenadines": "圣文森特和格林纳丁斯",
	"samoa": "萨摩亚", "sao tome and principe": "圣多美和普林西比", "saudi arabia": "沙特阿拉伯",
	"senegal": "塞内加尔", "serbia": "塞尔维亚", "seychelles": "塞舌尔", "republic of seychelles": "塞舌尔",
	"sierra leone": "塞拉利昂", "sierre lyone": "塞拉利昂", "singapore": "新加坡",
	"sint maarten": "荷属圣马丁", "slovakia": "斯洛伐克", "slovenia": "斯洛文尼亚",
	"solomon islands": "所罗门群岛", "somalia": "索马里", "south africa": "南非", "south sudan": "南苏丹",
	"republic of south sudan": "南苏丹", "spain": "西班牙", "sri lanka": "斯里兰卡", "sudan": "苏丹",
	"suriname": "苏里南", "swaziland": "斯威士兰", "sweden": "瑞典", "switzerland": "瑞士",
	"syria": "叙利亚", "syrian arab republic": "叙利亚", "taiwan": "中国台湾",
	"taiwan, province of china": "中国台湾", "tajikistan": "塔吉克斯坦", "tanzania": "坦桑尼亚",
	"thailand": "泰国", "timor-leste": "东帝汶", "togo": "多哥", "tonga": "汤加",
	"trinidad and tobago": "特立尼达和多巴哥", "tunisia": "突尼斯", "turkey": "土耳其",
	"turkmenistan": "土库曼斯坦", "turks and caicos islands": "特克斯和凯科斯群岛", "uae": "阿联酋",
	"united arab emirates": "阿联酋", "uganda": "乌干达", "ukraine": "乌克兰", "uk": "英国",
	"united kingdom": "英国", "unt. kingdom": "英国", "great britain": "英国", "united states": "美国",
	"united states of america": "美国",
	"usa":                      "美国", "usa (2)": "美国（虚拟）", "uruguay": "乌拉圭", "uzbekistan": "乌兹别克斯坦",
	"vanuatu": "瓦努阿图", "venezuela": "委内瑞拉", "vietnam": "越南", "viet nam": "越南",
	"yemen": "也门", "zambia": "赞比亚", "zimbabwe": "津巴布韦",
}

var countryQualifier = map[string]string{
	"virtual": "虚拟", "virt": "虚拟", "real": "实体", "vip": "VIP",
}

func normalizedCountry(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "未知" {
		return raw
	}
	// Already Chinese (or contains Chinese) — keep as-is.
	for _, r := range raw {
		if r >= 0x4e00 && r <= 0x9fff {
			return raw
		}
	}
	if v := countryISO[strings.ToUpper(raw)]; v != "" {
		return v
	}
	key := strings.ToLower(raw)
	key = strings.Join(strings.Fields(key), " ")
	if v := countryAlias[key]; v != "" {
		return v
	}
	// Try the part before a slash, e.g. "United States/USA".
	if i := strings.Index(key, "/"); i > 0 {
		if v := countryAlias[strings.TrimSpace(key[:i])]; v != "" {
			return v
		}
	}
	// Keep a parenthetical qualifier (virtual/real/VIP route) but translate it,
	// e.g. "United States (Virtual)" -> "美国（虚拟）". The route distinction
	// matters for price/reliability, so it stays visible.
	if i := strings.Index(key, "("); i > 0 {
		base := strings.TrimSpace(key[:i])
		qual := strings.TrimSpace(strings.Trim(key[i:], "() "))
		if bv := countryAlias[base]; bv != "" {
			if qv := countryQualifier[qual]; qv != "" {
				return bv + "（" + qv + "）"
			}
			return bv
		}
	}
	// Space-suffixed qualifier, e.g. "United States VIP" / "... virt".
	for _, q := range []string{" vip", " virt", " virtual", " real"} {
		if strings.HasSuffix(key, q) {
			if bv := countryAlias[strings.TrimSuffix(key, q)]; bv != "" {
				if qv := countryQualifier[strings.TrimSpace(q)]; qv != "" {
					return bv + "（" + qv + "）"
				}
				return bv
			}
		}
	}
	return raw
}
