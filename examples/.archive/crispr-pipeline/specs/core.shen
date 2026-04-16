\* ============================================================ *\
\* CRISPR Gene Editing Pipeline                                 *\
\* Shen Sequent-Calculus Type Specs                             *\
\*                                                              *\
\* Makes invalid edits, dangerous off-target hits, and          *\
\* protocol violations into type errors. Every step in the      *\
\* pipeline from guide RNA design to validated edit carries      *\
\* proof of safety.                                             *\
\*                                                              *\
\* The wet lab is a state machine: each step requires proof     *\
\* that the previous step succeeded and safety criteria are met.*\
\*                                                              *\
\* An edit that would hit an unintended gene cannot be          *\
\* constructed. A guide RNA with poor specificity cannot         *\
\* proceed to synthesis. This is the difference between         *\
\* catching a bad edit in silico vs. in a patient.              *\
\* ============================================================ *\


\* ============================================= *\
\*  LAYER 1: SEQUENCE PRIMITIVES                *\
\* ============================================= *\

(datatype gene-name
  X : string;
  (not (= X "")) : verified;
  ============================
  X : gene-name;)

(datatype chromosome
  X : string;
  (not (= X "")) : verified;
  ============================
  X : chromosome;)

(datatype genome-position
  Chrom : chromosome;
  Start : number;
  End : number;
  (>= Start 0) : verified;
  (> End Start) : verified;
  ==========================
  [Chrom Start End] : genome-position;)

(datatype strand
  X : string;
  (element? X [sense antisense]) : verified;
  ==========================================
  X : strand;)

\* A nucleotide sequence (validated non-empty, ACGT only checked downstream) *\
(datatype dna-sequence
  X : string;
  (not (= X "")) : verified;
  ============================
  X : dna-sequence;)

\* Guide RNA length: typically 17-24 nt for Cas9 *\
(datatype guide-length
  X : number;
  (>= X 17) : verified;
  (<= X 24) : verified;
  ======================
  X : guide-length;)


\* ============================================= *\
\*  LAYER 2: GUIDE RNA DESIGN                   *\
\*  The guide must pass multiple quality gates    *\
\*  before it can proceed to synthesis.           *\
\* ============================================= *\

\* --- PAM site validation --- *\
\* Cas9 requires an NGG PAM adjacent to the target *\

(datatype cas-variant
  X : string;
  (element? X [spCas9 saCas9 cas12a cas13]) : verified;
  =====================================================
  X : cas-variant;)

(datatype pam-sequence
  X : string;
  (not (= X "")) : verified;
  ============================
  X : pam-sequence;)

\* Proof that the PAM is correct for the chosen Cas variant *\
(datatype pam-valid
  Cas : cas-variant;
  Pam : pam-sequence;
  IsCorrect : boolean;
  (= IsCorrect true) : verified;
  ================================
  [Cas Pam IsCorrect] : pam-valid;)

\* --- GC content: should be 40-70% for good binding --- *\

(datatype gc-content
  X : number;
  (>= X 0) : verified;
  (<= X 100) : verified;
  =======================
  X : gc-content;)

(datatype gc-in-range
  GC : gc-content;
  (>= GC 40) : verified;
  (<= GC 70) : verified;
  ========================
  GC : gc-in-range;)

\* --- Secondary structure: guide must not fold on itself --- *\

(datatype folding-energy
  X : number;
  ===========
  X : folding-energy;)

\* Free energy must be above threshold (less negative = less stable = good) *\
(datatype no-secondary-structure
  Energy : folding-energy;
  Threshold : folding-energy;
  (>= Energy Threshold) : verified;
  ==================================
  [Energy Threshold] : no-secondary-structure;)

\* --- Complete guide RNA: all design criteria met --- *\

(datatype guide-rna
  Sequence : dna-sequence;
  Target : genome-position;
  Strand : strand;
  Length : guide-length;
  Cas : cas-variant;
  PamProof : pam-valid;
  GcProof : gc-in-range;
  FoldProof : no-secondary-structure;
  ====================================
  [Sequence Target Strand Length Cas PamProof GcProof FoldProof] : guide-rna;)


\* ============================================= *\
\*  LAYER 3: OFF-TARGET ANALYSIS                *\
\*  The critical safety layer. Every potential   *\
\*  off-target site must be scored. Guides with  *\
\*  dangerous off-targets cannot proceed.         *\
\* ============================================= *\

