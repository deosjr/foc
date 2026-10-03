# Fog of Command — Toy Version Spec

Oct 3, 2026 · @Sjoerd Dost

## Overview

Fog of Command is a single-player, turn-based strategy game set in the ancient world. The player commands armies only by writing free-text letters to generals, and learns what happened only from the generals' written reports. The toy version exists to prove the dispatch → interpretation → resolution → report loop end to end, in Go, with swappable model backends. Gameplay depth is explicitly not a goal yet.

**Goals**

- A complete game playable in a web browser: see the believed map, draft and revise letters, end turns, read reports, reach a win or loss.
- A deterministic engine owns all ground truth. Models only translate text into structured orders and structured facts into text.
- The decision model and the LLM each sit behind a Go interface, chosen by config, with no code changes to switch providers.
- Mock providers for both, so the game runs offline and tests are deterministic.
- Every turn is reproducible from a seed plus a recorded log of model responses.
- An after-action review that shows the truth beside what the player was told.

**Non-goals for the toy**

- Balance, a competent enemy AI, supply lines, politics or diplomacy.
- Polished graphics, animation, accounts or hosting. A local web page with a rudimentary SVG map is enough, and there is no terminal UI.
- Multiple eras or rulesets. The engine should not hard-code ancient-era numbers, but only one ruleset ships.
- Enemy generals running through the LLM pipeline. The enemy issues structured orders directly.

## PoC scope

This spec describes the full toy; the first build is a smaller proof of concept. The PoC must answer one question: **when the player writes a letter, does the general's action feel like a fair reading of it, and does his report feel like his?** Anything that doesn't serve that question is built later, so the sections below should be read with this table in hand.

| Area | PoC | Later |
| --- | --- | --- |
| Orders | `Hold`, `MoveToward`, `Retreat` | `Support`, `Entrench`, `Scout` |
| Generals | Two, with strongly contrasting traits (Velk and Saris) | Hesk; loyalty, refusals and plausibility handling |
| Decision providers | `mock` and `llm` | `jev`, other decision models |
| LLM providers | `mock` and one real adapter | The others |
| Interpretation policy | Clear, ambiguous and unclear branches (steps 3–5, with "unclear" meaning hold) | Steps 1, 2, 6 and 8; clarification letters |
| Distortion | Caution and vanity on numbers | Honesty-driven omissions |
| Couriers | Delay by distance | Interception |
| Enemy | One army following a fixed scripted route | The heuristic AI, a second army |
| Web UI | Map, composers with saved drafts, inbox, End turn with an `hx-indicator` spinner | SSE progress, route overlay, recognised-names line |
| Review | A debug drawer on each report showing the letter, probabilities, draw and chosen order | The `/review` page with side-by-side maps; rationales |
| Reproducibility | Response cache for development | Exact replay |
| Game length | Armies start two provinces apart; 10-turn cap | Full map and 20 turns |

The debug drawer is the most important PoC feature: it is how every threshold and trait formula gets tuned.

**Evaluation harness.** The PoC includes `foc eval`, which runs a fixed set of 30–50 letters from `testdata/eval/letters.yaml`, each with a hand-written context and the interpretation a human would expect, through any configured decision provider. It reports agreement on clear letters, the probability split on ambiguous ones, and outright errors. It needs no game loop or UI, and it is how providers and prompt changes are compared. Letters from played sessions can be added to the set over time.

**Concurrency.** Model calls for different generals run concurrently. All RNG draws happen afterwards, in a fixed order (generals sorted by id), so concurrency never changes outcomes.

**Known limitation: conditional orders.** Letters like "if they cross, attack; otherwise hold" are flattened to a single action in the PoC. Conditional standing orders (a trigger plus an action) are the planned next step and will extend `StandingOrder` and the question set.

**Budget.** Per general per turn: one decision call and one report call (plus one rationale call later). With two generals that is about four calls per turn; caching is on by default during development.

**The PoC is done when:**

- [ ] A 10-turn game plays end to end in the browser with real providers.
- [ ] On the eval set, the decision provider agrees with the expected action on at least 90% of clear letters.
- [ ] Given the same ambiguous letters, Velk and Saris visibly diverge in the direction their traits predict.
- [ ] No report fails validation twice over a full game, so no fallback letters are shown.
- [ ] A turn completes in under 15 seconds.

## Architecture and turn pipeline

The central invariant: **the engine is the only source of truth, and models never write to game state directly.** The decision model turns letters into probabilities over a closed action set. The engine turns those probabilities into orders, resolves them, and decides what each general saw. The LLM only renders already-decided facts as prose.

&#91;embedded content: architecture · letters out along the top, reports back along the bottom\]

Letters flow right along the top row and reports flow back along the bottom; the two dashed model boxes only translate between text and structure.

A turn runs these phases in order. Phases 3–9 run without player input.

