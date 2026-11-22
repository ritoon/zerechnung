import eu.europa.esig.dss.detailedreport.DetailedReport;
import eu.europa.esig.dss.detailedreport.jaxb.XmlBasicBuildingBlocks;
import eu.europa.esig.dss.detailedreport.jaxb.XmlConstraintsConclusion;
import eu.europa.esig.dss.diagnostic.DiagnosticData;
import eu.europa.esig.dss.diagnostic.SignatureWrapper;
import eu.europa.esig.dss.diagnostic.jaxb.XmlDigestMatcher;
import eu.europa.esig.dss.diagnostic.jaxb.XmlSignerRole;
import eu.europa.esig.dss.enumerations.DigestAlgorithm;
import eu.europa.esig.dss.enumerations.DigestMatcherType;
import eu.europa.esig.dss.enumerations.EncryptionAlgorithm;
import eu.europa.esig.dss.enumerations.Indication;
import eu.europa.esig.dss.enumerations.SignatureLevel;
import eu.europa.esig.dss.enumerations.SignaturePackaging;
import eu.europa.esig.dss.model.DSSDocument;
import eu.europa.esig.dss.model.FileDocument;
import eu.europa.esig.dss.model.Policy;
import eu.europa.esig.dss.model.SignatureValue;
import eu.europa.esig.dss.model.ToBeSigned;
import eu.europa.esig.dss.model.x509.CertificateToken;
import eu.europa.esig.dss.spi.DSSUtils;
import eu.europa.esig.dss.spi.validation.CommonCertificateVerifier;
import eu.europa.esig.dss.spi.x509.CommonCertificateSource;
import eu.europa.esig.dss.spi.x509.CommonTrustedCertificateSource;
import eu.europa.esig.dss.token.DSSPrivateKeyEntry;
import eu.europa.esig.dss.token.Pkcs12SignatureToken;
import eu.europa.esig.dss.validation.SignedDocumentValidator;
import eu.europa.esig.dss.validation.reports.Reports;
import eu.europa.esig.dss.xades.XAdESSignatureParameters;
import eu.europa.esig.dss.xades.signature.XAdESService;

import java.io.File;
import java.security.KeyStore;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Base64;
import java.util.List;

/**
 * FacturaeCheck runs EU DSS on the Facturae signatures of package facturae,
 * for conformance/signature.sh. It stands apart from DssCheck, which checks
 * the FatturaPA signatures.
 *
 * <pre>
 * java FacturaeCheck.java verify root.pem intermediate.pem file...
 * java FacturaeCheck.java sign signer.p12 password b2g.xml outdir name [pss] [v1]
 * </pre>
 *
 * verify validates XAdES files with root.pem as the only trust anchor,
 * without revocation data, and fails unless DSS finds each file a single
 * XAdES-EPES signature under the signature policy of Facturae v3.1
 * (identifier and SHA-1 digest), with the role emisor and ds:KeyInfo
 * signed, whose format, signing certificate reference, digests and
 * signature value pass. The policy document itself is not fetched, and the
 * certificate validation stays indeterminate, since the test PKI publishes
 * no revocation data.
 *
 * sign signs b2g.xml in XAdES enveloped under the policy of Facturae, with
 * the role emisor and ds:KeyInfo signed (dss-facturae-name.xml): in
 * EN 319 132-1 (SigningCertificateV2, SignerRoleV2), or in XAdES 1.3.2
 * (SigningCertificate, SignerRole) when v1 is given; with RSA-PSS when pss
 * is given.
 */
public class FacturaeCheck {
    static final String POLICY_ID = "http://www.facturae.es/politica_de_firma_formato_facturae/politica_de_firma_formato_facturae_v3_1.pdf";
    static final String POLICY_DIGEST = "Ohixl6upD6av8N7pEvDABhEL6hM=";

    public static void main(String[] args) throws Exception {
        if (args.length >= 4 && args[0].equals("verify")) {
            int failed = 0;
            for (int i = 3; i < args.length; i++) {
                List<String> errors = verify(args[1], args[2], args[i]);
                if (errors.isEmpty()) {
                    System.out.println("PASS " + args[i]);
                } else {
                    failed++;
                    System.out.println("FAIL " + args[i]);
                    for (String e : errors) {
                        System.out.println("  " + e);
                    }
                }
            }
            System.exit(failed == 0 ? 0 : 1);
        } else if (args.length >= 6 && args[0].equals("sign")) {
            List<String> flags = Arrays.asList(args).subList(6, args.length);
            sign(args[1], args[2], args[3], args[4], args[5], flags.contains("pss"), flags.contains("v1"));
        } else {
            System.err.println("usage: FacturaeCheck verify root.pem intermediate.pem file...");
            System.err.println("       FacturaeCheck sign signer.p12 password b2g.xml outdir name [pss] [v1]");
            System.exit(2);
        }
    }

