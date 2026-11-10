# -*- coding: utf-8 -*-
"""Chuyển markdown đã duyệt thành .docx theo kiểu tài liệu Việt Nam (Kit Việt - kv:xuat-word).

Dùng:
    python md_sang_docx.py noi-dung.md --kieu cong-van --ra cong-van-2026-11-03.docx

Kiểu có quốc hiệu - tiêu ngữ (thể thức hành chính): cong-van, to-trinh, quyet-dinh, thong-bao, quy-che, bien-ban
Hợp đồng (quốc hiệu giữa trang, ký hai bên): hop-dong
Bố cục báo cáo: bao-cao, ke-hoach, mo-ta-cong-viec, bieu-mau, thu. Kiểu lạ -> bao-cao kèm cảnh báo.
Đầu file markdown có thể có khối thông tin (giữa hai dòng ---), ví dụ:
    ---
    co_quan_chu_quan: TỔNG CÔNG TY ABC
    co_quan: CÔNG TY CỔ PHẦN XYZ
    so: 123/XYZ-KT
    trich_yeu: V/v giải trình số liệu thuế
    dia_danh: Hà Nội
    ngay: 03/11/2026            (bỏ trống -> để "ngày ... tháng ... năm ..." cho người ký điền)
    kinh_gui:
    - Chi cục Thuế khu vực ...
    noi_nhan:
    - Như trên;
    - Lưu: VT, KT.
    chuc_vu: GIÁM ĐỐC
    ky_thay: KT. GIÁM ĐỐC       (tùy chọn)
    nguoi_ky: Nguyễn Văn A
    ---
Hỗ trợ markdown: # tiêu đề, đoạn, - gạch đầu dòng, 1. đánh số, bảng | |, **đậm**, *nghiêng*, > trích, ---.
Thể thức chỉ ở mức phổ biến; người dùng phải đối chiếu quy định thể thức văn bản hiện hành.
"""
import argparse
import datetime
import re
import sys
from pathlib import Path

for _s in (sys.stdout, sys.stderr):
    try:
        _s.reconfigure(encoding="utf-8")
    except (AttributeError, ValueError):
        pass

try:
    from docx import Document
    from docx.enum.section import WD_SECTION  # noqa: F401  (kiểm tra bản python-docx)
    from docx.enum.table import WD_TABLE_ALIGNMENT
    from docx.enum.text import WD_ALIGN_PARAGRAPH
    from docx.oxml import OxmlElement
    from docx.oxml.ns import qn
    from docx.shared import Cm, Pt, RGBColor
except ImportError:
    print("Thiếu thư viện python-docx. Cài bằng lệnh:\n    pip install python-docx\n"
          "rồi chạy lại. (Nên cài trong môi trường ảo: python -m venv .venv)", file=sys.stderr)
    sys.exit(2)

# Văn bản hành chính: khối đầu có tên cơ quan, số ký hiệu, quốc hiệu - tiêu ngữ, địa danh - ngày.
KIEU_HANH_CHINH = ("cong-van", "to-trinh", "quyet-dinh", "thong-bao", "quy-che", "bien-ban")
# Tài liệu nội bộ / giao dịch: bố cục báo cáo (tiêu đề giữa, không quốc hiệu trừ khi khai co_quan).
KIEU_BO_CUC_BAO_CAO = ("bao-cao", "ke-hoach", "mo-ta-cong-viec", "bieu-mau", "thu")
KIEU = KIEU_HANH_CHINH + ("hop-dong",) + KIEU_BO_CUC_BAO_CAO
FONT = "Times New Roman"
QUOC_HIEU = "CỘNG HÒA XÃ HỘI CHỦ NGHĨA VIỆT NAM"
TIEU_NGU = "Độc lập - Tự do - Hạnh phúc"
TIEU_DE_MAC_DINH = {
    "to-trinh": "TỜ TRÌNH", "quyet-dinh": "QUYẾT ĐỊNH", "thong-bao": "THÔNG BÁO", "quy-che": "QUY CHẾ",
    "bien-ban": "BIÊN BẢN", "hop-dong": "HỢP ĐỒNG", "bao-cao": "BÁO CÁO", "ke-hoach": "KẾ HOẠCH",
    "mo-ta-cong-viec": "BẢN MÔ TẢ CÔNG VIỆC", "bieu-mau": "", "thu": "",
}