1. **Compose.** The player writes zero or more letters, each addressed to one general. Each becomes a `Dispatch` stamped with the turn it was sent.
2. **Transit.** The messaging system computes each dispatch's arrival turn from the courier route and rolls for interception (see Messaging). Dispatches not yet arrived stay in transit.
3. **Interpretation.** For each dispatch arriving this turn, the interpreter asks the decision model a fixed question set and receives probabilities. The general's personality policy converts those probabilities into one `Order`, or into a clarification request, or into a refusal.
4. **Standing orders.** Generals with no new dispatch continue their current `StandingOrder` (for example, keep marching toward a named province).
5. **Enemy orders.** The enemy AI issues structured orders directly, with full knowledge of its own armies.
6. **Resolution.** The engine resolves all orders simultaneously and deterministically.
7. **Perception.** For each general, the engine computes an `Observation`: what that general actually saw this turn, with the noise rules in Generals.
8. **Distortion.** The personality policy turns each `Observation` into `ReportFacts` by applying the general's biases deterministically, using the seeded RNG for omissions.
9. **Reporting.** The LLM writes a letter from `ReportFacts` in the general's voice. A validator checks it against the facts. The report then enters transit back to the player.
10. **Delivery.** Reports and clarification letters that arrive this turn land in the player's inbox, and the player's belief map is updated from them only.

Every phase appends structured records to the turn log, including every model request and response and every RNG draw, so a turn can be replayed exactly.

## Game world and rules

The toy ships one small map, three player generals, two enemy armies and a fully deterministic strength-based resolution. All numbers below live in a ruleset file (`rulesets/ancient.yaml`), not in code, so other eras can change them later.

**Map.** Ten provinces on an undirected graph, loaded from `maps/valley.json`. Each province has a name, a terrain type, an optional supply-centre flag and an owner. Terrain gives a defence multiplier. Each province also has a hand-placed `pos` in a 1000 × 600 layout space, used only by the web map.

```json
{
  "name": "The Valley of Ilth",
  "terrain": {"plain": 1.0, "hills": 1.25, "forest": 1.2, "marsh": 1.1, "ford": 1.15, "city": 1.5},
  "provinces": [
    {"id": "karsa",  "name": "Karsa",        "terrain": "city",   "supply": true,  "owner": "player",  "capital": true, "pos": [100, 300]},
    {"id": "velia",  "name": "Velia",        "terrain": "plain",  "supply": true,  "owner": "player",  "pos": [280, 190]},
    {"id": "duna",   "name": "Duna Hills",   "terrain": "hills",  "supply": false, "owner": "player",  "pos": [280, 420]},
    {"id": "oros",   "name": "Oros Ford",    "terrain": "ford",   "supply": false, "owner": "neutral", "pos": [460, 130]},
    {"id": "hollow", "name": "Hollow Wood",  "terrain": "forest", "supply": false, "owner": "neutral", "pos": [460, 300]},
    {"id": "marren", "name": "Marren",       "terrain": "plain",  "supply": true,  "owner": "neutral", "pos": [640, 210]},
    {"id": "lyde",   "name": "Lyde Marsh",   "terrain": "marsh",  "supply": false, "owner": "neutral", "pos": [460, 480]},
    {"id": "ilth",   "name": "Pass of Ilth", "terrain": "hills",  "supply": false, "owner": "enemy",   "pos": [660, 430]},
    {"id": "sarnos", "name": "Sarnos",       "terrain": "plain",  "supply": true,  "owner": "enemy",   "pos": [820, 200]},
    {"id": "kethra", "name": "Kethra",       "terrain": "city",   "supply": true,  "owner": "enemy",   "capital": true, "pos": [900, 370]}
  ],
  "edges": [
    ["karsa","velia"], ["karsa","duna"], ["velia","oros"], ["velia","hollow"], ["duna","hollow"],
    ["duna","lyde"], ["oros","marren"], ["hollow","marren"], ["lyde","ilth"], ["marren","ilth"],
    ["marren","sarnos"], ["ilth","kethra"], ["sarnos","kethra"]
  ]
}
```

**Armies.** Each army has an id, a side, a location and a strength in hundreds of men (30 = 3,000). Each player army is led by exactly one general; enemy armies have a commander name for flavour only. Starting forces: three player armies of 30, 25 and 20 near Karsa; two enemy armies of 30 and 25 near Kethra. An army below strength 5 disbands.

**Order types.** The engine accepts exactly these:

| Order | Parameters | Effect |
| --- | --- | --- |
| `Hold` | none | Stays and defends. |
| `MoveToward` | target province (any) | Steps one province along the shortest path toward the target. Persists as a standing order until reached. |
| `Support` | friendly army | Adds 50% of own strength to that army's move or hold this turn. Cut if the supporter is attacked. |
| `Entrench` | none | Holds with an extra +0.25 defence multiplier. Takes effect from the second consecutive turn. |
| `Scout` | adjacent province | Holds, and sees the target province exactly this turn. |
| `Retreat` | adjacent province | Moves away; cannot attack. Fails if the target is occupied by an enemy. |

**Resolution.** Simultaneous and deterministic; no dice in combat.

1. Compute each army's intended destination. Non-moving orders mean the army stays.
2. Cut supports: any supporter whose province is the destination of an enemy move loses its support.
3. For each province, gather contenders. A defender's power is strength × terrain multiplier (+0.25 if entrenched) + supports. An attacker's power is strength + supports.
4. The highest power takes the province. A tie means a stand-off: nobody enters and the defender stays.
5. Two armies moving into each other's provinces fight at the border with the same rule; the loser stays home.
6. Casualties per battle: loser loses 30% of its strength, winner loses 10% of its own. Round half up.
7. A dislodged defender retreats to the adjacent province with no enemy army that is closest to its own capital (ties broken by province id), or disbands if none exists.
8. A province's owner changes when an army ends the turn in it unopposed.

**Victory.** The player wins by taking Kethra or holding four of the five supply centres at the end of a turn. The player loses if Karsa falls or all armies are destroyed. Turn 20 ends the game with a score by supply centres held.

