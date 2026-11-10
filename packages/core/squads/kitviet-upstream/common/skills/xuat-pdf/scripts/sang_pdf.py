# -*- coding: utf-8 -*-
"""Chuyển bản chốt (.docx hoặc .md) thành PDF, giữ đúng dấu tiếng Việt (Kit Việt - kv:xuat-pdf).

Dùng:
    python sang_pdf.py bao-cao-2026-11-03.docx            # ưu tiên LibreOffice nếu có
    python sang_pdf.py ghi-chu.md --ra ghi-chu-2026-11-03.pdf
    python sang_pdf.py van-ban.docx --khong-dung-libreoffice --font C:/Windows/Fonts/arial.ttf

Thứ tự cách làm:
1. File .docx và máy có LibreOffice -> soffice --headless --convert-to pdf (giữ bố cục Word tốt nhất).
2. Không có LibreOffice -> dựng PDF bằng reportlab: .docx đọc bằng python-docx, .md đọc trực tiếp.
   Font Unicode tìm trên máy (Times New Roman / Arial trên Windows, DejaVu / Liberation trên Linux)
   hoặc truyền --font <đường dẫn .ttf>. Không dùng font mặc định của PDF vì mất dấu tiếng Việt.
"""
import argparse
import os
import re
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path
from xml.sax.saxutils import escape

for _s in (sys.stdout, sys.stderr):
    try:
        _s.reconfigure(encoding="utf-8")
    except (AttributeError, ValueError):
        pass

BO_FONT = [
    # (thường, đậm, nghiêng, đậm nghiêng)
    ("C:/Windows/Fonts/times.ttf", "C:/Windows/Fonts/timesbd.ttf", "C:/Windows/Fonts/timesi.ttf", "C:/Windows/Fonts/timesbi.ttf"),
    ("C:/Windows/Fonts/arial.ttf", "C:/Windows/Fonts/arialbd.ttf", "C:/Windows/Fonts/ariali.ttf", "C:/Windows/Fonts/arialbi.ttf"),
    ("/System/Library/Fonts/Supplemental/Times New Roman.ttf", "/System/Library/Fonts/Supplemental/Times New Roman Bold.ttf",
     "/System/Library/Fonts/Supplemental/Times New Roman Italic.ttf", "/System/Library/Fonts/Supplemental/Times New Roman Bold Italic.ttf"),
    ("/usr/share/fonts/truetype/liberation/LiberationSerif-Regular.ttf", "/usr/share/fonts/truetype/liberation/LiberationSerif-Bold.ttf",
     "/usr/share/fonts/truetype/liberation/LiberationSerif-Italic.ttf", "/usr/share/fonts/truetype/liberation/LiberationSerif-BoldItalic.ttf"),
    ("/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf", "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf",
     "/usr/share/fonts/truetype/dejavu/DejaVuSans-Oblique.ttf", "/usr/share/fonts/truetype/dejavu/DejaVuSans-BoldOblique.ttf"),
]
SOFFICE_THUONG_GAP = [
    "C:/Program Files/LibreOffice/program/soffice.exe",
    "C:/Program Files (x86)/LibreOffice/program/soffice.exe",
    "/Applications/LibreOffice.app/Contents/MacOS/soffice",
    "/usr/bin/soffice", "/usr/bin/libreoffice",
]


# ---------- cách 1: LibreOffice ----------

def tim_soffice():
    for name in ("soffice", "libreoffice"):
        p = shutil.which(name)
        if p:
            return p
    return next((p for p in SOFFICE_THUONG_GAP if os.path.exists(p)), None)


def qua_libreoffice(src, out):
    exe = tim_soffice()
    if not exe:
        return False
    with tempfile.TemporaryDirectory() as tmp:
        r = subprocess.run([exe, "--headless", "--convert-to", "pdf", "--outdir", tmp, str(src)],
                           capture_output=True, text=True, timeout=180)
        made = Path(tmp) / (src.stem + ".pdf")
        if r.returncode != 0 or not made.exists():
            print("LibreOffice không chuyển được, chuyển sang cách dựng bằng reportlab.", file=sys.stderr)
            return False
        shutil.move(str(made), str(out))
    return True


# ---------- cách 2: reportlab ----------