def chuan_kieu(kieu):
    """Kiểu lạ -> bao-cao kèm cảnh báo (không dừng), để skill nghề gọi kiểu mới vẫn chạy."""
    kieu = (kieu or "bao-cao").strip().lower()
    if kieu not in KIEU:
        print(f"Cảnh báo: kiểu '{kieu}' chưa có mẫu riêng - dùng bố cục bao-cao. "
              f"Kiểu hỗ trợ: {', '.join(KIEU)}.", file=sys.stderr)
        return "bao-cao"
    return kieu


# ---------- đọc markdown ----------

def tach_thong_tin(text):
    """Tách khối --- key: value --- ở đầu file. Giá trị nhiều dòng viết dạng '- mục'."""
    meta = {}
    if not text.startswith("---"):
        return meta, text
    lines = text.splitlines()
    end = next((i for i in range(1, len(lines)) if lines[i].strip() == "---"), None)
    if end is None:
        return meta, text
    key = None
    for line in lines[1:end]:
        m = re.match(r"^([a-z_]+)\s*:\s*(.*)$", line.strip())
        if m:
            key, val = m.group(1), m.group(2).strip()
            meta[key] = val if val else []
        elif line.strip().startswith("- ") and key is not None:
            if not isinstance(meta[key], list):
                meta[key] = [meta[key]] if meta[key] else []
            meta[key].append(line.strip()[2:].strip())
    return meta, "\n".join(lines[end + 1:])


def phan_khoi(body):
    """Chia thân markdown thành các khối: heading, para, bullet, number, table, quote, rule, code."""
    blocks, para = [], []
    lines = body.splitlines()

    def flush():
        if para:
            blocks.append(("para", " ".join(s.strip() for s in para)))
            para.clear()

    i = 0
    while i < len(lines):
        line = lines[i]
        s = line.strip()
        if s.startswith("```"):
            flush()
            code = []
            i += 1
            while i < len(lines) and not lines[i].strip().startswith("```"):
                code.append(lines[i])
                i += 1
            blocks.append(("code", "\n".join(code)))
        elif not s:
            flush()
        elif re.match(r"^#{1,6}\s", s):
            flush()
            level = len(s) - len(s.lstrip("#"))
            blocks.append(("heading", level, s[level:].strip()))
        elif re.match(r"^(-{3,}|\*{3,})$", s):
            flush()
            blocks.append(("rule",))
        elif s.startswith("|"):
            flush()
            rows = []
            while i < len(lines) and lines[i].strip().startswith("|"):
                row = lines[i].strip().strip("|")
                cells = [c.strip() for c in row.split("|")]
                if not all(re.match(r"^:?-{2,}:?$", c) for c in cells if c):
                    rows.append(cells)
                i += 1
            blocks.append(("table", rows))
            continue
        elif re.match(r"^\s*[-*+]\s+", line):
            flush()
            indent = (len(line) - len(line.lstrip())) // 2
            blocks.append(("bullet", indent, re.sub(r"^\s*[-*+]\s+", "", line)))
        elif re.match(r"^\s*\d+[.)]\s+", line):
            flush()
            indent = (len(line) - len(line.lstrip())) // 2
            m = re.match(r"^\s*(\d+)[.)]\s+(.*)$", line)
            blocks.append(("number", indent, m.group(1), m.group(2)))
        elif s.startswith(">"):
            flush()
            blocks.append(("quote", s.lstrip(">").strip()))
        else:
            para.append(line)
        i += 1
    flush()
    return blocks


INLINE_RE = re.compile(r"(\*\*[^*]+\*\*|__[^_]+__|\*[^*\s][^*]*\*|`[^`]+`)")


def them_chu(p, text, size=None, bold=None, italic=None):
    """Thêm chữ có **đậm**, *nghiêng*, `mã` vào đoạn p."""
    for part in INLINE_RE.split(text):
        if not part:
            continue
        b, it, mono = bold, italic, False
        if (part.startswith("**") and part.endswith("**")) or (part.startswith("__") and part.endswith("__")):
            part, b = part[2:-2], True
        elif part.startswith("`") and part.endswith("`"):
            part, mono = part[1:-1], True
        elif part.startswith("*") and part.endswith("*") and len(part) > 2:
            part, it = part[1:-1], True
        run = p.add_run(part)
        dat_font(run, size, b, it, "Courier New" if mono else FONT)
    return p


# ---------- định dạng ----------

