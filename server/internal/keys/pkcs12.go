package keys

import (
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/des"
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"fmt"
	"hash"
	"unicode/utf16"
)

// Object identifiers of PKCS#5, PKCS#7 and PKCS#12 (RFC 8018, RFC 7292).
var (
	oidData          = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 1}
	oidEncryptedData = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 6}

	oidKeyBag              = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 12, 10, 1, 1}
	oidPKCS8ShroudedKeyBag = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 12, 10, 1, 2}
	oidCertBag             = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 12, 10, 1, 3}
	oidX509Certificate     = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 22, 1}

	oidPBES2  = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 5, 13}
	oidPBKDF2 = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 5, 12}

	oidPBEWithSHAAnd3KeyTripleDESCBC = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 12, 1, 3}
	oidPBEWithSHAAnd128BitRC2CBC     = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 12, 1, 5}
	oidPBEWithSHAAnd40BitRC2CBC      = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 12, 1, 6}

	oidHMACWithSHA1   = asn1.ObjectIdentifier{1, 2, 840, 113549, 2, 7}
	oidHMACWithSHA224 = asn1.ObjectIdentifier{1, 2, 840, 113549, 2, 8}
	oidHMACWithSHA256 = asn1.ObjectIdentifier{1, 2, 840, 113549, 2, 9}
	oidHMACWithSHA384 = asn1.ObjectIdentifier{1, 2, 840, 113549, 2, 10}
	oidHMACWithSHA512 = asn1.ObjectIdentifier{1, 2, 840, 113549, 2, 11}

	oidAES128CBC   = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 2}
	oidAES192CBC   = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 22}
	oidAES256CBC   = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 42}
	oidDESEDE3CBC  = asn1.ObjectIdentifier{1, 2, 840, 113549, 3, 7}
	oidSHA1        = asn1.ObjectIdentifier{1, 3, 14, 3, 2, 26}
	oidSHA256      = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 1}
	oidSHA384      = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 2}
	oidSHA512      = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 3}
	errBadPassword = errors.New("wrong password, or corrupted file")
)

type pfxPDU struct {
	Version  int
	AuthSafe contentInfo
	MacData  macData `asn1:"optional"`
}

type contentInfo struct {
	ContentType asn1.ObjectIdentifier
	Content     asn1.RawValue `asn1:"tag:0,explicit,optional"`
}

type macData struct {
	Mac        digestInfo
	MacSalt    []byte
	Iterations int `asn1:"optional,default:1"`
}

type digestInfo struct {
	Algorithm pkix.AlgorithmIdentifier
	Digest    []byte
}

type encryptedData struct {
	Version              int
	EncryptedContentInfo encryptedContentInfo
}

type encryptedContentInfo struct {
	ContentType                asn1.ObjectIdentifier
	ContentEncryptionAlgorithm pkix.AlgorithmIdentifier
	EncryptedContent           []byte `asn1:"tag:0,optional"`
}

type safeBag struct {
	ID         asn1.ObjectIdentifier
	Value      asn1.RawValue   `asn1:"tag:0,explicit"`
	Attributes []asn1.RawValue `asn1:"set,optional"`
}

type certBag struct {
	ID   asn1.ObjectIdentifier
	Data []byte `asn1:"tag:0,explicit"`
}

type encryptedPrivateKeyInfo struct {
	Algorithm     pkix.AlgorithmIdentifier
	EncryptedData []byte
}