\* An off-target hit: where else in the genome this guide might cut *\
(datatype off-target-site
  Position : genome-position;
  Mismatches : number;
  Gene : string;
  (>= Mismatches 0) : verified;
  ==============================
  [Position Mismatches Gene] : off-target-site;)

\* Specificity score: how specific is this guide? (0-100, higher = more specific) *\
(datatype specificity-score
  X : number;
  (>= X 0) : verified;
  (<= X 100) : verified;
  =======================
  X : specificity-score;)

(datatype specificity-threshold
  X : number;
  (> X 0) : verified;
  (<= X 100) : verified;
  =======================
  X : specificity-threshold;)

\* Proof that specificity is above the safety threshold *\
(datatype specificity-sufficient
  Score : specificity-score;
  Threshold : specificity-threshold;
  (>= Score Threshold) : verified;
  ==================================
  [Score Threshold] : specificity-sufficient;)

\* --- Critical gene avoidance --- *\
\* No off-target hits in essential genes or known oncogenes *\

(datatype essential-gene
  Name : gene-name;
  IsEssential : boolean;
  (= IsEssential true) : verified;
  ==================================
  [Name IsEssential] : essential-gene;)

\* Proof: no off-target hit falls in an essential gene *\
(datatype essential-genes-clear
  Guide : guide-rna;
  OffTargetCount : number;
  EssentialHits : number;
  (>= OffTargetCount 0) : verified;
  (= EssentialHits 0) : verified;
  ==================================
  [Guide OffTargetCount EssentialHits] : essential-genes-clear;)

\* --- Combined off-target safety proof --- *\
(datatype off-target-safe
  Specificity : specificity-sufficient;
  EssentialClear : essential-genes-clear;
  ========================================
  [Specificity EssentialClear] : off-target-safe;)


\* ============================================= *\
\*  LAYER 4: EDIT TYPE SPECIFICATION            *\
\*  What kind of modification is being made?     *\
\* ============================================= *\

(datatype edit-type
  X : string;
  (element? X [knockout knockin correction deletion insertion base-edit
               prime-edit epigenetic-mod]) : verified;
  =================================================================
  X : edit-type;)

\* For knock-in / correction: the donor template *\
(datatype donor-template
  Sequence : dna-sequence;
  HomologyArmLeft : number;
  HomologyArmRight : number;
  (> HomologyArmLeft 0) : verified;
  (> HomologyArmRight 0) : verified;
  ====================================
  [Sequence HomologyArmLeft HomologyArmRight] : donor-template;)

\* For base editing: the target base change *\
(datatype base-change
  From : string;
  To : string;
  Position : number;
  (element? From [A C G T]) : verified;
  (element? To [A C G T]) : verified;
  (not (= From To)) : verified;
  (>= Position 0) : verified;
  ==============================
  [From To Position] : base-change;)

\* ============================================= *\
\*  LAYER 4a: COMPUTATIONAL SCORING             *\
\*  These types model the actual algorithms used  *\
\*  by real guide design tools (CRISPOR, Benchling*\
\*  CHOPCHOP, FlashFry) and off-target search    *\
\*  engines (Cas-OFFinder, BWA-based).            *\
\*                                                *\
\*  WHERE COMPUTATION PLAYS IN:                   *\
\*                                                *\
\*  1. Guide design: Paste a gene/exon sequence   *\
\*     into CRISPOR/Benchling. The tool finds     *\
\*     all 20-mers adjacent to a PAM (NGG for     *\
\*     SpCas9), scores each for cutting efficiency *\
\*     using ML models (Rule Set 2, CRISPRscan,   *\
\*     TIGER), and ranks them.                    *\
\*                                                *\
\*  2. Off-target search: For each candidate      *\
\*     guide, search the entire 3-billion-base    *\
\*     genome for every locus within N mismatches  *\
\*     (typically 0-4). Cas-OFFinder uses GPU-    *\
\*     accelerated exact enumeration; FlashFry    *\
\*     pre-indexes all PAM-adjacent sites.         *\
\*     Cost: minutes (GPU) to hours (CPU, 4+ mm). *\
\*                                                *\
\*  3. Off-target scoring: Each hit is scored by  *\
\*     CFD (Cutting Frequency Determination) or   *\
\*     MIT specificity score — position-weighted   *\
\*     mismatch penalty matrices from empirical    *\
\*     cutting data. Aggregate = 100/(100+sum).   *\
\*                                                *\
\*  4. Post-editing: CRISPResso2 takes FASTQ      *\
\*     reads + reference amplicon, aligns with     *\
\*     Needleman-Wunsch, quantifies indel          *\
\*     frequencies, classifies outcomes (NHEJ,     *\
\*     HDR, unmodified). GUIDE-seq maps genome-   *\
\*     wide off-target integration sites.          *\
\*                                                *\
\*  The Shen types below make the computational   *\
\*  quality gates into proof obligations:          *\
\*  - Efficiency score must exceed threshold       *\
\*  - Off-target search must be complete           *\
\*  - Reference genome must match cell line        *\
\*  - Post-edit quantification must confirm edit   *\
\* ============================================= *\

