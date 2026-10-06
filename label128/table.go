// Package label128 implements Code 128 encoding and decoding using only
// code sets B and C, with optimal (minimal-length) code selection.
package label128

// Code values.
const (
	CodeC   = 99
	CodeB   = 100
	StartA  = 103
	StartB  = 104
	StartC  = 105
	Stop    = 106
	modulus = 103
)

// widthPatterns holds the module widths for values 0..106. Values 0..102
// use six widths (bar/space alternating, starting with a bar); the stop
// symbol (106) uses seven, including its final bar.
var widthPatterns = [107]string{
	"212222", "222122", "222221", "121223", "121322", "131222",
	"122213", "122312", "132212", "221213", "221312", "231212",
	"112232", "122132", "122231", "113222", "123122", "123221",
	"223211", "221132", "221231", "213212", "223112", "312131",
	"311222", "321122", "321221", "312212", "322112", "322211",
	"212123", "212321", "232121", "111323", "131123", "131321",
	"112313", "132113", "132311", "211313", "231113", "231311",
	"112133", "112331", "132131", "113123", "113321", "133121",
	"313121", "211331", "231131", "213113", "213311", "213131",
	"311123", "311321", "331121", "312113", "312311", "332111",
	"314111", "221411", "431111", "111224", "111422", "121124",
	"121421", "141122", "141221", "112214", "112412", "122114",
	"122411", "142112", "142211", "241211", "221114", "413111",
	"241112", "134111", "111242", "121142", "121241", "114212",
	"124112", "124211", "411212", "421112", "421211", "212141",
	"214121", "412121", "111143", "111341", "131141", "114113",
	"114311", "411113", "411311", "113141", "114131", "311141",
	"411131", "211412", "211214", "211232", "2331112",
}

// patternOf returns the module widths of a code value as integers.
func patternOf(value int) []int {
	s := widthPatterns[value]
	widths := make([]int, len(s))
	for i := range s {
		widths[i] = int(s[i] - '0')
	}
	return widths
}

// checksum computes the modulo-103 check value for a full code sequence
// whose first element is the start code.
func checksum(codes []int) int {
	sum := codes[0]
	for i := 1; i < len(codes); i++ {
		sum += i * codes[i]
	}
	return sum % modulus
}
