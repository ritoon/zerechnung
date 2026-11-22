import eu.europa.esig.dss.cades.CAdESSignatureParameters;
import eu.europa.esig.dss.cades.signature.CAdESService;
import eu.europa.esig.dss.detailedreport.DetailedReport;
import eu.europa.esig.dss.detailedreport.jaxb.XmlBasicBuildingBlocks;
import eu.europa.esig.dss.detailedreport.jaxb.XmlConstraintsConclusion;
import eu.europa.esig.dss.diagnostic.DiagnosticData;
import eu.europa.esig.dss.diagnostic.SignatureWrapper;
import eu.europa.esig.dss.diagnostic.TimestampWrapper;
import eu.europa.esig.dss.diagnostic.jaxb.XmlDigestMatcher;
import eu.europa.esig.dss.enumerations.DigestAlgorithm;
import eu.europa.esig.dss.enumerations.EncryptionAlgorithm;
import eu.europa.esig.dss.enumerations.Indication;
import eu.europa.esig.dss.enumerations.SignatureLevel;
import eu.europa.esig.dss.enumerations.SignaturePackaging;
import eu.europa.esig.dss.model.DSSDocument;
import eu.europa.esig.dss.model.FileDocument;
import eu.europa.esig.dss.model.SignatureValue;
import eu.europa.esig.dss.model.ToBeSigned;
import eu.europa.esig.dss.model.x509.CertificateToken;
import eu.europa.esig.dss.pades.PAdESSignatureParameters;
import eu.europa.esig.dss.pades.signature.PAdESService;
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
import java.util.List;

/**
 * DssCheck runs EU DSS for conformance/signature.sh.
 *
 * <pre>
 * java DssCheck.java verify root.pem intermediate.pem file...
 * java DssCheck.java sign signer.p12 password fpa12.xml outdir name [pss]
 * java DssCheck.java verify-pades root.pem intermediate.pem file.pdf...
 * java DssCheck.java sign-pades signer.p12 password invoice.pdf outdir name [pss]
 * </pre>
 *
 * verify validates CAdES and XAdES files with root.pem as the only trust
 * anchor, without revocation data, and fails unless DSS finds each file a
 * single Baseline B signature whose format, signing certificate reference,
 * digests and signature value pass. The certificate validation itself stays
 * indeterminate, since the test PKI publishes no revocation data.
 *
 * sign signs fpa12.xml in XAdES Baseline B enveloped (dss-xades-name.xml)
 * and in CAdES Baseline B enveloping (dss-cades-name.xml.p7m), with RSA-PSS
 * when pss is given.
 *
 * verify-pades validates signed PDF the same way, and fails unless every
 * signature passes those checks at the PAdES level its file name gives
 * (pades-bb-: PAdES-BASELINE-B, pades-bt-: PAdES-BASELINE-T, pades-lta-:
 * PAdES-BASELINE-LTA), its byte range and signature dictionary are valid,
 * no undefined change follows it, and every time-stamp verifies and leads
 * to the trusted root.
 *
 * sign-pades signs invoice.pdf in PAdES Baseline B (dss-pades-name.pdf),
 * with RSA-PSS when pss is given.
 */