def nap_reportlab():
    try:
        import reportlab  # noqa: F401
    except ImportError:
        print("Thiếu thư viện reportlab (và python-docx nếu nguồn là .docx). Cài bằng lệnh:\n"
              "    pip install reportlab python-docx\nhoặc cài LibreOffice để chuyển .docx.", file=sys.stderr)
        sys.exit(2)


def dang_ky_font(font_path=None):
    """Đăng ký họ font Unicode 'KV' và trả về tên. Thoát nếu không tìm được font có dấu tiếng Việt."""
    from reportlab.lib.fonts import addMapping
    from reportlab.pdfbase import pdfmetrics
    from reportlab.pdfbase.ttfonts import TTFont

    bo = None
    if font_path:
        if not os.path.exists(font_path):
            print(f"Không thấy font: {font_path}", file=sys.stderr)
            sys.exit(1)
        bo = (font_path, None, None, None)
    else:
        bo = next((b for b in BO_FONT if os.path.exists(b[0])), None)
    if not bo:
        print("Không tìm thấy font Unicode trên máy. Truyền --font <file .ttf có dấu tiếng Việt>, "
              "ví dụ DejaVuSans.ttf (Linux: apt install fonts-dejavu-core).", file=sys.stderr)
        sys.exit(1)
    names = ("KV", "KV-Bold", "KV-Italic", "KV-BoldItalic")
    for name, path in zip(names, bo):
        pdfmetrics.registerFont(TTFont(name, path if path and os.path.exists(path) else bo[0]))
    addMapping("KV", 0, 0, "KV")
    addMapping("KV", 1, 0, "KV-Bold")
    addMapping("KV", 0, 1, "KV-Italic")
    addMapping("KV", 1, 1, "KV-BoldItalic")
    return "KV", bo[0]


def kieu_chu(font, co=13):
    from reportlab.lib.enums import TA_CENTER, TA_JUSTIFY
    from reportlab.lib.styles import ParagraphStyle
    base = ParagraphStyle("kv", fontName=font, fontSize=co, leading=co * 1.35, alignment=TA_JUSTIFY,
                          spaceBefore=4, firstLineIndent=0)
    return {
        "p": ParagraphStyle("p", parent=base, firstLineIndent=28),
        "li": ParagraphStyle("li", parent=base, leftIndent=28, firstLineIndent=0),
        "h1": ParagraphStyle("h1", parent=base, fontSize=co + 2, leading=(co + 2) * 1.3, alignment=TA_CENTER,
                             spaceBefore=10, spaceAfter=6),
        "h2": ParagraphStyle("h2", parent=base, spaceBefore=8),
        "h3": ParagraphStyle("h3", parent=base, spaceBefore=6),
        "cell": ParagraphStyle("cell", parent=base, fontSize=co - 2, leading=(co - 2) * 1.3, alignment=0, spaceBefore=0),
        "quote": ParagraphStyle("quote", parent=base, leftIndent=28),
        "code": ParagraphStyle("code", parent=base, fontSize=co - 3, leading=(co - 3) * 1.3, alignment=0),
        "base": base,
    }


def inline(text):
    t = escape(text)
    t = re.sub(r"\*\*([^*]+)\*\*", r"<b>\1</b>", t)
    t = re.sub(r"__([^_]+)__", r"<b>\1</b>", t)
    t = re.sub(r"(?<![*\w])\*([^*\s][^*]*)\*(?!\*)", r"<i>\1</i>", t)
    t = re.sub(r"`([^`]+)`", r"\1", t)
    return t


def bang_flow(rows, st, khung=True, ty_le=None):
    from reportlab.lib import colors
    from reportlab.lib.units import cm
    from reportlab.platypus import Paragraph, Table, TableStyle
    ncol = max(len(r) for r in rows)
    data = [[Paragraph(inline(r[c]) if c < len(r) else "", st["cell"]) for c in range(ncol)] for r in rows]
    rong = 16.5 * cm
    widths = [rong * w for w in ty_le] if ty_le else [rong / ncol] * ncol
    t = Table(data, colWidths=widths, repeatRows=1 if khung else 0)
    style = [("VALIGN", (0, 0), (-1, -1), "TOP")]
    if khung:
        style += [("GRID", (0, 0), (-1, -1), 0.5, colors.grey), ("BACKGROUND", (0, 0), (-1, 0), colors.HexColor("#E7E6E6"))]
    t.setStyle(TableStyle(style))
    return t