**Enemy AI.** A few lines of heuristics, deliberately dumb: if an enemy army is adjacent to Kethra and Kethra is empty, move back to defend; otherwise move toward the nearest player-owned or neutral supply centre; support the other enemy army if it is attacking an adjacent province. The AI sees true state.

## Generals and personality

Each general is a fixed set of numeric traits applied by Go code, plus a short bio and voice description used only by the LLM. Traits are never sent to the decision model, so reading comprehension stays neutral and character stays deterministic and learnable. The player sees the bio, never the numbers (except with `--debug`).

**Traits** are floats in \[0, 1\], loaded from `content/generals.yaml`:

| Trait | Used in | Effect |
| --- | --- | --- |
| Aggression | Interpretation | Raises the weight of attacking readings of an ambiguous order. |
| Caution | Interpretation, reporting | Raises the weight of holding readings; inflates reported enemy strength. |
| Initiative | Interpretation | Decides whether an unclear order is acted on by own judgement or sent back for clarification. |
| Honesty | Reporting | Lowers the chance that bad news is omitted from a report. |
| Vanity | Reporting | Shrinks reported own losses and inflates reported enemy losses. |
| Loyalty | Interpretation | Below a threshold, risky orders may be quietly refused. Falls when letters read as implausible. |

**The three toy generals:**

| General | Aggr. | Caut. | Init. | Hon. | Vain | Loyal | Bio shown to player |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Damar Velk | 0.85 | 0.15 | 0.6 | 0.5 | 0.8 | 0.7 | A veteran of the hill wars, quick to the charge and quicker to boast. |
| Ione Saris | 0.2 | 0.85 | 0.25 | 0.9 | 0.2 | 0.9 | Careful, literal-minded, and scrupulously truthful in her letters. |
| Hesk the Elder | 0.5 | 0.5 | 0.9 | 0.35 | 0.4 | 0.5 | An old campaigner who trusts his own eyes over any letter from court. |

**Interpretation policy** (in `internal/generals/policy.go`). Input: the decision model's answers (see Decision model). Output: an `Order`, a clarification request, or a refusal. Thresholds are config values.

1. If plausibility < 0.35: loyalty − 0.1, the general keeps his standing order, and his next report expresses concern about the letter. Stop.
2. If the letter carries no instruction (P(addressed) < 0.5): keep the standing order. Stop.
3. Let p be the action distribution. If max(p) ≥ 0.8, take the top action. This is a clear order and personality does not matter.
4. If max(p) < 0.4 or P(unclear) is the top answer: with probability = Initiative, act on own judgement (step 6); otherwise hold and send a clarification letter.
5. Otherwise the order is ambiguous. Reweight: aggressive actions (`MoveToward` any province the player does not own, `Support`) × (0.5 + Aggression); defensive actions (`Hold`, `Entrench`, `Retreat`) × (0.5 + Caution). Also blend in the letter's engagement score e: aggressive × (0.5 + e), defensive × (1.5 − e). Scout is neutral. Drop "unclear", renormalise over the remaining actions and sample with the interpretation RNG stream.
6. Own judgement uses a tiny per-general heuristic: aggressive generals move toward the nearest enemy army, cautious ones entrench, others hold.
7. Target and support parameters come from the target questions' top answer. If the chosen action needs a target and the top answer is "none" or "unclear", fall back to step 4.
8. Loyalty check: if Loyalty < 0.3 and the order attacks a province the general believes holds more strength than his own, refuse with probability (0.3 − Loyalty) × 3 and hold instead.

The interpreter stores the full probability vectors, the reweighted vector, the RNG draw and the chosen order as an `Interpretation` record. It is hidden from the player until the after-action review.

**Perception.** What a general observes after resolution:

- Own province and own army: exact strength, exact losses, battle outcome.
- Adjacent provinces: enemy presence exact; enemy strength ±25% uniform noise, rounded to the nearest 5.
- A scouted province: exact.
- Friendly armies in adjacent provinces: exact location and general's name.
- Enemy losses in a battle he fought: true value ±20% noise.

**Distortion** turns an `Observation` into `ReportFacts`, deterministically except for omission draws:

- Reported enemy strength = observed × (0.6 + 0.8 × Caution), rounded to the nearest 5.
- Reported own losses = true × (1 − 0.6 × Vanity); reported enemy losses = observed × (1 + 0.6 × Vanity).
- Each negative fact (lost battle, lost province, refused order, failed support) is omitted with probability 0.6 × (1 − Honesty), drawn from the reporting RNG stream.
- Omitted facts are recorded in the log for the after-action review.

## Messaging: couriers, latency, interception

Every letter, in either direction, is carried by a courier along the shortest path between the player's capital and the general's province. Couriers move 2 provinces per turn, so **delay in turns = floor(hops / 2)**: a general adjacent to Karsa acts on a letter the same turn; one 2–3 provinces away acts a turn later. Same-turn delivery over short distances is intended, since a turn is long enough for a rider to cover a province or two. The delay formula lives in the ruleset, so later modifiers such as weather, season or road quality can add turns.

