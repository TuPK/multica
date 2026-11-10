# -*- coding: utf-8 -*-
"""Chuyển đề cương slide (markdown) thành .pptx (Kit Việt - kv:xuat-pptx).

Dùng:
    python de_cuong_sang_pptx.py de-cuong.md --ra thuyet-trinh-2026-11-03.pptx

Cú pháp đề cương:
    # Tiêu đề bộ slide            -> slide tiêu đề
    Phụ đề / người trình bày      (các dòng thường ngay sau, thành phụ đề)
    ## Tiêu đề slide là câu kết luận
    - gạch đầu dòng (thụt 2 dấu cách = cấp 2)
    Hình: gợi ý hình/biểu đồ      -> đưa vào ghi chú để người làm slide chèn
    Nguồn: tên file, bảng         -> chữ nhỏ cuối slide
    Ghi chú: lời người nói        -> ghi chú người nói (có thể nhiều dòng, hoặc dùng > ...)
Tối đa 6 gạch mỗi slide; dư thì tự tách sang slide "(tiếp)" và in cảnh báo.
"""
import argparse
import re
import sys
from pathlib import Path

for _s in (sys.stdout, sys.stderr):
    try:
        _s.reconfigure(encoding="utf-8")
    except (AttributeError, ValueError):
        pass

try:
    from pptx import Presentation
    from pptx.util import Inches, Pt
except ImportError:
    print("Thiếu thư viện python-pptx. Cài bằng lệnh:\n    pip install python-pptx\nrồi chạy lại.", file=sys.stderr)
    sys.exit(2)

TOI_DA_GACH = 6


