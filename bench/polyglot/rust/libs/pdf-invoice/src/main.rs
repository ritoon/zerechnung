//! pdf-invoice 1.5.0 (PDFluent SDK, AGPL-3.0 or commercial): FDF/XFDF and
//! ZUGFeRD / Factur-X CII model, read, write, a few EN 16931 rules, and
//! attachment helpers on its lopdf fork (pdfluent-lopdf 0.39.9).

use harness::{Findings, Lib};
use pdf_invoice::embed::{AfRelationship, add_zugferd_xmp, embed_xml_attachment, extract_xml_attachment};
use pdf_invoice::zugferd::{ZugferdInvoice, ZugferdProfile};
use pdfluent_lopdf::Document;

const LIB: Lib = Lib { lib: "pdf-invoice", pkg: "pdf-invoice", version: "1.5.0" };

const NAMES: [&str; 5] = ["factur-x.xml", "zugferd-invoice.xml", "ZUGFeRD-invoice.xml", "xrechnung.xml", "order-x.xml"];

fn text(x: &[u8]) -> Result<&str, String> {
    std::str::from_utf8(x).map_err(|e| format!("not UTF-8: {e}"))
}

/// Adapter: load the PDF with the lopdf fork, then ask for each standard
/// attachment name (the crate looks one name up in the root /Names array).
fn extract(p: &[u8]) -> Result<Vec<u8>, String> {
    let doc = Document::load_mem(p).map_err(|e| e.to_string())?;
    for n in NAMES {
        if let Some(x) = extract_xml_attachment(&doc, n).map_err(|e| e.to_string())? {
            return Ok(x);
        }
    }
    Err("no attachment under a standard invoice name".into())
}

fn parse(x: &[u8]) -> Result<ZugferdInvoice, String> {
    ZugferdInvoice::from_xml(text(x)?).map_err(|e| e.to_string())
}

/// Profile requirements (`validate`) and the EN 16931 rules (`validate_en16931`).
fn validate(x: &[u8]) -> Result<(Vec<String>, pdf_invoice::En16931ValidationResult), String> {
    let inv = parse(x)?;
    Ok((inv.validate(), pdf_invoice::validate_en16931(&inv)))
}

fn summarize(r: &(Vec<String>, pdf_invoice::En16931ValidationResult)) -> Findings {
    let mut rules: Vec<String> = r.0.iter().map(|_| "PROFILE".to_owned()).collect();
    rules.extend(r.1.failed.iter().map(|(id, _)| id.clone()));
    Findings { errors: rules.len(), rules }
}

fn write(inv: &ZugferdInvoice) -> Result<String, String> {
    inv.to_xml().map_err(|e| e.to_string())
}

/// Adapter: load the host, add the attachment and the XMP, save.
fn embed(host: &[u8], xml: &[u8]) -> Result<Vec<u8>, String> {
    let mut doc = Document::load_mem(host).map_err(|e| e.to_string())?;
    embed_xml_attachment(&mut doc, "factur-x.xml", xml, AfRelationship::Alternative).map_err(|e| e.to_string())?;
    add_zugferd_xmp(&mut doc, ZugferdProfile::EN16931).map_err(|e| e.to_string())?;
    let mut out = Vec::with_capacity(host.len() + xml.len() + 8192);
    doc.save_to(&mut out).map_err(|e| e.to_string())?;
    Ok(out)
}

fn main() {
    harness::run(LIB, |h| {
        h.extract(
            "adaptateur : Document::load_mem (pdfluent-lopdf) + extract_xml_attachment essayé pour 5 noms standard",
            extract,
        );
        h.parse("ZugferdInvoice::from_xml (montants en f64)", parse);
        h.validate(
            "adaptateur : from_xml + validate (exigences du profil) + validate_en16931 (14 règles : BR-01 à BR-08, BR-CL-01, BR-CL-04, BR-CO-10, BR-CO-13, BR-CO-15, BR-S-08) ; pas de XSD",
            validate,
            summarize,
        );
        h.write("to_xml (valide d'abord le profil et les 14 règles EN 16931, refuse d'écrire sinon), modèle lu par from_xml", parse, write);
        h.embed(
            "adaptateur : Document::load_mem + embed_xml_attachment (Alternative) + add_zugferd_xmp + save_to ; réécrit tout le fichier, remplace le XMP sans conversion PDF/A",
            embed,
            extract,
        );
        h.unsupported(harness::Op::CheckPdf, "pas de contrôle de PDF hybride");
    });
}
