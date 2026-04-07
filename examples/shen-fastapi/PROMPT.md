# Shen + FastAPI Example: Python API with Strong Typing via Shen

**Vision**: Demonstrate Shen specs generating Pydantic models, FastAPI endpoints, dependency injectors, and validation logic. Even though Python/FastAPI may not be the absolute fastest for raw throughput, it excels in developer productivity, data science integration (Pandas, ML), and automatic OpenAPI docs. This serves as a comparison point for the performance-focused examples (Hono, Go, Rust).

This directly addresses your hesitation on FastAPI: we'll benchmark it against the others to show tradeoffs clearly. Shen can enforce invariants that even Pydantic v2 struggles with (complex stateful protocols, grounding proofs).

## Usefulness Fleshed Out
- Rapid prototyping of data-heavy APIs (e.g., research pipeline with NLP post-processing).
- Excellent for teams already in Python ecosystem.
- Shen provides the "missing" formal verification layer for dynamic Python.
- Generate async endpoints with proper backpressure modeling.

**Domain**: Same research assistant or Medicare plan validator API (extending existing medicare.shen).

**Key Specs to Define**:
- Complex nested validators using Shen datatypes (beyond Pydantic).
- Grounded data flows.
- Resource protocols (e.g., DB connection lifecycle).

**Tech Stack**:
- FastAPI + Uvicorn (or Hypercorn for async)
- Pydantic v2
- Python 3.11+
- Shen -> Python codegen (new shengen-py target?)

**Prompt for Development Loop**:
"Create a Shen-FastAPI bridge. The Shen spec should define the entire API surface and business rules. Generate Pydantic BaseModels from Shen datatypes where possible, and runtime guards. Implement the full research pipeline as in the CL version but in Python. Include a benchmark suite comparing to the Hono version on identical hardware. Highlight cases where Shen catches errors that would be runtime in pure FastAPI. Explore using Shen for dependency graph validation in FastAPI Depends()."

Create:
- specs/
- app/ (Python code)
- generated/ (from Shen)
- tests/
- pyproject.toml or requirements.txt
- Makefile adapted for Python + Shen

This example rhetorically demonstrates pragmatic choice: use the right tool for the job while keeping the symbolic specification unified. It opens the box to Python's rich ecosystem while maintaining high-assurance properties.