\* --- On-target efficiency scoring --- *\
\* ML models: Rule Set 2 (Doench 2016), CRISPRscan, TIGER (2023) *\
\* These predict cutting efficiency from sequence features *\
\* R-squared ~0.3-0.5 against actual cutting — imperfect but useful *\

(datatype scoring-model
  X : string;
  (element? X [rule-set-2 crisprscan tiger deepcrispr azimuth]) : verified;
  ==========================================================================
  X : scoring-model;)

(datatype efficiency-score
  X : number;
  (>= X 0) : verified;
  (<= X 100) : verified;
  =======================
  X : efficiency-score;)

(datatype efficiency-threshold
  X : number;
  (> X 0) : verified;
  (<= X 100) : verified;
  =======================
  X : efficiency-threshold;)

\* Proof that guide efficiency exceeds the design threshold *\
(datatype efficiency-sufficient
  Guide : guide-rna;
  Model : scoring-model;
  Score : efficiency-score;
  Threshold : efficiency-threshold;
  (>= Score Threshold) : verified;
  ==================================
  [Guide Model Score Threshold] : efficiency-sufficient;)

\* --- Off-target search completeness --- *\
\* Must search the full genome, not just a subset *\
\* Cas-OFFinder or FlashFry with specified mismatch count *\

(datatype search-tool
  X : string;
  (element? X [cas-offinder flashfry bowtie crispor-builtin benchling]) : verified;
  =================================================================================
  X : search-tool;)

(datatype mismatch-tolerance
  X : number;
  (>= X 0) : verified;
  (<= X 6) : verified;
  =====================
  X : mismatch-tolerance;)

(datatype genome-build
  X : string;
  (element? X [hg38 hg19 mm10 mm39 danRer11 sacCer3]) : verified;
  ================================================================
  X : genome-build;)

(datatype search-complete
  Tool : search-tool;
  Mismatches : mismatch-tolerance;
  Genome : genome-build;
  SitesFound : number;
  (>= SitesFound 0) : verified;
  (>= Mismatches 3) : verified;
  ================================
  [Tool Mismatches Genome SitesFound] : search-complete;)

\* --- CFD / MIT scoring of each off-target hit --- *\
\* CFD (Doench 2016): position × substitution matrix *\
\* MIT (Hsu 2013): position-weighted mismatch penalty *\

(datatype ot-scoring-method
  X : string;
  (element? X [cfd mit-specificity crispor-aggregate]) : verified;
  ================================================================
  X : ot-scoring-method;)

(datatype ot-aggregate-score
  Method : ot-scoring-method;
  Score : specificity-score;
  Guide : guide-rna;
  Search : search-complete;
  ==========================
  [Method Score Guide Search] : ot-aggregate-score;)

\* --- Reference genome match --- *\
\* If the cell line has SNPs at the target, the guide may not match *\
\* This is a common failure mode: design for hg38, cell line has a SNP *\

(datatype reference-verified
  Genome : genome-build;
  CellLine : cell-type;
  TargetSequenced : boolean;
  SequenceMatches : boolean;
  (= TargetSequenced true) : verified;
  (= SequenceMatches true) : verified;
  ======================================
  [Genome CellLine TargetSequenced SequenceMatches] : reference-verified;)

\* --- Post-editing computational analysis (CRISPResso2) --- *\
\* Takes FASTQ reads + reference amplicon, quantifies editing *\

(datatype analysis-tool
  X : string;
  (element? X [crispresso2 crispresso1 ampliCan ice]) : verified;
  ===============================================================
  X : analysis-tool;)