- **Arrival.** A dispatch sent in turn T with delay d is interpreted in turn T + d, before resolution. The route is computed at send time; on arrival the courier finds the general wherever he now is (toy simplification).
- **Order of arrival.** Each dispatch carries a sequence number. If two letters to one general arrive in the same turn, both are interpreted in order and the later one wins.
- **Interception.** For each province on the route that contains or is adjacent to an enemy army at the time of travel, roll 10% from the messaging RNG stream. An intercepted letter is lost silently. The player is never told; the general never receives it. The enemy AI ignores captured letters in the toy, but they are logged for the review.
- **Reports** travel back with the same rules, sent at the end of resolution. A report always states the turn it was written, so the player can tell stale news from fresh.
- **Clarification letters** from a general are reports with a `kind` of `clarification`; they travel the same way.
- **Standing orders.** Each general stores his last interpreted intent. `MoveToward` persists until the target is reached or the path is blocked by a battle loss; every other order reverts to `Hold` after one turn, except `Entrench`, which persists.
- **Player knowledge.** The player sees when each of their letters was sent but not when, or whether, it arrived. They learn that from the replies.

## Decision model interface and question set

The decision model answers typed questions about a context string with probabilities over a closed answer set. The interface mirrors the three primitives decision models such as Jev expose (choice, score, yes/no), so any provider can be adapted to it, including a plain LLM.

```go
package decision

type Kind string

const (
    KindChoice Kind = "choice" // one of Options, probability per option
    KindScore  Kind = "score"  // a value in [0,1]
    KindBinary Kind = "binary" // probability that the answer is yes
)

type Question struct {
    ID      string
    Kind    Kind
    Prompt  string
    Options []string // KindChoice only
}

type Request struct {
    State     string // plain text or JSON context
    Questions []Question
}

type Answer struct {
    QuestionID string
    Probs      map[string]float64 // KindChoice; sums to 1 after normalisation
    Score      float64            // KindScore
    PYes       float64            // KindBinary
    Confidence float64            // provider confidence if offered, else 1
}

type Response struct {
    Answers  []Answer
    Provider string
    Model    string
    Raw      []byte // raw provider payload, kept for the log
}

type Model interface {
    Decide(ctx context.Context, req Request) (Response, error)
}
```

The interpreter must normalise choice probabilities and treat a missing answer as an error, never as a default.

**State string.** Built by `internal/interpret/state.go` as plain labelled text. It contains: the general's name and current province; the names and ids of adjacent provinces; the full list of province names on the map; friendly generals and their last known locations; the general's standing order; the turn the letter was sent and the turn it arrived; then the letter text, fenced and labelled as the letter. It never contains traits, bios or true enemy positions.

**Question set** sent with every dispatch, in one request:

| ID | Kind | Prompt (abridged) | Options | Used for |
| --- | --- | --- | --- | --- |
| `plausible` | score | How much does this read as genuine, coherent orders from a sovereign to a field commander? | — | Loyalty and refusal (policy step 1) |
| `addressed` | binary | Does the letter instruct this general to do something? | — | Ignore pure news or praise (step 2) |
| `action` | choice | What is the general most plausibly being told to do? | hold, move, support, entrench, scout, retreat, unclear | Main action (steps 3–5) |
| `target` | choice | Which province is the main target of the instruction? | every province name, none, unclear | `MoveToward`, `Scout`, `Retreat` |
| `support_whom` | choice | Which friendly general, if any, should be supported? | each friendly general, none, unclear | `Support` |
| `engagement` | score | How strongly does the sovereign want battle rather than avoiding it? | — | Ambiguity reweighting (step 5) |

Province options use display names ("Oros Ford"), mapped back to ids in Go. Questions are built from templates in `prompts/questions.yaml` so they can be tuned without code changes.

**Worked example.** Ione Saris is in Velia. The letter reads: *"The enemy has been seen near the ford. Make sure they do not cross."* A plausible response:

```json
{"plausible": 0.93, "addressed": 0.95,
 "action": {"hold": 0.30, "move": 0.38, "entrench": 0.18, "support": 0.02, "scout": 0.07, "retreat": 0.0, "unclear": 0.05},
 "target": {"Oros Ford": 0.86, "Velia": 0.09, "unclear": 0.05},
 "engagement": 0.55}
```

No action reaches 0.8, so it is ambiguous. Saris's caution of 0.85 weights hold and entrench by 1.35, and her aggression of 0.2 weights move by 0.7. After the engagement blend and renormalising, she holds or entrenches in Velia about 63% of the time and marches to the ford about 28%. Damar Velk, given the same letter, marches onto the ford about 58% of the time. Both readings are defensible, which is the point.

**Providers to implement:**

- `mock`: deterministic keyword rules ("hold", "march to X", province name matching), so the game and tests run offline.
- `llm`: wraps any `llm.Model` from the next section. It asks for JSON with a probability per option, validates it against the question set, and normalises. This lets the game run with only one API key.
- `jev`: TypeSafe's hosted decision model over HTTPS. Pin an exact model version in config rather than a moving alias. The request and response mapping must be written from TypeSafe's current API docs; do not guess field names.
- Later candidates: OpenRouter's decisions endpoint, or a locally hosted open decision model such as Strands Decider 2B.

## LLM interface and report generation

The LLM writes prose from facts the engine has already decided: report letters, clarification letters, and the hidden rationale shown in the after-action review. It never decides outcomes.

```go
package llm

type Role string

const (
    RoleUser      Role = "user"
    RoleAssistant Role = "assistant"
)

type Message struct {
    Role    Role
    Content string
}

type Request struct {
    System      string
    Messages    []Message
    MaxTokens   int
    Temperature float64
    JSONSchema  []byte // optional; adapters that support structured output use it
}

type Response struct {
    Text       string
    Provider   string
    Model      string
    InputTok   int
    OutputTok  int
    Raw        []byte
}

type Model interface {
    Complete(ctx context.Context, req Request) (Response, error)
}
```