public class DssCheck {
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
        } else if (args.length >= 4 && args[0].equals("verify-pades")) {
            int failed = 0;
            for (int i = 3; i < args.length; i++) {
                List<String> errors = verifyPades(args[1], args[2], args[i]);
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
            sign(args[1], args[2], args[3], args[4], args[5], args.length > 6 && args[6].equals("pss"));
        } else if (args.length >= 6 && args[0].equals("sign-pades")) {
            signPades(args[1], args[2], args[3], args[4], args[5], args.length > 6 && args[6].equals("pss"));
        } else {
            System.err.println("usage: DssCheck verify root.pem intermediate.pem file...");
            System.err.println("       DssCheck sign signer.p12 password doc.xml outdir name [pss]");
            System.err.println("       DssCheck verify-pades root.pem intermediate.pem file.pdf...");
            System.err.println("       DssCheck sign-pades signer.p12 password invoice.pdf outdir name [pss]");
            System.exit(2);
        }
    }

    static Reports validate(String root, String intermediate, String file) {
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
        return validator.validateDocument();
    }

    static List<String> verify(String root, String intermediate, String file) {
        Reports reports = validate(root, intermediate, file);
        DiagnosticData dd = reports.getDiagnosticData();
        DetailedReport dr = reports.getDetailedReport();

        List<String> errors = new ArrayList<>();
        List<SignatureWrapper> sigs = dd.getSignatures();
        if (sigs.size() != 1) {
            errors.add(sigs.size() + " signatures, want 1");
            return errors;
        }
        SignatureWrapper sig = sigs.get(0);
        String level = String.valueOf(sig.getSignatureFormat());
        System.out.println("  " + file + ": " + level + ", "
                + sig.getSignatureAlgorithm() + ", indication "
                + reports.getSimpleReport().getIndication(sig.getId()) + " "
                + reports.getSimpleReport().getSubIndication(sig.getId()));
        if (sig.getSignatureFormat() != SignatureLevel.XAdES_BASELINE_B
                && sig.getSignatureFormat() != SignatureLevel.CAdES_BASELINE_B) {
            errors.add("format " + level + ", want XAdES-BASELINE-B or CAdES-BASELINE-B");
        }
        checkSignature(errors, sig, dr);
        return errors;
    }

    /** checkSignature adds the errors of the checks common to every format. */
    static void checkSignature(List<String> errors, SignatureWrapper sig, DetailedReport dr) {
        if (!sig.isSignatureIntact() || !sig.isSignatureValid()) {
            errors.add("signature value does not verify");
        }
        if (!sig.isStructuralValidationValid()) {
            errors.add("structure (XSD): " + sig.getStructuralValidationMessages());
        }
        if (!sig.isBLevelTechnicallyValid()) {
            errors.add("B-level not valid");
        }
        for (XmlDigestMatcher m : sig.getDigestMatchers()) {
            if (!m.isDataFound() || !m.isDataIntact()) {
                errors.add("digest " + m.getType() + " " + m.getUri() + ": found "
                        + m.isDataFound() + ", intact " + m.isDataIntact());
            }
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
    }

    static List<String> verifyPades(String root, String intermediate, String file) {
        Reports reports = validate(root, intermediate, file);
        DiagnosticData dd = reports.getDiagnosticData();
        DetailedReport dr = reports.getDetailedReport();
        String base = new File(file).getName();
        SignatureLevel want = base.startsWith("pades-bt-") ? SignatureLevel.PAdES_BASELINE_T
                : base.startsWith("pades-lta-") ? SignatureLevel.PAdES_BASELINE_LTA
                : SignatureLevel.PAdES_BASELINE_B;

        List<String> errors = new ArrayList<>();
        List<SignatureWrapper> sigs = dd.getSignatures();
        if (sigs.isEmpty()) {
            errors.add("no signature");
        }
        for (SignatureWrapper sig : sigs) {
            String name = sig.getFirstFieldName();
            System.out.println("  " + base + " " + name + ": " + sig.getSignatureFormat() + ", "
                    + sig.getSignatureAlgorithm() + ", DocMDP " + sig.getDocMDPPermissions()
                    + ", indication " + reports.getSimpleReport().getIndication(sig.getId()) + " "
                    + reports.getSimpleReport().getSubIndication(sig.getId()));
            List<String> errs = new ArrayList<>();
            if (sig.getSignatureFormat() != want) {
                errs.add("format " + sig.getSignatureFormat() + ", want " + want);
            }
            if (!sig.isSignatureByteRangeValid()) {
                errs.add("byte range not valid: " + sig.getSignatureByteRange());
            }
            if (!sig.isPdfSignatureDictionaryConsistent()) {
                errs.add("signature dictionary not consistent");
            }
            if (!sig.getPdfUndefinedChanges().isEmpty()) {
                errs.add("undefined changes after the signature: " + sig.getPdfUndefinedChanges().size());
            }
            checkSignature(errs, sig, dr);
            for (String e : errs) {
                errors.add(name + ": " + e);
            }
        }
        for (TimestampWrapper ts : dd.getTimestampList()) {
            System.out.println("  " + base + " time-stamp " + ts.getType() + " at " + ts.getProductionTime().toInstant());
            if (!ts.isMessageImprintDataFound() || !ts.isMessageImprintDataIntact()) {
                errors.add("time-stamp " + ts.getType() + ": the message imprint does not match");
            }
            if (!ts.isSignatureIntact() || !ts.isSignatureValid()) {
                errors.add("time-stamp " + ts.getType() + ": the signature does not verify");
            }
            if (ts.getSigningCertificate() == null || !ts.getSigningCertificate().isTrustedChain()) {
                errors.add("time-stamp " + ts.getType() + ": no chain to the trusted root");
            }
        }
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

    static void sign(String p12, String password, String docPath, String outDir, String name, boolean pss)
            throws Exception {
        DSSDocument doc = new FileDocument(docPath);
        try (Pkcs12SignatureToken token = new Pkcs12SignatureToken(new File(p12),
                new KeyStore.PasswordProtection(password.toCharArray()))) {
            DSSPrivateKeyEntry key = token.getKeys().get(0);
            CertificateToken cert = key.getCertificate();
            CommonCertificateVerifier cv = new CommonCertificateVerifier();

            XAdESSignatureParameters xp = new XAdESSignatureParameters();
            xp.setSignatureLevel(SignatureLevel.XAdES_BASELINE_B);
            xp.setSignaturePackaging(SignaturePackaging.ENVELOPED);
            xp.setDigestAlgorithm(DigestAlgorithm.SHA256);
            xp.setSigningCertificate(cert);
            xp.setCertificateChain(key.getCertificateChain());
            if (pss) {
                xp.setEncryptionAlgorithm(EncryptionAlgorithm.RSASSA_PSS);
            }
            XAdESService xs = new XAdESService(cv);
            ToBeSigned tbs = xs.getDataToSign(doc, xp);
            SignatureValue sv = token.sign(tbs, xp.getSignatureAlgorithm(), key);
            File xout = new File(outDir, "dss-xades-" + name + ".xml");
            xs.signDocument(doc, xp, sv).save(xout.getPath());
            System.out.println("wrote " + xout);

            CAdESSignatureParameters cp = new CAdESSignatureParameters();
            cp.setSignatureLevel(SignatureLevel.CAdES_BASELINE_B);
            cp.setSignaturePackaging(SignaturePackaging.ENVELOPING);
            cp.setDigestAlgorithm(DigestAlgorithm.SHA256);
            cp.setSigningCertificate(cert);
            cp.setCertificateChain(key.getCertificateChain());
            if (pss) {
                cp.setEncryptionAlgorithm(EncryptionAlgorithm.RSASSA_PSS);
            }
            CAdESService cs = new CAdESService(cv);
            tbs = cs.getDataToSign(doc, cp);
            sv = token.sign(tbs, cp.getSignatureAlgorithm(), key);
            File cout = new File(outDir, "dss-cades-" + name + ".xml.p7m");
            cs.signDocument(doc, cp, sv).save(cout.getPath());
            System.out.println("wrote " + cout);
        }
    }

    static void signPades(String p12, String password, String pdfPath, String outDir, String name, boolean pss)
            throws Exception {
        DSSDocument doc = new FileDocument(pdfPath);
        try (Pkcs12SignatureToken token = new Pkcs12SignatureToken(new File(p12),
                new KeyStore.PasswordProtection(password.toCharArray()))) {
            DSSPrivateKeyEntry key = token.getKeys().get(0);
            PAdESSignatureParameters pp = new PAdESSignatureParameters();
            pp.setSignatureLevel(SignatureLevel.PAdES_BASELINE_B);
            pp.setDigestAlgorithm(DigestAlgorithm.SHA256);
            pp.setSigningCertificate(key.getCertificate());
            pp.setCertificateChain(key.getCertificateChain());
            pp.setReason("Facture");
            pp.setLocation("Bruxelles");
            if (pss) {
                pp.setEncryptionAlgorithm(EncryptionAlgorithm.RSASSA_PSS);
            }
            PAdESService ps = new PAdESService(new CommonCertificateVerifier());
            ToBeSigned tbs = ps.getDataToSign(doc, pp);
            SignatureValue sv = token.sign(tbs, pp.getSignatureAlgorithm(), key);
            File out = new File(outDir, "dss-pades-" + name + ".pdf");
            ps.signDocument(doc, pp, sv).save(out.getPath());
            System.out.println("wrote " + out);
        }
    }
}