def dat_font(run, size=None, bold=None, italic=None, name=FONT):
    run.font.name = name
    rpr = run._element.get_or_add_rPr()
    rfonts = rpr.find(qn("w:rFonts"))
    if rfonts is None:
        rfonts = OxmlElement("w:rFonts")
        rpr.insert(0, rfonts)
    for attr in ("w:ascii", "w:hAnsi", "w:eastAsia", "w:cs"):
        rfonts.set(qn(attr), name)
    if size:
        run.font.size = Pt(size)
    if bold is not None:
        run.bold = bold
    if italic is not None:
        run.italic = italic


def chinh_style(doc, co_chu):
    """Đặt Times New Roman cho Normal và Heading, xóa màu/ font theme của mẫu mặc định."""
    for name, size in (("Normal", co_chu), ("Heading 1", co_chu), ("Heading 2", co_chu),
                       ("Heading 3", co_chu), ("Heading 4", co_chu), ("Title", co_chu + 2)):
        st = doc.styles[name]
        st.font.name = FONT
        st.font.size = Pt(size)
        rpr = st.element.get_or_add_rPr()
        rfonts = rpr.find(qn("w:rFonts"))
        if rfonts is None:
            rfonts = OxmlElement("w:rFonts")
            rpr.insert(0, rfonts)
        for attr in list(rfonts.attrib):
            if attr.endswith("Theme"):
                del rfonts.attrib[attr]
        for attr in ("w:ascii", "w:hAnsi", "w:eastAsia", "w:cs"):
            rfonts.set(qn(attr), FONT)
        if name != "Normal":
            st.font.color.rgb = RGBColor(0, 0, 0)
            st.font.bold = True
            st.font.italic = name in ("Heading 3", "Heading 4")
            st.paragraph_format.space_before = Pt(6)
            st.paragraph_format.space_after = Pt(3)
            st.paragraph_format.keep_with_next = True
    pf = doc.styles["Normal"].paragraph_format
    pf.space_before = Pt(6)
    pf.space_after = Pt(0)
    pf.line_spacing = 1.15


def dat_le(doc):
    for sec in doc.sections:
        sec.page_height, sec.page_width = Cm(29.7), Cm(21.0)
        sec.top_margin, sec.bottom_margin = Cm(2.0), Cm(2.0)
        sec.left_margin, sec.right_margin = Cm(3.0), Cm(1.5)
        sec.header_distance = Cm(1.0)


def them_field(p, code):
    """Chèn trường Word (PAGE, NUMPAGES, TOC...) vào đoạn p."""
    run = p.add_run()
    dat_font(run, 13)
    for tag, text in (("begin", None), ("instr", code), ("separate", None), ("end", None)):
        if tag == "instr":
            el = OxmlElement("w:instrText")
            el.set(qn("xml:space"), "preserve")
            el.text = f" {code} "
        else:
            el = OxmlElement("w:fldChar")
            el.set(qn("w:fldCharType"), tag)
        run._element.append(el)
    return run


def so_trang_tu_trang_2(doc):
    """Số trang giữa lề trên, không hiện ở trang đầu (cách làm phổ biến với văn bản hành chính)."""
    sec = doc.sections[0]
    sec.different_first_page_header_footer = True
    p = sec.header.paragraphs[0]
    p.alignment = WD_ALIGN_PARAGRAPH.CENTER
    them_field(p, "PAGE")


def ke_duoi(p, rong_cm, tong_cm):
    """Đường kẻ ngang ngắn dưới dòng chữ: dùng viền dưới của đoạn trống thụt hai bên."""
    pf = p.paragraph_format
    le = max((tong_cm - rong_cm) / 2, 0)
    pf.left_indent, pf.right_indent = Cm(le), Cm(le)
    pf.space_before, pf.space_after = Pt(0), Pt(0)
    ppr = p._p.get_or_add_pPr()
    bdr = OxmlElement("w:pBdr")
    bottom = OxmlElement("w:bottom")
    for k, v in (("w:val", "single"), ("w:sz", "6"), ("w:space", "1"), ("w:color", "000000")):
        bottom.set(qn(k), v)
    bdr.append(bottom)
    ppr.append(bdr)
    run = p.add_run()
    dat_font(run, 2)


def bo_vien_bang(table):
    tbl_pr = table._tbl.tblPr
    borders = OxmlElement("w:tblBorders")
    for edge in ("top", "left", "bottom", "right", "insideH", "insideV"):
        el = OxmlElement(f"w:{edge}")
        el.set(qn("w:val"), "nil")
        borders.append(el)
    tbl_pr.append(borders)