**Providers to implement:** `mock` (fills Go text templates from the facts, no network), `anthropic` (Messages API over plain `net/http`), and `openaicompat` (any OpenAI-style chat completions endpoint: OpenAI, OpenRouter, Ollama, llama.cpp, vLLM). Adapters use `net/http` directly rather than vendor SDKs, to keep them thin and uniform.

**ReportFacts** is the only input about the world the LLM receives. It is the distorted, omission-filtered output of the personality policy:

```json
{
  "general": "Damar Velk", "written_turn": 6, "location": "Oros Ford",
  "order_understood": "MoveToward Oros Ford", "order_source": "letter sent turn 5",
  "own_strength": 26, "own_losses": 2,
  "battle": {"place": "Oros Ford", "result": "won", "enemy_losses": 12},
  "sightings": [{"province": "Marren", "enemy_strength": 15, "certainty": "rumour"}],
  "friendly_contacts": ["Ione Saris in Velia"],
  "concerns": [],
  "refused": false
}
```

**Report prompt.** Stored in `prompts/report.tmpl`. The system prompt sets the persona (name, bio, voice notes from `generals.yaml`), the setting (an ancient-world field letter to one's sovereign, no modern concepts or units), and these hard rules:

- State only facts present in FACTS. Invent no battles, places, numbers or people.
- Every number you mention must appear in FACTS. Round words like "some" or "near" are allowed.
- Do not mention anything absent from FACTS, even to say it did not happen.
- 80–180 words, first person, addressed to the sovereign, signed with your name.

**Validation.** `internal/report/validate.go` checks the letter: every integer in the text must match a number in the facts (allowing ×100 for "2,600 men" against strength 26, and normalising number words such as "twelve hundred" to 1200 first); every province name mentioned must be in the facts. On failure, retry once with the violation appended to the prompt. On a second failure, fall back to the mock template letter and log the failure. The game must never block on an LLM.

**Example output** for the facts above, in Velk's voice:

> *To my lord in Karsa. As you commanded, I took the ford. They stood against us, and we broke them; twelve hundred of theirs lie in the shallows, and I lost scarcely two hundred. I hold Oros now with 2,600 good men. Riders say perhaps fifteen hundred wait in Marren, though I put little stock in it. Lady Saris keeps to Velia. Send word and I will push on. — Damar Velk*

**Clarification letters** use the same machinery with a `clarification` template: the facts include the parts of the letter the general could not resolve (for example, the target answer was "unclear").

**Rationale.** At interpretation time, a short in-character private note is generated from the letter and the chosen order ("The king says hold, but a ford is held from its far bank"). It is stored with the `Interpretation` record and shown only in the after-action review. Rationale generation can be disabled in config to save calls.

## Pluggable providers and configuration

Switching either model is a config change only. Each provider package registers a factory under a name; `main` builds the configured provider and wraps it in shared middleware.

```go
// internal/providers/registry.go
type DecisionFactory func(cfg ProviderConfig, deps Deps) (decision.Model, error)
type LLMFactory      func(cfg ProviderConfig, deps Deps) (llm.Model, error)

func RegisterDecision(name string, f DecisionFactory)
func RegisterLLM(name string, f LLMFactory)
func BuildDecision(cfg ProviderConfig, deps Deps) (decision.Model, error)
func BuildLLM(cfg ProviderConfig, deps Deps) (llm.Model, error)

// Deps carries what adapters may need, e.g. the built LLM for the
// "llm" decision provider, an *http.Client, and the logger.
```

Each adapter package calls `Register*` in `init()`, and `cmd/foc/main.go` blank-imports all of them. Adding a provider means adding one package and one import line.

**Middleware**, applied in this order around every provider, each a type that implements the same interface:

1. **Record/replay cache.** Keys on a hash of the provider name, model and request. In `record` mode it stores responses to `runs/<id>/responses.jsonl`; in `replay` mode it serves only from that file and errors on a miss. This makes any session exactly reproducible and makes tests free.
2. **Logging.** Writes request, response, latency and token counts to the turn log.
3. **Retry.** Exponential backoff on network errors, 429 and 5xx, at most 3 attempts.
4. **Timeout.** Per-call deadline from config.

**config.yaml**

```yaml
seed: 42
mode: live            # live | record | replay
replay_file: ""

decision:
  provider: jev       # mock | llm | jev
  model: jev-1.13.0   # always pin a version
  endpoint: https://api.typesafe.ai/v1/systemone
  api_key_env: TYPESAFE_API_KEY
  timeout: 10s

llm:
  provider: anthropic # mock | anthropic | openaicompat
  model: claude-sonnet-4-6
  endpoint: ""        # required for openaicompat, e.g. http://localhost:11434/v1
  api_key_env: ANTHROPIC_API_KEY
  temperature: 0.8
  max_tokens: 400
  timeout: 30s

interpretation:
  clear_threshold: 0.8
  unclear_threshold: 0.4
  plausibility_threshold: 0.35
  rationale: true

ruleset: rulesets/ancient.yaml
map: maps/valley.json
generals: content/generals.yaml
```

API keys are read only from the environment variable named in config, never from the file. Running with both providers set to `mock` must work with no network and no keys; that is the default `config.example.yaml`.

## Core data model

These types are the contract between packages. Field names may change during implementation; the split between true state, player belief and per-general knowledge must not. `GameState` is the truth and is never shown to the player outside debug and review. Package names below are illustrative; shared value types live in internal/model (see the layout section).

```go
package engine

type Side string // "player" | "enemy" | "neutral"

type Province struct {
    ID, Name string
    Terrain  string
    Supply   bool
    Capital  bool
    Owner    Side
    Adjacent []string
    Pos      [2]float64 // layout coordinates for the web map only
}

type Army struct {
    ID         string
    Side       Side
    Location   string
    Strength   int    // hundreds of men
    GeneralID  string // player armies only
    Entrenched int    // consecutive entrench turns
}

type OrderType string // Hold, MoveToward, Support, Entrench, Scout, Retreat

type Order struct {
    ArmyID        string
    Type          OrderType
    Target        string // province id, if any
    SupportArmyID string
}

type GameState struct {
    Turn      int
    Provinces map[string]*Province
    Armies    map[string]*Army
    Generals  map[string]*generals.General
    Standing  map[string]Order // by general id
    InTransit []messaging.Letter
    Over      bool
    Winner    Side
}
```

```go
package generals

type Traits struct {
    Aggression, Caution, Initiative, Honesty, Vanity, Loyalty float64
}

type General struct {
    ID, Name, Bio, Voice string
    Traits              Traits
    ArmyID              string
}
```

```go
package messaging

type LetterKind string // "dispatch" | "report" | "clarification"

type Letter struct {
    ID          string
    Seq         int
    Kind        LetterKind
    From, To    string // "sovereign" or a general id
    Body        string
    SentTurn    int
    ArriveTurn  int
    Route       []string
    Intercepted bool // truth only; never shown to the player during play
}
```

```go
package interpret

type Interpretation struct {
    LetterID     string
    GeneralID    string
    Turn         int
    Answers      decision.Response
    Reweighted   map[string]float64
    RNGDraw      float64
    Outcome      string // "order" | "clarify" | "refuse" | "ignored"
    Order        *engine.Order
    Rationale    string
}
```

```go
package report

type Observation struct { /* exact and noisy facts, as in Generals */ }

type ReportFacts struct { /* the JSON shape in the LLM section */ }

type Belief struct {
    // The player's map: one entry per province, built ONLY from delivered letters.
    Provinces map[string]BeliefEntry
}

type BeliefEntry struct {
    LastKnownOwner   engine.Side
    EnemyStrength    *int
    FriendlyGenerals []string
    AsOfTurn         int
    Source           string // general id
}
```

```go
package web

// PlayerView is the only game data the browser ever receives outside debug mode.
type PlayerView struct {
    Turn     int
    Season   string
    Belief   report.Belief
    Generals []GeneralView   // name, bio, last reported location and turn, courier delay
    Drafts   map[string]string // general id -> draft text
    Sent     []SentLetter      // body, sent turn, reply letter id if any
    Inbox    []messaging.Letter // delivered reports and clarifications only
    Over     bool
    Outcome  string
}
```

## Web interface, logging and reproducibility

The game is a local web app: the Go binary serves server-rendered HTML pages and fragments on 127.0.0.1, and the player plays in a browser. There is no terminal UI. The page shows only the player's belief, never the truth, and the server enforces that boundary rather than the browser.

**Stack.** Go backend plus htmx, with as little JavaScript as possible. The server renders all HTML with `html/template`, including the SVG map, and returns HTML fragments that htmx swaps into the page. There is no JSON API for the browser, no client-side state and no build step. Static assets are embedded with `go:embed`: a vendored, version-pinned htmx 2 file, its SSE extension and one stylesheet. The only hand-written script is `compose.js`, under 40 lines, which inserts a clicked province name at the textarea cursor; everything else must be done in templates and htmx attributes. One game per server process; no accounts.

&#91;embedded content: main screen wireframe · header, letters, believed map, inbox\]

A wireframe, not a visual design: the point is the layout, the as-of badges and that every mark on the map comes from a delivered report.

**Screen.** One page with a header and three areas:

- **Header.** Turn number, season and an **End turn** button (`hx-post="/turn"`). While the server works, a progress line subscribed through the htmx SSE extension shows the current phase ("couriers riding", "generals reading", "battle", "reports being written"). When the turn finishes, the server sends a final event that triggers htmx to reload the map, letters and inbox fragments.
- **Map (centre).** Server-rendered inline SVG: provinces as circles at their `pos`, roads as lines, supply centres marked, names labelled. Each province is shaded by its *last known* owner, with a small "T5" badge showing the turn that knowledge dates from. General tokens sit at their last reported location and fade as reports age. Enemy sightings show the reported strength, prefixed with "\~" when the report called it uncertain. Each province carries an SVG `<title>` for a native hover tooltip, and clicking it (`hx-get`) loads its full belief entry into a detail box below the map. A courier-route overlay is a second rendering of the map fragment, toggled by a link that requests it with a query parameter.
- **Letters (left).** One composer per general, a plain `<textarea>` with `hx-put` on `keyup changed delay:800ms`, so drafts are saved to the server as the player types. Drafts can be revised or discarded freely until the turn ends; **End turn** seals and dispatches them. The save response re-renders a small line under the textarea listing the province names the server recognised in the draft and any capitalised words it could not match, which catches typos before the decision model sees them. Clicking a province on the map while a composer is focused inserts its name at the cursor (the one `compose.js` behaviour). Each composer shows the general's bio, the turn of his last report and the expected courier delay. Below it, sent letters with their turn sent and either "no reply yet" or a link to the reply.
- **Inbox (right).** Delivered reports and clarification letters, newest first, each stamped "written turn N, arrived turn M". Selecting a report (`hx-get`) re-renders the map fragment with the provinces it mentions highlighted.

**Routes.** Pages and fragments are HTML. A route that serves a fragment checks the `HX-Request` header and returns the full page without it, so every view also works as a plain link. Each fragment is its own named template, so tests can render them in isolation.

| Method and path | Returns | Purpose |
| --- | --- | --- |
| `GET /` | page | The main screen, rendered from the `PlayerView`. |
| `GET /map` | fragment | The believed map. Query parameters: `routes=1` for the courier overlay, `highlight=<letter id>` to mark provinces a report mentions. |
| `GET /province/{id}` | fragment | The belief detail box for one province. |
| `PUT /drafts/{generalId}` | fragment | Saves the draft text and returns the recognised-names line. |
| `DELETE /drafts/{generalId}` | fragment | Discards the draft and returns an empty composer. |
| `GET /letters` | fragment | Composers and sent letters. |
| `GET /inbox` | fragment | Delivered letters, newest first. |
| `GET /letter/{id}` | fragment | One delivered letter in full. |
| `POST /turn` | fragment | Seals drafts and starts phases 2–10 in the background; returns the progress line. Rejects a second call while a turn is running. |
| `GET /turn/events` | SSE | Phase names as events, then a `turn-done` event that triggers htmx to refresh the map, letters and inbox. |
| `GET /review` | page | After-action review. 403 until the game is over, unless `--debug`. |
| `GET /debug/truth` | page | The true `GameState`. Registered only with `--debug`. |
| `POST /save` | fragment | Writes the run directory and returns a confirmation. |

**Belief boundary.** Non-debug templates receive only a `PlayerView`, built from `Belief`, the player's own letters and delivered letters. `GameState` is unreachable from them. Template data is typed, so a template cannot reach truth by accident. A test asserts that no `PlayerView` contains an enemy position or strength that did not appear in a delivered report.

**After-action review** is a second page, `/review`. Stepping turn by turn with previous and next buttons, it shows two maps side by side, belief on the left and truth on the right. Under them, per general: the letter as written, when it arrived or that it was intercepted, the decision probabilities, the reweighted distribution and draw, the chosen order, the private rationale, the true outcome, the facts distorted or omitted, and the report as delivered.

**Run directory** (`runs/<timestamp>-<seed>/`):

- `config.yaml`: a copy of the effective config.
- `turns.jsonl`: one record per event: letters, interpretations, orders, resolution results, observations, report facts, RNG draws.
- `responses.jsonl`: recorded model responses, used by replay mode.
- `state.json`: the final `GameState`.

**Determinism.** One root seed derives separate PCG streams (`math/rand/v2`) for messaging, interpretation, perception noise and reporting, so a change in one subsystem never shifts another's draws. Combat uses no randomness. Map iteration in Go is unordered, so every loop that affects outcomes must iterate sorted keys. Given the same seed, config, letters and recorded responses, `foc --replay runs/X` must reproduce `turns.jsonl` byte for byte, ignoring timestamp and latency fields.

**Flags:** `--config`, `--seed` (overrides config), `--debug`, `--replay <run dir>`, `--addr` (default `127.0.0.1:8080`), `--open` (opens the browser on start). Automated tests and demos drive the game through the `internal/game` Go API, or the HTTP routes with a scripted list of letters; they need no browser.

## Project layout, testing and milestones

Go 1.23 or later. Dependencies: the standard library plus `gopkg.in/yaml.v3`, plus vendored htmx 2 and its SSE extension as static files. Shared value types (`Order`, `Side`, `Traits`, `Letter`) live in `internal/model` so that engine, generals, messaging and interpret packages do not import each other in a cycle.

```
fog-of-command/
  cmd/foc/main.go                 # flags, config, wiring, blank-imports providers; `foc eval` subcommand
  internal/eval/                  # eval harness: load labelled letters, score a decision provider
  internal/model/                 # shared value types
  internal/engine/                # state, resolution, perception, victory
  internal/mapdata/               # map + ruleset loading and validation
  internal/generals/              # traits, interpretation policy, distortion
  internal/messaging/             # routes, delays, interception, standing orders
  internal/interpret/             # state string, question set, decision call
  internal/report/                # facts, prompts, validation, fallback templates
  internal/enemy/                 # enemy AI heuristics
  internal/game/                  # turn orchestration (phases 1–10), run directory
  internal/review/                # after-action review rendering
  internal/web/                   # HTTP handlers, PlayerView, SSE progress
  internal/providers/registry.go
  internal/providers/middleware/  # cache, logging, retry, timeout
  internal/providers/decision/{decision.go, mock/, llm/, jev/}
  internal/providers/llm/{llm.go, mock/, anthropic/, openaicompat/}
  maps/valley.json
  rulesets/ancient.yaml
  content/generals.yaml
  prompts/{questions.yaml, report.tmpl, clarification.tmpl, rationale.tmpl}
  web/templates/                  # html/template pages and fragments, incl. the SVG map
  web/static/                     # htmx.min.js, sse.js, compose.js, style.css (go:embed)
  config.example.yaml             # mock + mock, runs offline
  testdata/                       # incl. eval/letters.yaml
```

**Testing.**

- **Engine:** table-driven tests for each resolution case: uncontested move, bounce on tie, support adds power, support cut by attack, dislodge and retreat, disband on no retreat, head-to-head swap, terrain and entrench multipliers.
- **Policy:** fixed answer vectors + traits + seed → expected outcome, covering every step of the interpretation policy, including the Saris and Velk example.
- **Distortion and messaging:** golden tests for reported numbers and omissions; delay and interception with a fixed seed.
- **Report validation:** letters with an invented number, an invented province and a clean letter.
- **Adapters:** each provider tested against an `httptest` server with recorded fixtures. A shared contract test suite runs every decision adapter and every LLM adapter through the same cases; live API runs sit behind a `live` build tag.
- **Web routes:** `httptest` tests for every route, full page versus fragment depending on the HX-Request header, drafts surviving until the turn ends, a second concurrent `POST /turn` rejected, debug routes absent without `--debug`, and the belief-boundary leak test.
- **End to end:** a scripted campaign of letters in `testdata/campaign1.yaml`, played through the `internal/game` API with mock providers for 10 turns, compared to a golden `turns.jsonl`.
- **Templates:** golden-HTML tests for the key fragments (map, composer, inbox card, recognised-names line), rendered from fixed PlayerViews; manual browser checks otherwise. Logic belongs in Go, never in templates or script.

**Milestones**, each ending in something runnable:

*PoC phase* (see PoC scope):

1. **Engine only.** Map loading, resolution for `Hold`, `MoveToward` and `Retreat`, victory, driven by Go tests and a debug endpoint that accepts structured orders.
2. **Pipeline on mocks, in the browser.** Mock decision model and LLM, interpretation steps 3–5, number distortion, reports, courier delay. The web page with map, composers, inbox, End turn and the per-report debug drawer. Playable offline.
3. **Eval harness.** `foc eval` and the first 30–50 labelled letters, run against the mock and the `llm` decision adapter.
4. **Real providers.** One real LLM adapter with report validation and fallback, the `llm` decision adapter, caching and retry middleware. Check the PoC done-criteria. *(Status: built and verified end to end on a local Llama 3.1 8B through Ollama; the done-criteria wait on a Claude API key.)*

*Later phases*, reordered so the tools for judging friction arrive before the friction itself, and the letter loop deepens before the rules do:

5. **Review and polish.** Rationales, the `/review` page with side-by-side maps, SSE progress, route overlay. (The recognised-names line already shipped with milestone 2.)
6. **Full friction.** Interception, clarification letters, plausibility and loyalty handling, honesty-driven omissions, standing orders beyond movement.
7. **Full rules.** `Support`, `Entrench`, `Scout`, the third general, the heuristic enemy AI, the full map and 20 turns. Hesk needs the own-judgement step from milestone 6.
8. **More providers.** `jev`, the remaining LLM adapters, exact record/replay. The `jev` adapter waits on TypeSafe's API docs and a key.
9. **Conditional orders.** Trigger-plus-action standing orders and the questions to extract them. Needs design decisions first: which triggers generals recognise, how many conditions a letter may carry, and what happens when a trigger fires while a letter is in transit.

## Open questions and later extensions

**Open questions for the toy**

- Jev's exact request and response schema, auth header and how its answer types map onto choice, score and binary. Verify against TypeSafe's docs before writing the adapter.
- How long a turn represents: a season, a month or a week. It sets courier speed in provinces per turn and the season label in the header; the toy assumes a season.
- How much of each general's traits the player may learn, and how (rumours at court, a trait revealed after a lie is caught).
- Whether clarification letters should cost the player anything beyond the delay.
- Whether the map should show the player's own guesses: letting the player drag pencilled markers onto provinces would make deduction tangible, at the cost of more UI.

**Later extensions, out of scope now**

- **A richer command layer**, where most of the game's depth lies: conditional orders, commander's intent that generals reason from when letters arrive late, readback before acting, and generals writing to each other, with rivalries and failed coordination.
- **Symmetry.** Enemy generals running the same interpretation pipeline, so the enemy suffers friction too. Interception then becomes a weapon: captured letters reveal intent, and decoy letters mislead.
- **Information as a resource.** Spies, scouts and merchants as competing sources to cross-check reports, and pencilled guesses on the map.
- **Eras.** Rulesets for signal fires and beacon chains, horse relays and telegraph lines that can be cut. Weather and seasonal modifiers on courier delay.
- **A political layer.** Loyalty, ambition, succession, court intrigue, and generals who grow too successful.
- **Unusual multiplayer.** A human sovereign with human generals receiving the same letters under the same fog; the LLM pipeline fills any empty seat.
- **A data flywheel.** Every played turn yields a labelled example (letter, context, chosen order, player reaction) that grows the eval set and could eventually train a custom decision model.
- **Richer presentation.** A drawn parchment map, unit icons, letters rendered as tablets or scrolls, and animated replays of reported movements.
- **Hosting** the game for others, which brings accounts, saved games and per-player API keys.

**Sources**

- [What is Jev? (Sanity glossary)](https://www.sanity.io/glossary/jev-typesafe-ai-model): release date, hosted-only access, endpoint and model route.
- [What Is Jev? (OpenRouter)](https://openrouter.ai/blog/insights/what-is-jev/): question primitives and the OpenRouter decisions route.
- [TypeSafe AI Releases Jev (InfoQ)](https://www.infoq.com/news/2026/10/typesafe-ai-jev-released/): advice to pin a model version.
- [Amazon releases Strands Decider (TechCrunch)](https://techcrunch.com/2026/10/01/amazon-releases-its-own-jev-clone-as-decision-models-flood-the-web/): open, locally runnable alternative.
