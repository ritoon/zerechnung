package xmp

import (
	"strings"
	"sync"
)

// The value types of the properties of the schemas predefined by the XMP
// Specification 2005 and ISO 19005, which PDF/A requires the values to have
// (ISO 19005-3, 6.6.2.3.1). A type is written as a letter, possibly after
// the kind of array that holds the values:
//
//	T  simple text (Text, URI, URL, AgentName, ProperName, MIMEType, Locale,
//	   RenditionClass, choices of text)
//	B  Boolean: True or False
//	I  Integer
//	R  Real
//	D  Date
//	Q  Rational: a fraction of two integers
//	G  GPSCoordinate
//	L  language alternative (Lang Alt)
//	S  structure; its fields are not checked
var valueTypes = sync.OnceValue(func() map[string]map[string]string {
	m := map[string]map[string]string{}
	for ns, props := range map[string]string{
		nsDC: `contributor=bag:T coverage=T creator=seq:T date=seq:D description=L format=T identifier=T
			language=bag:T publisher=bag:T relation=bag:T rights=L source=T subject=bag:T title=L type=bag:T`,
		nsXMP: `Advisory=bag:T BaseURL=T CreateDate=D CreatorTool=T Identifier=bag:T Label=T MetadataDate=D
			ModifyDate=D Nickname=T Thumbnails=alt:S`,
		"http://ns.adobe.com/xap/1.0/rights/": `Certificate=T Marked=B Owner=bag:T UsageTerms=L WebStatement=T`,
		"http://ns.adobe.com/xap/1.0/mm/": `DerivedFrom=S DocumentID=T History=seq:S InstanceID=T ManagedFrom=S Manager=T
			ManageTo=T ManageUI=T ManagerVariant=T RenditionClass=T RenditionParams=T VersionID=T Versions=seq:S
			LastURL=T RenditionOf=S SaveID=I`,
		"http://ns.adobe.com/xap/1.0/bj/":   `JobRef=bag:S`,
		"http://ns.adobe.com/xap/1.0/t/pg/": `MaxPageSize=S NPages=I Fonts=bag:S Colorants=seq:S PlateNames=seq:T`,
		nsPDF:                               `Keywords=T PDFVersion=T Producer=T Trapped=T`,
		nsPDFAID:                            `part=I conformance=T amd=T corr=T`,
		"http://ns.adobe.com/photoshop/1.0/": `AuthorsPosition=T CaptionWriter=T Category=T City=T Country=T Credit=T
			DateCreated=D Headline=T Instructions=T Source=T State=T SupplementalCategories=bag:T
			TransmissionReference=T Urgency=I`,
		"http://ns.adobe.com/camera-raw-settings/1.0/": `AutoBrightness=B AutoContrast=B AutoExposure=B AutoShadows=B
			BlueHue=I BlueSaturation=I Brightness=I CameraProfile=T ChromaticAberrationB=I ChromaticAberrationR=I
			ColorNoiseReduction=I Contrast=I CropTop=R CropLeft=R CropBottom=R CropRight=R CropAngle=R CropWidth=R
			CropHeight=R CropUnits=I Exposure=R GreenHue=I GreenSaturation=I HasCrop=B HasSettings=B
			LuminanceSmoothing=I RawFileName=T RedHue=I RedSaturation=I Saturation=I Shadows=I ShadowTint=I
			Sharpness=I Temperature=I Tint=I ToneCurve=seq:T ToneCurveName=T Version=T VignetteAmount=I
			VignetteMidpoint=I WhiteBalance=T`,
		"http://ns.adobe.com/tiff/1.0/": `ImageWidth=I ImageLength=I BitsPerSample=seq:I Compression=I
			PhotometricInterpretation=I Orientation=I SamplesPerPixel=I PlanarConfiguration=I YCbCrSubSampling=seq:I
			YCbCrPositioning=I XResolution=Q YResolution=Q ResolutionUnit=I TransferFunction=seq:I WhitePoint=seq:Q
			PrimaryChromaticities=seq:Q YCbCrCoefficients=seq:Q ReferenceBlackWhite=seq:Q DateTime=D
			ImageDescription=L Make=T Model=T Software=T Artist=T Copyright=L`,
		"http://ns.adobe.com/exif/1.0/": `ExifVersion=T FlashpixVersion=T ColorSpace=I ComponentsConfiguration=seq:I
			CompressedBitsPerPixel=Q PixelXDimension=I PixelYDimension=I UserComment=L RelatedSoundFile=T
			DateTimeOriginal=D DateTimeDigitized=D ExposureTime=Q FNumber=Q ExposureProgram=I SpectralSensitivity=T
			ISOSpeedRatings=seq:I OECF=S ShutterSpeedValue=Q ApertureValue=Q BrightnessValue=Q ExposureBiasValue=Q
			MaxApertureValue=Q SubjectDistance=Q MeteringMode=I LightSource=I Flash=S FocalLength=Q
			SubjectArea=seq:I FlashEnergy=Q SpatialFrequencyResponse=S FocalPlaneXResolution=Q
			FocalPlaneYResolution=Q FocalPlaneResolutionUnit=I SubjectLocation=seq:I ExposureIndex=Q
			SensingMethod=I FileSource=I SceneType=I CFAPattern=S CustomRendered=I ExposureMode=I WhiteBalance=I
			DigitalZoomRatio=Q FocalLengthIn35mmFilm=I SceneCaptureType=I GainControl=I Contrast=I Saturation=I
			Sharpness=I DeviceSettingDescription=S SubjectDistanceRange=I ImageUniqueID=T GPSVersionID=T
			GPSLatitude=G GPSLongitude=G GPSAltitudeRef=I GPSAltitude=Q GPSTimeStamp=D GPSSatellites=T GPSStatus=T
			GPSMeasureMode=T GPSDOP=Q GPSSpeedRef=T GPSSpeed=Q GPSTrackRef=T GPSTrack=Q GPSImgDirectionRef=T
			GPSImgDirection=Q GPSMapDatum=T GPSDestLatitude=G GPSDestLongitude=G GPSDestBearingRef=T
			GPSDestBearing=Q GPSDestDistanceRef=T GPSDestDistance=Q GPSProcessingMethod=T GPSAreaInformation=T
			GPSDifferential=I`,
		"http://ns.adobe.com/exif/1.0/aux/": `Lens=T SerialNumber=T`,
		"http://ns.adobe.com/xmp/1.0/DynamicMedia/": `projectRef=S videoFrameRate=T videoFrameSize=S
			videoPixelAspectRatio=Q videoPixelDepth=T videoColorSpace=T videoAlphaMode=T videoAlphaPremultipleColor=S
			videoAlphaUnityIsTransparent=B videoCompressor=T videoFieldOrder=T pullDown=T audioSampleRate=I
			audioSampleType=T audioChannelType=T audioCompressor=T speakerPlacement=T fileDataRate=Q tapeName=T
			altTapeName=T startTimecode=S altTimecode=S duration=S scene=T shotName=T shotDate=D shotLocation=T
			logComment=T markers=seq:S contributedMedia=bag:S absPeakAudioFilePath=T relativePeakAudioFilePath=T
			videoModDate=D audioModDate=D metadataModDate=D artist=T album=T trackNumber=I genre=T copyright=T
			releaseDate=D composer=T engineer=T tempo=R instrument=T introTime=S outCue=S relativeTimestamp=S
			loop=B numberOfBeats=R key=T stretchMode=T timeScaleParams=S resampleParams=S beatSpliceParams=S
			timeSignature=T scaleType=T`,
	} {
		set := map[string]string{}
		for _, f := range strings.Fields(props) {
			name, typ, _ := strings.Cut(f, "=")
			set[name] = typ
		}
		m[ns] = set
	}
	return m
})