def tach_thong_tin(text):
    if text.startswith("---"):
        lines = text.splitlines()
        end = next((i for i in range(1, len(lines)) if lines[i].strip() == "---"), None)
        if end:
            return "\n".join(lines[end + 1:])
    return text


def md_sang_flow(text, st):
    from reportlab.platypus import HRFlowable, Paragraph, Preformatted, Spacer
    flows, para, lines, i = [], [], tach_thong_tin(text).splitlines(), 0

    def flush():
        if para:
            flows.append(Paragraph(inline(" ".join(x.strip() for x in para)), st["p"]))
            para.clear()

    while i < len(lines):
        line, s = lines[i], lines[i].strip()
        if s.startswith("```"):
            flush()
            code = []
            i += 1
            while i < len(lines) and not lines[i].strip().startswith("```"):
                code.append(lines[i])
                i += 1
            flows.append(Preformatted("\n".join(code), st["code"]))
        elif not s:
            flush()
        elif re.match(r"^#{1,6}\s", s):
            flush()
            lv = len(s) - len(s.lstrip("#"))
            txt = inline(s[lv:].strip())
            flows.append(Paragraph(f"<b>{txt.upper() if lv == 1 else txt}</b>" if lv < 3 else f"<b><i>{txt}</i></b>",
                                   st["h1" if lv == 1 else "h2" if lv == 2 else "h3"]))
        elif re.match(r"^(-{3,}|\*{3,})$", s):
            flush()
            flows.append(HRFlowable(width="100%", thickness=0.5))
        elif s.startswith("|"):
            flush()
            rows = []
            while i < len(lines) and lines[i].strip().startswith("|"):
                cells = [c.strip() for c in lines[i].strip().strip("|").split("|")]
                if not all(re.match(r"^:?-{2,}:?$", c) for c in cells if c):
                    rows.append(cells)
                i += 1
            if rows:
                flows += [Spacer(1, 4), bang_flow(rows, st), Spacer(1, 4)]
            continue
        elif re.match(r"^\s*([-*+]|\d+[.)])\s+", line):
            flush()
            lv = (len(line) - len(line.lstrip())) // 2
            m = re.match(r"^\s*([-*+]|\d+[.)])\s+(.*)$", line)
            nhan = "-" if m.group(1) in "-*+" else m.group(1)
            style = st["li"].clone("li%d" % lv, leftIndent=28 + 18 * lv)
            flows.append(Paragraph(f"{escape(nhan)} {inline(m.group(2))}", style))
        elif s.startswith(">"):
            flush()
            flows.append(Paragraph(f"<i>{inline(s.lstrip('>').strip())}</i>", st["quote"]))
        else:
            para.append(line)
        i += 1
    flush()
    return flows