def doc_de_cuong(text):
    """Trả về (slide tiêu đề, danh sách slide nội dung). Mỗi slide: dict title, bullets[(cap, chu)], notes[], nguon."""
    bia = {"title": "", "sub": [], "notes": []}
    slides, cur, che_do = [], None, None
    for line in text.splitlines():
        s = line.strip()
        if not s:
            continue
        if s.startswith("# ") and not bia["title"]:
            bia["title"], cur, che_do = s[2:].strip(), bia, "sub"
            continue
        if s.startswith("## "):
            cur = {"title": s[3:].strip(), "bullets": [], "notes": [], "nguon": ""}
            slides.append(cur)
            che_do = "body"
            continue
        if cur is None:
            continue
        m = re.match(r"^(ghi chú( thuyết trình)?|notes?)\s*:\s*(.*)$", s, re.I)
        if m:
            che_do = "notes"
            if m.group(3):
                cur["notes"].append(m.group(3).strip().strip('"'))
            continue
        if s.startswith(">"):
            cur["notes"].append(s.lstrip(">").strip())
            continue
        m = re.match(r"^(hình|biểu đồ)\s*:\s*(.*)$", s, re.I)
        if m:
            cur["notes"].append(f"[Gợi ý {m.group(1).lower()}: {m.group(2).strip()}]")
            continue
        m = re.match(r"^nguồn\s*:\s*(.*)$", s, re.I)
        if m and cur is not bia:
            cur["nguon"] = m.group(1).strip()
            continue
        if che_do == "notes":
            cur["notes"].append(s)
        elif cur is bia:
            bia["sub"].append(re.sub(r"^[-*]\s+", "", s))
        else:
            m = re.match(r"^(\s*)([-*+]|\d+[.)])\s+(.*)$", line)
            if m:
                cur["bullets"].append((min(len(m.group(1)) // 2, 2), m.group(3).strip()))
            else:
                cur["bullets"].append((0, s))
    return bia, slides


def tach_slide_dai(slides):
    out, canh_bao = [], []
    for sl in slides:
        b = sl["bullets"]
        if len(b) <= TOI_DA_GACH:
            out.append(sl)
            continue
        canh_bao.append(f"Slide \"{sl['title']}\" có {len(b)} gạch -> tách thành {(len(b) - 1) // TOI_DA_GACH + 1} slide.")
        for i in range(0, len(b), TOI_DA_GACH):
            phan = dict(sl, bullets=b[i:i + TOI_DA_GACH])
            if i:
                phan = dict(phan, title=sl["title"] + " (tiếp)", notes=[])
            out.append(phan)
    return out, canh_bao


def ghi_chu(run_text, para, font, size=None, bold=False):
    para.text = ""
    for part in re.split(r"(\*\*[^*]+\*\*)", run_text):
        if not part:
            continue
        r = para.add_run()
        dam = part.startswith("**") and part.endswith("**")
        r.text = part[2:-2] if dam else part
        r.font.name = font
        r.font.bold = bold or dam
        if size:
            r.font.size = Pt(size)


def tao_bo_slide(bia, slides, font="Arial", mau=None):
    """Trả về Presentation 16:9. Hàm dùng chung cho script Pro (mau: đường dẫn .pptx mẫu)."""
    prs = Presentation(mau) if mau else Presentation()
    if mau:
        # Bỏ slide mẫu có sẵn trong file của công ty, chỉ giữ bố cục và theme.
        ds = prs.slides._sldIdLst
        for sld in list(ds):
            prs.part.drop_rel(sld.rId)
            ds.remove(sld)
    else:
        prs.slide_width, prs.slide_height = Inches(13.333), Inches(7.5)
    s0 = prs.slides.add_slide(prs.slide_layouts[0])
    ghi_chu(bia["title"] or "[Tiêu đề bộ slide]", s0.shapes.title.text_frame.paragraphs[0], font, 40, True)
    if len(s0.placeholders) > 1:
        tf = s0.placeholders[1].text_frame
        for i, line in enumerate(bia["sub"] or [""]):
            p = tf.paragraphs[0] if i == 0 else tf.add_paragraph()
            ghi_chu(line, p, font, 20)
    if bia["notes"]:
        s0.notes_slide.notes_text_frame.text = "\n".join(bia["notes"])

    for sl in slides:
        s = prs.slides.add_slide(prs.slide_layouts[1])
        ghi_chu(sl["title"], s.shapes.title.text_frame.paragraphs[0], font, 30, True)
        body = s.placeholders[1]
        if not mau:
            body.left, body.top = Inches(0.8), Inches(1.6)
            body.width, body.height = Inches(11.7), Inches(5.0)
        tf = body.text_frame
        tf.word_wrap = True
        for i, (cap, chu) in enumerate(sl["bullets"] or [(0, "")]):
            p = tf.paragraphs[0] if i == 0 else tf.add_paragraph()
            p.level = cap
            ghi_chu(chu, p, font, 24 if cap == 0 else 20)
        if sl["nguon"]:
            box = s.shapes.add_textbox(Inches(0.8), prs.slide_height - Inches(0.7), Inches(11.7), Inches(0.4))
            ghi_chu(f"Nguồn: {sl['nguon']}", box.text_frame.paragraphs[0], font, 12)
        if sl["notes"]:
            s.notes_slide.notes_text_frame.text = "\n".join(sl["notes"])
    return prs


def main(argv=None):
    ap = argparse.ArgumentParser(description="Đề cương markdown -> .pptx")
    ap.add_argument("vao", help="file .md đề cương đã duyệt")
    ap.add_argument("--ra", help="file .pptx đầu ra (mặc định cùng tên)")
    ap.add_argument("--font", default="Arial", help="font chữ (mặc định Arial, có đủ dấu tiếng Việt)")
    a = ap.parse_args(argv)
    src = Path(a.vao)
    if not src.exists():
        print(f"Không thấy file: {src}", file=sys.stderr)
        return 1
    bia, slides = doc_de_cuong(src.read_text(encoding="utf-8-sig"))
    if not slides:
        print("Không thấy slide nào (mỗi slide bắt đầu bằng '## ').", file=sys.stderr)
        return 1
    slides, canh_bao = tach_slide_dai(slides)
    prs = tao_bo_slide(bia, slides, a.font)
    out = Path(a.ra) if a.ra else src.with_suffix(".pptx")
    prs.save(out)
    print(f"Đã tạo: {out} ({len(prs.slides)} slide)")
    for c in canh_bao:
        print("Cảnh báo: " + c)
    thieu = [sl["title"] for sl in slides if not sl["notes"]]
    if thieu:
        print(f"Cảnh báo: {len(thieu)} slide chưa có ghi chú người nói: " + "; ".join(thieu[:5]))
    return 0


if __name__ == "__main__":
    sys.exit(main())
