# Fog of Command

A single-player, turn-based strategy game in the ancient world. You command
your armies only by writing letters to your generals, and learn what happened
only from their reports. See [SPEC.md](SPEC.md) for the design.

This is the proof of concept: two generals (Damar Velk and Ione Saris), one
scripted enemy army, a 10-turn cap.

## Play

```bash
go run ./cmd/foc --open
```

With no `config.yaml`, both models are offline mocks: no network, no keys.
When the campaign ends, the **after-action review** (`/review`) steps through
each turn with what you believed beside what was true, and each general's
private rationale for what he did with your letters.
Add `--debug` for the debug drawer on each report (the letter, the decision
probabilities, the draw, the chosen order, and how the general distorted what
he saw) and the `/debug/truth` page.

## Switching models

Copy `config.example.yaml` to `config.yaml` and pick providers; it contains
ready-made blocks for Claude and for a local Ollama model. API keys are only
ever read from the environment variable named in the config.

- Decision providers: `mock` (keyword rules), `llm` (asks the configured LLM for JSON probabilities).
- LLM providers: `mock` (template letters), `anthropic`, `openaicompat` (OpenAI, OpenRouter, Ollama, llama.cpp, vLLM).

To run on a free local model instead (no key, nothing leaves the machine):

```bash
brew install ollama
OLLAMA_CONTEXT_LENGTH=8192 ollama serve   # leave running; the default context is too small
ollama pull llama3.1:8b
cp config.ollama.example.yaml config.yaml
```

An 8B model is good enough to exercise the whole pipeline, but expect 15–45
seconds a turn and weaker readings than Claude.

Set `cache_file` to cache real model responses across runs while developing,
and `mode: record` to write every response to the run directory so a game can
be replayed with `--replay runs/<dir>`.

## Evaluate a decision provider

```bash
go run ./cmd/foc eval -v
```

Runs the labelled letters in `testdata/eval/letters.yaml` through the
configured decision provider (override with `--decision llm`) and reports
agreement on clear letters, the split on ambiguous ones, and whether Velk and
Saris diverge the way their traits predict.

## Test

```bash
go test ./...
```

`go test ./internal/game/ -update` and `go test ./internal/web/ -update`
regenerate the golden files after an intended change. Live API tests run with
`go test -tags live ./internal/providers/contract/`.