var typeNames = map[byte]string{
	'T': "Text", 'B': "Boolean", 'I': "Integer", 'R': "Real", 'D': "Date", 'Q': "Rational",
	'G': "GPSCoordinate", 'L': "Lang Alt", 'S': "Struct",
}

// shape is the form of an XMP value.
type shape uint8

const (
	shapeSimple shape = iota
	shapeStruct
	shapeArray
)

// valueOf returns the form of the value of a property or array item n, the
// text of a simple value, and the array type (Bag, Seq, Alt) and items of
// an array.
func valueOf(data []byte, n *node) (sh shape, text string, kind string, items []*node) {
	if v, ok := n.attr(nsRDF, "parseType"); ok && v == "Resource" {
		return shapeStruct, "", "", nil
	}
	if _, ok := n.attr(nsRDF, "resource"); ok {
		return shapeSimple, "", "", nil
	}
	if len(n.children) == 0 {
		for _, a := range n.attrs {
			if a.space != nsRDF && a.space != nsXML {
				return shapeStruct, "", "", nil // property attributes: a structure
			}
		}
		return shapeSimple, n.text(data), "", nil
	}
	c := n.children[0]
	switch {
	case len(n.children) == 1 && c.space == nsRDF && (string(c.local) == "Bag" || string(c.local) == "Seq" || string(c.local) == "Alt"):
		for _, li := range c.children {
			if li.is(nsRDF, "li") {
				items = append(items, li)
			}
		}
		return shapeArray, "", string(c.local), items
	case len(n.children) == 1 && c.is(nsRDF, "Description"):
		// A value with qualifiers holds the value in rdf:value.
		if v := c.child(nsRDF, "value"); v != nil {
			return valueOf(data, v)
		}
		if v, ok := c.attr(nsRDF, "value"); ok {
			return shapeSimple, v, "", nil
		}
		return shapeStruct, "", "", nil
	}
	return shapeStruct, "", "", nil
}

