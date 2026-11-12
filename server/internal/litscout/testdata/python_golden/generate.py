"""Regenerates golden.json from LitScout's own Python code.

    python -m venv venv && venv/bin/pip install rapidfuzz pydantic httpx openai
    venv/bin/python generate.py /path/to/LitScout > golden.json

LitScout is https://github.com/Hoanghung0603/LitScout (commit e860b12 when this file was written). The
cases are fixed here; the expected values are whatever LitScout's code returns, so golden_test.go
compares the Go port with the Python original on the same inputs.
"""
import json
import random
import sys
from datetime import datetime

sys.path.insert(0, sys.argv[1] + "/src")

from litscout.models import PaperMetadata  # noqa: E402
from litscout.services.dedup import _same, _text, deduplicate  # noqa: E402
from litscout.services.relevance import _heuristic, prefilter_papers  # noqa: E402
from litscout.tools.openalex import restore_abstract  # noqa: E402
from rapidfuzz import fuzz  # noqa: E402


def paper(title, authors=(), doi=None, oa=None, abstract=None):
    return PaperMetadata(
        title=title, authors=list(authors), doi=doi, openalex_id=oa, abstract=abstract,
        metadata_source="OpenAlex", abstract_source="OpenAlex" if abstract else None,
    )


def dump(p):
    return {"title": p.title, "authors": p.authors, "doi": p.doi or "", "openalex_id": p.openalex_id or "",
            "abstract": p.abstract or ""}


rng = random.Random(20261112)

TITLES = [
    "A Survey of Retrieval Augmented Generation Methods for Legal Question Answering",
    "Integrating Knowledge Graph with Retrieval-Augmented Generation for Vietnamese Legal Question Answering",
    "A Self-Reflection Mechanism for Reducing Hallucination in Vietnamese Legal Question Answering Systems",
    "Enhancing legal research through knowledge-infused information retrieval for Vietnamese labor law",
    "GraphRAG: Unlocking LLM discovery on narrative private data",
    "Deep Residual Learning for Image Recognition",
    "Attention Is All You Need",
    "BERT: Pre-training of Deep Bidirectional Transformers for Language Understanding",
]

# normalisation: ASCII cases must match exactly; accent cases are listed separately
NORMALIZE_ASCII = [
    "  Graph-RAG: A Survey! ", "Hello,   World", "R&D (2020) -- results", "snake_case_name", "UPPER lower MiXeD",
    "tabs\tand\nnewlines", "numbers 123 and 4.5", "", "   ", "a-b_c.d",
]
NORMALIZE_ACCENT = ["Phân tích Pháp luật Việt Nam", "Café—Déjà vu", "Straße", "Ångström", "naïve résumé", "Đại học Bách khoa"]

# fuzzy ratio: random edits around the 96 threshold
RATIO_PAIRS = []
for t in TITLES:
    n = _text(t)
    for edits in (0, 1, 2, 3, 4, 6):
        chars = list(n)
        for _ in range(edits):
            i = rng.randrange(len(chars))
            op = rng.choice("sdi")
            if op == "s":
                chars[i] = rng.choice("abcdefghij")
            elif op == "d" and len(chars) > 3:
                del chars[i]
            else:
                chars.insert(i, rng.choice("abcdefghij"))
        RATIO_PAIRS.append([n, "".join(chars)])
RATIO_PAIRS += [["abc", "abd"], ["", ""], ["a", ""], ["same", "same"], ["kitten", "sitting"], ["flaw", "lawn"]]

SAME_CASES = []
base = TITLES[0]
variants = [
    (paper(base, ["Ann Lee"], "10.1/a"), paper("Other title", ["Zed"], "10.1/a")),
    (paper(base, ["Ann Lee"], None, "W1"), paper("Other", ["Zed"], None, "W1")),
    (paper(base, ["Ann Lee"]), paper(base.lower(), ["ann lee"])),
    (paper(base, ["Ann Lee"]), paper(base, ["Bo Chan"])),
    (paper(base, []), paper(base, [])),
    (paper(base, ["Ann Lee"]), paper(base + "s", ["Ann Lee"])),
    (paper(base, []), paper(base + "s", [])),
    (paper(base, ["Ann Lee"]), paper(base[:-12], ["Ann Lee"])),
    (paper("", ["Ann"]), paper("", ["Ann"])),
    (paper(base, ["Ann Lee"], "10.1/a"), paper(base, ["Ann Lee"], "10.1/b")),
]
for a, b in variants:
    SAME_CASES.append({"a": dump(a), "b": dump(b), "same": _same(a, b)})

