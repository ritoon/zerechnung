package codes

// The CIUS-RO (Romania) asks, for an address in Romania, the code of its
// county (județ) in the country subdivision (BT-39, BT-54, BT-68, BT-79), and
// for an address in Bucharest the number of its sector in the city (BT-37,
// BT-52, BT-66, BT-77). These lists are those of the schematron of the
// CIUS-RO 1.0.9 (variables ISO-3166-RO-CODES and SECTOR-RO-CODES), which
// package internal/rules/ro checks against them.

// RomanianCounties are the codes of ISO 3166-2:RO: the 41 counties and the
// municipality of Bucharest (RO-B), sorted.
var RomanianCounties = []string{
	"RO-AB", "RO-AG", "RO-AR", "RO-B", "RO-BC", "RO-BH", "RO-BN", "RO-BR",
	"RO-BT", "RO-BV", "RO-BZ", "RO-CJ", "RO-CL", "RO-CS", "RO-CT", "RO-CV",
	"RO-DB", "RO-DJ", "RO-GJ", "RO-GL", "RO-GR", "RO-HD", "RO-HR", "RO-IF",
	"RO-IL", "RO-IS", "RO-MH", "RO-MM", "RO-MS", "RO-NT", "RO-OT", "RO-PH",
	"RO-SB", "RO-SJ", "RO-SM", "RO-SV", "RO-TL", "RO-TM", "RO-TR", "RO-VL",
	"RO-VN", "RO-VS",
}

// BucharestSectors are the values of the city of an address in Bucharest
// that the CIUS-RO accepts: its six sectors, sorted.
var BucharestSectors = []string{"SECTOR1", "SECTOR2", "SECTOR3", "SECTOR4", "SECTOR5", "SECTOR6"}

// RomanianCounty reports whether s is a code of ISO 3166-2:RO (BR-RO-110,
// BR-RO-111, BR-RO-170, BR-RO-212).
func RomanianCounty(s string) bool { return in(RomanianCounties, s) }

// BucharestSector reports whether s is the code of a sector of Bucharest
// (BR-RO-100, BR-RO-101, BR-RO-160, BR-RO-202).
func BucharestSector(s string) bool { return in(BucharestSectors, s) }