(datatype read-count
  X : number;
  (> X 0) : verified;
  ====================
  X : read-count;)

(datatype read-depth-sufficient
  Reads : read-count;
  MinDepth : read-count;
  (>= Reads MinDepth) : verified;
  ================================
  [Reads MinDepth] : read-depth-sufficient;)

\* Genome-wide off-target detection (empirical, not computational prediction) *\
(datatype genome-wide-ot-method
  X : string;
  (element? X [guide-seq circle-seq discover-seq site-seq]) : verified;
  =====================================================================
  X : genome-wide-ot-method;)

\* --- Computationally scored guide: ready for ordering --- *\
\* This is the complete computational proof before wet lab begins *\

(datatype guide-computationally-validated
  Guide : guide-rna;
  Efficiency : efficiency-sufficient;
  OtScore : ot-aggregate-score;
  RefMatch : reference-verified;
  ================================
  [Guide Efficiency OtScore RefMatch] : guide-computationally-validated;)

\* --- Edit specification: guide + edit type + safety --- *\
(datatype edit-spec
  Guide : guide-computationally-validated;
  Type : edit-type;
  OffTargetSafety : off-target-safe;
  ====================================
  [Guide Type OffTargetSafety] : edit-spec;)


\* ============================================= *\
\*  LAYER 5: WET LAB PIPELINE STATE MACHINE     *\
\*  Each step requires proof of the previous.    *\
\*  Cannot skip steps. Cannot proceed with       *\
\*  failed quality checks.                       *\
\* ============================================= *\

(datatype experiment-id
  X : string;
  (not (= X "")) : verified;
  ============================
  X : experiment-id;)

(datatype timestamp
  X : number;
  (> X 0) : verified;
  ====================
  X : timestamp;)

(datatype operator-id
  X : string;
  (not (= X "")) : verified;
  ============================
  X : operator-id;)

\* --- Step 1: Design approved --- *\
(datatype design-approved
  Experiment : experiment-id;
  Spec : edit-spec;
  ApprovedBy : operator-id;
  ApprovedAt : timestamp;
  ==========================
  [Experiment Spec ApprovedBy ApprovedAt] : design-approved;)

\* --- Step 2: Guide synthesized --- *\
\* Requires approved design *\
(datatype guide-synthesized
  Design : design-approved;
  SynthesizedAt : timestamp;
  Purity : number;
  (> Purity 0) : verified;
  (>= Purity 90) : verified;
  ============================
  [Design SynthesizedAt Purity] : guide-synthesized;)

\* --- Step 3: Delivery / transfection --- *\

(datatype delivery-method
  X : string;
  (element? X [electroporation lipofection viral-vector rnp microinjection]) : verified;
  ======================================================================================
  X : delivery-method;)

(datatype cell-type
  X : string;
  (not (= X "")) : verified;
  ============================
  X : cell-type;)

(datatype transfection-efficiency
  X : number;
  (>= X 0) : verified;
  (<= X 100) : verified;
  =======================
  X : transfection-efficiency;)

(datatype cells-transfected
  Synthesis : guide-synthesized;
  Method : delivery-method;
  CellType : cell-type;
  Efficiency : transfection-efficiency;
  TransfectedAt : timestamp;
  ==============================
  [Synthesis Method CellType Efficiency TransfectedAt] : cells-transfected;)

\* --- Step 4: Selection (isolate edited cells) --- *\

(datatype selection-method
  X : string;
  (element? X [antibiotic-resistance facs flow-sorting puromycin]) : verified;
  ============================================================================
  X : selection-method;)

(datatype cells-selected
  Transfection : cells-transfected;
  SelectionType : selection-method;
  SurvivalRate : number;
  (> SurvivalRate 0) : verified;
  (<= SurvivalRate 100) : verified;
  ==================================
  [Transfection SelectionType SurvivalRate] : cells-selected;)

\* --- Step 5: Sequencing / validation --- *\
\* The final proof: did the edit actually work correctly? *\

(datatype sequencing-method
  X : string;
  (element? X [sanger ngs amplicon-seq whole-genome]) : verified;
  ===============================================================
  X : sequencing-method;)

(datatype on-target-efficiency
  X : number;
  (>= X 0) : verified;
  (<= X 100) : verified;
  =======================
  X : on-target-efficiency;)