    static List<String> verify(String root, String intermediate, String file) {
        CommonTrustedCertificateSource trusted = new CommonTrustedCertificateSource();
        trusted.addCertificate(DSSUtils.loadCertificate(new File(root)));
        CommonCertificateSource adjunct = new CommonCertificateSource();
        adjunct.addCertificate(DSSUtils.loadCertificate(new File(intermediate)));
        CommonCertificateVerifier cv = new CommonCertificateVerifier();
        cv.setTrustedCertSources(trusted);
        cv.setAdjunctCertSources(adjunct);
        cv.setCrlSource(null);
        cv.setOcspSource(null);
        cv.setAIASource(null);

        SignedDocumentValidator validator = SignedDocumentValidator.fromDocument(new FileDocument(file));
        validator.setCertificateVerifier(cv);
        Reports reports = validator.validateDocument();
        DiagnosticData dd = reports.getDiagnosticData();
        DetailedReport dr = reports.getDetailedReport();

        List<String> errors = new ArrayList<>();
        List<SignatureWrapper> sigs = dd.getSignatures();
        if (sigs.size() != 1) {
            errors.add(sigs.size() + " signatures, want 1");
            return errors;
        }
        SignatureWrapper sig = sigs.get(0);
        List<String> roles = new ArrayList<>();
        for (XmlSignerRole r : sig.getClaimedRoles()) {
            roles.add(r.getRole());
        }
        System.out.println("  " + file + ": " + sig.getSignatureFormat() + ", "
                + sig.getSignatureAlgorithm() + ", policy " + sig.getPolicyId() + ", roles " + roles
                + ", indication " + reports.getSimpleReport().getIndication(sig.getId()) + " "
                + reports.getSimpleReport().getSubIndication(sig.getId()));
        SignatureLevel level = sig.getSignatureFormat();
        if (level != SignatureLevel.XAdES_EPES && level != SignatureLevel.XAdES_BASELINE_B) {
            errors.add("format " + level + ", want XAdES-EPES or XAdES-BASELINE-B");
        }
        if (!POLICY_ID.equals(sig.getPolicyId())) {
            errors.add("policy " + sig.getPolicyId() + ", want " + POLICY_ID);
        }
        if (sig.getPolicyDigestAlgoAndValue() == null
                || sig.getPolicyDigestAlgoAndValue().getDigestMethod() != DigestAlgorithm.SHA1
                || !POLICY_DIGEST.equals(Base64.getEncoder().encodeToString(sig.getPolicyDigestAlgoAndValue().getDigestValue()))) {
            errors.add("policy digest is not the SHA-1 digest " + POLICY_DIGEST);
        }
        if (!roles.equals(List.of("emisor"))) {
            errors.add("claimed roles " + roles + ", want [emisor]");
        }
        if (!sig.isSignatureIntact() || !sig.isSignatureValid()) {
            errors.add("signature value does not verify");
        }
        if (!sig.isStructuralValidationValid()) {
            errors.add("structure (XSD): " + sig.getStructuralValidationMessages());
        }
        boolean keyInfo = false;
        for (XmlDigestMatcher m : sig.getDigestMatchers()) {
            keyInfo |= m.getType() == DigestMatcherType.KEY_INFO;
            if (!m.isDataFound() || !m.isDataIntact()) {
                errors.add("digest " + m.getType() + " " + m.getUri() + ": found "
                        + m.isDataFound() + ", intact " + m.isDataIntact());
            }
        }
        if (!keyInfo) {
            errors.add("ds:KeyInfo is not signed");
        }
        if (!sig.isSigningCertificateIdentified()) {
            errors.add("signing certificate not identified");
        }
        if (sig.getSigningCertificate() == null || !sig.getSigningCertificate().isTrustedChain()) {
            errors.add("no chain to the trusted root");
        }
        XmlBasicBuildingBlocks bbb = dr.getBasicBuildingBlockById(sig.getId());
        check(errors, "format checking (FC)", bbb.getFC());
        check(errors, "identification of the signing certificate (ISC)", bbb.getISC());
        check(errors, "validation context initialization (VCI)", bbb.getVCI());
        check(errors, "cryptographic verification (CV)", bbb.getCV());
        check(errors, "signature acceptance validation (SAV)", bbb.getSAV());
        return errors;
    }

    static void check(List<String> errors, String name, XmlConstraintsConclusion block) {
        if (block == null) {
            errors.add(name + ": not run");
            return;
        }
        Indication ind = block.getConclusion().getIndication();
        if (ind != Indication.PASSED) {
            errors.add(name + ": " + ind + " " + block.getConclusion().getSubIndication()
                    + " " + block.getConclusion().getErrors());
        }
    }

    static void sign(String p12, String password, String docPath, String outDir, String name, boolean pss, boolean v1)
            throws Exception {
        DSSDocument doc = new FileDocument(docPath);
        try (Pkcs12SignatureToken token = new Pkcs12SignatureToken(new File(p12),
                new KeyStore.PasswordProtection(password.toCharArray()))) {
            DSSPrivateKeyEntry key = token.getKeys().get(0);
            CertificateToken cert = key.getCertificate();

            XAdESSignatureParameters xp = new XAdESSignatureParameters();
            xp.setSignatureLevel(SignatureLevel.XAdES_BASELINE_B);
            xp.setSignaturePackaging(SignaturePackaging.ENVELOPED);
            xp.setDigestAlgorithm(DigestAlgorithm.SHA256);
            xp.setSigningCertificate(cert);
            xp.setCertificateChain(key.getCertificateChain());
            xp.setSignKeyInfo(true);
            xp.setEn319132(!v1);
            if (pss) {
                xp.setEncryptionAlgorithm(EncryptionAlgorithm.RSASSA_PSS);
            }
            Policy policy = new Policy();
            policy.setId(POLICY_ID);
            policy.setDescription("Política de Firma FacturaE v3.1");
            policy.setDigestAlgorithm(DigestAlgorithm.SHA1);
            policy.setDigestValue(Base64.getDecoder().decode(POLICY_DIGEST));
            xp.bLevel().setSignaturePolicy(policy);
            xp.bLevel().setClaimedSignerRoles(List.of("emisor"));

            XAdESService xs = new XAdESService(new CommonCertificateVerifier());
            ToBeSigned tbs = xs.getDataToSign(doc, xp);
            SignatureValue sv = token.sign(tbs, xp.getSignatureAlgorithm(), key);
            File out = new File(outDir, "dss-facturae-" + name + ".xml");
            xs.signDocument(doc, xp, sv).save(out.getPath());
            System.out.println("wrote " + out);
        }
    }
}
