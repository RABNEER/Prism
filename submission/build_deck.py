"""Build the editable deck and matching PDF from one slide layout."""

from pathlib import Path

from pptx import Presentation
from pptx.dml.color import RGBColor
from pptx.enum.shapes import MSO_SHAPE
from pptx.enum.text import MSO_ANCHOR
from pptx.util import Pt
from reportlab.lib.colors import HexColor
from reportlab.pdfbase import pdfmetrics
from reportlab.pdfbase.ttfonts import TTFont
from reportlab.pdfgen.canvas import Canvas


OUT = Path(__file__).resolve().parent
NAME = "PRISM - Hero Housing Solution Deck"
W, H = 960, 540
NAVY, INK, PAPER = "102132", "152C40", "F5F5EF"
TEAL, PALE, LIME = "09675B", "DAEDE5", "DEF978"
WHITE, MUTE, SOFT, LINE = "FFFFFF", "476074", "B4C7D4", "D9E2E2"
AMBER, DARK2 = "F9ECCF", "1B3448"
FONT_PATH = Path("C:/Windows/Fonts")
pdfmetrics.registerFont(TTFont("A", FONT_PATH / "arial.ttf"))
pdfmetrics.registerFont(TTFont("AB", FONT_PATH / "arialbd.ttf"))

ppt = Presentation()
ppt.slide_width, ppt.slide_height = Pt(W), Pt(H)
ppt.core_properties.title = "PRISM | Hero Housing Fintech Challenge"
ppt.core_properties.subject = "Problem Statement 1 — AI-Powered Digital Property Valuation Engine for India"
ppt.core_properties.author = "Team PRISM"
pdf = Canvas(str(OUT / f"{NAME}.pdf"), pagesize=(W, H))
pdf.setTitle(ppt.core_properties.title)
pdf.setAuthor("Team PRISM")
slide = None
page = 0


def rect(x, y, w, h, color):
    assert 0 <= x < x + w <= W and 0 <= y < y + h <= H
    shape = slide.shapes.add_shape(MSO_SHAPE.RECTANGLE, Pt(x), Pt(y), Pt(w), Pt(h))
    shape.fill.solid()
    shape.fill.fore_color.rgb = RGBColor.from_string(color)
    shape.line.fill.background()
    pdf.setFillColor(HexColor("#" + color))
    pdf.rect(x, H - y - h, w, h, fill=1, stroke=0)


def txt(value, x, y, w, size=17, color=INK, bold=False, leading=None, link=None):
    font = "AB" if bold else "A"
    leading = leading or size * 1.3
    assert x + w <= W and x >= 0 and y >= 0
    lines = []
    for paragraph in value.split("\n"):
        line = ""
        for word in paragraph.split():
            candidate = (line + " " + word).strip()
            if line and pdfmetrics.stringWidth(candidate, font, size) > w - 4:
                lines.append(line)
                line = word
            else:
                line = candidate
        lines.append(line)
    for i, line in enumerate(lines):
        top = y + i * leading
        assert top + size * 1.22 <= H, (page, line, top)
        assert pdfmetrics.stringWidth(line, font, size) <= w, (page, line)
        box = slide.shapes.add_textbox(Pt(x), Pt(top), Pt(w), Pt(size * 1.25))
        frame = box.text_frame
        frame.word_wrap = False
        frame.vertical_anchor = MSO_ANCHOR.TOP
        frame.margin_left = frame.margin_right = frame.margin_top = frame.margin_bottom = 0
        paragraph = frame.paragraphs[0]
        paragraph.space_before = paragraph.space_after = Pt(0)
        run = paragraph.add_run()
        run.text = line
        run.font.name, run.font.size, run.font.bold = "Arial", Pt(size), bold
        if link:
            run.hyperlink.address = link
            run.font.underline = True
            pdf.linkURL(link, (x, H - top - size - 3, x + pdfmetrics.stringWidth(line, font, size), H - top), relative=0)
        run.font.color.rgb = RGBColor.from_string(color)
        pdf.setFont(font, size)
        pdf.setFillColor(HexColor("#" + color))
        pdf.drawString(x, H - top - size * .95, line)
    return len(lines) * leading


