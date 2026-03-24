\* src/ai-gen.shen - AI generation logic in Shen *\
\* Defines how to construct prompts and process AI responses *\
\* The actual LLM call is delegated to the bridge; Shen owns the logic *\

\* --- Prompt construction --- *\
\* These are pure functions that build well-structured prompts *\

(define make-summary-prompt
  \* Build a prompt that asks the AI to summarize grounded sources *\
  { search-query --> (list grounded-source) --> ai-prompt }
  [QueryText _] Sources ->
    (let SystemMsg (cn "You are a research assistant. Summarize the following "
                   (cn "web sources about: " (value->string QueryText)))
         SourceTexts (format-sources Sources)
         UserMsg (cn "Based on these sources, provide a clear summary:\n\n"
                  SourceTexts)
      [SystemMsg UserMsg]))

(define make-analysis-prompt
  \* Build a prompt for deeper analysis of a topic *\
  { string --> (list grounded-source) --> ai-prompt }
  Topic Sources ->
    (let SystemMsg "You are an expert analyst. Provide structured analysis with key findings, implications, and open questions."
         SourceTexts (format-sources Sources)
         UserMsg (cn "Analyze this topic: " (cn Topic (cn "\n\nSources:\n" SourceTexts)))
      [SystemMsg UserMsg]))

(define make-comparison-prompt
  \* Build a prompt to compare multiple sources *\
  { (list grounded-source) --> ai-prompt }
  Sources ->
    (let SystemMsg "Compare and contrast the following sources. Identify agreements, disagreements, and unique insights from each."
         SourceTexts (format-sources Sources)
         UserMsg (cn "Compare these sources:\n\n" SourceTexts)
      [SystemMsg UserMsg]))

\* --- Source formatting --- *\
\* Turn grounded sources into text the AI can read *\

(define format-sources
  \* Format a list of grounded sources into a numbered text block *\
  { (list grounded-source) --> string }
  Sources -> (format-sources-h Sources 1))

(define format-sources-h
  { (list grounded-source) --> number --> string }
  [] _ -> ""
  [[Page Hit] | Rest] N ->
    (let Title (head Hit)
         Url (head (tail Hit))
         Content (head (tail Page))
         Header (cn "[" (cn (value->string N) (cn "] " (cn Title (cn " (" (cn (value->string Url) ")"))))))
         Body (truncate Content 500)
         Entry (cn Header (cn "\n" (cn Body "\n\n")))
      (cn Entry (format-sources-h Rest (+ N 1)))))

\* --- Response processing --- *\
\* Pure functions for processing AI output *\

(define extract-summary-text
  \* Get the text from an AI response *\
  { ai-response --> string }
  [Prompt Text Ts] -> Text)

(define summarize-with-sources
  \* Full pipeline: take grounded sources, generate summary, return research-summary *\
  { search-query --> (list grounded-source) --> research-summary }
  Query Sources ->
    (let Prompt (make-summary-prompt Query Sources)
         Response (ai-generate Prompt)
      [Query Sources Response]))

\* --- Generative UI content --- *\
\* Shen decides WHAT content to generate for each UI section *\

(define generate-ui-content
  \* Given a research summary, produce content blocks for the UI *\
  { research-summary --> (list ui-text-block) }
  [Query Sources Response] ->
    (let SummaryBlock [(extract-summary-text Response) "summary"]
         SourceCount [(cn "Based on " (cn (value->string (length Sources)) " sources")) "meta"]
         QueryBlock [(cn "Research: " (value->string (head Query))) "query-echo"]
      [QueryBlock SourceCount SummaryBlock]))

\* --- Utility --- *\

(define truncate
  \* Truncate a string to at most N characters *\
  { string --> number --> string }
  S N -> (if (<= (length S) N)
             S
             (cn (substr S 0 N) "...")))

(define value->string
  \* Convert any value to its string representation *\
  { A --> string }
  X -> (js.call "bridge.toString" X))

(define substr
  \* Extract a substring *\
  { string --> number --> number --> string }
  S Start End -> (js.call "bridge.substr" [S Start End]))
