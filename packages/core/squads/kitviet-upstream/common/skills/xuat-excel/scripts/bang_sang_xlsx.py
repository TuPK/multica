# -*- coding: utf-8 -*-
"""Chuyển bảng markdown / CSV thành .xlsx (Kit Việt - kv:xuat-excel).

Dùng:
    python bang_sang_xlsx.py bang.md [them.csv ...] --ra bang-2026-11-03.xlsx

- Mỗi bảng một sheet. Bảng trong markdown lấy tên sheet từ tiêu đề (#) gần nhất phía trên.
- Dòng đầu là tiêu đề cột: in đậm, nền xám, cố định (freeze), có lọc.
- Số kiểu Việt Nam "1.234.567" / "1.234,5" / "12,5%" thành số thật; ngày dd/mm/yyyy thành ngày.
- Chuỗi bắt đầu bằng 0 (SĐT, mã số thuế, mã hàng) giữ nguyên dạng chữ để không mất số 0.
- Ô bắt đầu bằng "=" giữ là công thức. Dấu ";" giữa tham số được đổi sang "," vì file .xlsx lưu công thức
  theo cú pháp gốc; Excel máy tiếng Việt vẫn hiện lại bằng ";" như thường.
"""
import argparse
import csv
import datetime
import io
import re
import sys
from pathlib import Path

for _s in (sys.stdout, sys.stderr):
    try:
        _s.reconfigure(encoding="utf-8")
    except (AttributeError, ValueError):
        pass

try:
    from openpyxl import Workbook
    from openpyxl.styles import Alignment, Border, Font, PatternFill, Side
    from openpyxl.utils import get_column_letter
except ImportError:
    print("Thiếu thư viện openpyxl. Cài bằng lệnh:\n    pip install openpyxl\nrồi chạy lại.", file=sys.stderr)
    sys.exit(2)

FMT_INT = "#,##0"
FMT_DEC = "#,##0.00"
FMT_PCT = "0.0%"
FMT_DATE = "dd/mm/yyyy"
VIEN = Side(style="thin", color="BFBFBF")


# ---------- đọc nguồn ----------

def bang_tu_markdown(text, ten_file):
    """Trả về danh sách (tên sheet, các dòng) từ mọi bảng | | trong markdown."""
    out, heading, rows = [], None, []
    in_code = False

    def flush():
        if rows:
            out.append((heading or f"{ten_file}-{len(out) + 1}", list(rows)))
            rows.clear()

    for line in text.splitlines():
        s = line.strip()
        if s.startswith("```"):
            in_code = not in_code
            continue
        if in_code:
            continue
        if s.startswith("|"):
            cells = [c.strip() for c in s.strip("|").split("|")]
            if not all(re.match(r"^:?-{2,}:?$", c) for c in cells if c):
                rows.append(cells)
            continue
        flush()
        m = re.match(r"^#{1,6}\s+(.*)$", s)
        if m:
            heading = m.group(1).strip()
    flush()
    return out


def bang_tu_csv(text, ten_file):
    # Excel máy tiếng Việt thường lưu CSV bằng ";" - chọn dấu xuất hiện nhiều nhất ở dòng đầu.
    dong_dau = text.splitlines()[0] if text.strip() else ""
    delim = max((";", ",", "\t"), key=dong_dau.count)
    rows = [r for r in csv.reader(io.StringIO(text), delimiter=delim) if any(c.strip() for c in r)]
    return [(ten_file, rows)]


# ---------- chuyển giá trị ----------

def doi_cong_thuc(s):
    """Đổi ';' ngăn tham số sang ',' (bỏ qua phần trong nháy kép)."""
    out, trong_nhay = [], False
    for ch in s:
        if ch == '"':
            trong_nhay = not trong_nhay
        out.append("," if ch == ";" and not trong_nhay else ch)
    return "".join(out)


def chuyen_gia_tri(s, kieu_so="vn"):
    """Trả về (giá trị, định dạng số hoặc None)."""
    s = s.strip()
    s = re.sub(r"^\*\*(.*)\*\*$", r"\1", s)
    if s == "":
        return None, None
    if s.startswith("="):
        return doi_cong_thuc(s), None
    m = re.match(r"^(\d{1,2})/(\d{1,2})/(\d{4})$", s)
    if m:
        try:
            return datetime.date(int(m.group(3)), int(m.group(2)), int(m.group(1))), FMT_DATE
        except ValueError:
            return s, None
    t = s.replace(" ", "").replace(" ", "")
    if re.match(r"^0\d+$", t):
        return s, "@"  # giữ số 0 đầu: SĐT, mã số thuế, mã hàng
    phan_tram = t.endswith("%")
    if phan_tram:
        t = t[:-1]
    if kieu_so == "vn":
        hop_le = re.match(r"^[-+]?\d{1,3}(\.\d{3})*(,\d+)?$", t) or re.match(r"^[-+]?\d+(,\d+)?$", t)
        so = t.replace(".", "").replace(",", ".") if hop_le else None
    else:
        hop_le = re.match(r"^[-+]?\d{1,3}(,\d{3})*(\.\d+)?$", t) or re.match(r"^[-+]?\d+(\.\d+)?$", t)
        so = t.replace(",", "") if hop_le else None
    if so is None:
        return s, None
    val = float(so)
    if phan_tram:
        return val / 100, FMT_PCT
    if val.is_integer() and "." not in so:
        # Số ngắn không có dấu ngăn (năm, STT, số lượng nhỏ) để dạng thường, tránh "2.026".
        return int(val), ("0" if re.match(r"^[-+]?\d{1,4}$", t) else FMT_INT)
    return val, FMT_DEC


