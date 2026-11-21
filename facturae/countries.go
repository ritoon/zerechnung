package facturae

// countries gives, for each country code of the Facturae schemas
// (CountryType, ISO 3166-1 alpha-3), its ISO 3166-1 alpha-2 code, the code
// of EN 16931 (BT-40, BT-55…). The two withdrawn codes of the schemas, ANT
// (Netherlands Antilles) and ZAR (Zaire), have none.
var countries = map[string]string{
	"ABW": "AW", "AFG": "AF", "AGO": "AO", "AIA": "AI", "ALB": "AL", "AND": "AD", "ARE": "AE", "ARG": "AR", "ARM": "AM", "ASM": "AS",
	"ATG": "AG", "AUS": "AU", "AUT": "AT", "AZE": "AZ", "BDI": "BI", "BEL": "BE", "BEN": "BJ", "BFA": "BF", "BGD": "BD", "BGR": "BG",
	"BHR": "BH", "BHS": "BS", "BIH": "BA", "BLR": "BY", "BLZ": "BZ", "BMU": "BM", "BOL": "BO", "BRA": "BR", "BRB": "BB", "BRN": "BN",
	"BTN": "BT", "BWA": "BW", "CAF": "CF", "CAN": "CA", "CHE": "CH", "CHL": "CL", "CHN": "CN", "CIV": "CI", "CMR": "CM", "COD": "CD",
	"COG": "CG", "COK": "CK", "COL": "CO", "COM": "KM", "CPV": "CV", "CRI": "CR", "CUB": "CU", "CYM": "KY", "CYP": "CY", "CZE": "CZ",
	"DEU": "DE", "DJI": "DJ", "DMA": "DM", "DNK": "DK", "DOM": "DO", "DZA": "DZ", "ECU": "EC", "EGY": "EG", "ERI": "ER", "ESH": "EH",
	"ESP": "ES", "EST": "EE", "ETH": "ET", "FIN": "FI", "FJI": "FJ", "FLK": "FK", "FRA": "FR", "FRO": "FO", "FSM": "FM", "GAB": "GA",
	"GBR": "GB", "GEO": "GE", "GGY": "GG", "GHA": "GH", "GIB": "GI", "GIN": "GN", "GLP": "GP", "GMB": "GM", "GNB": "GW", "GNQ": "GQ",
	"GRC": "GR", "GRD": "GD", "GRL": "GL", "GTM": "GT", "GUF": "GF", "GUM": "GU", "GUY": "GY", "HKG": "HK", "HND": "HN", "HRV": "HR",
	"HTI": "HT", "HUN": "HU", "IDN": "ID", "IMN": "IM", "IND": "IN", "IRL": "IE", "IRN": "IR", "IRQ": "IQ", "ISL": "IS", "ISR": "IL",
	"ITA": "IT", "JAM": "JM", "JEY": "JE", "JOR": "JO", "JPN": "JP", "KAZ": "KZ", "KEN": "KE", "KGZ": "KG", "KHM": "KH", "KIR": "KI",
	"KNA": "KN", "KOR": "KR", "KWT": "KW", "LAO": "LA", "LBN": "LB", "LBR": "LR", "LBY": "LY", "LCA": "LC", "LIE": "LI", "LKA": "LK",
	"LSO": "LS", "LTU": "LT", "LUX": "LU", "LVA": "LV", "MAC": "MO", "MAR": "MA", "MCO": "MC", "MDA": "MD", "MDG": "MG", "MDV": "MV",
	"MEX": "MX", "MHL": "MH", "MKD": "MK", "MLI": "ML", "MLT": "MT", "MMR": "MM", "MNE": "ME", "MNG": "MN", "MNP": "MP", "MOZ": "MZ",
	"MRT": "MR", "MSR": "MS", "MTQ": "MQ", "MUS": "MU", "MWI": "MW", "MYS": "MY", "MYT": "YT", "NAM": "NA", "NCL": "NC", "NER": "NE",
	"NFK": "NF", "NGA": "NG", "NIC": "NI", "NIU": "NU", "NLD": "NL", "NOR": "NO", "NPL": "NP", "NRU": "NR", "NZL": "NZ", "OMN": "OM",
	"PAK": "PK", "PAN": "PA", "PCN": "PN", "PER": "PE", "PHL": "PH", "PLW": "PW", "PNG": "PG", "POL": "PL", "PRI": "PR", "PRK": "KP",
	"PRT": "PT", "PRY": "PY", "PSE": "PS", "PYF": "PF", "QAT": "QA", "REU": "RE", "ROU": "RO", "RUS": "RU", "RWA": "RW", "SAU": "SA",
	"SDN": "SD", "SEN": "SN", "SGP": "SG", "SHN": "SH", "SJM": "SJ", "SLB": "SB", "SLE": "SL", "SLV": "SV", "SMR": "SM", "SOM": "SO",
	"SPM": "PM", "SRB": "RS", "STP": "ST", "SUR": "SR", "SVK": "SK", "SVN": "SI", "SWE": "SE", "SWZ": "SZ", "SYC": "SC", "SYR": "SY",
	"TCA": "TC", "TCD": "TD", "TGO": "TG", "THA": "TH", "TJK": "TJ", "TKL": "TK", "TKM": "TM", "TLS": "TL", "TON": "TO", "TTO": "TT",
	"TUN": "TN", "TUR": "TR", "TUV": "TV", "TWN": "TW", "TZA": "TZ", "UGA": "UG", "UKR": "UA", "URY": "UY", "USA": "US", "UZB": "UZ",
	"VAT": "VA", "VCT": "VC", "VEN": "VE", "VGB": "VG", "VIR": "VI", "VNM": "VN", "VUT": "VU", "WLF": "WF", "WSM": "WS", "YEM": "YE",
	"ZAF": "ZA", "ZMB": "ZM", "ZWE": "ZW",
}

// alpha3 is the inverse of countries.
var alpha3 = func() map[string]string {
	m := make(map[string]string, len(countries))
	for a3, a2 := range countries {
		m[a2] = a3
	}
	return m
}()

// countryOf returns the ISO 3166-1 alpha-2 code of a Facturae country code,
// and false when the code has none.
func countryOf(code string) (string, bool) {
	a2, ok := countries[code]
	return a2, ok
}

// countryCode returns the Facturae country code of an ISO 3166-1 alpha-2
// code, or the code itself when Facturae has none for it.
func countryCode(a2 string) string {
	if a3, ok := alpha3[a2]; ok {
		return a3
	}
	return a2
}