// lang reports whether n or one of its ancestors up to stop has an
// xml:lang attribute.
func lang(n, stop *node) bool {
	for ; n != nil; n = n.parent {
		if _, ok := n.attr(nsXML, "lang"); ok {
			return true
		}
		if n == stop {
			break
		}
	}
	return false
}

// checkSimple returns why text is not a valid simple value of type t, or "".
func checkSimple(t byte, text string) string {
	text = strings.TrimSpace(text)
	var ok bool
	switch t {
	case 'B':
		if text == "True" || text == "False" {
			return ""
		}
		return "is not True or False"
	case 'I':
		ok = isInteger(text)
	case 'R':
		ok = isReal(text)
	case 'D':
		ok = isDate(text)
	case 'Q':
		ok = isRational(text)
	case 'G':
		ok = isGPS(text)
	default:
		return ""
	}
	if ok {
		return ""
	}
	return "is not a valid " + typeNames[t]
}

// The value types below are written by hand; types_test.go checks them
// against the regular expressions they replace, given in their comments.

// isInteger reports whether s is an Integer: ^[+-]?[0-9]+$.
func isInteger(s string) bool {
	s = trimSign(s)
	return s != "" && digits(s) == len(s)
}

// isReal reports whether s is a Real: ^[+-]?([0-9]+\.?[0-9]*|\.[0-9]+)$.
func isReal(s string) bool {
	s = trimSign(s)
	if s != "" && s[0] == '.' {
		return len(s) > 1 && digits(s[1:]) == len(s)-1
	}
	n := digits(s)
	if n == 0 {
		return false
	}
	s = s[n:]
	if s != "" && s[0] == '.' {
		s = s[1:]
	}
	return digits(s) == len(s)
}

// isRational reports whether s is a Rational: ^[+-]?[0-9]+/[+-]?[0-9]+$.
func isRational(s string) bool {
	s = trimSign(s)
	n := digits(s)
	if n == 0 || n == len(s) || s[n] != '/' {
		return false
	}
	return isInteger(s[n+1:])
}

// isDate reports whether s is a Date:
// ^[0-9]{4}(-[0-9]{2}(-[0-9]{2}(T[0-9]{2}:[0-9]{2}(:[0-9]{2}(\.[0-9]+)?)?(Z|[+-][0-9]{2}:[0-9]{2})?)?)?)?$.
func isDate(s string) bool {
	if !digitsAt(s, 0, 4) {
		return false
	}
	s = s[4:]
	for range 2 { // -MM, then -DD
		if s == "" {
			return true
		}
		if len(s) < 3 || s[0] != '-' || !digitsAt(s, 1, 2) {
			return false
		}
		s = s[3:]
	}
	if s == "" {
		return true
	}
	if len(s) < 6 || s[0] != 'T' || !digitsAt(s, 1, 2) || s[3] != ':' || !digitsAt(s, 4, 2) {
		return false
	}
	s = s[6:]
	if len(s) >= 3 && s[0] == ':' && digitsAt(s, 1, 2) {
		s = s[3:]
		if len(s) >= 2 && s[0] == '.' && isDigit(s[1]) {
			s = s[1:]
			s = s[digits(s):]
		}
	}
	switch {
	case s == "", s == "Z":
		return true
	case len(s) == 6 && (s[0] == '+' || s[0] == '-'):
		return digitsAt(s, 1, 2) && s[3] == ':' && digitsAt(s, 4, 2)
	}
	return false
}

