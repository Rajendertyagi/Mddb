# Generates sample documents for the Windows file-upload audit.
# Run from repo root:  python scripts/gen_upload_samples.py
# Produces samples/ with txt/md/html/yaml/json/csv/pdf/docx files.
# PDF and DOCX are hand-built (valid minimal structures) so the audit
# does not depend on third-party document libraries at test time.
import zipfile, os

os.makedirs("samples", exist_ok=True)

# TXT
open("samples/doc.txt", "w", encoding="utf-8").write(
    "# Upload audit\nUNIQTXT plain text document for the windows audit."
)
# Markdown
open("samples/doc.md", "w", encoding="utf-8").write(
    "# Markdown audit\nUNIQMD Hello from markdown upload."
)
# HTML
open("samples/doc.html", "w", encoding="utf-8").write(
    "<html><body><h1>HTML audit</h1><p>UNIQHTML Hello from html upload.</p></body></html>"
)
# YAML
open("samples/doc.yaml", "w", encoding="utf-8").write(
    "title: YAML audit\nbody: UNIQYAML Hello from yaml upload.\n"
)
# JSON (unsupported format -> expect 400)
open("samples/doc.json", "w", encoding="utf-8").write('{"title":"JSON audit","body":"hello"}')
# CSV (unsupported format -> expect 400)
open("samples/doc.csv", "w", encoding="utf-8").write("title,body\nCSV audit,hello\n")

# Minimal valid PDF (hand-built with correct xref offsets)
stream = b"BT /F1 12 Tf 72 720 Td (UNIQPDF Hello from PDF upload) Tj ET"
objs = [
    b"1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n",
    b"2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj\n",
    b"3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 612 792]/Contents 4 0 R/Resources<</Font<</F1 5 0 R>>>>>>endobj\n",
    b"4 0 obj<</Length %d>>stream\n" % len(stream) + stream + b"\nendstream endobj\n",
    b"5 0 obj<</Type/Font/Subtype/Type1/BaseFont/Helvetica>>endobj\n",
]
p1 = b"%PDF-1.4\n"
buf = p1
offsets = [0] * 6
for i, o in enumerate(objs, start=1):
    offsets[i] = len(buf)
    buf += o
xref_pos = len(buf)
xref = b"xref\n0 6\n"
xref += b"0000000000 65535 f \n"
for i in range(1, 6):
    xref += ("%010d 00000 n \n" % offsets[i]).encode()
xref += b"trailer\n<</Size 6/Root 1 0 R>>\nstartxref\n%d\n%%%%EOF\n" % xref_pos
open("samples/doc.pdf", "wb").write(buf + xref)

# Minimal valid DOCX (OOXML zip)
with zipfile.ZipFile("samples/doc.docx", "w", zipfile.ZIP_DEFLATED) as z:
    z.writestr(
        "[Content_Types].xml",
        '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
        '<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">'
        '<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>'
        '<Default Extension="xml" ContentType="application/xml"/>'
        '<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>'
        "</Types>",
    )
    z.writestr(
        "_rels/.rels",
        '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
        '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">'
        '<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>'
        "</Relationships>",
    )
    z.writestr(
        "word/document.xml",
        '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
        '<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">'
        "<w:body><w:p><w:r><w:t>UNIQDOCX Hello from DOCX upload</w:t></w:r></w:p></w:body></w:document>",
    )

print("samples generated:", sorted(os.listdir("samples")))