def start(label, title=None, intro=None, dark=False):
    global slide, page
    if slide is not None:
        pdf.showPage()
    slide = ppt.slides.add_slide(ppt.slide_layouts[6])
    page += 1
    rect(0, 0, W, H, NAVY if dark else PAPER)
    rect(54, 499, 852, 1, DARK2 if dark else LINE)
    txt("PRISM   /   HERO HOUSING FINTECH CHALLENGE", 54, 510, 540, 10, SOFT if dark else MUTE, True)
    txt(f"{page:02d} / 10", 855, 508, 53, 11, SOFT if dark else MUTE, True)
    txt(label, 54, 33, 852, 11, LIME if dark else TEAL, True)
    if title:
        txt(title, 54, 58, 852, 34, WHITE if dark else INK, True)
    if intro:
        txt(intro, 54, 110, 852, 16, SOFT if dark else MUTE)


def card(x, y, w, h, label, title, body, dark=False, highlight=False):
    rect(x, y, w, h, DARK2 if dark else (PALE if highlight else WHITE))
    accent = LIME if dark else TEAL
    txt(label, x + 17, y + 15, w - 34, 10, accent, True)
    shift = txt(title, x + 17, y + 37, w - 34, 20, WHITE if dark else INK, True, 23)
    body_h = txt(body, x + 17, y + 46 + shift, w - 34, 15, SOFT if dark else MUTE, leading=19)
    assert 46 + shift + body_h <= h + 1, (page, label)


def banner(y, value, dark=False):
    rect(54, y, 852, 46, LIME if dark else PALE)
    txt(value, 70, y + 13, 818, 15, INK, True)


start("E-CELL, IIT BOMBAY   /   HERO HOUSING FINTECH CHALLENGE", dark=True)
rect(54, 92, 256, 25, DARK2)
txt("PROBLEM STATEMENT 01   /   SOLUTION DECK", 65, 98, 241, 9, LIME, True)
txt("PRISM", 48, 136, 490, 88, WHITE, True)
txt("Property Research, Intelligence & Smart Modeling", 56, 241, 510, 17, SOFT)
txt("An auditable property\nvaluation engine for India", 55, 291, 554, 32, WHITE, True, 39)
txt("Pune apartment pilot  /  Team PRISM", 56, 428, 524, 15, LIME, True)
for i, (label, body) in enumerate([
    ("01   EVIDENCE", "Unify fragmented property data"),
    ("02   ESTIMATE", "Price the asset and show uncertainty"),
    ("03   REVIEW", "Route the case with an audit trail"),
]):
    y = 145 + 83 * i
    rect(615 + i * 12, y, 291 - i * 12, 72, DARK2)
    rect(615 + i * 12, y, 3, 72, LIME)
    txt(label, 631 + i * 12, y + 13, 249 - i * 12, 11, LIME, True)
    txt(body, 631 + i * 12, y + 34, 249 - i * 12, 15, WHITE)

start("01   /   SCOPE THE MARKET", "Start with Pune apartments",
      "Lenders must reconcile conflicting records, inconsistent area definitions and uneven local evidence.")
card(54, 154, 271, 205, "FIRST MARKET", "Baner + Kharadi",
     "Pilot for residential apartments. Baner has a live ingestion test. Kharadi is enabled for discovery and is the next pilot check.", highlight=True)
card(344, 154, 271, 205, "RESIDENTIAL EXPANSION", "State by state",
     "Expand across Pune, then test other metros. Add registry adapters and validate local coverage, error and interval calibration before launch.")
card(635, 154, 271, 205, "COMMERCIAL DESIGN", "Separate model",
     "Start with office and retail. Use lease comps, vacancy and cap rates. Cross-check stabilized net operating income ÷ cap rate against sale comps.")
banner(407, "Success means faster lender review with measurable valuation error and clear evidence gaps.")

start("02   /   SOLUTION ARCHITECTURE", "The MCP makes evidence reusable",
      "A property request becomes a source-linked evidence package for valuation and lender review.", dark=True)
steps = [
    ("INPUT", "Property request", "Address, property type, area basis and case date."),
    ("WORKING PROTOTYPE", "Evidence MCP", "Discover sources, normalize records, flag conflicts and select comparable candidates."),
    ("PROPOSED", "Laya + valuation", "Score evidence, estimate value from verified comps and calibrate a range."),
    ("PROPOSED", "Review report", "Return factors, source IDs, risk flags and a routing recommendation."),
]
for i, (label, title, body) in enumerate(steps):
    x = 54 + i * 219
    card(x, 174, 195, 195, label, title, body, dark=True)
    if i < 3:
        txt("→", x + 196, 258, 22, 18, LIME, True)
banner(412, "Go MCP   /   Rust reconciliation   /   Python bridge + geodata   /   Next.js / React viewer", dark=True)

start("02   /   COMBINE DATA SOURCES", "Keep each source in its proper role",
      "Keep source URLs, retrieval times, event dates and immutable snapshots with every record.")