// ParsePKCS12 reads the private key and the certificates of a PKCS#12
// file. It knows the encryptions of OpenSSL 3 and of current Windows
// versions (PBES2 with PBKDF2 and AES) and the legacy ones (triple DES and
// RC2 with the PKCS#12 key derivation), and checks the integrity MAC.
func ParsePKCS12(data []byte, password string) (crypto.Signer, []*x509.Certificate, error) {
	var pfx pfxPDU
	if rest, err := asn1.Unmarshal(data, &pfx); err != nil {
		return nil, nil, fmt.Errorf("not a DER PKCS#12 file (%v); a BER file can be converted with openssl pkcs12", err)
	} else if len(rest) > 0 {
		return nil, nil, errors.New("trailing data after the PKCS#12 structure")
	}
	if pfx.Version != 3 {
		return nil, nil, fmt.Errorf("unsupported PKCS#12 version %d", pfx.Version)
	}
	if !pfx.AuthSafe.ContentType.Equal(oidData) {
		return nil, nil, errors.New("only password-protected PKCS#12 files are supported (no public-key integrity mode)")
	}
	var authSafe []byte
	if _, err := asn1.Unmarshal(pfx.AuthSafe.Content.Bytes, &authSafe); err != nil {
		return nil, nil, fmt.Errorf("authenticated safe: %w", err)
	}
	if len(pfx.MacData.Mac.Digest) > 0 {
		if err := checkMAC(&pfx.MacData, authSafe, password); err != nil {
			return nil, nil, err
		}
	}
	var infos []contentInfo
	if _, err := asn1.Unmarshal(authSafe, &infos); err != nil {
		return nil, nil, fmt.Errorf("authenticated safe: %w", err)
	}
	var key crypto.Signer
	var certs []*x509.Certificate
	for _, ci := range infos {
		var contents []byte
		switch {
		case ci.ContentType.Equal(oidData):
			if _, err := asn1.Unmarshal(ci.Content.Bytes, &contents); err != nil {
				return nil, nil, fmt.Errorf("safe contents: %w", err)
			}
		case ci.ContentType.Equal(oidEncryptedData):
			var ed encryptedData
			if _, err := asn1.Unmarshal(ci.Content.Bytes, &ed); err != nil {
				return nil, nil, fmt.Errorf("encrypted safe contents: %w", err)
			}
			var err error
			contents, err = pbDecrypt(ed.EncryptedContentInfo.ContentEncryptionAlgorithm, ed.EncryptedContentInfo.EncryptedContent, password)
			if err != nil {
				return nil, nil, fmt.Errorf("encrypted safe contents: %w", err)
			}
		default:
			return nil, nil, fmt.Errorf("unsupported content type %v", ci.ContentType)
		}
		var bags []safeBag
		if _, err := asn1.Unmarshal(contents, &bags); err != nil {
			return nil, nil, fmt.Errorf("safe bags: %w", err)
		}
		for _, b := range bags {
			switch {
			case b.ID.Equal(oidCertBag):
				var cb certBag
				if _, err := asn1.Unmarshal(b.Value.Bytes, &cb); err != nil {
					return nil, nil, fmt.Errorf("certificate bag: %w", err)
				}
				if !cb.ID.Equal(oidX509Certificate) {
					continue
				}
				c, err := x509.ParseCertificate(cb.Data)
				if err != nil {
					return nil, nil, fmt.Errorf("certificate bag: %w", err)
				}
				certs = append(certs, c)
			case b.ID.Equal(oidKeyBag), b.ID.Equal(oidPKCS8ShroudedKeyBag):
				if key != nil {
					return nil, nil, errors.New("the PKCS#12 file holds several private keys")
				}
				der := b.Value.Bytes
				if b.ID.Equal(oidPKCS8ShroudedKeyBag) {
					var err error
					if der, err = decryptPKCS8Password(b.Value.Bytes, password); err != nil {
						return nil, nil, fmt.Errorf("private key: %w", err)
					}
				}
				k, err := x509.ParsePKCS8PrivateKey(der)
				if err != nil {
					return nil, nil, fmt.Errorf("private key: %w", err)
				}
				s, ok := k.(crypto.Signer)
				if !ok {
					return nil, nil, fmt.Errorf("private key: %T cannot sign", k)
				}
				key = s
			}
		}
	}
	if key == nil {
		return nil, nil, errors.New("the PKCS#12 file holds no private key")
	}
	return key, certs, nil
}

func checkMAC(m *macData, content []byte, password string) error {
	var newHash func() hash.Hash
	var size int
	alg := m.Mac.Algorithm.Algorithm
	switch {
	case alg.Equal(oidSHA1):
		newHash, size = sha1.New, sha1.Size
	case alg.Equal(oidSHA256):
		newHash, size = sha256.New, sha256.Size
	case alg.Equal(oidSHA384):
		newHash, size = sha512.New384, sha512.Size384
	case alg.Equal(oidSHA512):
		newHash, size = sha512.New, sha512.Size
	default:
		return fmt.Errorf("unsupported MAC algorithm %v", alg)
	}
	if m.Iterations < 1 || m.Iterations > 10_000_000 {
		return fmt.Errorf("unreasonable MAC iteration count %d", m.Iterations)
	}
	// An empty password is encoded as an empty string or as a lone
	// terminator, depending on the producer: both are tried.
	candidates := [][]byte{bmpString(password)}
	if password == "" {
		candidates = append(candidates, nil)
	}
	for _, pw := range candidates {
		key := pkcs12KDF(newHash, pw, m.MacSalt, 3, m.Iterations, size)
		mac := hmac.New(newHash, key)
		mac.Write(content)
		if hmac.Equal(mac.Sum(nil), m.Mac.Digest) {
			return nil
		}
	}
	return errBadPassword
}

