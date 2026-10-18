package components

import "strings"

// Flag is a national team's flag as an emoji, two cells wide, from its
// team ID ("nat-france", "nat-france-women", "nat-usa-u23"); "" when the
// side has no flag of its own (the Basque Country) or isn't one we know.
// A terminal without a flag font shows the two letters instead, which
// still say which country it is.
func Flag(teamID string) string {
	slug := strings.TrimPrefix(teamID, "nat-")
	slug = strings.TrimSuffix(slug, "-women")
	for _, suffix := range []string{"-u17", "-u19", "-u20", "-u21", "-u23", "-c"} {
		slug = strings.TrimSuffix(slug, suffix)
	}
	if sub, ok := subdivisionFlags[slug]; ok {
		return tagFlag(sub)
	}
	code, ok := nationCodes[slug]
	if !ok {
		return ""
	}
	var b strings.Builder
	for _, r := range code {
		b.WriteRune(0x1F1E6 + (r - 'A'))
	}
	return b.String()
}

// tagFlag builds the black-flag tag sequence for a UK home nation
// ("gbeng"), which has no two-letter code.
func tagFlag(sub string) string {
	var b strings.Builder
	b.WriteRune(0x1F3F4)
	for _, r := range sub {
		b.WriteRune(0xE0000 + r)
	}
	b.WriteRune(0xE007F)
	return b.String()
}

var subdivisionFlags = map[string]string{
	"england":  "gbeng",
	"scotland": "gbsct",
	"wales":    "gbwls",
}

// nationCodes maps a nation's slug to its ISO 3166-1 alpha-2 code (XK for
// Kosovo, which every flag font carries). Northern Ireland, the Basque
// Country, Catalonia and Zanzibar have no emoji flag and are left out.
var nationCodes = map[string]string{
	"afghanistan": "AF", "albania": "AL", "algeria": "DZ", "american-samoa": "AS",
	"andorra": "AD", "angola": "AO", "anguilla": "AI", "antigua-and-barbuda": "AG",
	"argentina": "AR", "armenia": "AM", "aruba": "AW", "australia": "AU",
	"austria": "AT", "azerbaijan": "AZ", "bahamas": "BS", "bahrain": "BH",
	"bangladesh": "BD", "barbados": "BB", "belarus": "BY", "belgium": "BE",
	"belize": "BZ", "benin": "BJ", "bermuda": "BM", "bhutan": "BT",
	"bolivia": "BO", "bonaire": "BQ", "bosnia-herzegovina": "BA", "botswana": "BW",
	"brazil": "BR", "british-virgin-islands": "VG", "brunei-darussalam": "BN", "bulgaria": "BG",
	"burkina-faso": "BF", "burundi": "BI", "cambodia": "KH", "cameroon": "CM",
	"canada": "CA", "cape-verde": "CV", "cayman-islands": "KY", "central-african-republic": "CF",
	"chad": "TD", "chile": "CL", "china": "CN", "chinese-taipei": "TW",
	"colombia": "CO", "comoros": "KM", "congo": "CG", "congo-dr": "CD",
	"cook-islands": "CK", "costa-rica": "CR", "croatia": "HR", "cuba": "CU",
	"curacao": "CW", "cyprus": "CY", "czechia": "CZ", "denmark": "DK",
	"djibouti": "DJ", "dominica": "DM", "dominican-republic": "DO", "ecuador": "EC",
	"egypt": "EG", "el-salvador": "SV", "equatorial-guinea": "GQ", "eritrea": "ER",
	"estonia": "EE", "eswatini": "SZ", "ethiopia": "ET", "faroe-islands": "FO",
	"fiji": "FJ", "finland": "FI", "france": "FR", "french-guiana": "GF",
	"gabon": "GA", "gambia": "GM", "georgia": "GE", "germany": "DE",
	"ghana": "GH", "gibraltar": "GI", "great-britain": "GB", "greece": "GR",
	"grenada": "GD", "guadeloupe": "GP", "guam": "GU", "guatemala": "GT",
	"guinea": "GN", "guinea-bissau": "GW", "guyana": "GY", "haiti": "HT",
	"honduras": "HN", "hong-kong": "HK", "hungary": "HU", "iceland": "IS",
	"india": "IN", "indonesia": "ID", "iran": "IR", "iraq": "IQ",
	"israel": "IL", "italy": "IT", "ivory-coast": "CI", "jamaica": "JM",
	"japan": "JP", "jordan": "JO", "kazakhstan": "KZ", "kenya": "KE",
	"kosovo": "XK", "kuwait": "KW", "kyrgyz-republic": "KG", "laos": "LA",
	"latvia": "LV", "lebanon": "LB", "lesotho": "LS", "liberia": "LR",
	"libya": "LY", "liechtenstein": "LI", "lithuania": "LT", "luxembourg": "LU",
	"macau": "MO", "madagascar": "MG", "malawi": "MW", "malaysia": "MY",
	"maldives": "MV", "mali": "ML", "malta": "MT", "martinique": "MQ",
	"mauritania": "MR", "mauritius": "MU", "mexico": "MX", "moldova": "MD",
	"mongolia": "MN", "montenegro": "ME", "montserrat": "MS", "morocco": "MA",
	"mozambique": "MZ", "myanmar": "MM", "namibia": "NA", "nepal": "NP",
	"netherlands": "NL", "new-caledonia": "NC", "new-zealand": "NZ", "nicaragua": "NI",
	"niger": "NE", "nigeria": "NG", "north-korea": "KP", "north-macedonia": "MK",
	"northern-mariana-islands": "MP", "norway": "NO", "oman": "OM", "pakistan": "PK",
	"palestine": "PS", "panama": "PA", "papua-new-guinea": "PG", "paraguay": "PY",
	"peru": "PE", "philippines": "PH", "poland": "PL", "portugal": "PT",
	"puerto-rico": "PR", "qatar": "QA", "republic-of-ireland": "IE", "romania": "RO",
	"russia": "RU", "rwanda": "RW", "samoa": "WS", "san-marino": "SM",
	"sao-tome-and-principe": "ST", "saudi-arabia": "SA", "senegal": "SN", "serbia": "RS",
	"seychelles": "SC", "sierra-leone": "SL", "singapore": "SG", "sint-maarten": "SX",
	"slovakia": "SK", "slovenia": "SI", "solomon-islands": "SB", "somalia": "SO",
	"south-africa": "ZA", "south-korea": "KR", "south-sudan": "SS", "spain": "ES",
	"sri-lanka": "LK", "st-kitts-and-nevis": "KN", "st-lucia": "LC", "st-martin": "MF",
	"st-vincent-and-the-grenadines": "VC", "sudan": "SD", "suriname": "SR", "sweden": "SE",
	"switzerland": "CH", "syria": "SY", "tahiti": "PF", "tajikistan": "TJ",
	"tanzania": "TZ", "thailand": "TH", "timor-leste": "TL", "togo": "TG",
	"tonga": "TO", "trinidad-and-tobago": "TT", "tunisia": "TN", "turkiye": "TR",
	"turkmenistan": "TM", "turks-and-caicos-islands": "TC", "tuvalu": "TV", "uganda": "UG",
	"ukraine": "UA", "united-arab-emirates": "AE", "united-states": "US", "uruguay": "UY",
	"us-virgin-islands": "VI", "usa": "US", "uzbekistan": "UZ", "vanuatu": "VU",
	"venezuela": "VE", "vietnam": "VN", "yemen": "YE", "zambia": "ZM",
	"zimbabwe": "ZW",
}
