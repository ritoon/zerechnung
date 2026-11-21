package fatturapa

import (
	"errors"
	"fmt"
	"strings"
)

// File name extensions of the SdI (technical specifications 1.8.4, §2.2).
const (
	ExtXML = ".xml"     // unsigned, or signed in XAdES
	ExtP7M = ".xml.p7m" // signed in CAdES
	ExtZIP = ".zip"     // archive of files, each signed
)

// FileName returns the name of a file for the SdI (§2.2): the ISO 3166-1
// alpha-2 code of the country of the transmitter, its tax identifier (the
// codice fiscale for an Italian transmitter, 11 to 16 characters; 2 to 28
// characters otherwise), "_", a progressive of 1 to 5 characters [a-zA-Z0-9]
// that tells apart the files of the same transmitter, and ".xml.p7m" for a
// file signed in CAdES (cades set) or ".xml" otherwise, as in
// IT99999999999_00002.xml.p7m.
func FileName(country, transmitter, progressive string, cades bool) (string, error) {
	ext := ExtXML
	if cades {
		ext = ExtP7M
	}
	name := country + transmitter + "_" + progressive + ext
	if err := checkParts(country, transmitter, progressive); err != nil {
		return "", err
	}
	return name, nil
}

// CheckFileName checks a file name against the naming rules of the SdI
// (§2.2), whose violation the SdI rejects with the code 00001. The
// extension is ".xml", ".xml.p7m" or ".zip", in lower case as the
// specifications write them.
func CheckFileName(name string) error {
	_, err := splitName(name)
	return err
}

func splitName(name string) (ext string, err error) {
	base := name
	if i := strings.LastIndexAny(base, `/\`); i >= 0 {
		base = base[i+1:]
	}
	for _, e := range []string{ExtP7M, ExtXML, ExtZIP} {
		if strings.HasSuffix(base, e) {
			ext = e
			break
		}
	}
	if ext == "" {
		return "", fmt.Errorf("file name %q: the extension must be %s, %s or %s", base, ExtXML, ExtP7M, ExtZIP)
	}
	stem := strings.TrimSuffix(base, ext)
	i := strings.IndexByte(stem, '_')
	if i < 0 || strings.Count(stem, "_") != 1 {
		return "", fmt.Errorf("file name %q: the identifier of the transmitter and the progressive must be separated by one underscore", base)
	}
	if i < 2 {
		return "", fmt.Errorf("file name %q: no country code", base)
	}
	if err := checkParts(stem[:2], stem[2:i], stem[i+1:]); err != nil {
		return "", fmt.Errorf("file name %q: %w", base, err)
	}
	return ext, nil
}

func checkParts(country, transmitter, progressive string) error {
	if len(country) != 2 || !isUpper(country[0]) || !isUpper(country[1]) {
		return fmt.Errorf("the country code %q is not an ISO 3166-1 alpha-2 code", country)
	}
	minLen, maxLen := 2, 28
	if country == "IT" {
		minLen, maxLen = 11, 16
	}
	if len(transmitter) < minLen || len(transmitter) > maxLen || !isAlnum(transmitter) {
		return fmt.Errorf("the identifier of the transmitter %q must have %d to %d alphanumeric characters", transmitter, minLen, maxLen)
	}
	if progressive == "" || len(progressive) > 5 || !isAlnum(progressive) {
		return fmt.Errorf("the progressive %q must have 1 to 5 characters among [a-z], [A-Z] and [0-9]", progressive)
	}
	return nil
}

func isAlnum(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !('a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9') {
			return false
		}
	}
	return true
}

// checkNameFor checks a file name, and that its extension matches the
// format of its signature: ".xml.p7m" for CAdES, ".xml" for XAdES or an
// unsigned XML file. format is zero when the signature could not be read;
// xml tells that the file is XML.
func checkNameFor(name string, format Format, xml bool) error {
	ext, err := splitName(name)
	if err != nil {
		return err
	}
	switch {
	case ext == ExtZIP:
		return errors.New("a .zip archive is not a signed file: check each file it holds")
	case ext == ExtP7M && (format == XAdES || (format == 0 && xml)):
		return fmt.Errorf("file name %q: an XML file has the extension %s, not %s", name, ExtXML, ExtP7M)
	case ext == ExtXML && (format == CAdES || (format == 0 && !xml)):
		return fmt.Errorf("file name %q: a file signed in CAdES has the extension %s, not %s", name, ExtP7M, ExtXML)
	}
	return nil
}