def dong_o(cell, text, size, bold=False, italic=False, align=WD_ALIGN_PARAGRAPH.CENTER, first=False):
    p = cell.paragraphs[0] if first else cell.add_paragraph()
    p.alignment = align
    p.paragraph_format.space_before = Pt(0)
    p.paragraph_format.first_line_indent = Cm(0)
    them_chu(p, text, size, bold, italic)
    return p


def ngay_thang(meta):
    dia = meta.get("dia_danh") or "[địa danh]"
    ngay = meta.get("ngay") or ""
    if ngay == "hom-nay":
        d = datetime.date.today()
        return f"{dia}, ngày {d.day:02d} tháng {d.month:02d} năm {d.year}"
    m = re.match(r"^(\d{1,2})[/-](\d{1,2})[/-](\d{4})$", str(ngay))
    if m:
        return f"{dia}, ngày {int(m.group(1)):02d} tháng {int(m.group(2)):02d} năm {m.group(3)}"
    return f"{dia}, ngày ... tháng ... năm ......"


def as_list(v):
    if not v:
        return []
    return v if isinstance(v, list) else [v]


# ---------- các phần của văn bản ----------

def dau_hanh_chinh(doc, meta, kieu):
    """Khối đầu 2 cột: tên cơ quan + số, ký hiệu (trái) / quốc hiệu, tiêu ngữ + địa danh, ngày (phải)."""
    t = doc.add_table(rows=1, cols=2)
    bo_vien_bang(t)
    t.alignment = WD_TABLE_ALIGNMENT.CENTER
    trai, phai = t.rows[0].cells
    trai.width, phai.width = Cm(6.5), Cm(10.0)
    first = True
    if meta.get("co_quan_chu_quan"):
        dong_o(trai, str(meta["co_quan_chu_quan"]).upper(), 12, first=True)
        first = False
    dong_o(trai, str(meta.get("co_quan") or "[TÊN CƠ QUAN, TỔ CHỨC]").upper(), 12, bold=True, first=first)
    ke_duoi(trai.add_paragraph(), 2.5, 6.5)
    if kieu != "hop-dong":
        dong_o(trai, f"Số: {meta.get('so') or '......'}", 13)
    if kieu == "cong-van" and meta.get("trich_yeu"):
        dong_o(trai, str(meta["trich_yeu"]), 12)
    dong_o(phai, QUOC_HIEU, 12, bold=True, first=True)
    dong_o(phai, TIEU_NGU, 13, bold=True)
    ke_duoi(phai.add_paragraph(), 5.6, 10.0)
    dong_o(phai, ngay_thang(meta), 13, italic=True)
    doc.add_paragraph()


def dau_quoc_hieu_giua(doc, meta):
    """Hợp đồng: quốc hiệu, tiêu ngữ ở giữa trang."""
    for text, size in ((QUOC_HIEU, 12), (TIEU_NGU, 13)):
        p = doc.add_paragraph()
        p.alignment = WD_ALIGN_PARAGRAPH.CENTER
        p.paragraph_format.space_before = Pt(0)
        them_chu(p, text, size, bold=True)
    ke_duoi(doc.add_paragraph(), 5.6, 16.5)
    if meta.get("so"):
        p = doc.add_paragraph()
        p.alignment = WD_ALIGN_PARAGRAPH.CENTER
        them_chu(p, f"Số: {meta['so']}", 13)


def tieu_de(doc, text, phu=None, co_chu=14):
    p = doc.add_paragraph()
    p.alignment = WD_ALIGN_PARAGRAPH.CENTER
    p.paragraph_format.space_before = Pt(12)
    p.paragraph_format.keep_with_next = True
    them_chu(p, text.upper(), co_chu, bold=True)
    if phu:
        p2 = doc.add_paragraph()
        p2.alignment = WD_ALIGN_PARAGRAPH.CENTER
        p2.paragraph_format.space_before = Pt(0)
        them_chu(p2, phu, co_chu, bold=True)


def kinh_gui(doc, meta, co_chu):
    ds = as_list(meta.get("kinh_gui"))
    if not ds:
        return
    p = doc.add_paragraph()
    p.alignment = WD_ALIGN_PARAGRAPH.CENTER
    p.paragraph_format.space_before = Pt(12)
    if len(ds) == 1:
        them_chu(p, f"Kính gửi: {ds[0]}", co_chu)
    else:
        them_chu(p, "Kính gửi:", co_chu)
        for x in ds:
            q = doc.add_paragraph()
            q.paragraph_format.left_indent = Cm(5.0)
            q.paragraph_format.space_before = Pt(0)
            them_chu(q, f"- {x}", co_chu)