columns = [(70, 228), (304, 404), (714, 176)]
rect(54, 150, 852, 29, NAVY)
for (x, w), label in zip(columns, ["SOURCE", "USE AND TRANSFORMATION", "PILOT STATUS"]):
    txt(label, x, 158, w, 10, WHITE, True)
rows = [
    ("IGR Maharashtra + lender deeds", "Normalize registered sale price and area. Use government reference rates as separate features.", "Access pending"),
    ("MahaRERA + property inputs", "Join project ID and attributes. Flag disagreement on area or address.", "Project lookup"),
    ("Public listings via Firecrawl", "Extract asking price, area basis and date. Track freshness and local asking-price trends.", "Live Baner audit"),
    ("Overture + Sentinel-2", "Compute nearby amenities and distances. Add vegetation and built-up proxies from cloud-masked imagery.", "Map live / imagery planned"),
    ("Maha Metro + public notices", "Geocode dated projects. Keep announced and operational infrastructure separate.", "Planned"),
]
for i, values in enumerate(rows):
    y = 179 + i * 49
    rect(54, y, 852, 48, WHITE if i % 2 == 0 else "EBEFEC")
    for j, ((x, w), value) in enumerate(zip(columns, values)):
        txt(value, x, y + 8, w, 13, TEAL if j == 2 else INK, j != 1, 16)
txt("Match by locality and project, then check unit identifiers, area basis and dates. Keep unresolved records separate.", 54, 439, 852, 15, TEAL, True)

start("02   /   WORKING PROTOTYPE", "What the Baner run proved",
      "The MCP discovered sources and ingested individual listing pages through local Firecrawl and Go parsing.", dark=True)
for x, value, label, body in [
    (54, "14", "MCP TOOLS", "Discovery, ingestion, geocoding, evidence lookup, reconciliation and comp selection."),
    (344, "10", "REAL LISTINGS INGESTED", "One portal. Each listing retained an asking price and a retrievable source snapshot."),
    (635, "0", "ELIGIBLE EXAMPLE COMPS", "The parser did not capture area or publication date. The comp selector returned no matches."),
]:
    rect(x, 164, 271, 221, DARK2)
    txt(value, x + 17, 177, 238, 67, LIME, True)
    txt(label, x + 17, 264, 238, 12, WHITE, True)
    txt(body, x + 17, 290, 233, 15, SOFT, leading=20)
txt("11 Nov 2026 IST  /  Baner, Pune  /  Example subject uses 1,000 sq ft of carpet area", 54, 432, 852, 14, SOFT)

start("03   /   MODEL AND ACCURACY", "Prove the lift on held-out sales",
      "Acquire and audit completed-sale records before training or claiming valuation accuracy.")
card(54, 151, 271, 202, "BENCHMARK", "Start with comps",
     "Use median registered sale price per sq ft with consistent area basis. Add a hedonic benchmark and the lender’s incumbent AVM if available.")
card(344, 151, 271, 202, "CANDIDATE", "Train CatBoost",
     "Use sale comps, property attributes, geodata and market signals. Test the lift from Laya evidence scoring and deduplication.", highlight=True)
card(635, 151, 271, 202, "VALIDATION", "Split by date",
     "Train on older sales, calibrate on a later window and test on the newest. Group repeated units and use only evidence known by the cutoff.")
rect(54, 374, 852, 95, NAVY)
txt("EVALUATION SCORECARD", 70, 388, 818, 10, LIME, True)
txt("MAE in INR   /   MAPE   /   % within ±10%   /   empirical coverage of 90% conformal ranges", 70, 407, 814, 15, WHITE, True)
txt("Compare on identical cases. Report sample size, interval width, abstention and error by locality.", 70, 433, 813, 14, SOFT)

start("04   /   EXPLAINABILITY", "Show the lender the evidence",
      "A versioned report separates the estimate, uncertainty, contributing factors and source quality.")
rect(54, 151, 477, 314, NAVY)
txt("SAVED EVIDENCE EXAMPLE", 72, 170, 440, 11, LIME, True)
txt("Baner apartment, Pune", 72, 196, 440, 25, WHITE, True)
txt("₹2.90 crore", 70, 233, 445, 44, WHITE, True)
txt("Publisher asking price", 73, 293, 438, 16, SOFT)
txt("Area and publication date missing in extracted record.", 73, 328, 435, 15, WHITE)
txt("Case status   /   Insufficient comparable evidence", 73, 366, 430, 15, LIME, True)
txt("Source snapshot   pkg_ad1907b89d90…", 73, 419, 429, 12, SOFT)
card(552, 151, 354, 314, "PROPOSED VALUATION REPORT", "Structured output",
     "Value and calibrated range, when supported.\n\nTop SHAP factors, comp IDs, evidence quality and model version.\n\nA narrative reads the JSON. Validate every number and source reference before display.")