# ---------- ghi workbook ----------

def ten_sheet(ten, da_dung):
    ten = re.sub(r"[\[\]:*?/\\]", "-", ten).strip("' ") or "Bang"
    ten = ten[:31]
    goc, i = ten, 2
    while ten.lower() in da_dung:
        hau = f" ({i})"
        ten = goc[:31 - len(hau)] + hau
        i += 1
    da_dung.add(ten.lower())
    return ten


def ghi_sheet(ws, rows, kieu_so="vn"):
    ncol = max(len(r) for r in rows)
    do_rong = [0] * ncol
    for r_idx, row in enumerate(rows, start=1):
        for c_idx in range(1, ncol + 1):
            raw = row[c_idx - 1] if c_idx - 1 < len(row) else ""
            cell = ws.cell(row=r_idx, column=c_idx)
            if r_idx == 1:
                cell.value = re.sub(r"^\*\*(.*)\*\*$", r"\1", raw.strip())
                cell.font = Font(bold=True)
                cell.fill = PatternFill("solid", fgColor="E7E6E6")
                cell.alignment = Alignment(horizontal="center", vertical="center", wrap_text=True)
            else:
                val, fmt = chuyen_gia_tri(raw, kieu_so)
                cell.value = val
                if fmt:
                    cell.number_format = fmt
                if isinstance(val, str) and not val.startswith("="):
                    cell.alignment = Alignment(vertical="top", wrap_text=len(val) > 60)
            cell.border = Border(left=VIEN, right=VIEN, top=VIEN, bottom=VIEN)
            do_rong[c_idx - 1] = max(do_rong[c_idx - 1], min(len(str(raw)), 60))
    for i, w in enumerate(do_rong, start=1):
        ws.column_dimensions[get_column_letter(i)].width = max(8, w + 2)
    ws.freeze_panes = "A2"
    ws.auto_filter.ref = f"A1:{get_column_letter(ncol)}{len(rows)}"


def doc_nguon(paths):
    bang = []
    for p in paths:
        p = Path(p)
        text = p.read_text(encoding="utf-8-sig")
        if p.suffix.lower() in (".csv", ".tsv", ".txt"):
            bang += bang_tu_csv(text, p.stem)
        else:
            bang += bang_tu_markdown(text, p.stem)
    return bang


def tao_workbook(bang, kieu_so="vn"):
    """Trả về Workbook. Hàm dùng chung cho script Pro."""
    wb = Workbook()
    wb.remove(wb.active)
    da_dung = set()
    for ten, rows in bang:
        if not rows:
            continue
        ws = wb.create_sheet(ten_sheet(ten, da_dung))
        ghi_sheet(ws, rows, kieu_so)
    return wb


def main(argv=None):
    ap = argparse.ArgumentParser(description="Bảng markdown/CSV -> .xlsx, mỗi bảng một sheet")
    ap.add_argument("vao", nargs="+", help="file .md hoặc .csv")
    ap.add_argument("--ra", required=True, help="file .xlsx đầu ra")
    ap.add_argument("--kieu-so", choices=("vn", "en"), default="vn",
                    help="vn: 1.234.567,5 (mặc định); en: 1,234,567.5")
    a = ap.parse_args(argv)
    thieu = [p for p in a.vao if not Path(p).exists()]
    if thieu:
        print("Không thấy file: " + ", ".join(thieu), file=sys.stderr)
        return 1
    bang = doc_nguon(a.vao)
    if not bang:
        print("Không tìm thấy bảng nào trong file nguồn.", file=sys.stderr)
        return 1
    wb = tao_workbook(bang, a.kieu_so)
    wb.save(a.ra)
    print(f"Đã tạo: {a.ra} ({len(wb.sheetnames)} sheet: {', '.join(wb.sheetnames)})")
    so_cong_thuc = sum(1 for ws in wb for row in ws.iter_rows() for c in row if isinstance(c.value, str) and c.value.startswith("="))
    if so_cong_thuc:
        print(f"Có {so_cong_thuc} ô công thức - mở bằng Excel để tính lại và kiểm tra kết quả.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