DEDUP_INPUT = [
    paper(TITLES[1], ["Vuong Pham", "Minh Phan"], "10.59232/dst-v5i1p101", "W1", "abstract one"),
    paper(TITLES[1], ["Vuong Pham"], None, None),
    paper(TITLES[2], ["Thi Vuong Pham"], "10.64539/sjcs", "W2"),
    paper(TITLES[2].upper(), ["thi vuong pham"], None, None, "late abstract"),
    paper(TITLES[3], ["Vuong T. Pham"], None, "W3"),
    paper(TITLES[3], ["Someone Else"], None, None),
    paper(TITLES[4], [], None, None),
    paper(TITLES[4], [], None, None),
    paper(TITLES[0], ["Ann Lee"], "10.1/x", "W9"),
    paper(TITLES[0] + "s", ["Ann Lee"], None, None),
    paper(TITLES[5], ["Kaiming He"], None, "W5"),
    paper(TITLES[5], ["Kaiming He"], "10.1109/cvpr", "W5"),
]
unique = deduplicate(DEDUP_INPUT)
DEDUP = {
    "input": [dump(p) for p in DEDUP_INPUT],
    "unique": [dump(p) for p in unique],
}

PREFILTER_PAPERS = [
    paper("Vietnamese legal GraphRAG", [], abstract="x"),
    paper("Legal systems", [], abstract="vietnamese law"),
    paper("Cooking", [], abstract="recipes"),
    paper("Graph methods", [], abstract=None),
    paper("Legal graph", [], abstract=None),
    paper("Other", [], abstract="legal"),
    paper("Vietnamese cooking", [], abstract="graph"),
    paper("Nothing", [], abstract=None),
]
PREFILTER = []
for query in ("Vietnamese legal GraphRAG", "legal graph", "zzz"):
    for limit in (0, 2, 3, 5, 8, 10):
        got = prefilter_papers(query, PREFILTER_PAPERS, limit)
        PREFILTER.append({"query": query, "limit": limit, "titles": [p.title for p in got]})

HEURISTIC = []
for query in ("Vietnamese legal GraphRAG", "legal graph", "zzz", ""):
    for p in PREFILTER_PAPERS:
        HEURISTIC.append({"query": query, "title": p.title, "abstract": p.abstract or "", "score": _heuristic(query, p).score})

ABSTRACTS = [
    {"index": {"b": [1], "a": [1], "z": [0], "y": [2]}},
    {"index": {"Graph": [0], "RAG": [1], "for": [2], "law": [3]}},
    {"index": {"x": []}},
    {"index": {}},
    {"index": {"the": [0, 3], "cat": [1], "sat": [2]}},
]
for a in ABSTRACTS:
    a["text"] = restore_abstract(a["index"]) or ""

DOIS = ["https://doi.org/10.59232/DST-V5I1P101", "http://doi.org/10.1/ABC", "doi:10.1/X", "  10.1/Plain  ", "", "HTTPS://DOI.ORG/10.1/UP"]
DOI_OUT = [{"in": d, "out": (PaperMetadata(title="t", metadata_source="OpenAlex", doi=d).doi or "")} for d in DOIS]

print(json.dumps({
    "source": "LitScout e860b12 (python), rapidfuzz " + __import__("rapidfuzz").__version__,
    "normalize_ascii": [{"in": s, "out": _text(s)} for s in NORMALIZE_ASCII],
    "normalize_accent": [{"in": s, "python": _text(s)} for s in NORMALIZE_ACCENT],
    "ratio": [{"a": a, "b": b, "ratio": fuzz.ratio(a, b)} for a, b in RATIO_PAIRS],
    "same": SAME_CASES,
    "dedup": DEDUP,
    "prefilter": PREFILTER,
    "prefilter_papers": [dump(p) for p in PREFILTER_PAPERS],
    "heuristic": HEURISTIC,
    "abstracts": ABSTRACTS,
    "dois": DOI_OUT,
}, ensure_ascii=False, indent=1))
