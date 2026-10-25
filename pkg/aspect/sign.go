package aspect

// Sign names the twelve tropical zodiac signs.
//
// Astrolog source: astrolog renders signs with their canonical 0..11 offsets
// and three-letter abbreviations ("Ari", "Tau", …) throughout its long-form
// chart output. The ordering here matches that numbering; the zero value
// (SignUnknown) is reserved so an uninitialised Sign is recognisably invalid.
type Sign uint8

const (
	SignUnknown Sign = iota
	Aries
	Taurus
	Gemini
	Cancer
	Leo
	Virgo
	Libra
	Scorpio
	Sagittarius
	Capricorn
	Aquarius
	Pisces
)

// String returns a stable lowercase name for the sign.
func (s Sign) String() string {
	switch s {
	case Aries:
		return "aries"
	case Taurus:
		return "taurus"
	case Gemini:
		return "gemini"
	case Cancer:
		return "cancer"
	case Leo:
		return "leo"
	case Virgo:
		return "virgo"
	case Libra:
		return "libra"
	case Scorpio:
		return "scorpio"
	case Sagittarius:
		return "sagittarius"
	case Capricorn:
		return "capricorn"
	case Aquarius:
		return "aquarius"
	case Pisces:
		return "pisces"
	default:
		return "unknown"
	}
}