(datatype on-target-confirmed
  Selected : cells-selected;
  SeqMethod : sequencing-method;
  Efficiency : on-target-efficiency;
  SequencedAt : timestamp;
  (> Efficiency 0) : verified;
  ==============================
  [Selected SeqMethod Efficiency SequencedAt] : on-target-confirmed;)

\* --- Step 6: Off-target validation (in vivo) --- *\
\* Computational prediction isn't enough — verify empirically *\

(datatype off-target-sites-checked
  X : number;
  (> X 0) : verified;
  ====================
  X : off-target-sites-checked;)

(datatype off-target-hits-found
  X : number;
  (>= X 0) : verified;
  =====================
  X : off-target-hits-found;)

(datatype off-target-validated
  OnTarget : on-target-confirmed;
  SitesChecked : off-target-sites-checked;
  HitsFound : off-target-hits-found;
  SeqMethod : sequencing-method;
  ValidatedAt : timestamp;
  ============================
  [OnTarget SitesChecked HitsFound SeqMethod ValidatedAt] : off-target-validated;)

\* Proof that off-target hits are below acceptable threshold *\
(datatype off-target-acceptable
  Validation : off-target-validated;
  MaxAcceptable : number;
  (>= MaxAcceptable 0) : verified;
  (<= (head (tail (tail (tail Validation)))) MaxAcceptable) : verified;
  ====================================================================
  [Validation MaxAcceptable] : off-target-acceptable;)


\* ============================================= *\
\*  LAYER 6: VALIDATED EDIT — THE FINAL PROOF   *\
\*  A complete, validated gene edit that has      *\
\*  passed every quality gate from design         *\
\*  through empirical validation.                 *\
\* ============================================= *\

(datatype validated-edit
  OnTarget : on-target-confirmed;
  OffTarget : off-target-acceptable;
  ====================================
  [OnTarget OffTarget] : validated-edit;)


\* ============================================= *\
\*  LAYER 7: BIOSAFETY & ETHICS                 *\
\*  Additional proof obligations for clinical     *\
\*  or therapeutic applications.                  *\
\* ============================================= *\

(datatype biosafety-level
  X : string;
  (element? X [bsl-1 bsl-2 bsl-3 bsl-4]) : verified;
  ====================================================
  X : biosafety-level;)

(datatype irb-approval
  ProtocolId : string;
  ApprovedAt : timestamp;
  ExpiresAt : timestamp;
  (not (= ProtocolId "")) : verified;
  (> ExpiresAt ApprovedAt) : verified;
  ======================================
  [ProtocolId ApprovedAt ExpiresAt] : irb-approval;)

\* IRB approval must not be expired *\
(datatype irb-current
  Approval : irb-approval;
  Now : timestamp;
  (> (head (tail (tail Approval))) Now) : verified;
  ==================================================
  [Approval Now] : irb-current;)

\* For somatic cell therapy: not germline (ethical constraint) *\
(datatype somatic-only
  CellType : cell-type;
  IsGermline : boolean;
  (= IsGermline false) : verified;
  ==================================
  [CellType IsGermline] : somatic-only;)

\* --- Clinical-grade edit: all safety + ethics proofs --- *\
(datatype clinical-grade-edit
  Edit : validated-edit;
  Biosafety : biosafety-level;
  Irb : irb-current;
  Somatic : somatic-only;
  ==========================
  [Edit Biosafety Irb Somatic] : clinical-grade-edit;)


\* ============================================= *\
\*  LAYER 8: BATCH / MULTIPLEXING               *\
\*  Multiple edits in one experiment. Each must  *\
\*  independently satisfy all safety criteria.    *\
\* ============================================= *\

\* Proof that two guides don't interfere with each other *\
(datatype guides-non-interfering
  GuideA : guide-rna;
  GuideB : guide-rna;
  Interference : number;
  MaxInterference : number;
  (>= Interference 0) : verified;
  (<= Interference MaxInterference) : verified;
  (> MaxInterference 0) : verified;
  ==============================================
  [GuideA GuideB Interference MaxInterference] : guides-non-interfering;)

\* A multiplex edit where all pairs are non-interfering *\
(datatype multiplex-safe
  EditCount : number;
  AllPairsCleared : boolean;
  (> EditCount 1) : verified;
  (= AllPairsCleared true) : verified;
  ======================================
  [EditCount AllPairsCleared] : multiplex-safe;)