def docx_sang_flow(path, st):
    """Đọc .docx theo đúng thứ tự đoạn/bảng, giữ căn lề, đậm, nghiêng."""
    try:
        from docx import Document
        from docx.table import Table as DTable
        from docx.text.paragraph import Paragraph as DPara
    except ImportError:
        print("Thiếu python-docx để đọc .docx khi không có LibreOffice:\n    pip install python-docx", file=sys.stderr)
        sys.exit(2)
    from reportlab.lib.enums import TA_CENTER, TA_JUSTIFY, TA_LEFT, TA_RIGHT
    from reportlab.lib.units import cm
    from reportlab.platypus import Paragraph, Spacer, Table, TableStyle
    canh = {0: TA_LEFT, 1: TA_CENTER, 2: TA_RIGHT, 3: TA_JUSTIFY}
    doc = Document(path)

    def para_xml(p):
        parts = []
        for r in p.runs:
            t = escape(r.text)
            if not t:
                continue
            if r.bold:
                t = f"<b>{t}</b>"
            if r.italic:
                t = f"<i>{t}</i>"
            parts.append(t)
        return "".join(parts)

    def para_flow(p, style):
        xml = para_xml(p)
        if not xml.strip():
            return Spacer(1, 6)
        al = p.alignment if p.alignment is not None else (3 if style is st["p"] else 0)
        indent = p.paragraph_format.first_line_indent
        s2 = style.clone("d", alignment=canh.get(int(al), TA_JUSTIFY),
                         firstLineIndent=(indent.pt if indent is not None else 0))
        return Paragraph(xml, s2)

    flows = []
    for el in doc.element.body.iterchildren():
        tag = el.tag.split("}")[-1]
        if tag == "p":
            flows.append(para_flow(DPara(el, doc), st["p"]))
        elif tag == "tbl":
            t = DTable(el, doc)
            khung = (t.style is not None and t.style.name == "Table Grid")
            data = []
            for row in t.rows:
                data.append([[para_flow(p, st["cell"]) for p in cell.paragraphs] for cell in row.cells])
            ncol = max(len(r) for r in data)
            tb = Table(data, colWidths=[16.5 * cm / ncol] * ncol, repeatRows=1 if khung else 0)
            style = [("VALIGN", (0, 0), (-1, -1), "TOP")]
            if khung:
                style.append(("GRID", (0, 0), (-1, -1), 0.5, (0.5, 0.5, 0.5)))
            tb.setStyle(TableStyle(style))
            flows += [tb, Spacer(1, 4)]
    return flows


def dung_pdf(flows, out, font, tieu_de="", chan_trang=None, dau_trang=None):
    """Dựng PDF A4, lề 2/2/3/1,5 cm, số trang ở chân. Hàm dùng chung cho script Pro."""
    from reportlab.lib.pagesizes import A4
    from reportlab.lib.units import cm
    from reportlab.platypus import SimpleDocTemplate

    def ve_trang(canvas, doc):
        canvas.saveState()
        canvas.setFont(font, 10)
        canvas.drawCentredString(A4[0] / 2 + 0.75 * cm, 1.1 * cm, chan_trang(doc.page) if chan_trang else f"Trang {doc.page}")
        if dau_trang:
            dau_trang(canvas, doc)
        canvas.restoreState()

    d = SimpleDocTemplate(str(out), pagesize=A4, topMargin=2 * cm, bottomMargin=2 * cm, leftMargin=3 * cm,
                          rightMargin=1.5 * cm, title=tieu_de, author="", creator="Kit Viet - kv:xuat-pdf")
    d.build(flows, onFirstPage=ve_trang, onLaterPages=ve_trang)


def main(argv=None):
    ap = argparse.ArgumentParser(description=".docx / .md -> PDF giữ dấu tiếng Việt")
    ap.add_argument("vao", help="file .docx hoặc .md đã chốt")
    ap.add_argument("--ra", help="file .pdf đầu ra (mặc định cùng tên)")
    ap.add_argument("--font", help="đường dẫn font .ttf Unicode (mặc định tự tìm trên máy)")
    ap.add_argument("--co-chu", type=int, default=13)
    ap.add_argument("--khong-dung-libreoffice", action="store_true", help="bỏ qua LibreOffice, dựng bằng reportlab")
    a = ap.parse_args(argv)
    src = Path(a.vao)
    if not src.exists():
        print(f"Không thấy file: {src}", file=sys.stderr)
        return 1
    out = Path(a.ra) if a.ra else src.with_suffix(".pdf")
    if src.suffix.lower() == ".docx" and not a.khong_dung_libreoffice and qua_libreoffice(src, out):
        print(f"Đã tạo: {out} (qua LibreOffice)")
        return 0
    nap_reportlab()
    font, font_file = dang_ky_font(a.font)
    st = kieu_chu(font, a.co_chu)
    if src.suffix.lower() == ".docx":
        flows = docx_sang_flow(src, st)
        print("Không dùng LibreOffice: bố cục được dựng lại gần đúng, cần mở PDF xem lại khối đầu và chữ ký.")
    else:
        flows = md_sang_flow(src.read_text(encoding="utf-8-sig"), st)
    dung_pdf(flows, out, font, tieu_de=src.stem)
    print(f"Đã tạo: {out} (reportlab, font {Path(font_file).name})")
    return 0


if __name__ == "__main__":
    sys.exit(main())