def ghi_than(doc, blocks, co_chu, bo_tieu_de_dau=False):
    da_bo = not bo_tieu_de_dau
    for b in blocks:
        kind = b[0]
        if kind == "heading":
            if not da_bo and b[1] == 1:
                da_bo = True
                continue
            level = min(b[1], 4)
            h = doc.add_heading(level=level)
            h.paragraph_format.first_line_indent = Cm(1.0) if level >= 2 else Cm(0)
            them_chu(h, b[2], co_chu)
        elif kind == "para":
            p = doc.add_paragraph()
            p.alignment = WD_ALIGN_PARAGRAPH.JUSTIFY
            p.paragraph_format.first_line_indent = Cm(1.0)
            them_chu(p, b[1], co_chu)
        elif kind in ("bullet", "number"):
            p = doc.add_paragraph()
            p.alignment = WD_ALIGN_PARAGRAPH.JUSTIFY
            p.paragraph_format.first_line_indent = Cm(1.0 + 0.75 * b[1])
            label = "-" if kind == "bullet" else f"{b[2]}."
            them_chu(p, f"{label} {b[-1]}", co_chu)
        elif kind == "quote":
            p = doc.add_paragraph()
            p.paragraph_format.left_indent = Cm(1.0)
            them_chu(p, b[1], co_chu, italic=True)
        elif kind == "code":
            for line in b[1].splitlines() or [""]:
                p = doc.add_paragraph()
                p.paragraph_format.space_before = Pt(0)
                run = p.add_run(line)
                dat_font(run, 11, name="Courier New")
        elif kind == "rule":
            ke_duoi(doc.add_paragraph(), 16.5, 16.5)
        elif kind == "table":
            ghi_bang(doc, b[1], co_chu)


def ghi_bang(doc, rows, co_chu):
    if not rows:
        return
    ncol = max(len(r) for r in rows)
    t = doc.add_table(rows=len(rows), cols=ncol)
    t.style = "Table Grid"
    t.alignment = WD_TABLE_ALIGNMENT.CENTER
    size = max(co_chu - 1, 11)
    for r, row in enumerate(rows):
        for c in range(ncol):
            text = row[c] if c < len(row) else ""
            cell = t.cell(r, c)
            p = cell.paragraphs[0]
            p.paragraph_format.first_line_indent = Cm(0)
            p.paragraph_format.space_before = Pt(2)
            so = re.match(r"^[-+]?[\d.,]+%?$", text.replace(" ", "")) is not None
            p.alignment = WD_ALIGN_PARAGRAPH.CENTER if r == 0 else (WD_ALIGN_PARAGRAPH.RIGHT if so else WD_ALIGN_PARAGRAPH.LEFT)
            them_chu(p, text, size, bold=True if r == 0 else None)
    if rows:
        lap_header(t.rows[0])
    doc.add_paragraph().paragraph_format.space_before = Pt(0)


def lap_header(row):
    """Dòng tiêu đề bảng lặp lại khi bảng sang trang."""
    tr_pr = row._tr.get_or_add_trPr()
    el = OxmlElement("w:tblHeader")
    el.set(qn("w:val"), "true")
    tr_pr.append(el)


