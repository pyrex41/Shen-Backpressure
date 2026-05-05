The "Rosetta Stone" for shengen — one spec that exercises all six shengen categories, with a small program demonstrating each one's enforcement profile.

Stack: Go stdlib. Self-contained.

Domain: A document management system where one spec file exercises all six shengen categories:

1. **wrapper**: `DocumentId` (string wrapper, no validation)
2. **constrained**: `PageCount` (number, must be > 0)
3. **alias**: `DraftDocument = Document` (type synonym)
4. **composite**: `Document [DocumentId Title PageCount]` (no cross-field guards)
5. **guarded**: `PublishedDocument [Document ReviewerApproval]` where `(= Approved true) : verified`
6. **sumtype**: `AccessLevel = ReadOnly | ReadWrite` (two datatype blocks, same conclusion)

Shen spec (specs/core.shen):

```shen
(datatype document-id
  X : string;
  ==============
  X : document-id;)

(datatype page-count
  X : number;
  (> X 0) : verified;
  ====================
  X : page-count;)

(datatype document
  Id : document-id;
  Title : string;
  Pages : page-count;
  ========================
  [Id Title Pages] : document;)

(datatype draft-document
  D : document;
  ===============
  D : draft-document;)

(datatype published-document
  Doc : document;
  Approved : boolean;
  (= Approved true) : verified;
  ==============================
  [Doc Approved] : published-document;)

(datatype read-only-access
  User : string;
  Doc : document;
  ==================
  [User Doc] : access-level;)

(datatype read-write-access
  User : string;
  Doc : document;
  Role : string;
  (element? Role ["editor" "admin"]) : verified;
  ================================================
  [User Doc Role] : access-level;)
```

Generate Go guard types and write a program that:
- Creates each type, showing the constructor signature
- Demonstrates which constructors return errors (constrained, guarded) vs which are infallible (wrapper, composite)
- Shows the compile error when trying to bypass the opaque field
- Annotates each type with its category and enforcement profile

Target: ~40 lines of spec, ~60 lines of generated Go, ~50 lines of demonstration code.