start("06   /   LENDING WORKFLOW", "Route the case by evidence and risk",
      "Policy checks run first. PRISM recommends a review route and records the reason.")
for i, (label, condition, action) in enumerate([
    ("01", "Strong evidence + narrow range", "Desktop review if lender policy permits"),
    ("02", "Mixed evidence or wider range", "Analyst review and request missing evidence"),
    ("03", "Material conflict, weak evidence or mandatory check", "Physical valuation through the lender’s process"),
]):
    y = 155 + i * 58
    rect(54, y, 852, 54, [PALE, WHITE, AMBER][i])
    txt(label, 70, y + 15, 41, 15, TEAL, True)
    txt(condition, 118, y + 10, 353, 15, INK, True, 17)
    txt(action, 497, y + 10, 386, 15, INK, leading=17)
txt("Pilot measures", 54, 348, 850, 19, INK, True)
txt("Track physical referrals, missed high-risk cases, error by route and median and p95 turnaround in shadow mode.", 54, 378, 851, 16, MUTE)
txt("Stress-test LTV using the lower end of the range. Lender policy governs approved collateral values, mandatory inspections and valuer sign-off. See RBI HFC Directions, para. 215.", 54, 432, 852, 14, TEAL, True, 17)

start("05   /   AUTOMATION AND SCALE", "Scale only after the pilot passes",
      "Reuse the evidence schema and MCP tools while validating each new source and market.")
card(54, 157, 271, 227, "1   /   EVIDENCE PILOT", "Make the data usable",
     "Audit dates and area bases. Add authorized sale labels. Record source rights and measure completeness in both pilot localities.", highlight=True)
card(344, 157, 271, 227, "2   /   SHADOW PILOT", "Measure the workflow",
     "Run alongside lender decisions. Cache geodata, queue source jobs, measure latency and monitor missing fields, drift and interval coverage.")
card(635, 157, 271, 227, "3   /   EXPANSION", "Add markets in stages",
     "Add adapters for registries and city feeds. Calibrate by locality and asset class. Launch commercial only after lease and sale data pass review.")
banner(414, "Release gates   /   Authorized labels   /   Holdout lift   /   Interval coverage   /   Lender sign-off")

start("REFERENCES   /   DATA AND POLICY", "Sources and verification",
      "External references are clickable. Prototype results come from the team’s saved run artifacts.")
refs = [
    ("Challenge brief and organizer listing", "Problem Statement 1 and six requirements", "https://api.unstop.com/hackathons/hero-housing-fintech-challenge-iit-bombay-1802169"),
    ("Magicbricks Baner listing", "Source for the example asking-price record", "https://www.magicbricks.com/propertyDetails/3-BHK-1500-Sq-ft-Multistorey-Apartment-FOR-Sale-Baner-in-Pune&id=4d423836393730373635"),
    ("MahaRERA", "Project registry and disclosures", "https://maharera.maharashtra.gov.in/"),
    ("IGR Maharashtra", "Candidate source for registered records and reference rates", "https://igrmaharashtra.gov.in/"),
    ("Overture Places", "Mapped amenities and source attribution", "https://docs.overturemaps.org/guides/places/"),
    ("Copernicus Sentinel-2", "Proposed satellite features", "https://documentation.dataspace.copernicus.eu/Data/SentinelMissions/Sentinel2.html"),
    ("Maha Metro Pune project profile", "Dated transport and project context", "https://www.punemetrorail.org/project-profile"),
    ("RBI HFC Directions, 2025", "LTV and valuation governance reference", "https://rbi.org.in/scripts/BS_ViewMasDirections.aspx?id=12939"),
]
for i, (title, subtitle, link) in enumerate(refs):
    x = 54 + (i // 4) * 435
    y = 151 + (i % 4) * 58
    rect(x, y, 417, 54, WHITE)
    txt(title, x + 12, y + 7, 394, 15, TEAL, True, link=link)
    txt(subtitle, x + 12, y + 28, 394, 12, MUTE)
txt("Team artifacts   /   Baner live audit, 11 Nov 2026 IST   /   MCP demo verification, 7 Nov 2026", 54, 412, 851, 14, INK, True)
txt("See the supplied challenge brief, pp. 1–3, for the complete judging criteria and deliverables.", 54, 444, 851, 14, MUTE)

assert page == 10
pdf.showPage()
pdf.save()
ppt.save(OUT / f"{NAME}.pptx")