def khoi_ky(doc, meta, kieu, co_chu):
    doc.add_paragraph()
    t = doc.add_table(rows=1, cols=2)
    bo_vien_bang(t)
    trai, phai = t.rows[0].cells
    trai.width, phai.width = Cm(8.0), Cm(8.5)
    if kieu in ("hop-dong", "bien-ban"):
        if kieu == "hop-dong":
            cot = (("ĐẠI DIỆN BÊN A", meta.get("dai_dien_a")), ("ĐẠI DIỆN BÊN B", meta.get("dai_dien_b")))
        else:
            cot = (("THƯ KÝ", meta.get("thu_ky")), ("CHỦ TRÌ", meta.get("chu_tri")))
        for cell, (nhan, ten) in zip((trai, phai), cot):
            dong_o(cell, nhan, co_chu - 1, bold=True, first=True)
            dong_o(cell, "(Ký, ghi rõ họ tên)", 12, italic=True)
            for _ in range(3):
                cell.add_paragraph()
            dong_o(cell, str(ten or ""), co_chu - 1, bold=True)
        return
    nn = as_list(meta.get("noi_nhan"))
    if nn:
        dong_o(trai, "Nơi nhận:", 12, bold=True, italic=True, align=WD_ALIGN_PARAGRAPH.LEFT, first=True)
        for x in nn:
            dong_o(trai, f"- {x}", 11, align=WD_ALIGN_PARAGRAPH.LEFT)
    first = True
    if meta.get("ky_thay"):
        dong_o(phai, str(meta["ky_thay"]).upper(), co_chu - 1, bold=True, first=True)
        first = False
    dong_o(phai, str(meta.get("chuc_vu") or "[CHỨC VỤ NGƯỜI KÝ]").upper(), co_chu - 1, bold=True, first=first)
    dong_o(phai, "(Ký, ghi rõ họ tên, đóng dấu)", 12, italic=True)
    for _ in range(3):
        phai.add_paragraph()
    dong_o(phai, str(meta.get("nguoi_ky") or ""), co_chu - 1, bold=True)


def tieu_de_tu_than(blocks):
    for b in blocks:
        if b[0] == "heading" and b[1] == 1:
            return b[2]
    return None


def tao_van_ban(md_text, kieu, co_chu=None):
    """Trả về (Document, meta). Hàm dùng chung cho script Pro."""
    kieu = chuan_kieu(kieu)
    hanh_chinh = kieu in KIEU_HANH_CHINH
    co_chu = co_chu or (14 if hanh_chinh else 13)
    meta, body = tach_thong_tin(md_text)
    blocks = phan_khoi(body)
    doc = Document()
    chinh_style(doc, co_chu)
    dat_le(doc)
    so_trang_tu_trang_2(doc)

    ten = meta.get("tieu_de") or tieu_de_tu_than(blocks)
    if hanh_chinh or meta.get("co_quan") and kieu != "hop-dong":
        dau_hanh_chinh(doc, meta, kieu)
    elif kieu == "hop-dong":
        dau_quoc_hieu_giua(doc, meta)

    if kieu == "cong-van":
        kinh_gui(doc, meta, co_chu)
    else:
        chinh = TIEU_DE_MAC_DINH[kieu]
        if ten and (not chinh or ten.upper().startswith(chinh)):
            chinh, phu = ten, meta.get("trich_yeu")
        else:
            phu = meta.get("trich_yeu") or ten
        if chinh:
            tieu_de(doc, chinh, phu, co_chu)
        if kieu in ("to-trinh", "thu"):
            kinh_gui(doc, meta, co_chu)
    ghi_than(doc, blocks, co_chu, bo_tieu_de_dau=kieu != "cong-van" or bool(ten and not meta.get("tieu_de")))
    if hanh_chinh or kieu == "hop-dong" or meta.get("chuc_vu"):
        khoi_ky(doc, meta, kieu, co_chu)
    return doc, meta


def main(argv=None):
    ap = argparse.ArgumentParser(description="Markdown -> .docx theo kiểu tài liệu Việt Nam")
    ap.add_argument("vao", help="file .md đã được người dùng duyệt")
    ap.add_argument("--kieu", default="bao-cao", help="kiểu tài liệu: " + ", ".join(KIEU) + " (kiểu lạ -> bao-cao)")
    ap.add_argument("--ra", help="file .docx đầu ra (mặc định cùng tên)")
    ap.add_argument("--co-chu", type=int, choices=(13, 14), help="cỡ chữ nội dung (mặc định 14 với văn bản hành chính, 13 với báo cáo/hợp đồng)")
    a = ap.parse_args(argv)
    src = Path(a.vao)
    if not src.exists():
        print(f"Không thấy file: {src}", file=sys.stderr)
        return 1
    text = src.read_text(encoding="utf-8-sig")
    doc, _ = tao_van_ban(text, a.kieu, a.co_chu)
    out = Path(a.ra) if a.ra else src.with_suffix(".docx")
    doc.save(out)
    con = text.count("[cần bổ sung]")
    print(f"Đã tạo: {out}")
    if con:
        print(f"Cảnh báo: văn bản còn {con} chỗ [cần bổ sung] - điền trước khi gửi/ký.")
    print("Nhắc: đối chiếu thể thức với quy định hiện hành của cơ quan/đơn vị trước khi ban hành.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