// decryptPKCS8 decrypts an EncryptedPrivateKeyInfo encrypted with PBES2.
func decryptPKCS8(der, password []byte) ([]byte, error) {
	var info encryptedPrivateKeyInfo
	if _, err := asn1.Unmarshal(der, &info); err != nil {
		return nil, err
	}
	if !info.Algorithm.Algorithm.Equal(oidPBES2) {
		return nil, fmt.Errorf("unsupported key encryption %v: only PBES2 is", info.Algorithm.Algorithm)
	}
	return pbes2Decrypt(info.Algorithm.Parameters.FullBytes, info.EncryptedData, password)
}

// decryptPKCS8Password decrypts the key bag of a PKCS#12 file.
func decryptPKCS8Password(der []byte, password string) ([]byte, error) {
	var info encryptedPrivateKeyInfo
	if _, err := asn1.Unmarshal(der, &info); err != nil {
		return nil, err
	}
	return pbDecrypt(info.Algorithm, info.EncryptedData, password)
}

// pbDecrypt decrypts data with a password-based encryption of PKCS#5
// (PBES2, whose password is the UTF-8 string) or PKCS#12 (whose password
// is the BMPString).
func pbDecrypt(alg pkix.AlgorithmIdentifier, data []byte, password string) ([]byte, error) {
	if alg.Algorithm.Equal(oidPBES2) {
		return pbes2Decrypt(alg.Parameters.FullBytes, data, []byte(password))
	}
	var params struct {
		Salt       []byte
		Iterations int
	}
	if _, err := asn1.Unmarshal(alg.Parameters.FullBytes, &params); err != nil {
		return nil, fmt.Errorf("PBE parameters: %w", err)
	}
	if params.Iterations < 1 || params.Iterations > 10_000_000 {
		return nil, fmt.Errorf("unreasonable iteration count %d", params.Iterations)
	}
	pw := bmpString(password)
	var block cipher.Block
	var err error
	switch {
	case alg.Algorithm.Equal(oidPBEWithSHAAnd3KeyTripleDESCBC):
		block, err = des.NewTripleDESCipher(pkcs12KDF(sha1.New, pw, params.Salt, 1, params.Iterations, 24))
	case alg.Algorithm.Equal(oidPBEWithSHAAnd40BitRC2CBC):
		block, err = newRC2(pkcs12KDF(sha1.New, pw, params.Salt, 1, params.Iterations, 5), 40)
	case alg.Algorithm.Equal(oidPBEWithSHAAnd128BitRC2CBC):
		block, err = newRC2(pkcs12KDF(sha1.New, pw, params.Salt, 1, params.Iterations, 16), 128)
	default:
		return nil, fmt.Errorf("unsupported encryption %v", alg.Algorithm)
	}
	if err != nil {
		return nil, err
	}
	iv := pkcs12KDF(sha1.New, pw, params.Salt, 2, params.Iterations, block.BlockSize())
	return cbcDecrypt(block, iv, data)
}

