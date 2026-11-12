# Research Digest

## Query

Vietnamese legal GraphRAG

## Search Summary

- Search rounds: 3
- Papers retrieved: 72
- Unique papers: 52
- Selected papers: 3
- Warnings: 6

### Warnings

- MISSING_ABSTRACT: Findings of the Association for Computational Linguistics: EACL 2026
- MISSING_ABSTRACT: A survey on sentiment analysis methods, applications, and challenges
- MISSING_ABSTRACT: Findings of the Association for Computational Linguistics: EMNLP 2023
- MISSING_ABSTRACT: A review of semi-supervised learning for text classification
- SCORING_CANDIDATES_CAPPED: 22 -> 15 new papers
- INSUFFICIENT_RESULTS_AFTER_MAX_ROUNDS

## Selected Papers

### 1. Integrating Knowledge Graph with Retrieval-Augmented Generation for Vietnamese Legal Question Answering

Authors: Vuong T. Pham, Minh Phan  
Date: 2025-03-31  
DOI: 10.59232/dst-v5i1p101  
URL: https://doi.org/10.59232/dst-v5i1p101

Metadata source: OpenAlex  
Abstract source: OpenAlex

Relevance score: 10/10

#### Relevance

Directly addresses Vietnamese legal QA with knowledge graph-enhanced RAG and statutory hierarchy.

#### Summary

A framework integrates an ontology-driven Knowledge Graph with Retrieval-Augmented Generation for Vietnamese legal QA, preserving statute hierarchy and encoding semantic/cross-reference relations. It uses graph-aware retrieval with hybrid ranking (semantic similarity + graph confidence) and a constrained prompting template for explicit citations. Evaluated on three law domains, it achieves 85.7% answer accuracy (vs. 62.3% BM25, 71.5% pure LLM) and improves citation quality, aiming to increase traceability by grounding generation in retrieved statutory provisions.

#### Provenance

Summary generated only from the abstract returned by OpenAlex.

### 2. A Self-Reflection Mechanism for Reducing Hallucination in Vietnamese Legal Question Answering Systems

Authors: Thi Vuong Pham, Nguyet Minh Phan, Bao Quynh Cao, Cong Phuc Truong, Thanh Duy Nguyen, Minh Vy Tien, Ngoc Son Nguyen  
Date: 2026-03-02  
DOI: 10.64539/sjcs.v2i1.2026.380  
URL: https://doi.org/10.64539/sjcs.v2i1.2026.380

Metadata source: OpenAlex  
Abstract source: OpenAlex

Relevance score: 9/10

#### Relevance

明确使用Graph-RAG管道处理越南劳动法问答，高度相关

#### Summary

The paper proposes a self-reflection mechanism to reduce hallucinations in Vietnamese legal question-answering systems. It introduces an iterative generate-evaluate-refine loop built on a Graph-RAG pipeline, where each draft answer is scored using a hybrid measure combining contextual alignment with retrieved legal text and model confidence. The system iterates until a quality threshold or stopping limit is reached. On a Vietnamese Labor Law benchmark, the method improves accuracy from 81.5% to 86.7% and reduces hallucination from 18.7% to 9.3%, with minimal added latency. The study shows that pairing contextual alignment with confidence yields more stable results than either signal alone. The approach works without additional labeled data or fine-tuning and can be adapted to other Vietnamese legal domains requiring clause-level evidence.

#### Provenance

Summary generated only from the abstract returned by OpenAlex.

### 3. Enhancing legal research through knowledge-infused information retrieval for Vietnamese labor law

Authors: Vuong T. Pham, Hoang Huy Le, Thinh Phu Ngo, Binh T. Nguyen, Diem Nguyen, Hien D. Nguyen  
Date: 2024-10-08  
DOI: 10.11591/ijai.v13.i4.pp3962-3973  
URL: https://doi.org/10.11591/ijai.v13.i4.pp3962-3973

Metadata source: OpenAlex  
Abstract source: OpenAlex

Relevance score: 6/10

#### Relevance

涉及越南劳动法检索和知识注入，但未明确使用GraphRAG

#### Summary

This research enhances legal document retrieval for Vietnamese labor law by using a dataset of 300,000 documents across 20 types and 27 legal fields, constructing a tailored Q&A dataset, and introducing a legal ontology built from the data to improve retrieval performance through knowledge infusion.

#### Provenance

Summary generated only from the abstract returned by OpenAlex.

## Scope

This workflow uses metadata and abstracts from configured APIs only. It does not download PDFs, scrape restricted full text, or bypass paywalls.