// isGPS reports whether s is a GPSCoordinate:
// ^[0-9]{1,3},[0-9]{1,2}(,[0-9]{1,2}|\.[0-9]+)[NSEW]$.
func isGPS(s string) bool {
	n := digits(s)
	if n < 1 || n > 3 || n == len(s) || s[n] != ',' {
		return false
	}
	s = s[n+1:]
	n = digits(s)
	if n < 1 || n > 2 || n == len(s) {
		return false
	}
	sep := s[n]
	s = s[n+1:]
	n = digits(s)
	switch {
	case sep == ',' && n >= 1 && n <= 2:
	case sep == '.' && n >= 1:
	default:
		return false
	}
	s = s[n:]
	return len(s) == 1 && (s[0] == 'N' || s[0] == 'S' || s[0] == 'E' || s[0] == 'W')
}

func isDigit(c byte) bool { return '0' <= c && c <= '9' }

// digits returns the number of ASCII digits s starts with.
func digits(s string) int {
	n := 0
	for n < len(s) && isDigit(s[n]) {
		n++
	}
	return n
}

// digitsAt reports whether s holds n ASCII digits from i.
func digitsAt(s string, i, n int) bool {
	return len(s) >= i+n && digits(s[i:i+n]) == n
}

// trimSign removes one leading + or -.
func trimSign(s string) string {
	if s != "" && (s[0] == '+' || s[0] == '-') {
		return s[1:]
	}
	return s
}

// checkValue returns why the value of property n does not have type typ
// (see valueTypes), or "".
func checkValue(data []byte, n *node, typ string) string {
	kind, item, isArray := strings.Cut(typ, ":")
	if !isArray {
		item, kind = typ, ""
	}
	sh, text, arr, items := valueOf(data, n)
	t := item[0]
	switch {
	case t == 'L':
		if sh != shapeArray || arr != "Alt" {
			return "a language alternative (rdf:Alt) is expected"
		}
		for _, li := range items {
			if !lang(li, n) {
				return "an item of the language alternative has no xml:lang"
			}
		}
		return ""
	case kind != "":
		want := strings.ToUpper(kind[:1]) + kind[1:]
		if sh != shapeArray || arr != want {
			return "an rdf:" + want + " array is expected"
		}
		for _, li := range items {
			if msg := checkItem(data, li, t); msg != "" {
				return "an item " + msg
			}
		}
		return ""
	}
	if msg := describe(sh, text, t); msg != "" {
		return "the value " + msg
	}
	return ""
}

// checkAttr checks the value of a property written as an attribute: a
// simple value.
func checkAttr(text, typ string) string {
	if strings.Contains(typ, ":") || typ == "L" || typ == "S" {
		return "a simple value is written where " + typeLabel(typ) + " is expected"
	}
	if msg := checkSimple(typ[0], text); msg != "" {
		return "the value " + msg
	}
	return ""
}

// typeLabel names a type of valueTypes: "Seq Date", "Lang Alt"...
func typeLabel(typ string) string {
	kind, item, isArray := strings.Cut(typ, ":")
	if !isArray {
		return typeNames[typ[0]]
	}
	return strings.ToUpper(kind[:1]) + kind[1:] + " " + typeNames[item[0]]
}

// checkItem checks an array item or a simple value against type t.
func checkItem(data []byte, n *node, t byte) string {
	sh, text, _, _ := valueOf(data, n)
	return describe(sh, text, t)
}

func describe(sh shape, text string, t byte) string {
	switch {
	case t == 'S' && sh != shapeStruct:
		return "is not a structure"
	case t != 'S' && sh == shapeArray:
		return "is an array, not a " + typeNames[t]
	case t != 'S' && sh == shapeStruct:
		return "is a structure, not a " + typeNames[t]
	case sh == shapeSimple:
		return checkSimple(t, text)
	}
	return ""
}