func pbes2Decrypt(paramsDER, data, password []byte) ([]byte, error) {
	var params struct {
		KDF pkix.AlgorithmIdentifier
		Enc pkix.AlgorithmIdentifier
	}
	if _, err := asn1.Unmarshal(paramsDER, &params); err != nil {
		return nil, fmt.Errorf("PBES2 parameters: %w", err)
	}
	if !params.KDF.Algorithm.Equal(oidPBKDF2) {
		return nil, fmt.Errorf("unsupported key derivation %v: only PBKDF2 is", params.KDF.Algorithm)
	}
	var kdf struct {
		Salt       []byte
		Iterations int
		KeyLength  int                      `asn1:"optional"`
		PRF        pkix.AlgorithmIdentifier `asn1:"optional"`
	}
	if _, err := asn1.Unmarshal(params.KDF.Parameters.FullBytes, &kdf); err != nil {
		return nil, fmt.Errorf("PBKDF2 parameters: %w", err)
	}
	if kdf.Iterations < 1 || kdf.Iterations > 10_000_000 {
		return nil, fmt.Errorf("unreasonable iteration count %d", kdf.Iterations)
	}
	prf := sha1.New
	switch p := kdf.PRF.Algorithm; {
	case len(p) == 0, p.Equal(oidHMACWithSHA1):
	case p.Equal(oidHMACWithSHA224):
		prf = sha256.New224
	case p.Equal(oidHMACWithSHA256):
		prf = sha256.New
	case p.Equal(oidHMACWithSHA384):
		prf = sha512.New384
	case p.Equal(oidHMACWithSHA512):
		prf = sha512.New
	default:
		return nil, fmt.Errorf("unsupported PBKDF2 function %v", p)
	}
	var keyLen int
	var newCipher func([]byte) (cipher.Block, error)
	switch e := params.Enc.Algorithm; {
	case e.Equal(oidAES128CBC):
		keyLen, newCipher = 16, aes.NewCipher
	case e.Equal(oidAES192CBC):
		keyLen, newCipher = 24, aes.NewCipher
	case e.Equal(oidAES256CBC):
		keyLen, newCipher = 32, aes.NewCipher
	case e.Equal(oidDESEDE3CBC):
		keyLen, newCipher = 24, des.NewTripleDESCipher
	default:
		return nil, fmt.Errorf("unsupported encryption %v", e)
	}
	var iv []byte
	if _, err := asn1.Unmarshal(params.Enc.Parameters.FullBytes, &iv); err != nil {
		return nil, fmt.Errorf("encryption parameters: %w", err)
	}
	key, err := pbkdf2.Key(prf, string(password), kdf.Salt, kdf.Iterations, keyLen)
	if err != nil {
		return nil, err
	}
	block, err := newCipher(key)
	if err != nil {
		return nil, err
	}
	return cbcDecrypt(block, iv, data)
}

// cbcDecrypt decrypts in CBC mode and removes the PKCS#7 padding. A bad
// padding means, nearly always, a wrong password.
func cbcDecrypt(block cipher.Block, iv, data []byte) ([]byte, error) {
	bs := block.BlockSize()
	if len(iv) != bs || len(data) == 0 || len(data)%bs != 0 {
		return nil, errors.New("malformed encrypted data")
	}
	out := make([]byte, len(data))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(out, data)
	n := int(out[len(out)-1])
	if n == 0 || n > bs {
		return nil, errBadPassword
	}
	for _, c := range out[len(out)-n:] {
		if int(c) != n {
			return nil, errBadPassword
		}
	}
	return out[:len(out)-n], nil
}

// bmpString encodes a password as PKCS#12 wants it: UTF-16 big endian
// followed by a null character.
func bmpString(s string) []byte {
	u := utf16.Encode([]rune(s))
	out := make([]byte, 0, 2*len(u)+2)
	for _, c := range u {
		out = append(out, byte(c>>8), byte(c))
	}
	return append(out, 0, 0)
}

// pkcs12KDF derives keys, IVs and MAC keys from a password (RFC 7292,
// appendix B.2). id is 1 for a key, 2 for an IV and 3 for a MAC key.
func pkcs12KDF(newHash func() hash.Hash, password, salt []byte, id byte, iterations, n int) []byte {
	h := newHash()
	u, v := h.Size(), h.BlockSize()
	fill := func(b []byte) []byte {
		if len(b) == 0 {
			return nil
		}
		out := make([]byte, v*((len(b)+v-1)/v))
		for i := range out {
			out[i] = b[i%len(b)]
		}
		return out
	}
	d := make([]byte, v)
	for i := range d {
		d[i] = id
	}
	in := append(fill(salt), fill(password)...)
	var out []byte
	for len(out) < n {
		h.Reset()
		h.Write(d)
		h.Write(in)
		a := h.Sum(nil)
		for range iterations - 1 {
			h.Reset()
			h.Write(a)
			a = h.Sum(a[:0])
		}
		out = append(out, a...)
		if len(out) >= n {
			break
		}
		// I_j = (I_j + B + 1) mod 2^(8v), B being A repeated to v bytes.
		b := make([]byte, v)
		for i := range b {
			b[i] = a[i%u]
		}
		for j := 0; j < len(in); j += v {
			carry := 1
			for k := v - 1; k >= 0; k-- {
				s := int(in[j+k]) + int(b[k]) + carry
				in[j+k] = byte(s)
				carry = s >> 8
			}
		}
	}
	return out[:n]
}
